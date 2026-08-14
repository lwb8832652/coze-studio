# P1M-C1 Mode-free Typed Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add pure Go admission/decision contracts, fail-closed validators, and a deterministic gate-off baseline decision producer without wiring them into production execution or persistence.

**Architecture:** Domain entity files define data-only, mode-free value types. Application files own pure validation and the baseline producer; neither layer performs I/O, persistence, ID allocation, runtime selection, or ADK wiring. Workbench authority is updated only to record an implemented-but-unwired boundary.

**Tech Stack:** Go, standard library `errors`/`fmt`/`reflect`/`strings`, table-driven Go tests, Workbench execution-graph Node tooling.

---

## Scope guard

Implementation may create exactly these six files:

- `backend/domain/agentthread/entity/adaptive_execution.go`
- `backend/domain/agentthread/entity/adaptive_execution_test.go`
- `backend/application/agentthread/adaptive_admission.go`
- `backend/application/agentthread/adaptive_admission_test.go`
- `backend/application/agentthread/adaptive_baseline_decision.go`
- `backend/application/agentthread/adaptive_baseline_decision_test.go`

Authority work may modify only current Workbench authority documents and the graph digest contract. Do not modify repository, MySQL, runtime config, executor, IDL, generated clients, frontend, migrations, or the deferred P1L plan. If any such change appears necessary, stop and return to design.

### Task 1: Define the mode-free contracts and pure validators

**Files:**

- Create: `backend/domain/agentthread/entity/adaptive_execution.go`
- Create: `backend/domain/agentthread/entity/adaptive_execution_test.go`
- Create: `backend/application/agentthread/adaptive_admission.go`
- Create: `backend/application/agentthread/adaptive_admission_test.go`

- [ ] **Step 1: Write the failing entity contract tests**

Create compile-time and reflection tests that require the exact public data shape, enum values, and absence of JSON tags/retired product controls:

```go
func TestAdaptiveExecutionContractConstants(t *testing.T) {
	require.Equal(t, "workbench-adaptive-admission.v1", AdaptiveAdmissionSchemaV1)
	require.Equal(t, "workbench-adaptive-decision.v1", ExecutionDecisionSchemaV1)
	require.Equal(t, AdaptiveAdmissionSource("fresh"), AdaptiveAdmissionSourceFresh)
	require.Equal(t, AdaptiveAdmissionSource("typed_inheritance"), AdaptiveAdmissionSourceTypedInheritance)
	require.Equal(t, AdaptiveAdmissionSource("legacy_decoder"), AdaptiveAdmissionSourceLegacyDecoder)
	require.Equal(t, "workbench-adaptive-legacy-decoder.v1", AdaptiveLegacyDecoderVersionV1)
	require.Equal(t, ExecutionDecisionKind("clarification"), ExecutionDecisionClarification)
	require.Equal(t, ExecutionDecisionKind("direct"), ExecutionDecisionDirect)
	require.Equal(t, ExecutionDecisionKind("execute"), ExecutionDecisionExecute)
	require.Equal(t, ExecutionShape("single_step"), ExecutionShapeSingleStep)
	require.Equal(t, ExecutionShape("multi_step"), ExecutionShapeMultiStep)
}

func TestAdaptiveExecutionContractsAreInternalModeFreeValues(t *testing.T) {
	for _, value := range []any{
		AdaptiveAdmissionSnapshot{}, AdaptiveCapabilities{}, AdaptiveLimits{},
		ExecutionDecision{}, AdaptiveAcceptanceCheck{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			require.Empty(t, field.Tag.Get("json"), "%s.%s must not freeze a wire codec", typeOf.Name(), field.Name)
			require.NotContains(t, []string{
				"RequestedPolicy", "Mode", "ThinkingEnabled", "ReasoningEffort",
				"IsPlanMode", "SubagentEnabled", "MaxConcurrentSubagents",
			}, field.Name)
		}
	}
}
```

- [ ] **Step 2: Run the entity tests and verify the RED is feature-missing**

Run from `backend`:

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/agentthread/entity -run '^TestAdaptiveExecutionContract' -count=1
```

Expected: compile failure naming missing `AdaptiveAdmissionSnapshot`, `ExecutionDecision`, or their constants. A fixture, dependency, or unrelated compile failure is not an acceptable RED.

- [ ] **Step 3: Add the exact domain value types**

Create `adaptive_execution.go` with the standard repository copyright header and these data-only declarations:

```go
package entity

const (
	AdaptiveAdmissionSchemaV1      = "workbench-adaptive-admission.v1"
	AdaptiveLegacyDecoderVersionV1 = "workbench-adaptive-legacy-decoder.v1"
	ExecutionDecisionSchemaV1      = "workbench-adaptive-decision.v1"
)

type AdaptiveAdmissionSource string

const (
	AdaptiveAdmissionSourceFresh            AdaptiveAdmissionSource = "fresh"
	AdaptiveAdmissionSourceTypedInheritance AdaptiveAdmissionSource = "typed_inheritance"
	AdaptiveAdmissionSourceLegacyDecoder     AdaptiveAdmissionSource = "legacy_decoder"
)

type AdaptiveCapabilities struct {
	PlanAllowed             bool
	ReadOnlyToolsAllowed    bool
	SandboxWritesAllowed    bool
	HumanInteractionAllowed bool
	SubagentsAllowed        bool
}

type AdaptiveLimits struct {
	MaxToolCalls                 int
	MaxReplans                   int
	MaxVerificationRepairs       int
	MaxConsecutiveNoProgress     int
	MaxActiveDurationSeconds     int
}

type AdaptiveAdmissionSnapshot struct {
	Schema                    string
	FeatureGateEnabled        bool
	Source                    AdaptiveAdmissionSource
	SourceRunID               *int64
	SourceExecutionGeneration *uint64
	SourceConfigDigest        string
	DecoderVersion            string
	Capabilities              AdaptiveCapabilities
	Limits                    AdaptiveLimits
}

type ExecutionDecisionKind string

const (
	ExecutionDecisionClarification ExecutionDecisionKind = "clarification"
	ExecutionDecisionDirect        ExecutionDecisionKind = "direct"
	ExecutionDecisionExecute       ExecutionDecisionKind = "execute"
)

type ExecutionShape string

const (
	ExecutionShapeSingleStep ExecutionShape = "single_step"
	ExecutionShapeMultiStep  ExecutionShape = "multi_step"
)

type AdaptiveAcceptanceCheck struct {
	CheckID         string
	Kind            string
	TargetRef       string
	SafeDescription string
}

type ExecutionDecision struct {
	Schema                    string
	DecisionID                string
	DecisionRevision          uint64
	ExecutionRunID            int64
	JournalRunID              int64
	AttemptID                 string
	ExecutionGeneration       uint64
	PlanScopeRunID            *int64
	GoalSummary               string
	Deliverables              []string
	AcceptanceChecks          []AdaptiveAcceptanceCheck
	Decision                  ExecutionDecisionKind
	ExecutionShape            ExecutionShape
	ClarificationQuestion     *string
	SafeSummary               string
	CreatedAt                 int64
}
```

- [ ] **Step 4: Run the entity tests and verify GREEN**

Run the command from Step 2. Expected: PASS.

- [ ] **Step 5: Write failing validator tests**

Create table-driven tests in `adaptive_admission_test.go`. Use these canonical helpers so every invalid case changes exactly one contract field:

```go
func validFreshAdaptiveAdmission() domainentity.AdaptiveAdmissionSnapshot {
	return domainentity.AdaptiveAdmissionSnapshot{
		Schema: domainentity.AdaptiveAdmissionSchemaV1,
		Source: domainentity.AdaptiveAdmissionSourceFresh,
		Capabilities: domainentity.AdaptiveCapabilities{
			PlanAllowed: true,
			ReadOnlyToolsAllowed: true,
			HumanInteractionAllowed: true,
		},
		Limits: domainentity.AdaptiveLimits{
			MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
			MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
		},
	}
}

func validMultiStepDecision() domainentity.ExecutionDecision {
	planScopeRunID := int64(30)
	return domainentity.ExecutionDecision{
		Schema: domainentity.ExecutionDecisionSchemaV1,
		DecisionID: "decision-1", DecisionRevision: 1,
		ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
		ExecutionGeneration: 1, PlanScopeRunID: &planScopeRunID,
		GoalSummary: "Execute the submitted task.",
		Deliverables: []string{}, AcceptanceChecks: []domainentity.AdaptiveAcceptanceCheck{},
		Decision: domainentity.ExecutionDecisionExecute,
		ExecutionShape: domainentity.ExecutionShapeMultiStep,
		SafeSummary: "Use the baseline multi-step execution path.", CreatedAt: 1,
	}
}
```

Required test groups and assertions:

```go
func TestValidateAdaptiveAdmissionSnapshot(t *testing.T) {
	// Accept fresh, typed inheritance, and legacy decoder exact tuples.
	// Reject unknown/mixed sources, zero lineage, non-lowercase/non-64 digest,
	// arbitrary decoder version, legacy gate-on, SubagentsAllowed, zero/over-limit limits.
	// Every rejection must satisfy errors.Is(err, ErrAdaptiveAdmissionInvalid).
}

func TestValidateExecutionDecision(t *testing.T) {
	// Accept clarification, direct, execute/single_step and execute/multi_step.
	// Reject every XOR mismatch, nil collections, zero IDs/revision/generation/time,
	// empty required text, and each exact byte/count budget overflow.
	// Every rejection must satisfy errors.Is(err, ErrExecutionDecisionInvalid).
}

func TestValidateExecutionDecisionAgainstAdmission(t *testing.T) {
	// Require PlanAllowed for multi_step and HumanInteractionAllowed for clarification.
	// Assert errors.Is(err, ErrAdaptiveDecisionBlockedPolicy).
	// Deep-copy decision first and require.Equal after rejection to prove no downgrade/mutation.
}
```

- [ ] **Step 6: Run validator tests and verify the RED is feature-missing**

Run from `backend`:

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/agentthread -run '^TestValidate(AdaptiveAdmissionSnapshot|ExecutionDecision|ExecutionDecisionAgainstAdmission)$' -count=1
```

Expected: compile failure naming the missing validators/errors.

- [ ] **Step 7: Implement pure fail-closed validation**

Create `adaptive_admission.go` with stable sentinel errors and no I/O:

```go
var (
	ErrAdaptiveAdmissionInvalid      = errors.New("adaptive admission is invalid")
	ErrExecutionDecisionInvalid      = errors.New("execution decision is invalid")
	ErrAdaptiveDecisionBlockedPolicy = errors.New("adaptive decision is blocked by policy")
)

const (
	maxAdaptiveToolCalls             = 24
	maxAdaptiveReplans               = 2
	maxAdaptiveVerificationRepairs   = 2
	maxAdaptiveConsecutiveNoProgress = 3
	maxAdaptiveActiveDurationSeconds = 1200
	maxAdaptiveOpaqueIDBytes         = 191
	maxAdaptiveSummaryBytes          = 1024
	maxAdaptiveDeliverables          = 16
	maxAdaptiveDeliverableBytes      = 512
	maxAdaptiveAcceptanceChecks      = 32
	maxAdaptiveCheckKindBytes        = 64
	maxAdaptiveCheckDescriptionBytes = 512
)

func ValidateAdaptiveAdmissionSnapshot(snapshot domainentity.AdaptiveAdmissionSnapshot) error
func ValidateExecutionDecision(decision domainentity.ExecutionDecision) error
func ValidateExecutionDecisionAgainstAdmission(
	snapshot domainentity.AdaptiveAdmissionSnapshot,
	decision domainentity.ExecutionDecision,
) error
```

Implementation rules:

- use `len(string)` so budgets are bytes, not runes;
- validate schema and complete source tuple before capabilities/limits;
- validate the legacy digest with exactly 64 lowercase `[0-9a-f]` bytes;
- reject nil `Deliverables` and nil `AcceptanceChecks`, while accepting non-nil empty slices;
- require nonempty/bounded `DecisionID`, `AttemptID`, `GoalSummary`, and `SafeSummary`;
- require positive revision, run IDs, generation, optional Plan ID, and Unix milliseconds;
- implement the four XOR forms exactly;
- call both structural validators before capability checks;
- return `ErrAdaptiveDecisionBlockedPolicy` only for structurally valid Plan/Human capability mismatch;
- never mutate either input.

- [ ] **Step 8: Run focused and package tests**

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/agentthread/entity ./application/agentthread -run 'AdaptiveAdmission|ExecutionDecision' -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/agentthread/entity ./application/agentthread -run '^$' -count=1
```

Expected: both commands PASS; the second reports successful compile/no tests to run.

- [ ] **Step 9: Verify scope and commit Task 1**

```bash
gofmt -w backend/domain/agentthread/entity/adaptive_execution.go \
  backend/domain/agentthread/entity/adaptive_execution_test.go \
  backend/application/agentthread/adaptive_admission.go \
  backend/application/agentthread/adaptive_admission_test.go
git diff --check
git status --short
git add backend/domain/agentthread/entity/adaptive_execution.go \
  backend/domain/agentthread/entity/adaptive_execution_test.go \
  backend/application/agentthread/adaptive_admission.go \
  backend/application/agentthread/adaptive_admission_test.go
git commit -m "feat: add mode-free adaptive contracts"
```

Expected: only the four Task 1 files are staged; the deferred untracked P1L plan remains unstaged.

### Task 2: Add the deterministic gate-off baseline producer

**Files:**

- Create: `backend/application/agentthread/adaptive_baseline_decision.go`
- Create: `backend/application/agentthread/adaptive_baseline_decision_test.go`

- [ ] **Step 1: Write the failing baseline producer tests**

Use a valid fresh admission and freeze the complete expected value:

```go
func TestBaselineDecisionProducerProducesFixedGateOffDecision(t *testing.T) {
	admission := validFreshAdaptiveAdmission()
	request := BaselineDecisionRequest{
		Admission: admission,
		DecisionID: "decision-1", DecisionRevision: 2,
		ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
		ExecutionGeneration: 3, PlanScopeRunID: 30, CreatedAt: 1710000000000,
	}

	result, err := (BaselineDecisionProducer{}).Produce(request)
	require.NoError(t, err)
	require.Equal(t, domainentity.ExecutionDecisionExecute, result.Decision)
	require.Equal(t, domainentity.ExecutionShapeMultiStep, result.ExecutionShape)
	require.Equal(t, "Execute the submitted task.", result.GoalSummary)
	require.Equal(t, "Use the baseline multi-step execution path.", result.SafeSummary)
	require.NotNil(t, result.Deliverables)
	require.Empty(t, result.Deliverables)
	require.NotNil(t, result.AcceptanceChecks)
	require.Empty(t, result.AcceptanceChecks)
	require.NotNil(t, result.PlanScopeRunID)
	require.Equal(t, request.PlanScopeRunID, *result.PlanScopeRunID)
	require.Equal(t, admission, request.Admission)
}

func TestBaselineDecisionProducerFailsClosed(t *testing.T) {
	// gate-on => ErrAdaptiveProducerUnavailable
	// PlanAllowed=false => ErrAdaptiveDecisionBlockedPolicy
	// invalid admission and every zero request identity => the structural sentinel
}

func TestBaselineDecisionProducerIsDeterministic(t *testing.T) {
	// Produce twice from the same request and require.Equal complete values.
	// Assert the request and nested admission remain value-equal to a deep copy.
}
```

- [ ] **Step 2: Run the focused test and verify the RED is feature-missing**

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/agentthread -run '^TestBaselineDecisionProducer' -count=1
```

Expected: compile failure naming missing `BaselineDecisionProducer` or `BaselineDecisionRequest`.

- [ ] **Step 3: Implement the minimal pure producer**

Create `adaptive_baseline_decision.go`:

```go
var ErrAdaptiveProducerUnavailable = errors.New("adaptive decision producer is unavailable")

type BaselineDecisionRequest struct {
	Admission             domainentity.AdaptiveAdmissionSnapshot
	DecisionID            string
	DecisionRevision      uint64
	ExecutionRunID        int64
	JournalRunID          int64
	AttemptID             string
	ExecutionGeneration   uint64
	PlanScopeRunID        int64
	CreatedAt             int64
}

type BaselineDecisionProducer struct{}

func (BaselineDecisionProducer) Produce(request BaselineDecisionRequest) (domainentity.ExecutionDecision, error) {
	if err := ValidateAdaptiveAdmissionSnapshot(request.Admission); err != nil {
		return domainentity.ExecutionDecision{}, err
	}
	if request.Admission.FeatureGateEnabled {
		return domainentity.ExecutionDecision{}, ErrAdaptiveProducerUnavailable
	}
	if !request.Admission.Capabilities.PlanAllowed {
		return domainentity.ExecutionDecision{}, ErrAdaptiveDecisionBlockedPolicy
	}
	planScopeRunID := request.PlanScopeRunID
	decision := domainentity.ExecutionDecision{
		Schema: domainentity.ExecutionDecisionSchemaV1,
		DecisionID: request.DecisionID, DecisionRevision: request.DecisionRevision,
		ExecutionRunID: request.ExecutionRunID, JournalRunID: request.JournalRunID,
		AttemptID: request.AttemptID, ExecutionGeneration: request.ExecutionGeneration,
		PlanScopeRunID: &planScopeRunID,
		GoalSummary: "Execute the submitted task.",
		Deliverables: []string{}, AcceptanceChecks: []domainentity.AdaptiveAcceptanceCheck{},
		Decision: domainentity.ExecutionDecisionExecute,
		ExecutionShape: domainentity.ExecutionShapeMultiStep,
		SafeSummary: "Use the baseline multi-step execution path.",
		CreatedAt: request.CreatedAt,
	}
	if err := ValidateExecutionDecisionAgainstAdmission(request.Admission, decision); err != nil {
		return domainentity.ExecutionDecision{}, err
	}
	return decision, nil
}
```

Do not add context, interfaces, callbacks, model input, task text, progress, verification, persistence, JSON tags, or ID generation.

- [ ] **Step 4: Run focused and combined tests**

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/agentthread -run '^TestBaselineDecisionProducer' -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/agentthread/entity ./application/agentthread -run 'AdaptiveAdmission|ExecutionDecision|BaselineDecision' -count=1
```

Expected: both commands PASS.

- [ ] **Step 5: Verify scope and commit Task 2**

```bash
gofmt -w backend/application/agentthread/adaptive_baseline_decision.go \
  backend/application/agentthread/adaptive_baseline_decision_test.go
git diff --check
git add backend/application/agentthread/adaptive_baseline_decision.go \
  backend/application/agentthread/adaptive_baseline_decision_test.go
git commit -m "feat: add baseline execution decision producer"
```

Expected: only the two Task 2 files are staged; no production consumer is modified.

### Task 3: Record the unwired boundary and run final verification

**Files:**

- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Modify only if the graph verifier reports the expected structure mismatch: `scripts/workbench-execution-graph/contract.mjs`

- [ ] **Step 1: Add narrowly scoped authority facts**

Record all of these exact facts and no stronger claim:

- P1M-C1 defines mode-free admission/decision Go values and pure validators.
- The baseline producer is deterministic and gate-off only.
- The boundary is `implemented_unwired`: no persistence, codec, coordinator, Execute/Resume consumer, IDL, or frontend exposure exists.
- Historical runtime controls and the package-private server-owned subagent compatibility seam remain until later retirement.
- P1L remains deferred and whole-Thread DELETE remains hard-disabled.

In the graph JSON, add or update a current node for the C1 contract boundary with locators to the six new files. Connect it to P1M-B1 admission as a future/internal contract dependency, but do not add it to a production execution chain or claim it reaches ADK/persistence.

- [ ] **Step 2: Verify authority structure and update only the genuine digest mismatch**

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
```

If the only failure is `canonical_profile_structure_mismatch`, independently recompute the stable projection digest, then replace only `WORKBENCH_PROFILE_STRUCTURE_DIGEST` in `contract.mjs`. Re-run verify and require PASS. Any locator, schema, status, chain, or unrelated validation error must be fixed in the authority sources rather than hidden by a digest change.

- [ ] **Step 3: Run fresh Go verification**

From `backend`:

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/agentthread/entity ./application/agentthread -run 'AdaptiveAdmission|ExecutionDecision|BaselineDecision' -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/agentthread/entity ./application/agentthread -run '^$' -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/agentthread -count=1
```

Expected: all commands PASS. If the full application package encounters a sandbox-only loopback restriction, rerun that exact command with approved escalation; do not silently replace it with a narrower command.

- [ ] **Step 4: Run structural and source-scope checks**

```bash
gofmt -d backend/domain/agentthread/entity/adaptive_execution.go \
  backend/domain/agentthread/entity/adaptive_execution_test.go \
  backend/application/agentthread/adaptive_admission.go \
  backend/application/agentthread/adaptive_admission_test.go \
  backend/application/agentthread/adaptive_baseline_decision.go \
  backend/application/agentthread/adaptive_baseline_decision_test.go
git diff --check
rg -n 'requested_policy|thinking_enabled|reasoning_effort|is_plan_mode|subagent_enabled|max_concurrent_subagents|"auto"|"pro"|"ultra"' \
  backend/domain/agentthread/entity/adaptive_execution.go \
  backend/application/agentthread/adaptive_admission.go \
  backend/application/agentthread/adaptive_baseline_decision.go
git status --short
```

Expected: gofmt and diff checks are empty; retired-field scan has no matches; status contains only the six implementation files/commits, authority changes, and the known unstaged P1L plan.

- [ ] **Step 5: Rebuild and validate the Workbench graph**

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
node --test scripts/workbench-execution-graph.test.mjs
```

Expected: all commands PASS. Do not commit generated `graphify-out` artifacts.

- [ ] **Step 6: Obtain independent reviews**

Provide reviewers the design SHA, exact code/authority range, six-file implementation allowlist, and fresh command outputs. Require:

```text
P0=0
P1=0
```

The spec review must verify every C1 rule and non-goal. The quality review must verify purity, immutability, deterministic output, error taxonomy, test strength, source scope, and absence of production wiring. Fix and re-review every P0/P1 before proceeding.

- [ ] **Step 7: Commit authority and final state**

```bash
git add docs/superpowers/context/project-context.md \
  docs/superpowers/context/workbench-execution-chain.md \
  docs/superpowers/context/workbench-execution-graph.json \
  docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md
git diff --quiet scripts/workbench-execution-graph/contract.mjs || \
  git add scripts/workbench-execution-graph/contract.mjs
git commit -m "docs: record P1M-C1 typed contracts"
git status --short
```

Expected: the final status contains only the deferred untracked P1L plan. Do not merge, push, deploy, or enable production execution in this plan.
