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
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestADKOffloadVirtualPathValidation(t *testing.T) {
	valid := "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" +
		strings.Repeat("a", 64) + ".txt"
	parsed, err := parseADKOffloadVirtualPath(valid)
	require.NoError(t, err)
	require.Equal(t, int64(20), parsed.RunID)
	require.Equal(t, "trunc", parsed.Phase)
	require.Equal(t, strings.Repeat("a", 64), parsed.Name)

	invalid := []string{
		"",
		"relative/path",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/../secret.txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc\\secret.txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/0/trunc/" + strings.Repeat("a", 64) + ".txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/not-a-run/trunc/" + strings.Repeat("a", 64) + ".txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/output/" + strings.Repeat("a", 64) + ".txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/not-hex.txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" + strings.Repeat("A", 64) + ".txt",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" + strings.Repeat("a", 64) + ".txt/extra",
		"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" + strings.Repeat("a", 64) + ".txt\x00",
	}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			_, err := parseADKOffloadVirtualPath(value)
			require.Error(t, err)
		})
	}
}

func TestADKToolResultReductionConfigDefaultsOverridesAndValidation(
	t *testing.T,
) {
	budget := ADKContextBudget{ContextWindowTokens: 128000}
	config, err := adkToolResultReductionConfigFromRun(
		&RunSummary{},
		budget,
	)
	require.NoError(t, err)
	require.Equal(t, 50000, config.MaxLengthForTrunc)
	require.Equal(t, int64(100000), config.MaxTokensForClear)
	require.Equal(t, defaultADKMaxOffloadBytes, config.OffloadLimits.MaxOffloadBytes)

	config, err = adkToolResultReductionConfigFromRun(
		&RunSummary{Config: `{
			"tool_result_reduction":{
				"max_length_for_trunc":100,
				"max_tokens_for_clear":2000,
				"clear_retention_suffix_limit":2,
				"clear_at_least_tokens":200,
				"max_offload_bytes":4096,
				"default_read_bytes":512,
				"max_read_bytes":1024
			}
		}`},
		budget,
	)
	require.NoError(t, err)
	require.Equal(t, 100, config.MaxLengthForTrunc)
	require.Equal(t, int64(2000), config.MaxTokensForClear)
	require.Equal(t, 2, config.ClearRetentionSuffixLimit)
	require.Equal(t, int64(200), config.ClearAtLeastTokens)
	require.Equal(t, 4096, config.OffloadLimits.MaxOffloadBytes)
	require.Equal(t, 512, config.OffloadLimits.DefaultReadBytes)
	require.Equal(t, 1024, config.OffloadLimits.MaxReadBytes)

	invalid := []string{
		`{"tool_result_reduction":{"max_length_for_trunc":0}}`,
		`{"tool_result_reduction":{"max_tokens_for_clear":128000}}`,
		`{"tool_result_reduction":{"clear_retention_suffix_limit":0}}`,
		`{"tool_result_reduction":{"clear_at_least_tokens":100000}}`,
		`{"tool_result_reduction":{"max_offload_bytes":100,"max_length_for_trunc":101}}`,
		`{"tool_result_reduction":{"default_read_bytes":1025,"max_read_bytes":1024}}`,
	}
	for _, value := range invalid {
		_, err := adkToolResultReductionConfigFromRun(
			&RunSummary{Config: value},
			budget,
		)
		require.Error(t, err, value)
	}
}

func TestADKOffloadBackendWritesRegistersAndReadsHistoricalRun(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	events := &recordingRunEventSink{}
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	backend, err := NewADKOffloadBackend(
		run,
		objectStorage,
		registry,
		events,
		ADKOffloadLimits{},
	)
	require.NoError(t, err)
	path := adkOffloadVirtualPath(run.RunID, "trunc", "call-1")

	err = backend.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: path,
		Content:  "first\nsecond\nthird",
	})
	require.NoError(t, err)

	expectedKey := "agent-runtime/30/10/runs/20/tool-results/trunc/" +
		strings.TrimSuffix(lastADKOffloadPathSegment(path), ".txt") + ".txt"
	require.Equal(t, "first\nsecond\nthird", string(objectStorage.objects[expectedKey]))
	require.Len(t, registry.calls, 1)
	require.Equal(t, int64(20), registry.calls[0].RunID)
	require.Equal(t, path, registry.calls[0].VirtualPath)
	require.Equal(t, expectedKey, registry.calls[0].ObjectURI)
	require.Equal(t, int64(len("first\nsecond\nthird")), registry.calls[0].SizeBytes)
	require.Len(t, registry.calls[0].Digest, 64)
	require.Equal(
		t,
		[]string{"context.tool_result_offloaded"},
		events.eventTypes(),
	)
	require.NotContains(t, events.events[0].Payload, "first")
	require.NotContains(t, events.events[0].Payload, expectedKey)

	resumeBackend, err := NewADKOffloadBackend(
		&RunSummary{
			RunID:     21,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
		objectStorage,
		registry,
		&recordingRunEventSink{},
		ADKOffloadLimits{},
	)
	require.NoError(t, err)
	chunk, err := resumeBackend.ReadRange(
		context.Background(),
		path,
		6,
		6,
	)
	require.NoError(t, err)
	require.Equal(t, "second", chunk.Content)
	require.Equal(t, int64(6), chunk.OffsetByte)
	require.Equal(t, int64(12), chunk.NextOffsetByte)
	require.Equal(t, int64(len("first\nsecond\nthird")), chunk.TotalBytes)
}

func TestADKOffloadBackendReadsUploadedThreadFile(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	objectStorage.objects["agent-runtime/30/10/uploads/report.md"] = []byte(
		"# report\nhello upload",
	)
	registry := &recordingADKRuntimeFileRegistry{
		resolved: map[string]*RuntimeFileSummary{
			"/mnt/user-data/uploads/report.md": {
				FileID:    90,
				ObjectURI: "agent-runtime/30/10/uploads/report.md",
			},
		},
	}
	backend, err := NewADKOffloadBackend(
		&RunSummary{
			RunID:     20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
		objectStorage,
		registry,
		&recordingRunEventSink{},
		ADKOffloadLimits{},
	)
	require.NoError(t, err)

	chunk, err := backend.ReadRange(
		context.Background(),
		"/mnt/user-data/uploads/report.md",
		0,
		8,
	)

	require.NoError(t, err)
	require.Equal(t, "# report", chunk.Content)
	require.Equal(t, int64(8), chunk.NextOffsetByte)
	require.Equal(t, int64(len("# report\nhello upload")), chunk.TotalBytes)
	require.Equal(t, int64(20), registry.resolveReq.RunID)
	require.Equal(t, "/mnt/user-data/uploads/report.md", registry.resolveReq.VirtualPath)
}

func TestADKOffloadBackendRejectsCrossRunWriteAndLimits(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	backend, err := NewADKOffloadBackend(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		objectStorage,
		registry,
		&recordingRunEventSink{},
		ADKOffloadLimits{
			MaxOffloadBytes:  4,
			DefaultReadBytes: 2,
			MaxReadBytes:     3,
		},
	)
	require.NoError(t, err)

	err = backend.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: adkOffloadVirtualPath(21, "trunc", "call-1"),
		Content:  "data",
	})
	require.Error(t, err)

	err = backend.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: adkOffloadVirtualPath(20, "trunc", "call-1"),
		Content:  "large",
	})
	require.Error(t, err)
	require.Empty(t, objectStorage.objects)
	require.Empty(t, registry.calls)

	_, err = backend.ReadRange(
		context.Background(),
		adkOffloadVirtualPath(20, "trunc", "call-1"),
		0,
		4,
	)
	require.Error(t, err)
}

func TestADKOffloadBackendRejectsUnregisteredRuntimeFileRead(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	backend, err := NewADKOffloadBackend(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		objectStorage,
		registry,
		&recordingRunEventSink{},
		ADKOffloadLimits{},
	)
	require.NoError(t, err)
	virtualPath := adkOffloadVirtualPath(20, "trunc", "call-1")
	parsed, err := parseADKOffloadVirtualPath(virtualPath)
	require.NoError(t, err)
	objectStorage.objects[backend.objectKey(parsed)] = []byte("orphan content")

	_, err = backend.ReadRange(
		context.Background(),
		virtualPath,
		0,
		12,
	)

	require.Error(t, err)
	require.NotContains(t, err.Error(), "orphan content")
}

func TestADKOffloadBackendSupportsConcurrentIdempotentWrites(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	backend, err := NewADKOffloadBackend(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		objectStorage,
		registry,
		&recordingRunEventSink{},
		ADKOffloadLimits{},
	)
	require.NoError(t, err)
	virtualPath := adkOffloadVirtualPath(20, "trunc", "call-1")

	var wait sync.WaitGroup
	errs := make(chan error, 20)
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- backend.Write(
				context.Background(),
				&filesystem.WriteRequest{
					FilePath: virtualPath,
					Content:  "same content",
				},
			)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	require.Len(t, objectStorage.objects, 1)
	require.Equal(t, "same content", string(objectStorage.objects[backend.objectKey(parsedADKOffloadPath{
		RunID: 20,
		Phase: "trunc",
		Name: strings.TrimSuffix(
			lastADKOffloadPathSegment(virtualPath),
			".txt",
		),
	})]))
	require.Len(t, registry.calls, 20)
}

func TestADKOffloadBackendPreservesDeterministicObjectWhenRegistrationFails(
	t *testing.T,
) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{
		err: errors.New("database unavailable"),
	}
	backend, err := NewADKOffloadBackend(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		objectStorage,
		registry,
		&recordingRunEventSink{},
		ADKOffloadLimits{},
	)
	require.NoError(t, err)
	path := adkOffloadVirtualPath(20, "clear", "call-1")
	parsed, err := parseADKOffloadVirtualPath(path)
	require.NoError(t, err)
	objectKey := backend.objectKey(parsed)
	objectStorage.objects[objectKey] = []byte("previous content")

	err = backend.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: path,
		Content:  "private content",
	})

	require.Error(t, err)
	require.Equal(t, "private content", string(objectStorage.objects[objectKey]))
	require.Empty(t, objectStorage.deleted)
	require.NotContains(t, err.Error(), "private content")
}

func TestADKReadOffloadToolReturnsBoundedByteRanges(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	backend, err := NewADKOffloadBackend(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		objectStorage,
		&recordingADKRuntimeFileRegistry{},
		&recordingRunEventSink{},
		ADKOffloadLimits{
			MaxOffloadBytes:  32,
			DefaultReadBytes: 6,
			MaxReadBytes:     12,
		},
	)
	require.NoError(t, err)
	path := adkOffloadVirtualPath(20, "trunc", "call-1")
	require.NoError(t, backend.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: path,
		Content:  "first\nsecond\nthird",
	}))
	readTool, err := newADKReadOffloadTool(backend)
	require.NoError(t, err)

	output, err := readTool.InvokableRun(
		context.Background(),
		fmt.Sprintf(`{"file_path":%q}`, path),
	)
	require.NoError(t, err)
	require.Contains(t, output, "offset_byte: 0")
	require.Contains(t, output, "next_offset_byte: 6")
	require.Contains(t, output, "total_bytes: 18")
	require.Contains(t, output, "content:\nfirst\n")

	output, err = readTool.InvokableRun(
		context.Background(),
		fmt.Sprintf(
			`{"file_path":%q,"offset_byte":6,"limit_bytes":6}`,
			path,
		),
	)
	require.NoError(t, err)
	require.Contains(t, output, "content:\nsecond")

	_, err = readTool.InvokableRun(
		context.Background(),
		fmt.Sprintf(
			`{"file_path":%q,"limit_bytes":13}`,
			path,
		),
	)
	require.Error(t, err)
}

func TestADKAgentOffloadsLargeToolResultAndReadsItBack(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	events := &recordingRunEventSink{}
	largeTool, err := toolutils.InferTool(
		"large_tool",
		"Return a large diagnostic result.",
		func(context.Context, struct{}) (string, error) {
			return strings.Repeat("diagnostic-result-", 8), nil
		},
	)
	require.NoError(t, err)
	chatModel := &offloadRoundTripChatModel{}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		OffloadBackendFactory: ADKOffloadBackendFactoryFunc(func(
			_ context.Context,
			run *RunSummary,
			limits ADKOffloadLimits,
		) (*ADKOffloadBackend, error) {
			return NewADKOffloadBackend(
				run,
				objectStorage,
				registry,
				events,
				limits,
			)
		}),
		EventSink: events,
	})
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ADKToolProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]tool.BaseTool, error) {
			return []tool.BaseTool{largeTool}, nil
		}),
		assembler,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
		SpaceID:  30,
		Config: `{
			"max_iterations":5,
			"tool_result_reduction":{
				"max_length_for_trunc":16,
				"default_read_bytes":64,
				"max_read_bytes":256
			}
		}`,
	})
	require.NoError(t, err)

	agentEvents := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("run diagnostics")},
	})

	require.NotEmpty(t, agentEvents)
	require.NoError(t, agentEvents[len(agentEvents)-1].Err)
	require.Equal(t, 3, chatModel.calls)
	require.Contains(t, chatModel.readBack, "diagnostic-result-")
	require.Len(t, objectStorage.objects, 1)
	require.Len(t, registry.calls, 1)
	require.Contains(t, events.eventTypes(), "context.tool_result_offloaded")
}

type offloadRoundTripChatModel struct {
	calls    int
	readBack string
}

var testADKOffloadPathFinder = regexp.MustCompile(
	`/mnt/user-data/workspace/\.coze/tool-results/runs/[1-9][0-9]*/(?:trunc|clear)/[0-9a-f]{64}\.txt`,
)

func (m *offloadRoundTripChatModel) Generate(
	_ context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.calls++
	switch m.calls {
	case 1:
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-large",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "large_tool",
				Arguments: `{}`,
			},
		}}), nil
	case 2:
		var offloadPath string
		for _, message := range input {
			if message == nil || message.Role != schema.Tool {
				continue
			}
			match := testADKOffloadPathFinder.FindString(message.Content)
			if match != "" {
				offloadPath = match
			}
		}
		if offloadPath == "" {
			return nil, fmt.Errorf("large tool result was not offloaded")
		}
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-read",
			Type: "function",
			Function: schema.FunctionCall{
				Name: "read_file",
				Arguments: fmt.Sprintf(
					`{"file_path":%q,"limit_bytes":64}`,
					offloadPath,
				),
			},
		}}), nil
	case 3:
		for _, message := range input {
			if message != nil &&
				message.Role == schema.Tool &&
				message.ToolName == "read_file" {
				m.readBack = message.Content
			}
		}
		if m.readBack == "" {
			return nil, fmt.Errorf("read_file result was not returned")
		}
		return schema.AssistantMessage("diagnostics complete", nil), nil
	default:
		return nil, fmt.Errorf("unexpected model call %d", m.calls)
	}
}

func (m *offloadRoundTripChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stream is not implemented")
}

type recordingADKOffloadStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
}

func newRecordingADKOffloadStorage() *recordingADKOffloadStorage {
	return &recordingADKOffloadStorage{objects: map[string][]byte{}}
}

func (s *recordingADKOffloadStorage) PutObject(
	_ context.Context,
	key string,
	content []byte,
	_ ...storage.PutOptFn,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = append([]byte(nil), content...)
	return nil
}

func (s *recordingADKOffloadStorage) GetObject(
	_ context.Context,
	key string,
) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, exists := s.objects[key]
	if !exists {
		return nil, storage.ErrObjectNotFound
	}
	return append([]byte(nil), content...), nil
}

func (s *recordingADKOffloadStorage) DeleteObject(
	_ context.Context,
	key string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return nil
}

type recordingADKRuntimeFileRegistry struct {
	mu         sync.Mutex
	calls      []*RegisterRuntimeFileRequest
	resolved   map[string]*RuntimeFileSummary
	resolveReq *ResolveRuntimeFileRequest
	err        error
}

func (r *recordingADKRuntimeFileRegistry) RegisterRuntimeFile(
	_ context.Context,
	req *RegisterRuntimeFileRequest,
) (*RuntimeFileSummary, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, false, r.err
	}
	cloned := *req
	r.calls = append(r.calls, &cloned)
	return &RuntimeFileSummary{FileID: int64(len(r.calls))}, len(r.calls) == 1, nil
}

func (r *recordingADKRuntimeFileRegistry) ResolveRuntimeFile(
	_ context.Context,
	req *ResolveRuntimeFileRequest,
) (*RuntimeFileSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clonedReq := *req
	r.resolveReq = &clonedReq
	if r.err != nil {
		return nil, r.err
	}
	if r.resolved != nil {
		if file := r.resolved[req.VirtualPath]; file != nil {
			cloned := *file
			return &cloned, nil
		}
	}
	for index, call := range r.calls {
		if call.RunID == req.RunID && call.VirtualPath == req.VirtualPath {
			return &RuntimeFileSummary{
				FileID:    int64(index + 1),
				ObjectURI: call.ObjectURI,
			}, nil
		}
	}
	return nil, fmt.Errorf("runtime file is not registered")
}
