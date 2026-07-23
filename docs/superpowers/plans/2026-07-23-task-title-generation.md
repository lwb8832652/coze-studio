# Task Title Generation Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace raw prompt-based task titles with clean provisional titles, then preserve the existing asynchronous AI title update after the first successful exchange.

**Architecture:** Keep title ownership in the Go `agentthread` application package. Add one deterministic source-cleaning boundary shared by thread creation, model prompting, and fallback generation; keep the existing transactional `context.thread_title_updated` event and expected-title concurrency guard unchanged.

**Tech Stack:** Go, Eino chat model, Testify, existing Coze Agent Thread application service and run processor.

---

### Task 1: Add deterministic provisional task titles

**Files:**
- Modify: `backend/application/agentthread/service.go:195-285`
- Test: `backend/application/agentthread/service_test.go`

- [ ] **Step 1: Write failing tests for provisional titles**

Add table-driven tests that call the real package helper:

```go
func TestTaskThreadTitleBuildsCleanProvisionalTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		message string
		want    string
	}{
		{
			name:    "skill creator intent",
			message: "我想创建一个技能，请先询问我技能用途、使用场景和期望输出。 @skill-creator",
			want:    "创建技能",
		},
		{
			name:    "long message",
			message: "请根据这段很长的需求整理项目上线计划，包含排期、风险、负责人、验收标准以及回滚方案",
			want:    "请根据这段很长的需求整理项目上线计划，包含排期、风险、负责人、…",
		},
		{
			name:    "resource mention only",
			message: "@skill-creator",
			want:    "新建任务",
		},
		{
			name:    "explicit title keeps caller intent",
			title:   "  @skill-creator 专项任务  ",
			message: "我想创建一个技能",
			want:    "@skill-creator 专项任务",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, taskThreadTitle(tt.title, tt.message))
		})
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```bash
cd backend
go test -gcflags="all=-l -N" ./application/agentthread \
  -run TestTaskThreadTitleBuildsCleanProvisionalTitle -count=1
```

Expected: FAIL because the current helper returns the raw message and resource mention.

- [ ] **Step 3: Implement the source cleaner and provisional-title policy**

In `service.go`, add focused package helpers:

```go
const (
	explicitTaskTitleMaxRunes    = 80
	provisionalTaskTitleMaxRunes = 32
	defaultTaskThreadTitle       = "新建任务"
)

var taskTitleResourceMentionRE = regexp.MustCompile(
	`(?i)(^|\s)@[\p{L}\p{N}_.-]+`,
)

func cleanTaskTitleSource(value string) string {
	value = taskTitleResourceMentionRE.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.Trim(value, " \t\r\n，。,.；;:：")
}

func provisionalTaskThreadTitle(message string) string {
	cleaned := cleanTaskTitleSource(message)
	if cleaned == "" {
		return defaultTaskThreadTitle
	}
	if strings.Contains(cleaned, "创建一个技能") ||
		strings.Contains(cleaned, "创建技能") {
		return "创建技能"
	}
	return truncateTaskTitle(cleaned, provisionalTaskTitleMaxRunes, true)
}

func truncateTaskTitle(value string, limit int, ellipsis bool) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	if ellipsis && limit > 1 {
		return string(runes[:limit-1]) + "…"
	}
	return string(runes[:limit])
}

func taskThreadTitle(title, message string) string {
	if explicit := strings.TrimSpace(title); explicit != "" {
		return truncateTaskTitle(explicit, explicitTaskTitleMaxRunes, false)
	}
	return provisionalTaskThreadTitle(message)
}
```

Add `regexp` to imports. Keep explicit titles free from resource-marker rewriting.

- [ ] **Step 4: Run the test and verify GREEN**

Run the same targeted command.

Expected: PASS.

### Task 2: Clean model title prompts and generated output

**Files:**
- Modify: `backend/application/agentthread/title_generator.go:122-215`
- Test: `backend/application/agentthread/title_generator_test.go`

- [ ] **Step 1: Extend the model-generator test**

Update the existing test input so both messages contain resource markers:

```go
UserMessage: "@skill-creator " + longUserMessage,
AssistantMessage: "<think>思考过程</think>推荐春秋两季。 @web-search",
```

Add assertions:

```go
require.NotContains(t, prompt, "@skill-creator")
require.NotContains(t, prompt, "@web-search")
require.Contains(t, prompt, "Do not include tool names, skill names, or @mentions.")
```

Add a second test proving generated resource markers are removed:

```go
func TestModelRunTitleGeneratorRemovesResourceMentionsFromTitle(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage(`"创建 Agent 技能 @skill-creator"`, nil),
	}
	generator := NewModelRunTitleGenerator(func(
		ctx context.Context,
		modelID int64,
	) (model.BaseChatModel, bool, error) {
		return chatModel, true, nil
	})

	title, err := generator.GenerateTitle(context.Background(), RunTitleGenerationInput{
		Run:         &RunSummary{Config: `{"model_id":100002}`},
		UserMessage: "创建技能 @skill-creator",
	})

	require.NoError(t, err)
	require.Equal(t, "创建 Agent 技能", title)
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
cd backend
go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestModelRunTitleGenerator' -count=1
```

Expected: FAIL because the prompt and normalized output still contain resource mentions.

- [ ] **Step 3: Apply the shared cleaning boundary**

In `buildRunTitlePrompt`, clean user and assistant text before truncation:

```go
userMessage := truncateRunTitlePromptText(
	cleanTaskTitleSource(input.UserMessage),
	defaultRunTitlePromptChars,
)
assistantMessage := truncateRunTitlePromptText(
	cleanTaskTitleSource(
		generatedThreadTitleThinkTagRE.ReplaceAllString(
			strings.TrimSpace(input.AssistantMessage),
			"",
		),
	),
	defaultRunTitlePromptChars,
)
```

Strengthen the final prompt:

```go
return fmt.Sprintf(
	"Generate a concise title (max %d words) for this conversation.\n"+
		"Do not include tool names, skill names, or @mentions.\n"+
		"User: %s\nAssistant: %s\n\n"+
		"Return ONLY the title, no quotes, no explanation.",
	cfg.MaxWords,
	userMessage,
	assistantMessage,
)
```

In `normalizeGeneratedThreadTitleWithLimit`, call
`cleanTaskTitleSource(title)` after stripping think tags and quotes. Return an
empty string when no semantic text remains.

- [ ] **Step 4: Run the tests and verify GREEN**

Run the same targeted command.

Expected: PASS.

### Task 3: Use the clean deterministic fallback without overriding manual titles

**Files:**
- Modify: `backend/application/agentthread/runner.go:755-890`
- Test: `backend/application/agentthread/runner_test.go:255-430`

- [ ] **Step 1: Write the failing fallback test**

Add `errors` to the test imports, then add a fallback test with a failing title
generator:

```go
type failingRunTitleGenerator struct{}

func (failingRunTitleGenerator) GenerateTitle(
	ctx context.Context,
	input RunTitleGenerationInput,
) (string, error) {
	return "", errors.New("title model unavailable")
}

func TestRunProcessorFallsBackToCleanTitleWhenGeneratorFails(t *testing.T) {
	userMessage := "我想创建一个技能，请先询问用途和期望输出。 @skill-creator"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitle("", userMessage)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{ID: 10, Title: initialTitle},
		claimedRuns: []*entity.Run{{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusRunning,
			Input:    input,
			WorkerID: "worker-a",
		}},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "请补充技能用途",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(
		app,
		RunExecutorFunc(func(
			ctx context.Context,
			run *RunSummary,
		) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "请补充技能用途"}, nil
		}),
		RunProcessorOptions{
			WorkerID:       "worker-a",
			BatchSize:      1,
			TitleGenerator: failingRunTitleGenerator{},
		},
	)

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Equal(t, "创建技能", initialTitle)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.Empty(t, domainSVC.finalizeRunSuccessReq.ThreadTitle)
	require.Nil(t, domainSVC.updateThreadTitleReq)
	require.NotContains(
		t,
		domainSVC.finalizeRunSuccessReq.TitleEventPayload,
		"@skill-creator",
	)
}
```

The existing
`TestRunProcessorDoesNotOverrideExistingThreadTitleOnFollowUp` remains the
regression test for manual-title protection and must run with this new test.

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
cd backend
go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestRunProcessor(FallsBackToCleanTitle|DoesNotOverrideExistingThreadTitle)' \
  -count=1
```

Expected: FAIL because the fallback still uses the raw user message.

- [ ] **Step 3: Route fallback generation through the provisional-title helper**

In `RunProcessor.generatedThreadTitle`, preserve the existing priority order:
explicit executor title, model title generator, deterministic fallback. Replace
the raw fallback return with:

```go
return provisionalTaskThreadTitle(userMessage)
```

Do not change `prepareGeneratedThreadTitle`: its comparison against
`taskThreadTitle("", userMessage)` is the guard that prevents generated titles
from overwriting manual edits.

- [ ] **Step 4: Run the tests and verify GREEN**

Run the same targeted command.

Expected: PASS.

### Task 4: Verify the complete title lifecycle

**Files:**
- Verify: `backend/application/agentthread`
- Verify: `frontend/apps/coze-studio/src/pages/tasks`

- [ ] **Step 1: Run the full Agent Thread package tests**

```bash
cd backend
go test -gcflags="all=-l -N" ./application/agentthread -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the frontend title synchronization tests**

```bash
cd frontend/apps/coze-studio
npx vitest run \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: PASS with the hidden `context.thread_title_updated` event still
updating the detail header and task list.

- [ ] **Step 3: Run static verification**

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
```

Expected: PASS.

- [ ] **Step 4: Verify in the Codex in-app browser**

Create a skill task from:

```text
http://localhost:8080/space/7645565700475453440/chats/new
```

Use:

```text
我想创建一个技能，请先询问我技能用途、使用场景和期望输出。 @skill-creator
```

Expected:

- While running, the task header shows `创建技能`.
- The task title contains no `@skill-creator`.
- After the first successful answer, the title changes to a concise AI title
  when the title model succeeds.
- The browser console has no new errors.

- [ ] **Step 5: Commit the implementation**

```bash
git add \
  backend/application/agentthread/service.go \
  backend/application/agentthread/service_test.go \
  backend/application/agentthread/title_generator.go \
  backend/application/agentthread/title_generator_test.go \
  backend/application/agentthread/runner.go \
  backend/application/agentthread/runner_test.go
git commit -m "feat: improve generated task titles"
```
