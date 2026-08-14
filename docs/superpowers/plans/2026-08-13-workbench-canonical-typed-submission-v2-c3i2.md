# Workbench Canonical Typed Submission V2 C3i2 First-Party Cutover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After C3i1 is committed and its parity gate is green, switch exactly the five first-party Workbench mutation flows to the generated typed V2 contract without changing routes, UI, persistence, or server behavior.

**Architecture:** Add one focused serializer beside `canonicalThreadClient`; it converts the existing composer state into generated `CanonicalInitialRunSubmissionV2`, `CanonicalRunSubmissionV2`, and `CanonicalHumanInteractionResponseV2` values. Extend the existing client request union so pages can select V1 or V2, then cut over root atomic/deferred, follow-up, top-level retry, and Human Resume while preserving upload ordering and idempotency attempt lifecycles.

**Tech Stack:** React 18, TypeScript 5.8, generated `@coze-studio/api-schema/workbench-thread` types, Vitest, Testing Library, Rush/rushx, ESLint, Rsbuild.

---

## Delivery lock and file map

- Start only from a committed C3i1 revision whose Go/TypeScript contract generation, strict server acceptance, and semantic parity corpus are green. If any C3i1 parity assertion fails, stop: C3i2 remains locked.
- Delivery first: no new route, page, UI control, migration, Human Journal Attempt rollover, legacy decoder, gate-on producer, P1D/P2 loop, MySQL race implementation, generic extension field, or V1 retirement.
- Do not edit `idl/workbench/thread.thrift`, generated Go, or generated TypeScript in C3i2.
- Do not read, edit, stage, delete, or commit the user-owned untracked P1L/C2 plan files named in the approved design.
- Create `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-typed-submission-v2.ts`: the only composer-to-V2 serializer and Human-response normalizer.
- Create `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-typed-submission-v2.test.ts`: absence, order, scalar-string, and flow-shape snapshots.
- Modify `frontend/apps/coze-studio/src/pages/workbench/thread-client/workbench-thread-client.ts`: version-exclusive request unions using generated types.
- Modify `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`: send the selected V2 field on existing routes; keep V1 readable.
- Modify root/task writer files and their existing focused tests only; authority files change last, after all five flows are green.

### Task 1: Lock the C3i1 gate and add the shared typed serializer

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-typed-submission-v2.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-typed-submission-v2.test.ts`
- Read only: `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`
- Test: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`

- [ ] **Step 1: Verify the prerequisite revision and C3i1 contract test**

Run:

```bash
git log -1 --oneline
cd frontend/packages/arch/api-schema && rushx test -- workbench-thread-contract.test.ts
```

Expected: HEAD contains the committed C3i1 slice and the generated-contract test passes. Do not begin if C3i1 is uncommitted or the test fails.

- [ ] **Step 2: Write failing serializer snapshots**

```ts
import { describe, expect, it } from 'vitest';
import {
  createInitialSubmissionV2,
  createRetrySubmissionV2,
  createTurnSubmissionV2,
  normalizeHumanResponseV2,
} from '../canonical-typed-submission-v2';

describe('canonical typed submission v2', () => {
  it.each([
    ['enabled-auto', enabledAutoSkillsAndMCPFixture(), undefined, [], []],
    ['enabled-explicit', enabledExplicitSkillsAndMCPFixture(), ['skill.b', 'skill.a'], ['mcp.b'], ['mcp.b']],
    ['disabled-configured', disabledConfiguredSkillsAndMCPFixture(), [], [], ['mcp.saved']],
  ] as const)('preserves the exact Skill/MCP presence state: %s', (_name, payload, explicitSkills, enabledMCP, allowedMCP) => {
    const value = createInitialSubmissionV2(payload);
    expect(value.composer.explicit_enable_skills).toEqual(explicitSkills);
    expect(value.composer.enable_mcp).toEqual(enabledMCP);
    expect(value.composer.allowed_mcp_tools).toEqual(allowedMCP);
    expect(value.composer.allowed_skills).toEqual(payload.runtimeSettings.skills.allowed_skills);
  });

  it('preserves ordered selections, decimal IDs and exact optional-object absence', () => {
    const value = createInitialSubmissionV2(fixturePayload());
    expect(value).toEqual({
      schema_version: 'coze.workbench.initial_run_submission.v2',
      input: { message: 'ship it', uploaded_files: [] },
      composer: {
        model_type: '123',
        model_name: 'model-a',
        explicit_enable_skills: ['skill.b', 'skill.a'],
        allowed_skills: ['skill.b', 'skill.a'],
        enable_mcp: [],
        enable_kbs: ['kb.b', 'kb.a'],
        enable_databases: ['db.a'],
        allowed_mcp_tools: [],
      },
      config: exactRuntimeConfigV2WithoutRetryOrFailover(),
    });
    expect(value.config).not.toHaveProperty('model_retry');
    expect(value.config).not.toHaveProperty('model_failover');
    expect(value.config).not.toHaveProperty('reasoning');
  });

  it('omits enabled failover with no candidates and stringifies ordered candidates', () => {
    expect(createInitialSubmissionV2(enabledEmptyFailoverFixture()).config)
      .not.toHaveProperty('model_failover');
    expect(createInitialSubmissionV2(enabledFailoverFixture()).config.model_failover)
      .toEqual({
        candidate_model_ids: ['22', '11'],
        max_retries: 2,
        failover_empty_output: true,
        failover_finish_reasons: ['length'],
      });
  });

  it('keeps file order for turns and typed lineage only for retries', () => {
    expect(createTurnSubmissionV2(fixturePayload(), ['9', '3'], 'workbench_new_task'))
      .toMatchObject({
        kind: 'turn',
        input: { uploaded_files: [{ file_id: '9' }, { file_id: '3' }] },
        metadata: { source: 'workbench_new_task' },
      });
    expect(createRetrySubmissionV2(fixturePayload(), '77')).toMatchObject({
      kind: 'retry',
      input: { message: 'ship it', uploaded_files: [] },
      lineage: { source_run_id: '77' },
      metadata: { source: 'task_retry' },
    });
  });

  it('normalizes Human response without null or prohibited fields', () => {
    expect(normalizeHumanResponseV2({
      schema: 'coze.human_interaction_response.v1',
      interaction_id: 'interaction-1',
      kind: 'clarification',
      decision: 'answered',
      answer: 'yes',
    })).toEqual({
      schema: 'coze.human_interaction_response.v1',
      interaction_id: 'interaction-1',
      kind: 'clarification',
      decision: 'answered',
      answer: 'yes',
    });
  });
});
```

- [ ] **Step 3: Run the new test and observe RED**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/thread-client/__tests__/canonical-typed-submission-v2.test.ts`

Expected: FAIL because `canonical-typed-submission-v2.ts` does not exist.

- [ ] **Step 4: Implement the focused generated-type serializer**

Use generated types, copy every list to preserve order, convert safe model numbers to decimal strings, omit `reasoning`, omit disabled retry/failover objects, and reject non-safe model IDs before transport:

```ts
import type {
  CanonicalHumanInteractionResponseV2,
  CanonicalInitialRunSubmissionV2,
  CanonicalRunConfigV2,
  CanonicalRunSubmissionV2,
} from '@coze-studio/api-schema/workbench-thread';
import type { HumanInteractionResponse } from './types';
import type { WorkbenchComposerSubmitPayload } from '../components/types';

const modelID = (value: number | undefined) => {
  if (value === undefined) return undefined;
  if (!Number.isSafeInteger(value) || value <= 0) throw new Error('模型 ID 无效');
  return String(value);
};

const configV2 = (p: WorkbenchComposerSubmitPayload): CanonicalRunConfigV2 => ({
  runtime: 'eino_adk',
  memory_retrieval: { ...p.runtimeSettings.memory_retrieval, scopes: [...p.runtimeSettings.memory_retrieval.scopes] },
  skills: { enabled: p.runtimeSettings.skills.enabled, visibility: 'deferred' },
  mcp_tools: { enabled: p.runtimeSettings.mcp_tools.enabled, visibility: 'deferred' },
  web_tools: {
    enabled: p.runtimeSettings.web_tools.enabled,
    visibility: 'deferred',
    http: { ...p.runtimeSettings.web_tools.http, allowed_hosts: [...p.runtimeSettings.web_tools.http.allowed_hosts] },
    search: { ...p.runtimeSettings.web_tools.search },
  },
  ...(p.runtimeSettings.model_retry.enabled ? { model_retry: {
    max_retries: p.runtimeSettings.model_retry.max_retries,
    backoff_ms: p.runtimeSettings.model_retry.backoff_ms,
    retry_empty_output: p.runtimeSettings.model_retry.retry_empty_output,
    retry_finish_reasons: [...p.runtimeSettings.model_retry.retry_finish_reasons],
  }} : {}),
  ...(p.runtimeSettings.model_failover.enabled &&
  p.runtimeSettings.model_failover.candidate_model_ids.length > 0 ? { model_failover: {
    candidate_model_ids: p.runtimeSettings.model_failover.candidate_model_ids.map(id => modelID(id)!),
    max_retries: Math.min(
      p.runtimeSettings.model_failover.max_retries,
      p.runtimeSettings.model_failover.candidate_model_ids.length,
    ),
    failover_empty_output: p.runtimeSettings.model_failover.failover_empty_output,
    failover_finish_reasons: [...p.runtimeSettings.model_failover.failover_finish_reasons],
  }} : {}),
  token_usage: { ...p.runtimeSettings.token_usage },
});

const base = (p: WorkbenchComposerSubmitPayload) => ({
  input: { message: p.message, uploaded_files: [] },
  composer: {
    ...(modelID(p.modelType) ? { model_type: modelID(p.modelType) } : {}),
    ...(p.modelName ? { model_name: p.modelName } : {}),
    ...(p.enable_skills === undefined ? {} : { explicit_enable_skills: [...p.enable_skills] }),
    allowed_skills: [...p.runtimeSettings.skills.allowed_skills],
    enable_mcp: [...p.enable_mcp],
    enable_kbs: [...p.enable_kbs],
    enable_databases: [...p.enable_databases],
    allowed_mcp_tools: [...(p.runtimeSettings.mcp_tools.allowed_tools ?? [])],
  },
  config: configV2(p),
});

export const createInitialSubmissionV2 = (p: WorkbenchComposerSubmitPayload): CanonicalInitialRunSubmissionV2 => ({
  schema_version: 'coze.workbench.initial_run_submission.v2', ...base(p),
});
export const createTurnSubmissionV2 = (p: WorkbenchComposerSubmitPayload, ids: string[], source: 'workbench_new_task' | 'workbench_detail_followup'): CanonicalRunSubmissionV2 => ({
  schema_version: 'coze.workbench.run_submission.v2', kind: 'turn', ...base(p),
  input: { message: p.message, uploaded_files: ids.map(file_id => ({ file_id })) }, metadata: { source },
});
export const createRetrySubmissionV2 = (p: WorkbenchComposerSubmitPayload, sourceRunID: string): CanonicalRunSubmissionV2 => ({
  schema_version: 'coze.workbench.run_submission.v2', kind: 'retry', ...base(p),
  lineage: { source_run_id: sourceRunID }, metadata: { source: 'task_retry' },
});
export const normalizeHumanResponseV2 = (r: HumanInteractionResponse): CanonicalHumanInteractionResponseV2 =>
  r.kind === 'clarification'
    ? { schema: r.schema, interaction_id: r.interaction_id, kind: r.kind, decision: r.decision, ...(r.answer ? { answer: r.answer } : {}), ...(r.choice_id ? { choice_id: r.choice_id } : {}) }
    : { schema: r.schema, interaction_id: r.interaction_id, kind: r.kind, decision: r.decision, ...(r.comment ? { comment: r.comment } : {}) };
```

- [ ] **Step 5: Run serializer and generated-contract tests GREEN**

Run:

```bash
cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/thread-client/__tests__/canonical-typed-submission-v2.test.ts
cd ../../../frontend/packages/arch/api-schema && rushx test -- src/__tests__/workbench-thread-contract.test.ts
```

Expected: both test files PASS.

- [ ] **Step 6: Commit the serializer boundary**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-typed-submission-v2.ts frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-typed-submission-v2.test.ts
git commit -m "feat(workbench): add typed submission v2 serializer"
```

### Task 2: Make `canonicalThreadClient` version-exclusive and V2-capable

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/workbench-thread-client.ts:113-207`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts:1200-1520`
- Test: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts`
- Test: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts`

- [ ] **Step 1: Add failing transport snapshots for atomic/deferred, turn/retry, and Resume**

Add table cases that call the public client and assert the mutation selector is exactly one of `initial_submission_v2`, `deferred_initial_submission_v2`, `submission_v2`, or `response_v2`; Resume must also assert `Idempotency-Key`. Thread V2 keeps the closed root Thread metadata derived only from `title` and source, but has no V1 `coze` or execution fields. Run/Resume has no root V1 `input`, `config`, `context`, `metadata`, `coze`, or `response`; the typed envelope's own closed `metadata.source` is expected.

- [ ] **Step 2: Run transport tests RED**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts`

Expected: FAIL because request interfaces/client branches do not accept or emit V2.

- [ ] **Step 3: Add exact version-exclusive request unions**

```ts
type V2ThreadSafe = WorkbenchScopedRequest & WorkbenchAbortOptions &
  WorkbenchIdempotencyOptions & {
    title?: string;
    message?: never; assistant_id?: never; command?: never; config?: never;
    context?: never; metadata?: never; stream_mode?: never;
    multitask_strategy?: never; on_disconnect?: never; durability?: never;
    defer_start?: never;
  };
type V2ThreadSubmission = V2ThreadSafe & (
  | { initial_submission_v2: CanonicalInitialRunSubmissionV2; deferred_initial_submission_v2?: never }
  | { initial_submission_v2?: never; deferred_initial_submission_v2: CanonicalInitialRunSubmissionV2 }
);
type V2RunSubmission = WorkbenchThreadRequest & WorkbenchAbortOptions &
  WorkbenchIdempotencyOptions & {
    assistant_id?: 'agent';
    submission_v2: CanonicalRunSubmissionV2;
    stream_mode?: string; multitask_strategy?: string; on_disconnect?: string; durability?: string;
    input?: never; command?: never; config?: never; context?: never; metadata?: never;
    message_content?: never; message_metadata?: never; attempt_kind?: never; source_run_id?: never;
  };
type V2ResumeSubmission = WorkbenchRunRequest & WorkbenchAbortOptions &
  WorkbenchIdempotencyOptions & {
    interrupt_id: string;
    response_v2: CanonicalHumanInteractionResponseV2;
    response?: never;
  };
```

Keep current V1 interfaces as the other union member; add `assistant_id?: 'agent'` and existing safe route/header options to V2. Do not add `any`, map, raw JSON, or a catch-all extension.

- [ ] **Step 4: Branch only at the existing client serialization point**

```ts
const body = 'initial_submission_v2' in request
  ? { metadata: threadMetadata, initial_submission_v2: request.initial_submission_v2 }
  : 'deferred_initial_submission_v2' in request
    ? { metadata: threadMetadata, deferred_initial_submission_v2: request.deferred_initial_submission_v2 }
    : legacyCreateThreadBody(request);

const runBody = 'submission_v2' in request
  ? { assistant_id: requestAssistantID(request.assistant_id), submission_v2: request.submission_v2, ...safeRunOptions(request) }
  : legacyCreateRunBody(request);

const resumeBody = 'response_v2' in request
  ? { interrupt_id: interruptID, response_v2: request.response_v2 }
  : { interrupt_id: interruptID, response: canonicalResumeResponse(request.response) };
```

Use small local helpers for the extracted legacy bodies and safe run options so both branches stay reviewable. Pages continue through the singleton; do not call a generated transport.

- [ ] **Step 5: Run transport and service parity tests GREEN, then commit**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts`

Expected: PASS, including pre-existing V1 cases and new V2 snapshots.

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client/workbench-thread-client.ts frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts
git commit -m "feat(workbench): transport typed submission v2"
```

### Task 3: Cut over root atomic and deferred attachment flows

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx:370-445`
- Test: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [ ] **Step 1: Change payload snapshots first**

Assert no-file submit sends `initial_submission_v2` with no idempotency key. Assert attachment submit sends `deferred_initial_submission_v2`, waits for upload, then sends one typed turn whose file IDs preserve upload response order and whose new-task key matches `${thread_id}:${request_uuid}:new-task`. Assert upload rejection makes zero Run calls.

- [ ] **Step 2: Run root tests RED**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/__tests__/workbench.test.tsx`

Expected: payload snapshots still show V1 `config/input/metadata`.

- [ ] **Step 3: Replace only the writer payloads**

```ts
const initial = createInitialSubmissionV2(submitPayload);
const response = await createTaskThread({
  space_id,
  title: undefined,
  ...(files.length ? { deferred_initial_submission_v2: initial } : { initial_submission_v2: initial }),
});
// Only after upload succeeds:
await createTaskThreadRun({
  space_id,
  thread_id: thread.thread_id,
  assistant_id: 'agent',
  submission_v2: createTurnSubmissionV2(
    submitPayload,
    (uploadResponse.data?.files ?? []).map(file => String(file.file_id)),
    'workbench_new_task',
  ),
  idempotency_key: createNewTaskRunIdempotencyKey(thread.thread_id),
});
```

Do not change navigation, optimistic thread upsert, upload APIs, error UI, or key reuse rules.

- [ ] **Step 4: Run root tests GREEN and commit**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/__tests__/workbench.test.tsx`

Expected: PASS for atomicity, upload-before-run, ordered IDs, and upload failure.

```bash
git add frontend/apps/coze-studio/src/pages/workbench/index.tsx frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx
git commit -m "feat(workbench): use typed v2 for new tasks"
```

### Task 4: Cut over follow-up and top-level retry

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts:45-105`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts:20-300`
- Test: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-follow-up.test.ts`
- Test: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-run-actions-hook.test.tsx`

- [ ] **Step 1: Make both writer suites fail on exact V2 shapes**

Follow-up snapshot: `submission_v2.kind=turn`, source `workbench_detail_followup`, ordered uploaded IDs, same captured ambiguous key, and no V1 overflow. Retry snapshot: `kind=retry`, typed `lineage.source_run_id`, input prompt retained, attachments `[]`, metadata exactly `{source:'task_retry'}`, deterministic key unchanged, and no `source_thread_id`, `requested_at`, Message, or MessageMetadata field.

- [ ] **Step 2: Run both suites RED**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/tasks/__tests__/task-follow-up.test.ts src/pages/tasks/__tests__/task-run-actions-hook.test.tsx`

Expected: FAIL on legacy writer shapes.

- [ ] **Step 3: Replace the follow-up call after upload**

```ts
await createTaskThreadRun({
  thread_id: threadId,
  space_id: spaceId,
  assistant_id: 'agent',
  submission_v2: createTurnSubmissionV2(
    payload,
    (uploadResponse.data?.files ?? []).map(file => String(file.file_id)),
    'workbench_detail_followup',
  ),
  idempotency_key: idempotencyKey ?? createFollowUpIdempotencyKey(threadId),
});
```

- [ ] **Step 4: Replace only top-level retry; leave subagent retry untouched**

```ts
const response = await createTaskThreadRun({
  thread_id: submittedTaskDetailId,
  space_id: submittedSpaceID,
  assistant_id: 'agent',
  submission_v2: createRetrySubmissionV2(retryPayload, sourceRunId),
  idempotency_key: `${submittedSpaceID}:${submittedTaskDetailId}:${sourceRunId}:task_retry`,
});
```

On 409, retain the deterministic key, do not automatically resubmit, and continue the existing authoritative refresh path. Do not alter `retryTaskThreadSubagentRun`.

- [ ] **Step 5: Run both suites GREEN and commit**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/tasks/__tests__/task-follow-up.test.ts src/pages/tasks/__tests__/task-run-actions-hook.test.tsx`

Expected: PASS for upload ordering, ambiguity reuse, same-Thread retry, refresh, stale response, and no duplicate mutation.

```bash
git add frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/task-follow-up.test.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/task-run-actions-hook.test.tsx
git commit -m "feat(tasks): use typed v2 for follow-up and retry"
```

### Task 5: Cut over Resume with a page-scoped semantic attempt

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts:990-1220`
- Test: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail-follow-up-actions.test.tsx`
- Test: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts`

- [ ] **Step 1: Add failing lifecycle tests**

Cover: timeout/network/5xx/response-decode ambiguity reuses identical normalized `response_v2` and key; changed answer or interaction allocates a new key; confirmed 2xx commits returned Run then clears attempt before refresh; refresh failure does not re-send; proven pre-dispatch abort or definitive non-409 4xx clears; 409 refreshes without rotation or automatic resubmit; task-scope change clears. Assert key matches `human-resume:<uuid>` and body contains no `response` or lineage.

- [ ] **Step 2: Run Resume tests RED**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/tasks/__tests__/task-detail-follow-up-actions.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts`

Expected: FAIL because Resume uses V1 `response` and has no captured attempt key.

- [ ] **Step 3: Add the semantic attempt ref and exact identity**

```ts
type HumanResumeAttempt = {
  fingerprint: string;
  idempotencyKey: string;
  responseV2: CanonicalHumanInteractionResponseV2;
};
const humanResumeAttemptRef = useRef<HumanResumeAttempt>();
const responseV2 = normalizeHumanResponseV2(response);
const fingerprint = JSON.stringify([
  submittedSpaceID, submittedTaskDetailId,
  pendingHumanInteraction.sourceRunId,
  pendingHumanInteraction.interruptId,
  responseV2,
]);
if (humanResumeAttemptRef.current?.fingerprint !== fingerprint) {
  humanResumeAttemptRef.current = {
    fingerprint,
    idempotencyKey: `human-resume:${globalThis.crypto.randomUUID()}`,
    responseV2,
  };
}
```

Clear the ref in the existing task-scope effects. Pass `response_v2` and `idempotency_key` through `resumeTaskThreadRun`. This layer cannot prove whether an `AbortError` happened before or after fetch dispatch, so it conservatively retains every AbortError and every non-`WorkbenchClientError`. Use the existing public `WorkbenchClientError.status` and `outcome` fields with this exact helper:

```ts
type HumanResumeAttemptDisposition = 'retain' | 'clear';
const humanResumeAttemptDisposition = (
  error: unknown,
): HumanResumeAttemptDisposition => {
  if (!(error instanceof WorkbenchClientError)) {
    return 'retain'; // AbortError, network, timeout and decode ambiguity
  }
  if (error.status === 409) {
    return 'retain';
  }
  return error.outcome === 'rejected' &&
    error.status !== undefined &&
    error.status >= 400 &&
    error.status < 500
    ? 'clear'
    : 'retain';
};
```

The RED table must cover AbortError (retain), `outcome=unknown` network/decode (retain), 5xx `failed` (retain), 409 `rejected` (retain), non-409 4xx `rejected` (clear), success (clear after local commit), changed semantic fingerprint (replace with a new key), and task scope change (clear). Never auto-resubmit.

- [ ] **Step 4: Commit locally before refresh**

After 2xx, first run `commitTopLevelRun?.(resumeResult.data)`, then clear `humanResumeAttemptRef.current`, then fetch detail. Keep the successful-mutation guard so refresh failure cannot cause a second write.

- [ ] **Step 5: Run Resume suites GREEN and commit**

Run: `cd frontend/apps/coze-studio && rushx test -- src/pages/tasks/__tests__/task-detail-follow-up-actions.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts`

Expected: all lifecycle cases PASS.

```bash
git add frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail-follow-up-actions.test.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts
git commit -m "feat(tasks): make typed resume idempotent"
```

### Task 6: Prove all five writers, update authority, and close only C3i2

**Files:**
- Modify: `docs/superpowers/context/workbench-chat.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Modify: `scripts/workbench-execution-graph/contract.mjs`
- Test: all focused frontend files above

- [ ] **Step 1: Run the structural writer scan**

Run:

```bash
rg -n "createTaskThread\(|createTaskThreadRun\(|resumeTaskThreadRun\(" frontend/apps/coze-studio/src/pages/workbench/index.tsx frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts
rg -n "\b(input|config|context|metadata|coze|response):|interrupt_before|interrupt_after|checkpoint_during|checkpoint_after|checkpoint_id|goto|update" frontend/apps/coze-studio/src/pages/workbench/index.tsx frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts
```

Expected: exactly five first-party flow call sites use the appropriate V2 field; the second scan has no execution-overflow object in those calls and no retired-control emission. Legitimate UI state/property matches must be manually inspected, not blindly deleted.

- [ ] **Step 2: Run the complete focused frontend matrix**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/canonical-typed-submission-v2.test.ts src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-follow-up.test.ts src/pages/tasks/__tests__/task-run-actions-hook.test.tsx src/pages/tasks/__tests__/task-detail-follow-up-actions.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts
rushx lint
rushx build
```

Expected: focused Vitest, lint, bundled TypeScript checking, and production build all exit 0.

- [ ] **Step 3: Update authority with the narrow truth**

Record: “C3i1 accepts typed V2 and C3i2's five first-party writers now use it.” Also record the explicit non-claims: Human Resume is not globally closed, V1 readers/third-party compatibility remain, real MySQL typed recovery race is not verified, and P1M is not PASS. Update the execution graph node/edges/anchors only where the five writer paths changed; do not publish `graphify-out/`.

- [ ] **Step 4: Verify both authority sources and derived graph**

Run:

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
git diff --check
git status --short
```

Expected: all graph commands and `git diff --check` exit 0; status contains only intentional C3i2 authority changes plus the two untouched user-owned untracked plan files.

- [ ] **Step 5: Request independent exact-diff review and commit the authority closeout**

Require zero unresolved P0/P1/P2 findings for version exclusivity, payload loss, idempotency rotation, duplicate writes, attachment order, and overclaiming. Fix findings and rerun the affected fresh commands before committing.

```bash
git add docs/superpowers/context/workbench-chat.md docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json docs/superpowers/context/project-context.md docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md scripts/workbench-execution-graph/contract.mjs
git commit -m "docs(workbench): record typed v2 writer cutover"
```

Expected final state: C3i2 is committed and reviewable; no merge, push, deploy, V1 retirement, gate-on enrollment, or P1M PASS claim is performed by this plan.
