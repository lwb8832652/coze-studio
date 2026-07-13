# DeerFlow Agent Run Ownership And Lease Evidence

**Date:** 2026-07-11

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX AI baseline:** `c9a26fca1`

## Scope

This note records the ownership and lifecycle evidence required before adding
durable run leases to the Go worker queue. The user-visible target remains
DeerFlow run lifecycle parity. Durable leases and fencing are a production
hardening needed because NewX AI executes runs through horizontally scalable
database workers instead of DeerFlow's process-local task registry.

## DeerFlow Source Contract

Locked source files:

- `backend/packages/harness/deerflow/runtime/runs/manager.py`
- `backend/packages/harness/deerflow/persistence/run/model.py`
- `backend/packages/harness/deerflow/persistence/run/sql.py`

Verified behavior:

1. `RunManager` states that ownership is an in-memory registry protected by one
   `asyncio.Lock` (`manager.py` lines 107-129).
2. Creating a run writes the in-memory registry and persistent row while that
   process-local lock is held (`manager.py` lines 359-397).
3. Status changes mutate the local record and then perform a best-effort store
   update (`manager.py` lines 474-482).
4. Cancellation addresses only a task known to the current process and updates
   status without a durable owner token (`manager.py` lines 509-537).
5. Startup reconciliation treats persisted pending/running rows without a live
   local task as orphaned and marks them failed (`manager.py` lines 635-680).
6. The SQL run model contains status and timestamps but no owner, lease token,
   expiry, heartbeat or generation (`model.py` lines 13-49).
7. `update_status` is keyed only by `run_id`; it has no owner/generation fence
   (`sql.py` lines 159-166).

DeerFlow therefore does not provide a durable multi-worker lease contract.
Copying this literally into the Go queue would permit duplicate execution and
late writes from a stale worker. That is an upstream deployment limitation, not
a parity target.

## NewX AI Current Gap

The current repository:

- claims pending or queued-resume rows with a transaction and status compare;
- records only `worker_id` and `started_at`;
- has no lease expiry or heartbeat;
- allows a running transition to finish when only status and optional
  `worker_id` match;
- cannot distinguish a stale execution from a newer claim made by the same
  worker identifier;
- cannot safely discover, renew or release expired ownership.

## Approved Lease Contract

Each top-level claim must atomically:

- transition the eligible row to `running`;
- set `lease_owner`, a cryptographically random `lease_token`,
  `lease_expires_at` and `heartbeat_at`;
- clear any previous cancellation request;
- increment monotonic `execution_generation`;
- return the persisted lease fields to the caller.

Heartbeat, release and worker finalization must match all of:

- run id and `running` status;
- lease owner;
- lease token;
- execution generation;
- an unexpired lease at the operation timestamp.

Successful completion, failure or interruption clears owner, token, expiry,
heartbeat, cancellation request and legacy `worker_id`, while retaining the
generation for audit and stale-write rejection. A repository release may move
an unfinalized run only to `pending` or protected `queued`, clear ownership and
allow the next claim to increment generation again.

Cancellation is intentionally not redesigned in this slice. The existing
immediate cancel transition remains compatible until the separate cancellation
slice introduces `cancel_requested_at` polling and transactionally fenced final
commit.

## Acceptance

- two claimers cannot own one run;
- token and generation change on a later claim;
- heartbeat extends only a live matching lease;
- wrong owner/token/generation and expired leases return a typed lease-lost
  error;
- expired running leases can be listed deterministically;
- fenced worker finalization clears active lease fields;
- migration hash and validation pass with local Atlas;
- all affected domain/application tests and the full backend pass.

## NewX AI Implementation Evidence

Implemented contract:

- schema: `docker/atlas/migrations/20260711000100_agent_run_leases.sql`;
- persistence and compare-and-update predicates:
  `backend/domain/agentthread/repository/mysql.go`;
- domain contract and validation:
  `backend/domain/agentthread/service/service.go` and `service_impl.go`;
- internal runtime projection and lease operations:
  `backend/application/agentthread/dto.go`, `thread_app.go`, and `service.go`;
- fenced ordinary and resume finalization:
  `backend/application/agentthread/runner.go` and `resume_runner.go`;
- public redaction regression fixture:
  `backend/application/agentthread/public_projection_test.go`.

Verified 2026-07-11:

- RED: repository lease tests initially failed because lease fields and methods
  did not exist; protected queued release initially succeeded incorrectly;
- GREEN: duplicate claim prevention, renewal, expiry, stale token/generation,
  protected queued release, generation increment and terminal clearing pass;
- Atlas Community v0.35.0 hash and migration validation exit successfully;
- targeted domain/application/handler/router tests and `go vet` pass;
- serial full backend passes with
  `go test -p 1 -gcflags="all=-N -l" ./... -count=1`.

## Batch Isolation, Heartbeat And Stale Recovery

Completed 2026-07-11 on top of the lease contract above.

DeerFlow source behavior remains the reference for the visible lifecycle:
`RunManager.reconcile_orphaned_inflight_runs` lists persisted `pending` and
`running` rows that have no process-local task and moves them to an explicit
error instead of leaving the UI indefinitely active (`manager.py` lines
635-680). DeerFlow does not have a durable owner token, heartbeat or
multi-worker stale-write fence.

NewX AI preserves that terminal guarantee and adds the controls required by its
database-backed, horizontally scalable Go workers:

- one infrastructure failure no longer aborts the rest of a claimed ordinary
  or checkpoint-resume batch;
- every still-owned, unfinalized ordinary run is released to `pending`; a
  protected checkpoint-resume run is released only to `queued`;
- ordinary and resume execution share a 60-second lease and 20-second default
  heartbeat, with an injected clock/ticker for deterministic tests;
- heartbeat loss cancels the active execution context; process shutdown
  releases the lease without writing a false `failed` terminal state;
- stale reconciliation uses a separate expired-lease CAS matching run id,
  owner, token, generation, `running` status and expired timestamp;
- the latest active, decodable and runtime-compatible checkpoint creates one
  protected resume run using `run-recovery:<source_run_id>:<generation>`;
- a retry after resume creation but before source reconciliation reuses the
  existing idempotent run;
- if no compatible checkpoint exists, the source becomes `failed` with bounded
  `run_abandoned` metadata; if recovery is queued, the source becomes
  `interrupted` with bounded `run_recovered` metadata;
- lease credentials remain internal and are not copied into recovery command,
  metadata, REST, SSE or LangGraph-compatible projections.

Implementation:

- heartbeat controller: `backend/application/agentthread/run_lease_heartbeat.go`;
- stale recovery processor: `backend/application/agentthread/run_lease_recovery.go`;
- batch and finalization integration: `runner.go` and `resume_runner.go`;
- stale CAS: `backend/domain/agentthread/repository/mysql.go`;
- worker bootstrap and environment contract:
  `backend/application/agentthread/worker.go`, `application.go`,
  `docker/.env.debug.example`, and
  `docs/superpowers/runbooks/deerflow-parity-runtime-operations.md`.

Verification:

- RED/GREEN batch isolation for ordinary and protected resume runs;
- RED/GREEN ordinary/resume heartbeat, lease-loss cancellation and shutdown
  release tests;
- RED/GREEN stale-CAS generation fence and old-worker rejection test;
- RED/GREEN Eino ADK checkpoint selection, invalid-checkpoint skip,
  create-before-reconcile retry idempotency and no-checkpoint abandonment tests;
- affected repository, domain, application, Workbench and bootstrap packages;
- targeted `go vet`, `gofmt`, and `git diff --check`;
- serial full backend:
  `go test -p 1 -gcflags="all=-N -l" ./... -count=1`.

Cancellation intent fencing and transactional message/title/success commit are
recorded separately in
`2026-07-11-deerflow-agent-run-cancellation-fence.md`.
