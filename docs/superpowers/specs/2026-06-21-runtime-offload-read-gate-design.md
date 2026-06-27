# M4.3 Runtime Offload Read Gate Design

## Goal

Require `read_file` and runtime offload range reads to resolve an active
registered `agent_files` row before reading object storage.

## Scope

This slice covers only Eino ADK tool-result offload reads. It does not add
general workspace browsing, upload APIs, artifact preview/download, output
promotion, shell execution, write/edit/list/glob/grep tools, retention cleanup,
or sandbox providers.

## Policy

Before object storage is touched, the ADK offload backend parses the requested
virtual path and asks the Coze runtime file registry to resolve it with the
active run's `space_id` and `thread_id` plus the source `run_id` from the path.

The domain runtime file service then requires:

- an `agent_files` row for `(source_run_id, virtual_path)`;
- `space_id` and `thread_id` matching the active run scope;
- `run_id` matching the source run in the virtual path;
- `file_kind = workspace`;
- `status = active`;
- the stored virtual path and filename to match the validated path;
- the stored object URI to match
  `agent-runtime/{space_id}/{thread_id}/runs/{source_run_id}/tool-results/{trunc|clear}/{sha256}.txt`;
- positive size, lowercase content SHA-256 digest, and valid JSON metadata.

The ADK backend reads the service-returned object URI. It no longer relies on
recomputing an object key from model-provided virtual path input alone.

## Resume Semantics

Fresh active runs may still read offloaded files from a source run when that
source run belongs to the same space and thread. This preserves resumed-run
readback while preventing cross-thread, cross-space, deleted, unregistered, or
stale invalid offload files from being read.

## Security Boundary

Object storage is not an authorization source. A deterministic object key or a
physically present object does not make a file readable. The durable
`agent_files` row is the Coze-owned access boundary for tool-result offload
reads.

Errors remain content-free. Read failures must not expose stored content,
object URIs, prompt text, completion text, checkpoint bytes, provider raw
responses, stack traces, credentials, or object storage diagnostics.

## Testing

Tests cover red/green behavior for orphan object reads, historical same-thread
readback after registration, registry resolve mapping, domain resolve
validation, and repository lookup by `(run_id, virtual_path)`.
