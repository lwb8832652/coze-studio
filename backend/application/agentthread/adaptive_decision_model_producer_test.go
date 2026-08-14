/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/stretchr/testify/require"
)

const adaptiveDecisionValidToolArguments = `{
	"schema":"coze.adaptive_decision_candidate.v1",
	"goal_summary":"Implement the requested change.",
	"deliverables":[],
	"acceptance_checks":[],
	"decision":"execute",
	"execution_shape":"single_step",
	"safe_summary":"Use one bounded implementation step."
}`

func TestModelAdaptiveDecisionProducerClaimsCallsOnceRecordsUsageAndReplays(t *testing.T) {
	order := make([]string, 0, 3)
	chatModel := &adaptiveDecisionToolModelStub{
		generate: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			order = append(order, "generate")
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, time.Now().Add(time.Second), deadline, 300*time.Millisecond)
			call, ok := ctx.Value(adkUsageCallContextKey).(adkUsageCall)
			require.True(t, ok)
			require.Equal(t, "adaptive_decision", call.usageKind)
			require.Equal(t, "test-model", call.modelName)
			require.Equal(t, "test-provider", call.provider)
			options := model.GetCommonOptions(nil, opts...)
			require.NotNil(t, options.ToolChoice)
			require.Equal(t, schema.ToolChoiceForced, *options.ToolChoice)
			require.Equal(t, []string{adaptiveDecisionModelToolName}, options.AllowedToolNames)
			prompt := adaptiveDecisionPromptText(input)
			require.Contains(t, prompt, "implement only this")
			require.Contains(t, prompt, `"has_attachments":true`)
			require.NotContains(t, prompt, "operation-key-secret")
			require.NotContains(t, prompt, "lease-token-secret")
			return adaptiveDecisionToolResponse(adaptiveDecisionValidToolArguments, &schema.TokenUsage{
				PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20,
			}), nil
		},
	}
	repo := &adaptiveDecisionModelOperationRepositoryStub{
		readErr: domainrepo.ErrAdaptiveDecisionModelOperationNotFound,
		prepareResult: &domainrepo.PrepareAdaptiveDecisionModelOperationResult{
			Owned: true,
			Operation: &domainrepo.AdaptiveDecisionModelOperation{
				Status: domainrepo.AdaptiveDecisionModelOperationStatusCalling,
			},
		},
		order: &order,
	}
	usage := &adaptiveDecisionUsageCollectorStub{order: &order}
	idGen := &adaptiveDecisionIDGeneratorStub{ids: []int64{101, 102}}
	producer := NewModelAdaptiveDecisionProducer(ModelAdaptiveDecisionProducerOptions{
		Provider: func(context.Context) (model.ToolCallingChatModel, AdaptiveDecisionModelIdentity, error) {
			return chatModel, AdaptiveDecisionModelIdentity{Provider: "test-provider", Model: "test-model"}, nil
		},
		UsageCollector: usage, OperationRepository: repo, IDGen: idGen,
		Now: func() int64 { return 1700000000 }, Timeout: time.Second,
	})
	ctx, request := adaptiveDecisionModelTestInvocation(t)

	candidate, err := producer.Produce(ctx, request)

	require.NoError(t, err)
	require.Equal(t, domainentity.ExecutionDecisionExecute, candidate.Decision)
	require.Equal(t, domainentity.ExecutionShapeSingleStep, candidate.ExecutionShape)
	require.Equal(t, []string{"generate", "usage", "complete"}, order)
	require.Len(t, chatModel.tools, 1)
	require.Equal(t, adaptiveDecisionModelToolName, chatModel.tools[0].Name)
	require.Equal(t, 1, chatModel.generateCalls)
	require.Equal(t, 1, idGen.calls)
	require.Len(t, usage.records, 1)
	require.Len(t, usage.records[0].IdempotencyKey, 64)
	require.Equal(t, "test-provider", usage.records[0].Provider)
	require.Equal(t, "test-model", usage.records[0].ModelName)
	require.Equal(t, int64(20), usage.records[0].TotalTokens)
	require.Len(t, repo.completeRequests, 1)
	require.Equal(t, domainrepo.AdaptiveDecisionModelOperationStatusCompleted, repo.completeRequests[0].Status)
	require.NotEmpty(t, repo.completeRequests[0].ResultPayload)
	require.Len(t, repo.prepareRequests[0].RequestFingerprint, 64)
	require.NotEmpty(t, repo.prepareRequests[0].ClaimToken)

	repo.readErr = nil
	repo.readOperation = repo.completedOperation
	replayed, err := producer.Produce(ctx, request)
	require.NoError(t, err)
	require.Equal(t, candidate, replayed)
	require.Equal(t, 1, chatModel.generateCalls)
	require.Equal(t, 1, idGen.calls)
}

func TestModelAdaptiveDecisionProducerNeverRecallsTerminalOrClaimOnlyOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation *domainrepo.AdaptiveDecisionModelOperation
		wantOK    bool
	}{
		{name: "completed", wantOK: true, operation: &domainrepo.AdaptiveDecisionModelOperation{
			Status:        domainrepo.AdaptiveDecisionModelOperationStatusCompleted,
			ResultPayload: []byte(adaptiveDecisionValidToolArguments),
		}},
		{name: "calling", operation: &domainrepo.AdaptiveDecisionModelOperation{
			Status: domainrepo.AdaptiveDecisionModelOperationStatusCalling,
		}},
		{name: "failed", operation: &domainrepo.AdaptiveDecisionModelOperation{
			Status: domainrepo.AdaptiveDecisionModelOperationStatusFailed, ErrorCode: "model_error",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCalls := 0
			producer := NewModelAdaptiveDecisionProducer(ModelAdaptiveDecisionProducerOptions{
				Provider: func(context.Context) (model.ToolCallingChatModel, AdaptiveDecisionModelIdentity, error) {
					providerCalls++
					return nil, AdaptiveDecisionModelIdentity{}, errors.New("must not be called")
				},
				UsageCollector:      &adaptiveDecisionUsageCollectorStub{},
				OperationRepository: &adaptiveDecisionModelOperationRepositoryStub{readOperation: test.operation},
				IDGen:               &adaptiveDecisionIDGeneratorStub{ids: []int64{1, 2}}, Now: func() int64 { return 1 },
				Timeout: time.Second,
			})
			ctx, request := adaptiveDecisionModelTestInvocation(t)

			candidate, err := producer.Produce(ctx, request)
			if test.wantOK {
				require.NoError(t, err)
				require.Equal(t, domainentity.ExecutionDecisionExecute, candidate.Decision)
			} else {
				require.Error(t, err)
			}
			require.Zero(t, providerCalls)
		})
	}
}

func TestModelAdaptiveDecisionProducerNeverCallsModelWhenPrepareLosesOwnership(t *testing.T) {
	tests := []struct {
		name      string
		operation *domainrepo.AdaptiveDecisionModelOperation
		wantOK    bool
	}{
		{name: "completed", wantOK: true, operation: &domainrepo.AdaptiveDecisionModelOperation{
			Status:        domainrepo.AdaptiveDecisionModelOperationStatusCompleted,
			ResultPayload: []byte(adaptiveDecisionValidToolArguments),
		}},
		{name: "calling", operation: &domainrepo.AdaptiveDecisionModelOperation{
			Status: domainrepo.AdaptiveDecisionModelOperationStatusCalling,
		}},
		{name: "failed", operation: &domainrepo.AdaptiveDecisionModelOperation{
			Status: domainrepo.AdaptiveDecisionModelOperationStatusFailed, ErrorCode: "model_error",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCalls := 0
			idGen := &adaptiveDecisionIDGeneratorStub{ids: []int64{1, 2}}
			repo := &adaptiveDecisionModelOperationRepositoryStub{
				readErr: domainrepo.ErrAdaptiveDecisionModelOperationNotFound,
				prepareResult: &domainrepo.PrepareAdaptiveDecisionModelOperationResult{
					Owned: false, Operation: test.operation,
				},
			}
			producer := NewModelAdaptiveDecisionProducer(ModelAdaptiveDecisionProducerOptions{
				Provider: func(context.Context) (model.ToolCallingChatModel, AdaptiveDecisionModelIdentity, error) {
					providerCalls++
					return nil, AdaptiveDecisionModelIdentity{}, errors.New("must not be called")
				},
				UsageCollector: &adaptiveDecisionUsageCollectorStub{}, OperationRepository: repo,
				IDGen: idGen, Now: func() int64 { return 1 }, Timeout: time.Second,
			})
			ctx, request := adaptiveDecisionModelTestInvocation(t)

			_, err := producer.Produce(ctx, request)
			if test.wantOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Zero(t, providerCalls)
			require.Equal(t, 1, idGen.calls)
			require.Empty(t, repo.completeRequests)
		})
	}
}

func TestModelAdaptiveDecisionProducerRejectsUntrustedModelResponsesAndCompletesFailed(t *testing.T) {
	validUsage := &schema.TokenUsage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}
	tests := []struct {
		name     string
		response *schema.Message
	}{
		{name: "plain content", response: &schema.Message{Role: schema.Assistant, Content: "plain", ResponseMeta: &schema.ResponseMeta{Usage: validUsage}}},
		{name: "wrong tool", response: &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Function: schema.FunctionCall{Name: "other", Arguments: adaptiveDecisionValidToolArguments}}}, ResponseMeta: &schema.ResponseMeta{Usage: validUsage}}},
		{name: "multiple tools", response: &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{Function: schema.FunctionCall{Name: adaptiveDecisionModelToolName, Arguments: adaptiveDecisionValidToolArguments}},
			{Function: schema.FunctionCall{Name: adaptiveDecisionModelToolName, Arguments: adaptiveDecisionValidToolArguments}},
		}, ResponseMeta: &schema.ResponseMeta{Usage: validUsage}}},
		{name: "invalid arguments", response: adaptiveDecisionToolResponse(`{"schema":"wrong"}`, validUsage)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chatModel := &adaptiveDecisionToolModelStub{generate: func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
				return test.response, nil
			}}
			repo := adaptiveDecisionOwnedRepositoryStub()
			usage := &adaptiveDecisionUsageCollectorStub{}
			producer := NewModelAdaptiveDecisionProducer(ModelAdaptiveDecisionProducerOptions{
				Provider: func(context.Context) (model.ToolCallingChatModel, AdaptiveDecisionModelIdentity, error) {
					return chatModel, AdaptiveDecisionModelIdentity{Provider: "test-provider", Model: "test-model"}, nil
				},
				UsageCollector:      usage,
				OperationRepository: repo, IDGen: &adaptiveDecisionIDGeneratorStub{ids: []int64{1, 2}},
				Now: func() int64 { return 1 }, Timeout: time.Second,
			})
			ctx, request := adaptiveDecisionModelTestInvocation(t)

			_, err := producer.Produce(ctx, request)

			require.Error(t, err)
			require.Equal(t, 1, chatModel.generateCalls)
			require.Len(t, usage.records, 1)
			require.Len(t, repo.completeRequests, 1)
			require.Equal(t, domainrepo.AdaptiveDecisionModelOperationStatusFailed, repo.completeRequests[0].Status)
			require.Equal(t, "invalid_result", repo.completeRequests[0].ErrorCode)
			require.Nil(t, repo.completeRequests[0].ResultPayload)
		})
	}
}

func TestModelAdaptiveDecisionProducerFailsClosedForMissingOrInvalidUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage *schema.TokenUsage
	}{
		{name: "missing"},
		{name: "negative", usage: &schema.TokenUsage{PromptTokens: -1, CompletionTokens: 2, TotalTokens: 1}},
		{name: "inconsistent total", usage: &schema.TokenUsage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 6}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chatModel := &adaptiveDecisionToolModelStub{generate: func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
				return adaptiveDecisionToolResponse(adaptiveDecisionValidToolArguments, test.usage), nil
			}}
			repo := adaptiveDecisionOwnedRepositoryStub()
			usage := &adaptiveDecisionUsageCollectorStub{}
			producer := NewModelAdaptiveDecisionProducer(ModelAdaptiveDecisionProducerOptions{
				Provider: func(context.Context) (model.ToolCallingChatModel, AdaptiveDecisionModelIdentity, error) {
					return chatModel, AdaptiveDecisionModelIdentity{Provider: "test-provider", Model: "test-model"}, nil
				},
				UsageCollector: usage, OperationRepository: repo,
				IDGen: &adaptiveDecisionIDGeneratorStub{ids: []int64{1, 2}},
				Now:   func() int64 { return 1 }, Timeout: time.Second,
			})
			ctx, request := adaptiveDecisionModelTestInvocation(t)

			_, err := producer.Produce(ctx, request)

			require.Error(t, err)
			require.Empty(t, usage.records)
			require.Len(t, repo.completeRequests, 1)
			require.Equal(t, domainrepo.AdaptiveDecisionModelOperationStatusFailed, repo.completeRequests[0].Status)
			require.Equal(t, "usage_error", repo.completeRequests[0].ErrorCode)
			require.Nil(t, repo.completeRequests[0].ResultPayload)
		})
	}
}

func TestModelAdaptiveDecisionProducerCompletesOwnedFailuresWithoutRetry(t *testing.T) {
	tests := []struct {
		name             string
		providerErr      error
		usageErr         error
		generate         func(context.Context) (*schema.Message, error)
		wantCode         string
		wantGenerateCall int
	}{
		{name: "provider", providerErr: errors.New("provider unavailable"), wantCode: "provider_error"},
		{name: "usage", usageErr: errors.New("usage unavailable"), wantCode: "usage_error", wantGenerateCall: 1,
			generate: func(context.Context) (*schema.Message, error) {
				return adaptiveDecisionToolResponse(adaptiveDecisionValidToolArguments, &schema.TokenUsage{
					PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5,
				}), nil
			}},
		{name: "timeout", wantCode: "model_error", wantGenerateCall: 1,
			generate: func(ctx context.Context) (*schema.Message, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCalls := 0
			chatModel := &adaptiveDecisionToolModelStub{generate: func(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
				return test.generate(ctx)
			}}
			repo := adaptiveDecisionOwnedRepositoryStub()
			usage := &adaptiveDecisionUsageCollectorStub{err: test.usageErr}
			producer := NewModelAdaptiveDecisionProducer(ModelAdaptiveDecisionProducerOptions{
				Provider: func(context.Context) (model.ToolCallingChatModel, AdaptiveDecisionModelIdentity, error) {
					providerCalls++
					if test.providerErr != nil {
						return nil, AdaptiveDecisionModelIdentity{}, test.providerErr
					}
					return chatModel, AdaptiveDecisionModelIdentity{Provider: "test-provider", Model: "test-model"}, nil
				},
				UsageCollector: usage, OperationRepository: repo,
				IDGen: &adaptiveDecisionIDGeneratorStub{ids: []int64{1, 2}},
				Now:   func() int64 { return 1 }, Timeout: 20 * time.Millisecond,
			})
			ctx, request := adaptiveDecisionModelTestInvocation(t)

			_, err := producer.Produce(ctx, request)

			require.Error(t, err)
			require.Equal(t, 1, providerCalls)
			require.Equal(t, test.wantGenerateCall, chatModel.generateCalls)
			require.Len(t, repo.completeRequests, 1)
			require.Equal(t, domainrepo.AdaptiveDecisionModelOperationStatusFailed, repo.completeRequests[0].Status)
			require.Equal(t, test.wantCode, repo.completeRequests[0].ErrorCode)
		})
	}
}

func TestNewEnvAdaptiveDecisionModelProviderRejectsUnsetOrInvalidModelIDBeforeBuild(t *testing.T) {
	originalValue, hadOriginalValue := os.LookupEnv(adaptiveDecisionModelIDEnv)
	originalBuild := buildAdaptiveDecisionModelByID
	buildCalls := 0
	buildAdaptiveDecisionModelByID = func(context.Context, int64) (model.ToolCallingChatModel, adaptiveDecisionBuiltModelIdentity, error) {
		buildCalls++
		return nil, adaptiveDecisionBuiltModelIdentity{}, errors.New("unexpected build")
	}
	t.Cleanup(func() {
		buildAdaptiveDecisionModelByID = originalBuild
		if hadOriginalValue {
			_ = os.Setenv(adaptiveDecisionModelIDEnv, originalValue)
		} else {
			_ = os.Unsetenv(adaptiveDecisionModelIDEnv)
		}
	})

	provider := NewEnvAdaptiveDecisionModelProvider()
	for _, value := range []string{"", "0", "-1", "builtin", " 42 ", "42x"} {
		t.Run(fmt.Sprintf("value_%q", value), func(t *testing.T) {
			if value == "" {
				require.NoError(t, os.Unsetenv(adaptiveDecisionModelIDEnv))
			} else {
				require.NoError(t, os.Setenv(adaptiveDecisionModelIDEnv, value))
			}
			_, _, err := provider(context.Background())
			require.Error(t, err)
		})
	}
	require.Zero(t, buildCalls)
}

func TestNewEnvAdaptiveDecisionModelProviderReadsExplicitModelIDOnEveryCall(t *testing.T) {
	originalValue, hadOriginalValue := os.LookupEnv(adaptiveDecisionModelIDEnv)
	originalBuild := buildAdaptiveDecisionModelByID
	modelIDs := make([]int64, 0, 2)
	chatModel := &adaptiveDecisionToolModelStub{generate: func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
		return nil, errors.New("not called")
	}}
	buildAdaptiveDecisionModelByID = func(_ context.Context, modelID int64) (model.ToolCallingChatModel, adaptiveDecisionBuiltModelIdentity, error) {
		modelIDs = append(modelIDs, modelID)
		return chatModel, adaptiveDecisionBuiltModelIdentity{Provider: "gpt", Model: fmt.Sprintf("model-%d", modelID)}, nil
	}
	t.Cleanup(func() {
		buildAdaptiveDecisionModelByID = originalBuild
		if hadOriginalValue {
			_ = os.Setenv(adaptiveDecisionModelIDEnv, originalValue)
		} else {
			_ = os.Unsetenv(adaptiveDecisionModelIDEnv)
		}
	})
	provider := NewEnvAdaptiveDecisionModelProvider()

	require.NoError(t, os.Setenv(adaptiveDecisionModelIDEnv, "42"))
	_, first, err := provider(context.Background())
	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionModelIdentity{Provider: "gpt", Model: "model-42"}, first)
	require.NoError(t, os.Setenv(adaptiveDecisionModelIDEnv, "43"))
	_, second, err := provider(context.Background())
	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionModelIdentity{Provider: "gpt", Model: "model-43"}, second)
	require.Equal(t, []int64{42, 43}, modelIDs)
}

type adaptiveDecisionToolModelStub struct {
	tools         []*schema.ToolInfo
	generate      func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error)
	generateCalls int
}

func (m *adaptiveDecisionToolModelStub) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.generateCalls++
	return m.generate(ctx, input, opts...)
}

func (m *adaptiveDecisionToolModelStub) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream is not supported")
}

func (m *adaptiveDecisionToolModelStub) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.tools = append([]*schema.ToolInfo(nil), tools...)
	return m, nil
}

type adaptiveDecisionModelOperationRepositoryStub struct {
	readOperation      *domainrepo.AdaptiveDecisionModelOperation
	readErr            error
	prepareResult      *domainrepo.PrepareAdaptiveDecisionModelOperationResult
	prepareErr         error
	completeErr        error
	prepareRequests    []domainrepo.PrepareAdaptiveDecisionModelOperationRequest
	completeRequests   []domainrepo.CompleteAdaptiveDecisionModelOperationRequest
	completedOperation *domainrepo.AdaptiveDecisionModelOperation
	order              *[]string
}

func (s *adaptiveDecisionModelOperationRepositoryStub) ReadAdaptiveDecisionModelOperation(
	context.Context,
	domainrepo.ReadAdaptiveDecisionModelOperationRequest,
) (*domainrepo.AdaptiveDecisionModelOperation, error) {
	return s.readOperation, s.readErr
}

func (s *adaptiveDecisionModelOperationRepositoryStub) PrepareAdaptiveDecisionModelOperation(
	_ context.Context,
	req domainrepo.PrepareAdaptiveDecisionModelOperationRequest,
) (*domainrepo.PrepareAdaptiveDecisionModelOperationResult, error) {
	s.prepareRequests = append(s.prepareRequests, req)
	return s.prepareResult, s.prepareErr
}

func (s *adaptiveDecisionModelOperationRepositoryStub) CompleteAdaptiveDecisionModelOperation(
	_ context.Context,
	req domainrepo.CompleteAdaptiveDecisionModelOperationRequest,
) (*domainrepo.CompleteAdaptiveDecisionModelOperationResult, error) {
	if s.order != nil {
		*s.order = append(*s.order, "complete")
	}
	s.completeRequests = append(s.completeRequests, req)
	if s.completeErr != nil {
		return nil, s.completeErr
	}
	s.completedOperation = &domainrepo.AdaptiveDecisionModelOperation{
		Status: req.Status, ResultPayload: append([]byte(nil), req.ResultPayload...), ErrorCode: req.ErrorCode,
	}
	return &domainrepo.CompleteAdaptiveDecisionModelOperationResult{Operation: s.completedOperation}, nil
}

type adaptiveDecisionIDGeneratorStub struct {
	ids   []int64
	err   error
	calls int
}

func (g *adaptiveDecisionIDGeneratorStub) GenMultiIDs(context.Context, int) ([]int64, error) {
	g.calls++
	return append([]int64(nil), g.ids...), g.err
}

type adaptiveDecisionUsageCollectorStub struct {
	records []AgentTokenUsage
	err     error
	order   *[]string
}

func (c *adaptiveDecisionUsageCollectorStub) Record(_ context.Context, _ *RunSummary, usage AgentTokenUsage) error {
	if c.order != nil {
		*c.order = append(*c.order, "usage")
	}
	c.records = append(c.records, usage)
	return c.err
}

func adaptiveDecisionModelTestInvocation(t *testing.T) (context.Context, AdaptiveDecisionRequest) {
	t.Helper()
	run := &RunSummary{
		RunID: 21, ThreadID: 11, SpaceID: 31, CreatorID: 41,
		LeaseOwner: "lease-owner-secret", LeaseToken: "lease-token-secret", ExecutionGeneration: 3,
	}
	attempt := &domainentity.RunAttempt{
		ThreadID: 11, ExecutionRunID: 21, JournalRunID: 22, AttemptID: "attempt-1",
	}
	admission := baselineDecisionRequest().Admission
	admission.FeatureGateEnabled = true
	ctx := withAdaptiveDecisionModelInvocation(context.Background(), run, attempt, "operation-key-secret")
	return ctx, AdaptiveDecisionRequest{
		Admission: admission,
		SemanticInput: AdaptiveDecisionSemanticInput{
			Messages:       []AdaptiveDecisionSemanticMessage{{Role: "user", Content: "implement only this"}},
			HasAttachments: true,
		},
	}
}

func adaptiveDecisionToolResponse(arguments string, usage *schema.TokenUsage) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "call-1", Type: "function",
			Function: schema.FunctionCall{Name: adaptiveDecisionModelToolName, Arguments: arguments},
		}},
		ResponseMeta: &schema.ResponseMeta{FinishReason: "tool_calls", Usage: usage},
	}
}

func adaptiveDecisionPromptText(messages []*schema.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		if message != nil {
			builder.WriteString(message.Content)
		}
	}
	return builder.String()
}

func adaptiveDecisionOwnedRepositoryStub() *adaptiveDecisionModelOperationRepositoryStub {
	return &adaptiveDecisionModelOperationRepositoryStub{
		readErr: domainrepo.ErrAdaptiveDecisionModelOperationNotFound,
		prepareResult: &domainrepo.PrepareAdaptiveDecisionModelOperationResult{
			Owned:     true,
			Operation: &domainrepo.AdaptiveDecisionModelOperation{Status: domainrepo.AdaptiveDecisionModelOperationStatusCalling},
		},
	}
}
