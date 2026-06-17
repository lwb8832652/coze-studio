# Phase 47 - Checkpoint Resume State Loader

## Goal

Introduce a replay state loader boundary for checkpoint resume runs. This phase converts checkpoint JSON payloads into an internal Go Harness resume input, but still does not invoke the planner or execute pending sends.

## Scope

- Add `HarnessResumeInput`.
- Convert checkpoint `channel_values` into:
  - checkpoint messages
  - reconstructed `AgentHarnessState`
  - executed steps
  - step results
  - memory context
- Convert checkpoint `pending_sends` into pending `AgentStep` values.
- Preserve checkpoint `channel_versions` and metadata for later replay validation.
- Emit `run.resume.loaded` after successful conversion.
- Keep the Phase 46 safe failure:
  - `error_code = checkpoint_replay_not_implemented`
  - `error_message = checkpoint replay is not implemented yet`

## Out Of Scope

- Running pending steps.
- Calling the planner.
- Calling tools or models.
- Writing new checkpoints from resumed execution.
- Starting a background resume worker loop.
- Changing the normal pending run worker path.

## Loader Contract

The loader accepts:

- the claimed resume run,
- parsed `command.resume`,
- the selected checkpoint.

It returns a `HarnessResumeInput` with:

- current resume run identifiers,
- source checkpoint run identifier,
- checkpoint namespace,
- raw channel values,
- channel versions,
- checkpoint metadata,
- checkpoint messages,
- reconstructed harness state,
- pending steps.

The processor uses the loaded input only for validation and observability in this phase. Future phases can pass the same value into a replay executor.

## Verification

- `go test -count=1 ./application/agentthread -run 'Test(LoadHarnessResumeInput|ResumeRunProcessor)'`
- `go test -count=1 ./application/agentthread -run 'Test(RunProcessor|ResumeRunProcessor|LoadHarnessResumeInput)'`
- `go test -count=1 ./application/agentthread ./domain/agentthread/...`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`

## Next Phase

Phase 48 should add the replay executor boundary that can consume `HarnessResumeInput` and run only the pending steps, while still keeping model/tool calls behind existing harness interfaces.
