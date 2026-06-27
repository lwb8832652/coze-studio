# M4.2 Runtime Offload Object URI Policy Design

## Goal

Make runtime offload file registration enforce deterministic object-storage
scope before `agent_files` records are persisted.

## Scope

This slice tightens only the registration boundary for Eino ADK tool-result
offload files. It does not introduce general artifact APIs, upload mounting,
workspace browsing, shell execution, `ls`, `glob`, `grep`, write/edit tools,
preview/download endpoints, retention workers, or sandbox providers.

## Policy

`RegisterRuntimeFile` accepts only object URIs matching:

```text
agent-runtime/{space_id}/{thread_id}/runs/{run_id}/tool-results/{trunc|clear}/{sha256}.txt
```

The object URI must be relative, clean, free of backslashes and control
characters, and must not contain a URL scheme. The `{run_id}`, phase, and file
name must match the validated virtual path:

```text
/mnt/user-data/workspace/.coze/tool-results/runs/{run_id}/{trunc|clear}/{sha256}.txt
```

After the service loads the authoritative run row, the object URI's
`space_id`, `thread_id`, and `run_id` must match that row. This prevents a
crafted caller from registering metadata that points at another tenant,
thread, run, phase, or offload file.

The `{sha256}.txt` path segment is the stable offload file identity generated
from the tool-call context. The persisted `Digest` field remains the content
SHA-256 and is still validated independently as a lowercase 64-character hex
string.

## Security Boundary

The ADK offload backend already generates deterministic object keys, but the
domain registration service is the durable defensive gate. Future callers,
new adapters, retries, or replay code must not be able to bypass space,
thread, run, phase, and filename binding by passing a clean but wrong relative
object key.

Events and errors remain content-free. The service does not emit object URIs,
tool output, prompt text, completion text, checkpoint bytes, credentials,
object storage provider responses, or raw diagnostic content.

## Testing

Domain service tests cover successful registration plus rejection of object
URLs, cross-space object keys, cross-thread object keys, cross-run object keys,
phase mismatches, filename mismatches, and backslash-containing object keys.
