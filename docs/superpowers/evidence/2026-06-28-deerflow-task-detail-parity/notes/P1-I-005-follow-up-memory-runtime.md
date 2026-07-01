# P1-I-005 Follow-up Memory Runtime Evidence

Date: 2026-07-01

## Scope

Validate that Coze's Go/Eino runtime can:

- keep prior turns available for follow-up runs;
- enqueue terminal transcripts for async memory update;
- run the memory flush worker from normal application startup;
- write durable memory without exposing unsafe payload metadata.

## Coze Browser Evidence

- Follow-up recall task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657470660221861888`
- Screenshot:
  `screenshots/coze/P1-I-005-follow-up-recall.png`
- Result:
  - initial prompt asked the agent to remember project code name `青鸢计划`;
  - follow-up asked for the remembered project code name;
  - page preserved both user turns and assistant turns;
  - follow-up answer returned `青鸢计划`.

## Durable Memory Evidence

- Fresh memory-flush task after rebuilding backend:
  `http://localhost:8080/space/7656275718757679104/tasks/7657481311426183168`
- Screenshot:
  `screenshots/coze/P1-I-005-memory-flush-success.png`
- Safe database summary:
  - `agent_threads.status`: `idle`
  - `agent_thread_messages`: 2 total, 1 user + 1 assistant
  - `agent_transcript_snapshots`: 2
  - `agent_memory_flush_jobs`: 1 job, `succeeded`, `attempt_count=2`
  - `agent_thread_memories`: 1 `long_term` memory
  - `agent_run_events`: includes `memory.update_queued` and
    `memory.update_completed`
  - memory metadata keys: `category`

The written memory content was the bounded user/project fact only; memory
metadata did not include prompt, completion, tool argument/result, checkpoint,
object URI, signed URL, credential, or raw provider fields.

## Fixes Confirmed

- `backend/application/application.go` now starts
  `agentthread.StartMemoryFlushWorkerFromEnv` during normal application
  initialization, alongside run/resume/artifact/guardrail workers.
- `backend/application/agentthread/memory_flush_processor.go` records safe
  memory extraction error categories such as `model_call_failed` or
  `decode_failed` while continuing to avoid transcript leakage.
- `docs/superpowers/runbooks/local-debug-and-test.md` documents that local
  DeerFlow memory parity requires both the memory flush worker and model
  extractor to be enabled.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestMemoryFlushWorker|TestApplicationProcessMemoryFlushJobs|TestModelMemoryExtractor|TestThreadMemoryProvider|TestADKMemoryMiddleware' -count=1
```

Result: passed.

## Remaining Follow-up

P1-I-006 should verify the task memory UI/API surface against the runtime-written
memory row and confirm management actions still agree with recall/update
behavior.
