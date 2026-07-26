# Workbench Execution Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a committed, source-anchored Workbench execution-chain contract and a deterministic verifier/builder that produces a queryable directed Graphify graph without treating inferred edges as authority.

**Architecture:** The committed Markdown document explains the current production chain, while a JSON contract owns nodes, explicit edges, ordered chains, framework roles, exclusions, and required queries. A small dependency-free Node.js CLI validates source anchors and manifest versions, renders a deterministic corpus, runs Graphify's code-only AST extraction, injects the contract's explicit directed relationships, and verifies freshness and query coverage.

**Tech Stack:** Node.js 22 ESM and built-in `node:test`; JSON and Markdown; Graphify 0.9.x CLI; CodeGraph/codebase-memory; Git; existing React, Thrift, Hertz, GORM/MySQL, Eino ADK, MCP, Prometheus, cron, and Feishu source anchors.

---

## File Map

- Create `docs/superpowers/context/workbench-execution-chain.md`: human-readable current-production chain and framework/boundary ledger.
- Create `docs/superpowers/context/workbench-execution-graph.json`: authoritative machine contract.
- Create `docs/superpowers/runbooks/workbench-execution-graph.md`: update, build, query, audit, and recovery commands.
- Create `scripts/workbench-execution-graph.mjs`: dependency-free CLI entry point.
- Create `scripts/workbench-execution-graph/contract.mjs`: contract, source-anchor, version, chain, exclusion, and changed-file validation.
- Create `scripts/workbench-execution-graph/derived.mjs`: deterministic corpus, digest, Graphify AST build, explicit-edge merge, and derived-graph verification.
- Create `scripts/workbench-execution-graph.test.mjs`: built-in Node unit and integration tests, including fault injection.
- Modify `.gitignore`: ignore the generated stable pointer and complete
  `docs/superpowers/context/workbench-execution-graphify-versions/` tree.
- Modify `AGENTS.md`: add the short mandatory lookup/update commands for Workbench execution work; keep detailed behavior in the context/runbook documents.

### Task 1: Contract Validator Skeleton

**Files:**

- Create: `scripts/workbench-execution-graph/contract.mjs`
- Create: `scripts/workbench-execution-graph.test.mjs`

- [x] **Step 1: Write failing shape, uniqueness, and reachability tests**

Use `node:test`, a temporary repository root, and a minimal fixture containing one source file. The first tests must assert these exact error codes:

```js
assert.deepEqual((await validateContract(validContract, fixture)).errors, []);
assert.match(
  (await validateContract(withDuplicateNode, fixture)).errors.join('\n'),
  /duplicate_node_id/,
);
assert.match(
  (await validateContract(withDanglingEdge, fixture)).errors.join('\n'),
  /edge_target_missing/,
);
assert.match(
  (await validateContract(withBrokenOrder, fixture)).errors.join('\n'),
  /chain_edge_order_invalid/,
);
assert.match(
  (await validateContract(withMissingEvidence, fixture)).errors.join('\n'),
  /chain_test_evidence_missing/,
);
```

- [x] **Step 2: Run the tests and confirm RED**

Run: `node --test scripts/workbench-execution-graph.test.mjs`

Expected: FAIL because `contract.mjs` does not exist.

- [x] **Step 3: Implement deterministic contract validation**

Export these functions with no third-party dependencies:

```js
export const CURRENT_STATUS = 'current';
export const validateContract = async (contract, options) => ({
  errors,
  warnings,
  resolvedVersions,
});
export const validateSourceAnchor = async (anchor, options) => [];
export const validateOrderedChain = (chain, nodesByID, edgesByID) => [];
export const normalizeRepoPath = (repoRoot, candidate) => normalizedPath;
```

Validation must cover schema version `1`, unique IDs, edge endpoints, allowed relation values, ordered node/edge pairs, current-node requirements, evidence existence, repository containment, and exact literal `locator` matches. Return sorted stable messages in the form `error_code: detail`; do not throw on ordinary contract defects.

- [x] **Step 4: Run the tests and confirm GREEN**

Run: `node --test scripts/workbench-execution-graph.test.mjs`

Expected: all Task 1 tests pass.

- [x] **Step 5: Commit the validator foundation**

```bash
git add scripts/workbench-execution-graph/contract.mjs scripts/workbench-execution-graph.test.mjs
git commit -m "test(workbench): add execution graph contract validator"
```

### Task 2: Canonical Contract and Human Context

**Files:**

- Create: `docs/superpowers/context/workbench-execution-graph.json`
- Create: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `scripts/workbench-execution-graph.test.mjs`

- [x] **Step 1: Add a failing test that validates the real repository contract**

```js
test('canonical Workbench execution contract is valid', async () => {
  const contract = JSON.parse(await readFile(CONTRACT_PATH, 'utf8'));
  const result = await validateContract(contract, { repoRoot: REPO_ROOT });
  assert.deepEqual(result.errors, []);
});
```

- [x] **Step 2: Run the canonical test and confirm RED**

Run: `node --test --test-name-pattern="canonical Workbench" scripts/workbench-execution-graph.test.mjs`

Expected: FAIL because the contract does not exist.

- [x] **Step 3: Author the machine contract with exact stable IDs**

The contract must include these ordered chain IDs, each with explicit nodes, explicit edges, and at least one existing test anchor:

```text
entry.workbench_immediate
entry.workbench_deferred
entry.task_detail_followup
entry.langgraph_compat
entry.scheduled_task
entry.feishu_message
run.atomic_create
run.pending_worker_execute
run.event_projection
control.cancel
control.human_resume
control.subagent_retry
control.checkpoint_resume
control.lease_recovery
control.multitask_rollback
data.memory
data.artifact
data.token_usage
data.guardrail_audit
data.mcp_runtime_audit
framework.canonical_stack
framework.eino_sdk
framework.eino_middleware_order
boundary.runtime_compatibility
```

Use this exact node shape for production symbols:

```json
{
  "id": "runtime.adk_executor.execute",
  "label": "ADKExecutor.Execute",
  "layer": "agent_runtime",
  "kind": "function",
  "production_status": "current",
  "source_anchors": [
    {
      "path": "backend/application/agentthread/adk_executor.go",
      "symbol": "(*ADKExecutor).Execute",
      "locator": "func (e *ADKExecutor) Execute("
    }
  ],
  "evidence": [
    {
      "path": "backend/application/agentthread/adk_executor_test.go",
      "locator": "func TestADKExecutor"
    }
  ]
}
```

Framework nodes must additionally include `runtime_scope`, `package`, `version`, and a manifest-backed `version_source`. Include React, React Router, Thriftgo, Hertz, Go stdlib, GORM, MySQL baseline, Eino, Eino-ext model/tool adapters, mcp-go, Prometheus, Sonic, robfig/cron, and the Feishu SDK. Internal OSS and `agentthread` DDD nodes use source anchors instead of pretending to be external packages.

The Eino middleware chain must list these exact ordered IDs:

```text
middleware.reduction
middleware.filesystem
middleware.uploaded_files
middleware.patch_tool_calls
middleware.tool_error_normalization
middleware.memory
middleware.skill
middleware.transcript
middleware.summarization
middleware.plan_task
middleware.provider_capability
middleware.multimodal_budget
middleware.tool_search
middleware.parity_state
middleware.context_budget
middleware.safety_finish
middleware.subagent_limit
middleware.semantic_loop
```

Required queries must include expanded graph-vocabulary terms and exact required IDs for framework inventory, frontend-to-Eino execution, Eino SDK execution, middleware order, cancel/resume/retry, artifact and memory, and runtime compatibility boundaries.

- [x] **Step 4: Author the human-readable chain from the same facts**

Document the canonical sequence, all chain families, framework roles, Eino primitives and middleware order, persistence/lease semantics, event projection, and explicit non-runtime boundaries. Every section must cite repository-relative source files and symbols already present in the JSON contract. State plainly that LangGraph is an API adapter, DeerFlow is parity/config semantics, legacy is historical compatibility, and the Workbench queue is MySQL-backed rather than Redis/Kafka/RabbitMQ.

- [x] **Step 5: Run the real-contract test and confirm GREEN**

Run: `node --test --test-name-pattern="canonical Workbench" scripts/workbench-execution-graph.test.mjs`

Expected: PASS with no missing paths, locators, endpoints, or chain evidence.

- [x] **Step 6: Commit the authority layer**

```bash
git add docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json scripts/workbench-execution-graph.test.mjs
git commit -m "docs(workbench): record canonical execution graph contract"
```

### Task 3: Framework, Boundary, and Change-Detection Rules

**Files:**

- Modify: `scripts/workbench-execution-graph/contract.mjs`
- Modify: `scripts/workbench-execution-graph.test.mjs`

- [x] **Step 1: Write failing framework and boundary tests**

Add fault-injection tests for these exact conditions:

```js
assert.match(errorsFor(versionDrift), /framework_version_mismatch/);
assert.match(
  errorsFor(canonicalWithoutProductionAnchor),
  /canonical_framework_anchor_missing/,
);
assert.match(errorsFor(swappedMiddleware), /middleware_order_mismatch/);
assert.match(
  errorsFor(langGraphAsRuntime),
  /noncanonical_runtime_in_execution_chain/,
);
assert.match(errorsFor(k2CurrentNode), /forbidden_current_node/);
assert.match(errorsFor(chatTaskCurrentNode), /forbidden_current_node/);
```

Test the pure changed-file policy:

```js
assert.deepEqual(
  evaluateAuthorityChanges(['backend/application/agentthread/adk_executor.go']),
  ['authority_files_not_updated'],
);
assert.deepEqual(evaluateAuthorityChanges([CONTRACT_RELATIVE]), [
  'authority_files_must_change_together',
]);
assert.deepEqual(
  evaluateAuthorityChanges([CONTRACT_RELATIVE, CONTEXT_RELATIVE]),
  [],
);
```

- [x] **Step 2: Run the new tests and confirm RED**

Run: `node --test --test-name-pattern="framework|middleware|forbidden|authority" scripts/workbench-execution-graph.test.mjs`

Expected: FAIL because version resolution and boundary checks are not implemented.

- [x] **Step 3: Implement manifest-backed versions and boundary checks**

Support these version source types:

```js
json_dependency; // package.json dependency/devDependency/peerDependency
go_module; // exact module line in backend/go.mod
go_directive; // Go language version in backend/go.mod
text_pattern; // anchored capture such as mysql:8.4.5
platform; // browser-native only
```

Validate the exact `adkMiddlewareOrder` literals from `backend/application/agentthread/adk_middleware.go`. Reject `compatibility_contract`, `historical_compatibility`, `ui_only`, `optional_observability`, and `build_or_test_only` nodes from chains marked `canonical_executor: true`. Enforce exclusion terms for K2 and retired ChatTask production nodes without rejecting explanatory exclusion records.

Export:

```js
export const resolveVersion = async (node, options) => resolvedVersion;
export const validateFrameworkScopes = contract => [];
export const validateMiddlewareOrder = async (contract, options) => [];
export const evaluateAuthorityChanges = (changedPaths, scope) => [];
export const gitChangedPaths = async (repoRoot, baseRef) => [];
```

- [x] **Step 4: Run the tests and confirm GREEN**

Run: `node --test scripts/workbench-execution-graph.test.mjs`

Expected: all contract, framework, exclusion, middleware, and changed-file tests pass.

- [x] **Step 5: Commit framework validation**

```bash
git add scripts/workbench-execution-graph/contract.mjs scripts/workbench-execution-graph.test.mjs
git commit -m "feat(workbench): verify execution framework boundaries"
```

### Task 4: Deterministic Graphify Builder

**Files:**

- Create: `scripts/workbench-execution-graph/derived.mjs`
- Modify: `scripts/workbench-execution-graph.test.mjs`
- Modify: `.gitignore`

- [x] **Step 1: Write failing deterministic corpus and merge tests**

The tests must prove that source files are copied under their repository-relative paths, the ledger is byte-stable across two builds, contract edges remain directed and `confidence: "EXTRACTED"`, and an AST graph cannot overwrite an explicit contract node or edge.

```js
assert.equal(first.digest, second.digest);
assert.equal(graph.directed, true);
assert.equal(
  findLink(graph, 'entry.workbench', 'api.create_thread').relation,
  'calls',
);
assert.equal(
  findLink(graph, 'entry.workbench', 'api.create_thread').confidence,
  'EXTRACTED',
);
```

- [x] **Step 2: Run builder tests and confirm RED**

Run: `node --test --test-name-pattern="corpus|Graphify|explicit" scripts/workbench-execution-graph.test.mjs`

Expected: FAIL because `derived.mjs` does not exist.

- [x] **Step 3: Implement deterministic corpus and digest generation**

Export:

```js
export const renderContractLedger = (contract, resolvedVersions) => markdown;
export const collectCorpusFiles = async (contract, options) => files;
export const writeCorpus = async (contract, options) => ({
  digest,
  files,
  corpusDir,
});
export const sha256Files = async (files, root) => digest;
```

Write generated files only in managed Workbench graph build/version roots below `docs/superpowers/context/`. Render `contract-ledger.md`, copy the committed authority Markdown, and copy every unique source/evidence/version file under `corpus/source/<repo-relative-path>`. Sort all paths and JSON keys before hashing.

- [x] **Step 4: Implement Graphify AST build and explicit graph merge**

Run Graphify headlessly and locally:

```bash
graphify extract <corpus-dir> --out <derived-root> --code-only --no-cluster
```

Then load Graphify's extracted `graphify-out/graph.json`, force `directed: true`, preserve valid AST nodes/links, inject contract nodes with `source_file` and `source_location`, and inject explicit edges with `chain_ids`. Write an overlay-free execution view to `graph.json` and a same-base business retrieval view with query overlay to `query-graph.json`. Write `build-meta.json` containing schema version, Git commit/dirty/status digest, corpus digest, builder digest, both graph counts, Graphify version, and generation timestamp. The builder digest covers the CLI, contract validator, and derived builder so an old-algorithm graph fails closed. Publish the marked and verified directory into an ignored version root, atomically replace one stable symlink, and retain a previous pointer; migrate a legacy graph directory with rollback and reject ordinary directories. Use subprocess argument arrays, never shell interpolation.

Treat `ENOENT` from the Graphify process as `graphify_unavailable` and fail `build` without
altering a previously valid derived graph. Build into a sibling temporary directory and rename
it into place only after extraction and merge succeed.

- [x] **Step 5: Ignore all generated artifacts**

Add exactly:

```gitignore
docs/superpowers/context/workbench-execution-graphify
docs/superpowers/context/workbench-execution-graphify-previous
docs/superpowers/context/workbench-execution-graphify-versions/
```

- [x] **Step 6: Run the tests and confirm GREEN**

Run: `node --test scripts/workbench-execution-graph.test.mjs`

Expected: deterministic builder and merge tests pass without network access.

- [x] **Step 7: Commit the builder**

```bash
git add .gitignore scripts/workbench-execution-graph/derived.mjs scripts/workbench-execution-graph.test.mjs
git commit -m "feat(workbench): build directed execution graph corpus"
```

### Task 5: CLI and Derived-Graph Verification

**Files:**

- Create: `scripts/workbench-execution-graph.mjs`
- Modify: `scripts/workbench-execution-graph/derived.mjs`
- Modify: `scripts/workbench-execution-graph.test.mjs`

- [x] **Step 1: Write failing CLI and derived fault-injection tests**

Cover `verify`, `build`, `verify-derived`, unknown commands, stale corpus/builder digests, dangling endpoints, duplicate relation edges, self-loops, missing required nodes, broken ordered paths, forbidden current nodes, and Graphify query smoke failures. CLI errors must be concise and must exit nonzero.

- [x] **Step 2: Run the CLI tests and confirm RED**

Run: `node --test --test-name-pattern="CLI|derived|stale|query" scripts/workbench-execution-graph.test.mjs`

Expected: FAIL because the CLI and derived verifier do not exist.

- [x] **Step 3: Implement the CLI contract**

Support exactly:

```text
node scripts/workbench-execution-graph.mjs verify [--changed-from <ref>]
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Also support test-only path overrides through `--repo-root`, `--contract`, `--authority`, and `--derived-root`. The production defaults must resolve from the script's own location, not the caller's current directory.

- [x] **Step 4: Implement derived health and query verification**

Export:

```js
export const verifyDerivedGraph = async (contract, options) => ({
  errors,
  warnings,
});
export const verifyRequiredQueries = (graph, requiredQueries) => [];
export const verifyOrderedPaths = (graph, chains) => [];
```

Check current corpus and builder digests, directed mode, endpoint validity, duplicate `(source,target,relation)` tuples, self-loops, required nodes/edges, chain order, framework scope, and exclusion absence. For each required query, verify exact IDs internally. Require every smoke-node label to exist verbatim in contract-provided `expanded_terms`, then retrieve each label with an independent Graphify CLI query so broad-query start-node limits or token truncation cannot hide a missing anchor.

- [x] **Step 5: Run all tests and real verification**

Run:

```bash
node --test scripts/workbench-execution-graph.test.mjs
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
```

Expected: all tests pass and both real verifications report zero errors.

- [x] **Step 6: Commit the CLI**

```bash
git add scripts/workbench-execution-graph.mjs scripts/workbench-execution-graph/derived.mjs scripts/workbench-execution-graph.test.mjs
git commit -m "feat(workbench): verify directed execution graph"
```

### Task 6: Runbook and Repository Entry Point

**Files:**

- Create: `docs/superpowers/runbooks/workbench-execution-graph.md`
- Modify: `AGENTS.md`
- Modify: `scripts/workbench-execution-graph.test.mjs`

- [x] **Step 1: Write a failing documentation-reference test**

Assert that every command named in `AGENTS.md` and the runbook is accepted by the CLI help text, and that both documents reference the committed contract and authority Markdown.

- [x] **Step 2: Run the documentation test and confirm RED**

Run: `node --test --test-name-pattern="documentation" scripts/workbench-execution-graph.test.mjs`

Expected: FAIL because the runbook and AGENTS entry do not exist.

- [x] **Step 3: Write the operational runbook**

Include prerequisites, normal verify/build/query flow, Graphify and CodeGraph responsibilities, source-update checklist, `--changed-from origin/dev`, expected generated paths, stale-graph handling, fault-injection commands using temporary contract copies, security exclusions, and the two-stage dev merge audit. Include these query examples:

```bash
graphify query "workbench execution runtime framework" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/query-graph.json
graphify query "eino adk runner chat model middleware tool event" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/query-graph.json
graphify path "Workbench Immediate Submit" "TaskDetail Event Projection" --graph docs/superpowers/context/workbench-execution-graphify/graphify-out/graph.json
```

- [x] **Step 4: Add a short AGENTS.md entry**

Under the Workbench/Agent Runtime guidance, require reading `workbench-execution-chain.md` before changing the chain and running:

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Keep detailed maintenance rules in the runbook rather than expanding AGENTS.md further.

- [x] **Step 5: Run tests and commit documentation**

Run: `node --test scripts/workbench-execution-graph.test.mjs`

Expected: all tests, including documentation references, pass.

```bash
git add AGENTS.md docs/superpowers/runbooks/workbench-execution-graph.md scripts/workbench-execution-graph.test.mjs
git commit -m "docs(workbench): document execution graph maintenance"
```

### Task 7: Build, Query, Refresh, and First Audit

**Files:**

- Generated and ignored: `docs/superpowers/context/workbench-execution-graphify/**`
- Modify only if verification finds authority defects: the committed contract, context, runbook, or scripts above

- [x] **Step 1: Run the complete test and static verification suite**

```bash
node --test scripts/workbench-execution-graph.test.mjs
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
git diff --check origin/dev...HEAD
```

Expected: all commands succeed with zero contract or formatting errors.

- [x] **Step 2: Build and verify the actual directed graph**

```bash
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Expected: Graphify graph is directed, digest is current, every required query passes, K2/current ChatTask nodes are absent, and canonical paths cross real source-anchored nodes.

- [x] **Step 3: Run user-critical graph queries**

Query and record concise results for:

```text
Workbench immediate submit -> persisted Run -> worker -> Eino -> EventSource
TaskDetail follow-up with upload -> atomic Run
Eino SDK Runner/ChatModelAgent/middleware/tool/subagent usage
Cancel / human resume / subagent retry / lease recovery
Memory / Artifact / Token / Guardrail / MCP audit
LangGraph / DeerFlow / legacy compatibility boundaries
```

Expected: each result cites current source anchors and does not route through generic documentation-only `references` edges.

- [x] **Step 4: Refresh and verify CodeGraph/codebase-memory**

```bash
codegraph sync /private/tmp/coze-studio-workbench-execution-graph
codegraph status /private/tmp/coze-studio-workbench-execution-graph
codegraph explore -p /private/tmp/coze-studio-workbench-execution-graph --max-files 20 Workbench TaskThread RunWorker ADKExecutor EventSource
```

Expected: index is current, key symbols are found, and no retired ChatTask production symbol is reported as part of the canonical path.

- [x] **Step 5: Run security and scope scans**

```bash
git status --short
git diff --name-only origin/dev...HEAD
rg -n "prompt|completion|tool_arguments|tool_results|checkpoint_bytes|credential|object_uri|raw_provider|raw_audit" docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json
rg -n "K2|ChatTask" docs/superpowers/context/workbench-execution-graph.json
```

Expected: sensitive terms occur only in bounded exclusion/safety declarations and never as
runtime values; K2 and ChatTask occur only in explicit exclusions; no business source file
changed.

- [x] **Step 5a: Close independent-review findings and rerun the real graph**

The first independent review must be treated as blocking. Complete all of the following before
Step 6:

- assign deterministic IDs to Graphify AST edges, require non-empty node/link output and full
  parseable-source coverage, and verify the final temporary graph before atomic installation;
- add query-intent overlay nodes in a separate retrieval graph, keep the execution graph free of
  retrieval shortcuts, validate required nodes/edges against their assigned graph, and bridge
  contract facts only to Graphify AST file nodes;
- bind the contract to caller-enforced `workbench_execution_v1` with a complete structural digest, prevent
  query/chain/exclusion or critical-node self-closure, and enforce edge direction plus
  ordered/side-edge membership;
- add LangGraph stateless backing-thread creation, Scheduled new/existing conversation branches,
  Feishu new/existing session branches, and the correctly directed new-run policy exclusion of
  historical legacy;
- reject sensitive corpus paths by lexical and real path, reject all corpus symlinks, require
  monitored paths to cover every contract-referenced source, test, generated client/model,
  generated/custom route and manifest, and publish with an atomic version pointer;
- detect untracked files, both sides of renames, tracked file type changes and NUL-delimited Git
  paths; refuse unmanaged/out-of-scope derived roots, retain a previous version pointer, record
  Git dirty provenance, and require source-anchored evidence for every exclusion;
- rerun all tests, build/verify-derived, Graphify business queries, CodeGraph sync, security scans,
  and an independent read-only re-review.

Final first-audit evidence: 53/53 Node tests, Prettier, canonical verify, real Graphify build and
`verify-derived`, CodeGraph sync, security/scope scans and merge-tree audit all passed. The final
independent read-only review reported no P0/P1/P2 findings.

- [x] **Step 6: Commit any audit-only corrections and stop before merge**

```bash
git add AGENTS.md .gitignore docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json docs/superpowers/runbooks/workbench-execution-graph.md scripts/workbench-execution-graph.mjs scripts/workbench-execution-graph
git commit -m "feat(workbench): complete canonical execution knowledge graph"
```

If there are no audit corrections, do not create an empty commit. Present the first-audit evidence and wait for explicit user confirmation before merging into local `dev`. Do not push.
