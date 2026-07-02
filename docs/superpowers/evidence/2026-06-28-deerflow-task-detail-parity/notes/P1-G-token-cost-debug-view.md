# P1-G Token Cost And Debug View

Date: 2026-07-02

## Scope

This slice closes the pricing-ready Token view gap on top of the DeerFlow-style
Token popover already covered by P1-A-007.

Implemented behavior:

- top-bar aggregate Token usage keeps the existing DeerFlow rows and display
  modes;
- cost is shown only when backend usage has a positive `cost_micros` total and
  all visible usage rows resolve to one currency;
- mixed-currency usage suppresses cost rather than adding misleading totals;
- paginated usage rows suppress cost when the frontend cannot prove the
  aggregate currency is complete;
- `调试` mode shows bounded debugging metadata: call count, source token
  distribution, and provider/model attribution;
- raw usage JSON, raw metadata, prompts, hidden run config, and provider
  payloads remain hidden from the UI.

## DeerFlow Baseline

Existing DeerFlow parity evidence:

- `notes/P1-A-007-token-popover-modes.md`
- `notes/TD-TOKEN-002-token-usage-modes.md`

P1-G intentionally preserves the P1-A DeerFlow baseline for normal viewing:
`Token 用量`, `输入`, `输出`, `总计`, display modes
`关闭` / `总览` / `每轮` / `调试`, and the explanatory note.

## Coze Source Evidence

- Top-bar popover:
  `frontend/apps/coze-studio/src/pages/tasks/task-token-usage-indicator.tsx`
- Token mapping:
  `frontend/apps/coze-studio/src/pages/tasks/task-detail-token-usage.ts`
- Thread detail loader:
  `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
- Regression coverage:
  `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

## Data Contract

Backend already returns safe token data:

- aggregate counters: `input_tokens`, `output_tokens`, `total_tokens`,
  `cost_micros`, `call_count`, and source totals;
- row metadata: `provider`, `model_name`, `currency`, `cost_micros`.

No backend schema change was required. The frontend now combines the aggregate
with the current page of token usage rows to derive:

- a single normalized currency, or empty currency when rows contain none or
  more than one currency;
- empty currency when the API response is paginated and the frontend does not
  have all token usage rows needed to prove aggregate currency;
- unique provider/model attributions for debug mode;
- per-run currency and attribution for existing per-turn/subagent usage maps.

## Safety Boundary

The UI must not expose:

- `raw_usage`;
- row `metadata`;
- prompt or completion text;
- tool arguments or tool results;
- hidden run config;
- credentials, object URIs, signed URLs, or raw provider bodies.

P1-G only surfaces bounded numeric totals and short provider/model labels.
Provider/model labels are truncated before display.

## Verification

Red tests were observed before implementation:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "canonical thread token usage"
```

Post-implementation checks passed:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token"
npx tsc --noEmit --project tsconfig.json
```

Covered behavior:

- single-currency positive cost renders `成本 USD 0.002500`;
- mixed `USD` / `CNY` usage does not render cost;
- incomplete paginated usage rows do not render cost;
- default popover does not show provider/model or raw secret payloads;
- debug mode shows call count, lead-agent/tool token distribution, and
  `openai / gpt-4.1`;
- per-turn token usage remains scoped to safe token counts and does not expose
  raw provider payloads.

## Remaining Gap

No open P1-G blocker remains. More detailed billing reconciliation, historical
cost trend charts, and provider-specific price tables are P2 product hardening
items unless a DeerFlow-visible regression appears.
