# TD-DOC-001/002 ADK output file and present_files backend chain

## Scope

- Case IDs: `TD-DOC-001`, `TD-DOC-002`.
- Goal: match DeerFlow's generated-file flow at the backend boundary.
- Not included: real browser screenshot, live prompt run, preview/download sample.

## DeerFlow Baseline

- `present_files` is the explicit surfacing tool.
- It accepts only `/mnt/user-data/outputs/*`.
- The frontend detects AI tool calls named `present_files` and renders an
  artifact file list from `toolCall.args.filepaths`.
- Reference files:
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/tools/builtins/present_file_tool.py`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/utils.ts`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-list.tsx`

## Coze Implementation

- Added ADK runtime tools:
  - `write_file`: writes only `/mnt/user-data/outputs/*` content to object
    storage and registers an `AgentFileKindOutput`.
  - `present_files`: resolves existing output files and registers task
    artifacts through `ArtifactSVC.RegisterArtifact`.
- Existing internal ADK tool-result offload remains under
  `/mnt/user-data/workspace/.coze/tool-results/runs/*` and is not promoted to
  artifacts.
- Tool results and `artifact.presented` events include virtual path, file ID,
  artifact ID, size, content type, and preview mode only. They do not include
  `agent-runtime` object URIs.

## Code Evidence

- `backend/application/agentthread/adk_artifact_tools.go`
- `backend/application/agentthread/artifact_output.go`
- `backend/domain/agentthread/service/runtime_file.go`
- `backend/application/application.go`
- `backend/application/agentthread/adk_artifact_tools_test.go`
- `backend/domain/agentthread/service/runtime_file_test.go`

## Verification

```bash
go test ./domain/agentthread/service -run 'TestRuntimeFileService|TestArtifactServiceRegistersActiveOutputFile' -count=1
go test ./application/agentthread -run 'TestApplication(ListArtifacts|ReadArtifactContent|CreateArtifactSignedURL|DeleteArtifact|RestoreArtifact|WriteOutputFile|PresentOutputFiles)|TestADKArtifactToolCatalog|TestDefaultADKToolProviderCanWireArtifactTools' -count=1
go test ./domain/agentthread/service ./application/agentthread ./application -count=1
```

Both commands passed locally on 2026-06-28.

## Remaining Acceptance

- Run the standard document generation prompt in the browser.
- Confirm the model calls `write_file` then `present_files`, or otherwise
  creates an output file and explicitly presents it.
- Capture `GET /api/workbench/task_threads/:thread_id/artifacts` response
  summary with safe fields only.
- Verify the artifact appears in the panel and generated-file message card,
  then preview/download through existing artifact APIs.
