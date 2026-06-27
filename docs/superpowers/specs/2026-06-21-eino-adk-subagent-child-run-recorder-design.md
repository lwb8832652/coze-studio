# Eino ADK Subagent Child Run Recorder Design

## Goal

M3.14 connects subagent lifecycle events to durable child run rows. Eino
`AgentTool` invocation now has a Coze-owned recorder boundary that creates a
`run_kind='subagent'` child row when the child starts and updates that row when
the child succeeds or fails.

## Recorder Contract

`ADKSubagentRunRecorder` has two operations:

- `StartADKSubagentRun`
- `FinishADKSubagentRun`

The recorder receives the parent `RunSummary` and normalized
`ADKSubagentDefinition`. It returns the child `RunSummary`, which is then
included in lifecycle event payloads as `child_run_id`.

## Application Implementation

`ApplicationADKSubagentRunRecorder` uses existing application service
boundaries:

- `CreateRun` with `ParentRunID`, `RunKindSubagent`, and initial
  `RunStatusRunning`;
- `CompleteRun` for successful terminal status;
- `FailRun` for failed terminal status with sanitized error messages.

Child run config is content-free and includes `runtime: "eino_adk"` plus
agent identity fields. Child run input is an empty message list; tool
arguments, model input, model output, and final text are not persisted in the
child row by this recorder.

## Provider Wiring

`ADKSubagentToolProvider` accepts
`WithADKSubagentToolProviderRunRecorder`. The recorder is independent from the
event sink: a configured recorder still writes child rows even if no event sink
is configured.

The production default tool provider accepts
`WithDefaultADKToolProviderSubagentRunRecorder`, and `application.Init` passes
`NewApplicationADKSubagentRunRecorder(agentThreadSVC)`.

## Deferred Work

Open follow-up work:

- expose child-run listing in the task detail UI;
- join child run rows with subagent lifecycle events in frontend state cards;
- roll up child token usage and terminal status to the parent task;
- classify cancellation and timeout separately from generic failure;
- add child run retry/resume semantics.
