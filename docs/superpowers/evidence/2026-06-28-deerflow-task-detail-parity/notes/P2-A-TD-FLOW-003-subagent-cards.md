# P2-A TD-FLOW-003 Subagent Cards

## DeerFlow Reference

- Frontend message grouping detects task-tool calls through
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/utils.ts`
  `hasSubagent(message)`.
- DeerFlow renders subagent/tool progress through workspace message components:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-group.tsx`
  and
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/subtask-card.tsx`.
- Backend `task` tool emits subagent progress with safe status and task output
  summaries from
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/tools/builtins/task_tool.py`.

## Coze Current Behavior

- Coze projects subagent state from canonical parent/child runs plus
  `subagent.run.*` lifecycle events.
- UI file:
  `frontend/apps/coze-studio/src/pages/tasks/task-subagent-runs-section.tsx`.
- Projection file:
  `frontend/apps/coze-studio/src/pages/tasks/task-detail-subagents.ts`.
- Backend event source:
  `backend/application/agentthread/adk_subagent_tool_provider.go`.

## Safety Boundary

The task detail UI shows only bounded metadata:

- subagent name;
- status and status text;
- elapsed time;
- terminal classification;
- model attribution;
- run-level token/cost/call summaries;
- retry summary;
- lifecycle timeline titles and bounded errors.

It does not expose child prompts, raw tool arguments, raw tool results,
checkpoint bytes, provider payloads, object URIs, or credentials.

## Verification

Targeted test:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "subagent"
```

Result:

- `2 passed | 55 skipped`.

Covered behaviors:

- parent run is loaded once and child runs are loaded by `parent_run_id`;
- retry placeholder runs are not recursively treated as parent runs;
- parent token usage request includes child runs;
- child token usage requests are not made independently;
- UI shows `子智能体执行`, running/failed child status, model attribution,
  token/cost/call summaries, elapsed time, retry summary, lifecycle timeline,
  and bounded failure message.
