# Workflow LLM Prompt Required Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent workflow LLM nodes from calling a model without any effective system or user prompt, while showing the existing required-field feedback in the editor.

**Architecture:** Keep prompt semantics unchanged: either system prompt or user prompt may contain fixed text or variable expressions. Add one shared rule at the frontend form boundary and one fail-closed rule in the backend canvas/runtime prompt path. The backend accepts a system-only prompt by retaining the existing empty user-message compatibility behavior.

**Tech Stack:** React/TypeScript, Flowgram form validation, Vitest, Go, Eino prompt messages, Go test.

---

### Task 1: Add the frontend system-or-user prompt validation rule

**Files:**
- Create: `frontend/packages/workflow/playground/src/nodes-v2/llm/validators/llm-prompt-validator.ts`
- Create: `frontend/packages/workflow/playground/src/nodes-v2/llm/validators/__tests__/llm-prompt-validator.test.ts`
- Modify: `frontend/packages/workflow/playground/src/nodes-v2/llm/llm-form-meta.tsx`
- Modify: `frontend/packages/workflow/playground/src/nodes-v2/llm/user-prompt/index.tsx`

- [x] **Step 1: Write the failing validator test**

Create a pure predicate with this contract:

```ts
export const isLLMPromptPairValid = (
  systemPrompt?: string,
  userPrompt?: string,
): boolean => false;
```

The test must assert that the predicate returns `false` for `undefined`, empty strings, and
whitespace-only strings, and returns `true` for system-only fixed text, user-only fixed text,
and variable templates such as `{{input}}`.

- [x] **Step 2: Run the focused frontend test and verify it fails for the missing rule**

Run:

```bash
cd frontend
pnpm exec vitest --run packages/workflow/playground/src/nodes-v2/llm/validators/__tests__/llm-prompt-validator.test.ts
```

Expected: the test fails because the validator is not implemented.

- [x] **Step 3: Implement the minimal validator and connect form validation**

Implement the predicate using `trim()` and use the existing i18n error key from the form
validator. Register it on the
user-prompt form field in `LLM_FORM_META`; read the system prompt from `formValues` and do
not inspect whether either value contains a variable. Preserve the existing model-specific
rule: models declaring `is_up_required` still require the user prompt exactly as before. For
other models, apply the new system-or-user rule. Keep the existing model-switch
revalidation and required-indicator rendering.

Add the system-prompt field to the user-prompt field dependencies so changing either prompt
revalidates the existing feedback. Keep the existing `FormItemFeedback` under the user
prompt as the single validation feedback location and do not change the prompt editor layout.

- [x] **Step 4: Run the focused frontend test and verify it passes**

Run the same Vitest command. Expected: all prompt-pair cases pass.

### Task 2: Add backend fail-closed prompt validation

**Files:**
- Modify: `backend/domain/workflow/internal/nodes/llm/prompt.go`
- Modify: `backend/domain/workflow/internal/nodes/llm/llm.go`
- Modify: `backend/domain/workflow/internal/nodes/llm/prompt_test.go`

- [x] **Step 1: Write failing backend tests**

Add tests for a pure prompt-content validator:

```go
func TestValidatePromptPair(t *testing.T) {
    require.Error(t, validatePromptPair("", ""))
    require.Error(t, validatePromptPair(" \n", "\t"))
    require.NoError(t, validatePromptPair("role", ""))
    require.NoError(t, validatePromptPair("", "{{input}}"))
}
```

Also add a message-level case showing that an empty system message and empty user message
are rejected, while a system-only message is accepted.

- [x] **Step 2: Run the focused backend test and verify it fails**

Run:

```bash
cd backend
go test -gcflags="all=-l -N" ./domain/workflow/internal/nodes/llm -run 'TestValidatePromptPair|TestValidatePromptMessages'
```

Expected: compilation or assertion failure because the validators do not exist yet.

- [x] **Step 3: Implement validation at both static and rendered prompt boundaries**

Add a package-local `validatePromptPair(systemPrompt, userPrompt string) error` that trims
both values and returns an error containing `system prompt or user prompt is required` when
both are empty. Call it in `Config.Adapt` immediately after copying the converted LLM
parameters, so malformed saved canvases fail before model construction.

Add a message-level check in `prompts.Format` after rendering both templates and before the
existing empty-user-message compatibility fallback. Treat a message as effective when it
has non-whitespace text or non-empty multimodal content. Reject only when both rendered
messages are ineffective; preserve the existing empty user message when the system message
is effective.

- [x] **Step 4: Run the focused backend test and verify it passes**

Run the same Go test command. Expected: all static and message-level validation cases pass.

### Task 3: Run regression verification and inspect the final diff

**Files:**
- No additional files.

- [x] **Step 1: Run frontend workflow package tests**

Run:

```bash
cd frontend
pnpm --filter @coze-workflow/playground test -- --runInBand
```

Expected: exit code 0.

- [x] **Step 2: Run backend LLM package tests**

Run:

```bash
cd backend
go test -gcflags="all=-l -N" ./domain/workflow/internal/nodes/llm
```

Expected: exit code 0.

- [x] **Step 3: Run formatting and diff checks**

Run:

```bash
gofmt -w backend/domain/workflow/internal/nodes/llm/prompt.go backend/domain/workflow/internal/nodes/llm/llm.go backend/domain/workflow/internal/nodes/llm/prompt_test.go
git diff --check
git status --short
```

Expected: no whitespace errors; only the planned prompt validation files are changed in the
current defect branch.

- [x] **Step 4: Perform browser regression**

On the existing local workflow page, verify that an LLM node with both prompt fields empty
shows the standard required feedback and does not create a successful model execution. Then
enter only a system prompt or only a user prompt, run the workflow with a simple input, and
verify that execution succeeds with the configured prompt content.

- [x] **Step 5: Commit the implementation**

```bash
git add frontend/packages/workflow/playground/src/nodes-v2/llm backend/domain/workflow/internal/nodes/llm
git commit -m "fix: validate workflow llm prompts before execution"
```
