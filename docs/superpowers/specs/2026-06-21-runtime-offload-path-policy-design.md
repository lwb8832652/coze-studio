# M4.1 Runtime Offload Path Policy Design

## Goal

Make runtime offload file registration enforce the same path policy as the ADK
offload backend, so database registration cannot accept crafted workspace file
records outside the active run's bounded tool-result area.

## Scope

This slice only tightens the Coze-owned runtime file registration boundary for
tool-result offload files. It does not expose general workspace browsing,
uploads, artifacts, shell execution, `ls`, `glob`, `grep`, write/edit tools, or
sandbox providers.

## Policy

`RegisterRuntimeFile` accepts only workspace files whose virtual path matches:

```text
/mnt/user-data/workspace/.coze/tool-results/runs/{run_id}/{trunc|clear}/{sha256}.txt
```

The `{run_id}` segment must equal the request `RunID`. The phase must be
`trunc` or `clear`. The filename must be a lowercase 64-character hex digest
with `.txt`. The request `FileName` must equal the path base. Existing object
URI, size, digest, metadata, and run ownership checks remain in force.

## Security Boundary

The registration service remains a defensive layer even when the ADK offload
backend already generates deterministic paths. This prevents future callers or
buggy adapters from registering files for another run, arbitrary workspace
paths, unsupported phase directories, or non-digest names that could collide
with human-readable paths.

Events and errors remain content-free. No tool output, object URI, prompt,
completion, checkpoint bytes, URL, filename from user input, or provider raw
payload is emitted in run events.

## Testing

Domain service tests cover successful registration with a valid digest path and
rejection of cross-run paths, unsupported phases, short/non-hex file names,
uppercase digest names, object URLs, invalid metadata, and non-workspace file
kinds.
