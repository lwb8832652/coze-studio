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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	threadrepository "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	threadservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestEncodeADKTranscriptPreservesCompleteMessages(t *testing.T) {
	imageURL := "https://example.test/image.png"
	messages := []*schema.Message{
		nil,
		{
			Role:    schema.User,
			Content: "inspect this",
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL:      &imageURL,
						MIMEType: "image/png",
					},
				},
			}},
		},
		{
			Role:             schema.Assistant,
			Content:          "",
			ReasoningContent: "need a tool",
			ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "inspect_image",
					Arguments: `{"path":"image.png"}`,
				},
			}},
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: "tool_calls",
			},
		},
		{
			Role:       schema.Tool,
			Content:    "image is valid",
			ToolCallID: "call-1",
			ToolName:   "inspect_image",
		},
		{
			Role:    schema.Assistant,
			Content: "analysis complete",
			AssistantGenMultiContent: []schema.MessageOutputPart{{
				Type: schema.ChatMessagePartTypeText,
				Text: "analysis complete",
			}},
		},
	}

	raw, digest, count, err := encodeADKTranscript(messages)

	require.NoError(t, err)
	require.Len(t, digest, 64)
	require.Equal(t, int32(4), count)
	var decoded []*schema.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	require.Len(t, decoded, 4)
	require.Equal(t, "need a tool", decoded[1].ReasoningContent)
	require.Equal(t, "inspect_image", decoded[1].ToolCalls[0].Function.Name)
	require.Equal(t, "call-1", decoded[2].ToolCallID)
	require.Equal(t, imageURL, *decoded[0].UserInputMultiContent[0].Image.URL)
	require.Equal(t, "analysis complete", decoded[3].AssistantGenMultiContent[0].Text)

	rawAgain, digestAgain, countAgain, err := encodeADKTranscript(messages)
	require.NoError(t, err)
	require.Equal(t, raw, rawAgain)
	require.Equal(t, digest, digestAgain)
	require.Equal(t, count, countAgain)
}

func TestEncodeADKTranscriptRejectsEmptyAndUnsupportedExtra(t *testing.T) {
	_, _, _, err := encodeADKTranscript([]*schema.Message{nil})
	require.ErrorContains(t, err, "transcript messages are required")

	_, _, _, err = encodeADKTranscript([]*schema.Message{{
		Role:    schema.User,
		Content: "hello",
		Extra: map[string]any{
			"invalid": make(chan struct{}),
		},
	}})
	require.ErrorContains(t, err, "encode eino adk transcript")
}

func TestEncodeADKTranscriptIgnoresVolatileEinoMessageIDForDigest(t *testing.T) {
	firstRaw, firstDigest, _, err := encodeADKTranscript([]*schema.Message{{
		Role:    schema.Assistant,
		Content: "stable answer",
		Extra: map[string]any{
			"_eino_msg_id": "first-generated-id",
			"trace_scope":  "business-value",
		},
	}})
	require.NoError(t, err)
	secondRaw, secondDigest, _, err := encodeADKTranscript([]*schema.Message{{
		Role:    schema.Assistant,
		Content: "stable answer",
		Extra: map[string]any{
			"_eino_msg_id": "second-generated-id",
			"trace_scope":  "business-value",
		},
	}})
	require.NoError(t, err)

	require.NotEqual(t, firstRaw, secondRaw)
	require.Equal(t, firstDigest, secondDigest)

	_, changedBusinessDigest, _, err := encodeADKTranscript([]*schema.Message{{
		Role:    schema.Assistant,
		Content: "stable answer",
		Extra: map[string]any{
			"_eino_msg_id": "third-generated-id",
			"trace_scope":  "different-business-value",
		},
	}})
	require.NoError(t, err)
	require.NotEqual(t, firstDigest, changedBusinessDigest)
}

func TestADKTranscriptHooksPersistSummaryInputWithoutMemoryQueue(t *testing.T) {
	store := &recordingADKTranscriptStore{}
	queue := &recordingADKMemoryFlushQueue{}
	events := &recordingRunEventSink{}
	run := &RunSummary{
		RunID:       20,
		ThreadID:    10,
		SpaceID:     30,
		CreatorID:   40,
		AssistantID: "lead",
	}
	hooks := NewADKTranscriptHooks(run, store, queue, events)
	messages := []*schema.Message{
		schema.UserMessage("remember that I prefer Go"),
		schema.AssistantMessage("I will use Go.", nil),
	}

	err := hooks.PersistSummaryInput(context.Background(), messages)

	require.NoError(t, err)
	require.Len(t, store.calls, 1)
	require.Equal(t, TranscriptKindSummaryInput, store.calls[0].Kind)
	require.JSONEq(t, store.calls[0].Messages, store.snapshot.Messages)
	require.Empty(t, queue.calls)
	require.Equal(t, []string{"context.transcript_persisted"}, events.eventTypes())

	err = hooks.PersistSummaryInput(context.Background(), messages)
	require.NoError(t, err)
	require.Len(t, store.calls, 2)
	require.Empty(t, queue.calls)
	require.Equal(t, store.calls[0].IdempotencyKey, store.calls[1].IdempotencyKey)
}

func TestADKTranscriptHooksFailClosedForSnapshotAndOpenForQueue(t *testing.T) {
	messages := []*schema.Message{
		schema.UserMessage("remember this"),
		schema.AssistantMessage("noted", nil),
	}
	run := &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30}

	t.Run("snapshot failure", func(t *testing.T) {
		store := &recordingADKTranscriptStore{err: errors.New("snapshot unavailable")}
		events := &recordingRunEventSink{}
		hooks := NewADKTranscriptHooks(
			run,
			store,
			&recordingADKMemoryFlushQueue{},
			events,
		)

		err := hooks.PersistTerminal(context.Background(), messages)

		require.ErrorContains(t, err, "persist eino adk transcript")
		require.Empty(t, events.events)
	})

	t.Run("queue failure", func(t *testing.T) {
		store := &recordingADKTranscriptStore{}
		queue := &recordingADKMemoryFlushQueue{
			err: errors.New("queue unavailable"),
		}
		events := &recordingRunEventSink{}
		hooks := NewADKTranscriptHooks(run, store, queue, events)

		err := hooks.PersistTerminal(context.Background(), messages)

		require.NoError(t, err)
		require.Equal(t, []string{
			"context.transcript_persisted",
			"memory.update_failed",
		}, events.eventTypes())
		require.NotContains(t, events.events[1].Payload, "remember this")
		require.Contains(t, events.events[1].Payload, "queue unavailable")
	})
}

func TestADKTranscriptMiddlewarePersistsSuccessfulTerminalState(t *testing.T) {
	store := &recordingADKTranscriptStore{}
	queue := &recordingADKMemoryFlushQueue{}
	events := &recordingRunEventSink{}
	middleware := NewADKTranscriptMiddleware(NewADKTranscriptHooks(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		store,
		queue,
		events,
	))
	hiddenReminder := schema.UserMessage("<system-reminder>hidden memory</system-reminder>")
	hiddenReminder.Extra = map[string]any{
		adkHideFromUIExtraKey:             true,
		adkDynamicContextReminderExtraKey: true,
	}
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		hiddenReminder,
		schema.UserMessage("<uploaded_files>/mnt/user-data/private.pdf</uploaded_files>"),
		schema.AssistantMessage("I can inspect the uploaded file.", nil),
		schema.UserMessage("<uploaded_files>/mnt/user-data/private.pdf</uploaded_files>\nremember that I prefer Go"),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "search_private_file",
				Arguments: `{"path":"/mnt/user-data/private.pdf","token":"tool-secret"}`,
			},
		}}),
		schema.ToolMessage("tool-secret result", "call-1"),
		schema.AssistantMessage("Noted: use Go.", nil),
	}}

	gotCtx, err := middleware.AfterAgent(context.Background(), state)

	require.NoError(t, err)
	require.NotNil(t, gotCtx)
	require.Len(t, store.calls, 2)
	require.Equal(t, TranscriptKindTerminal, store.calls[0].Kind)
	require.Equal(t, TranscriptKindTerminal, store.calls[1].Kind)
	require.Contains(t, store.calls[0].Messages, "tool-secret")
	require.Contains(t, store.calls[0].Messages, "hidden memory")
	require.NotContains(t, store.calls[1].Messages, "tool-secret")
	require.NotContains(t, store.calls[1].Messages, "hidden memory")
	require.NotContains(t, store.calls[1].Messages, "<uploaded_files>")
	require.NotContains(t, store.calls[1].Messages, "private.pdf")
	require.NotContains(t, store.calls[1].Messages, "search_private_file")
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"kind":"terminal",
		"digest":"`+store.calls[1].Digest+`",
		"purpose":"memory_flush",
		"source_digest":"`+store.calls[0].Digest+`",
		"source_snapshot_id":501,
		"memory_flush":{
			"purpose":"memory_flush",
			"correction_detected":false,
			"reinforcement_detected":false,
			"schema":"coze.adk_memory_flush.v1"
		}
	}`, store.calls[1].Metadata)
	require.Len(t, queue.calls, 1)
	require.Equal(t, store.snapshots[1].SnapshotID, queue.calls[0].SnapshotID)
	require.Equal(t, store.calls[1].IdempotencyKey, queue.calls[0].IdempotencyKey)
	require.Equal(t, []string{
		"context.transcript_persisted",
		"memory.update_queued",
	}, events.eventTypes())

	memoryMessages := decodeADKTranscriptMessages(t, store.calls[1].Messages)
	require.Len(t, memoryMessages, 2)
	require.Equal(t, schema.User, memoryMessages[0].Role)
	require.Equal(t, "remember that I prefer Go", memoryMessages[0].Content)
	require.Empty(t, memoryMessages[0].UserInputMultiContent)
	require.Equal(t, schema.Assistant, memoryMessages[1].Role)
	require.Equal(t, "Noted: use Go.", memoryMessages[1].Content)
	require.Empty(t, memoryMessages[1].ToolCalls)
}

func TestADKTranscriptMemoryFlushMetadataCapturesCorrectionAndReinforcement(
	t *testing.T,
) {
	for _, testCase := range []struct {
		name          string
		user          string
		correction    bool
		reinforcement bool
	}{
		{
			name:       "correction",
			user:       "不对，改用 Go 语言",
			correction: true,
		},
		{
			name:          "reinforcement",
			user:          "对，就是这样。",
			reinforcement: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &recordingADKTranscriptStore{}
			queue := &recordingADKMemoryFlushQueue{}
			hooks := NewADKTranscriptHooks(
				&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
				store,
				queue,
				&recordingRunEventSink{},
			)

			err := hooks.PersistTerminal(context.Background(), []*schema.Message{
				schema.UserMessage(testCase.user),
				schema.AssistantMessage("已记录。", nil),
			})

			require.NoError(t, err)
			require.Len(t, store.calls, 1)
			require.Len(t, queue.calls, 1)
			metadata := decodeADKTranscriptMetadata(t, store.calls[0].Metadata)
			require.Contains(t, metadata, "memory_flush")
			flush, ok := metadata["memory_flush"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, testCase.correction, flush["correction_detected"])
			require.Equal(t, testCase.reinforcement, flush["reinforcement_detected"])
			require.Equal(t, adkTranscriptMemoryFlushPurpose, flush["purpose"])
			require.NotContains(t, store.calls[0].Metadata, testCase.user)
		})
	}
}

func TestADKTranscriptMiddlewareSkipsTerminalSnapshotOnModelFailureOrCancel(
	t *testing.T,
) {
	for _, testCase := range []struct {
		name     string
		modelErr error
	}{
		{name: "failure", modelErr: errors.New("model unavailable")},
		{name: "cancel", modelErr: context.Canceled},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &recordingADKTranscriptStore{}
			chatModel := &erroringTranscriptChatModel{err: testCase.modelErr}
			assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
				TranscriptStore: store,
			})
			bundle, err := assembler.Build(
				context.Background(),
				ADKMiddlewareBuildInput{
					Run:   &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
					Model: chatModel,
				},
			)
			require.NoError(t, err)
			agent, err := adk.NewChatModelAgent(
				context.Background(),
				&adk.ChatModelAgentConfig{
					Name:        "lead",
					Description: "terminal failure test",
					Model:       chatModel,
					Handlers:    bundle.Handlers,
				},
			)
			require.NoError(t, err)

			events := collectADKAgentEvents(t, agent, &adk.AgentInput{
				Messages: []*schema.Message{schema.UserMessage("hello")},
			})

			require.NotEmpty(t, events)
			require.Error(t, events[len(events)-1].Err)
			require.Empty(t, store.calls)
		})
	}
}

func TestADKTranscriptHooksSkipMemoryQueueWithoutFinalAssistantAnswer(t *testing.T) {
	store := &recordingADKTranscriptStore{}
	queue := &recordingADKMemoryFlushQueue{}
	hooks := NewADKTranscriptHooks(
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		store,
		queue,
		&recordingRunEventSink{},
	)

	err := hooks.PersistSummaryInput(context.Background(), []*schema.Message{
		schema.UserMessage("search"),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "search",
				Arguments: `{}`,
			},
		}}),
	})

	require.NoError(t, err)
	require.Empty(t, queue.calls)
}

func TestApplicationADKContextStoreMapsTranscriptAndMemoryRequests(t *testing.T) {
	domainSVC := &recordingThreadService{
		transcriptSnapshot: &entity.TranscriptSnapshot{
			ID:             501,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			Kind:           entity.TranscriptKindSummaryInput,
			Digest:         strings.Repeat("a", 64),
			IdempotencyKey: "summary_input:" + strings.Repeat("a", 64),
			MessageCount:   2,
			Messages:       `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]`,
		},
		memoryFlushJob: &entity.MemoryFlushJob{
			ID:                   601,
			ThreadID:             10,
			RunID:                20,
			SpaceID:              30,
			UserID:               40,
			AssistantID:          "lead",
			TranscriptSnapshotID: 501,
			IdempotencyKey:       "summary_input:" + strings.Repeat("a", 64),
			Status:               entity.MemoryFlushJobStatusPending,
		},
		transcriptCreated:  true,
		memoryFlushCreated: true,
	}
	store := NewApplicationADKContextStore(&ApplicationService{
		ThreadSVC: domainSVC,
	})
	run := &RunSummary{
		RunID:       20,
		ThreadID:    10,
		SpaceID:     30,
		CreatorID:   40,
		AssistantID: "lead",
	}
	req := ADKTranscriptPersistRequest{
		Kind:           TranscriptKindSummaryInput,
		Digest:         strings.Repeat("a", 64),
		IdempotencyKey: "summary_input:" + strings.Repeat("a", 64),
		MessageCount:   2,
		Messages:       `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]`,
		Metadata:       `{"runtime":"eino_adk"}`,
	}

	snapshot, created, err := store.PersistTranscript(
		context.Background(),
		run,
		req,
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(501), snapshot.SnapshotID)
	require.Equal(t, req.Kind, snapshot.Kind)
	require.Equal(t, req.Messages, snapshot.Messages)
	require.Equal(t, int64(10), domainSVC.persistTranscriptReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.persistTranscriptReq.RunID)
	require.Equal(
		t,
		entity.TranscriptKindSummaryInput,
		domainSVC.persistTranscriptReq.Kind,
	)

	queued, err := store.EnqueueMemoryFlush(
		context.Background(),
		run,
		ADKMemoryFlushRequest{
			SnapshotID:     snapshot.SnapshotID,
			Kind:           snapshot.Kind,
			Digest:         snapshot.Digest,
			IdempotencyKey: snapshot.IdempotencyKey,
			MessageCount:   snapshot.MessageCount,
		},
	)

	require.NoError(t, err)
	require.True(t, queued)
	require.Equal(t, int64(501), domainSVC.enqueueMemoryFlushReq.TranscriptSnapshotID)
	require.Equal(t, snapshot.IdempotencyKey, domainSVC.enqueueMemoryFlushReq.IdempotencyKey)
}

func TestADKMiddlewarePersistsSummaryInputAndTerminalTranscripts(t *testing.T) {
	store := &recordingADKTranscriptStore{}
	queue := &recordingADKMemoryFlushQueue{}
	events := &recordingRunEventSink{}
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("summary or final", nil),
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		TranscriptStore:  store,
		MemoryFlushQueue: queue,
		EventSink:        events,
	})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run: &RunSummary{
			RunID:    20,
			ThreadID: 10,
			SpaceID:  30,
			Config: `{
				"context_budget":{
					"summarization_messages":2
				}
			}`,
		},
		Model: chatModel,
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "transcript middleware test",
		Model:       chatModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	agentEvents := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage("first"),
			schema.AssistantMessage("second", nil),
			schema.UserMessage("third"),
		},
	})

	require.NotEmpty(t, agentEvents)
	require.NoError(t, agentEvents[len(agentEvents)-1].Err)
	require.Len(t, store.calls, 2)
	require.Equal(t, TranscriptKindSummaryInput, store.calls[0].Kind)
	require.Equal(t, TranscriptKindTerminal, store.calls[1].Kind)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKTranscriptReplayWithFreshAgentPersistsEachDigestOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))
	require.NoError(t, db.Exec(`
		CREATE TABLE agent_transcript_snapshots (
			id integer PRIMARY KEY,
			thread_id integer NOT NULL,
			run_id integer NOT NULL,
			space_id integer NOT NULL,
			kind text NOT NULL,
			digest text NOT NULL,
			idempotency_key text NOT NULL,
			message_count integer NOT NULL,
			messages json NOT NULL,
			metadata json,
			created_at integer NOT NULL,
			UNIQUE(run_id, idempotency_key)
		);
		CREATE TABLE agent_memory_flush_jobs (
			id integer PRIMARY KEY,
			thread_id integer NOT NULL,
			run_id integer NOT NULL,
			space_id integer NOT NULL,
			user_id integer NOT NULL,
			assistant_id text NOT NULL,
			transcript_snapshot_id integer NOT NULL,
			idempotency_key text NOT NULL,
			status text NOT NULL,
			attempt_count integer NOT NULL,
			worker_id text NOT NULL DEFAULT '',
			last_error text NOT NULL,
			available_at integer NOT NULL,
			lease_expires_at integer NOT NULL DEFAULT 0,
			started_at integer NOT NULL DEFAULT 0,
			ended_at integer NOT NULL DEFAULT 0,
			created_at integer NOT NULL,
			updated_at integer NOT NULL,
			UNIQUE(run_id, idempotency_key)
		)
	`).Error)

	repo := threadrepository.NewThreadRepository(db)
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID:        10,
		SpaceID:   30,
		CreatorID: 40,
		AgentID:   50,
		Title:     "transcript replay",
		Status:    entity.ThreadStatusRunning,
		Source:    entity.ThreadSourceWeb,
	}))
	require.NoError(t, repo.CreateRun(context.Background(), &entity.Run{
		ID:          20,
		ThreadID:    10,
		SpaceID:     30,
		CreatorID:   40,
		AssistantID: "lead",
		Status:      entity.RunStatusRunning,
		Command:     `{}`,
		Input:       `{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"second"},{"role":"user","content":"third"}]}`,
		Config:      `{}`,
		Context:     `{}`,
		Metadata:    `{}`,
		StreamMode:  `["messages"]`,
	}))
	domainSVC := threadservice.NewService(&threadservice.Components{
		Repo:  repo,
		IDGen: &sequentialTranscriptIDGen{next: 1000},
	})
	app := &ApplicationService{ThreadSVC: domainSVC}
	contextStore := NewApplicationADKContextStore(app)
	run := &RunSummary{
		RunID:       20,
		ThreadID:    10,
		SpaceID:     30,
		CreatorID:   40,
		AssistantID: "lead",
		Config: `{
			"context_budget":{
				"summarization_messages":2
			}
		}`,
	}
	newMessages := func() []*schema.Message {
		return []*schema.Message{
			schema.UserMessage("first"),
			schema.AssistantMessage("second", nil),
			schema.UserMessage("third"),
		}
	}

	for attempt := 0; attempt < 2; attempt++ {
		chatModel := &replaySummarizingChatModel{}
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
			TranscriptStore:  contextStore,
			MemoryFlushQueue: contextStore,
			EventSink:        &recordingRunEventSink{},
		})
		bundle, buildErr := assembler.Build(
			context.Background(),
			ADKMiddlewareBuildInput{
				Run:   run,
				Model: chatModel,
			},
		)
		require.NoError(t, buildErr)
		agent, buildErr := adk.NewChatModelAgent(
			context.Background(),
			&adk.ChatModelAgentConfig{
				Name:        "lead",
				Description: "transcript replay",
				Model:       chatModel,
				Handlers:    bundle.Handlers,
			},
		)
		require.NoError(t, buildErr)
		events := collectADKAgentEvents(t, agent, &adk.AgentInput{
			Messages: newMessages(),
		})
		require.NotEmpty(t, events)
		require.NoError(t, events[len(events)-1].Err)
	}

	var snapshotCount int64
	require.NoError(t, db.Table("agent_transcript_snapshots").
		Count(&snapshotCount).Error)
	var persistedSnapshots []struct {
		Kind           string
		IdempotencyKey string
		Messages       string
	}
	require.NoError(t, db.Table("agent_transcript_snapshots").
		Select("kind", "idempotency_key", "messages").
		Order("created_at ASC, id ASC").
		Scan(&persistedSnapshots).Error)
	require.Equal(t, int64(2), snapshotCount)
	require.Len(t, persistedSnapshots, 2)
	require.Equal(t, string(TranscriptKindSummaryInput), persistedSnapshots[0].Kind)
	require.Equal(t, string(TranscriptKindTerminal), persistedSnapshots[1].Kind)
	require.Contains(t, persistedSnapshots[1].Messages, einoMessageIDExtraKey)
	var jobCount int64
	require.NoError(t, db.Table("agent_memory_flush_jobs").
		Count(&jobCount).Error)
	require.Equal(t, int64(0), jobCount)
}

type recordingADKTranscriptStore struct {
	calls     []ADKTranscriptPersistRequest
	snapshot  ADKTranscriptSnapshot
	snapshots []ADKTranscriptSnapshot
	err       error
}

func (s *recordingADKTranscriptStore) PersistTranscript(
	_ context.Context,
	_ *RunSummary,
	req ADKTranscriptPersistRequest,
) (ADKTranscriptSnapshot, bool, error) {
	s.calls = append(s.calls, req)
	if s.err != nil {
		return ADKTranscriptSnapshot{}, false, s.err
	}
	for _, snapshot := range s.snapshots {
		if snapshot.IdempotencyKey == req.IdempotencyKey {
			return snapshot, false, nil
		}
	}
	snapshot := ADKTranscriptSnapshot{
		SnapshotID:     int64(501 + len(s.snapshots)),
		Kind:           req.Kind,
		Digest:         req.Digest,
		IdempotencyKey: req.IdempotencyKey,
		MessageCount:   req.MessageCount,
		Messages:       req.Messages,
	}
	s.snapshots = append(s.snapshots, snapshot)
	if s.snapshot.SnapshotID == 0 {
		s.snapshot = snapshot
	}
	return snapshot, true, nil
}

type recordingADKMemoryFlushQueue struct {
	calls []ADKMemoryFlushRequest
	err   error
}

func (q *recordingADKMemoryFlushQueue) EnqueueMemoryFlush(
	_ context.Context,
	_ *RunSummary,
	req ADKMemoryFlushRequest,
) (bool, error) {
	q.calls = append(q.calls, req)
	if q.err != nil {
		return false, q.err
	}
	return len(q.calls) == 1, nil
}

type replaySummarizingChatModel struct{}

func (m *replaySummarizingChatModel) Generate(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	if adkUsageKindFromContext(ctx) == string(ADKMiddlewareSummarization) {
		return schema.AssistantMessage("stable summary", nil), nil
	}
	return schema.AssistantMessage("stable final answer", nil), nil
}

func (m *replaySummarizingChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

type erroringTranscriptChatModel struct {
	err error
}

func (m *erroringTranscriptChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, m.err
}

func (m *erroringTranscriptChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, m.err
}

type sequentialTranscriptIDGen struct {
	mu   sync.Mutex
	next int64
}

func (g *sequentialTranscriptIDGen) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	value := g.next
	g.next++
	return value, nil
}

func (g *sequentialTranscriptIDGen) GenMultiIDs(
	_ context.Context,
	count int,
) ([]int64, error) {
	if count < 0 {
		return nil, fmt.Errorf("count must be non-negative")
	}
	result := make([]int64, count)
	for index := range result {
		value, err := g.GenID(context.Background())
		if err != nil {
			return nil, err
		}
		result[index] = value
	}
	return result, nil
}

func transcriptPayloadContains(payload, value string) bool {
	return strings.Contains(payload, value)
}

func decodeADKTranscriptMessages(t *testing.T, raw string) []*schema.Message {
	t.Helper()
	var messages []*schema.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &messages))
	return messages
}

func decodeADKTranscriptMetadata(t *testing.T, raw string) map[string]any {
	t.Helper()
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &metadata))
	return metadata
}
