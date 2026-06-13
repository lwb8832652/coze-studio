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

package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestCreateThreadRequiresTitle(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	_, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "  ",
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestCreateThreadDefaultsToIdleWebTask(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	thread, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "分析订单异常",
	})

	require.NoError(t, err)
	require.Equal(t, int64(901), thread.ID)
	require.Equal(t, entity.ThreadStatusIdle, thread.Status)
	require.Equal(t, entity.ThreadSourceWeb, thread.Source)
	require.Equal(t, int64(1), thread.SpaceID)
	require.Equal(t, int64(2), thread.CreatorID)
	require.Equal(t, "分析订单异常", thread.Title)
	require.NotZero(t, thread.CreatedAt)
	require.Equal(t, thread.CreatedAt, thread.UpdatedAt)
	require.Equal(t, thread.CreatedAt, thread.LastMessageAt)
}

func TestCreateThreadTrimsTitleAndPreservesSource(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 902}})

	thread, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID:      1,
		UserID:       2,
		AgentID:      3,
		Title:        "  IM 任务  ",
		Source:       entity.ThreadSourceIM,
		LegacyTaskID: 100,
		Metadata:     `{"channel":"slack"}`,
	})

	require.NoError(t, err)
	require.Equal(t, "IM 任务", thread.Title)
	require.Equal(t, entity.ThreadSourceIM, thread.Source)
	require.Equal(t, int64(3), thread.AgentID)
	require.Equal(t, int64(100), thread.LegacyTaskID)
	require.Equal(t, `{"channel":"slack"}`, thread.Metadata)
}

func TestListThreadsNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(901)})
	_, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "A",
	})
	require.NoError(t, err)
	_, err = svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "B",
	})
	require.NoError(t, err)

	threads, total, err := svc.ListThreads(context.Background(), &ListThreadsRequest{
		SpaceID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, threads, 2)
	require.Equal(t, int32(1), repo.lastListReq.Page)
	require.Equal(t, int32(20), repo.lastListReq.PageSize)
}

func TestListThreadsRequiresRequest(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	_, _, err := svc.ListThreads(context.Background(), nil)

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestAppendMessageRequiresContentAndValidRole(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	_, err := svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 1,
		Role:     entity.MessageRoleUser,
		Content:  "  ",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 1,
		Role:     entity.MessageRole("bad"),
		Content:  "hello",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestAppendMessageCreatesMessageWithGeneratedID(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	message, err := svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 10,
		RunID:    20,
		Role:     entity.MessageRoleUser,
		Content:  "  请分析客户反馈  ",
		Metadata: `{"source":"web"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1001), message.ID)
	require.Equal(t, int64(10), message.ThreadID)
	require.Equal(t, int64(20), message.RunID)
	require.Equal(t, entity.MessageRoleUser, message.Role)
	require.Equal(t, "请分析客户反馈", message.Content)
	require.Equal(t, `{"source":"web"}`, message.Metadata)
	require.NotZero(t, message.CreatedAt)
	require.Len(t, repo.messages[10], 1)
}

func TestAppendMessageRequiresExistingThread(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	_, err := svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 10,
		Role:     entity.MessageRoleUser,
		Content:  "hello",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "thread")
	require.Empty(t, repo.messages[10])
}

func TestListMessagesNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	repo.messages[10] = []*entity.Message{
		{ID: 1, ThreadID: 10, Role: entity.MessageRoleUser, Content: "第一条", CreatedAt: 1},
		{ID: 2, ThreadID: 10, Role: entity.MessageRoleAssistant, Content: "第二条", CreatedAt: 2},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	messages, total, err := svc.ListMessages(context.Background(), &ListMessagesRequest{
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, messages, 2)
	require.Equal(t, int32(1), repo.lastMessageListReq.Page)
	require.Equal(t, int32(50), repo.lastMessageListReq.PageSize)
}

func TestCreateRunRequiresExistingThreadAndInput(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 0,
		Input:    `{}`,
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    "  ",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    `{}`,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thread")
}

func TestCreateRunDefaultsStatusAndRuntimeOptions(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    `{"messages":[{"role":"user","content":"hello"}]}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2001), run.ID)
	require.Equal(t, int64(10), run.ThreadID)
	require.Equal(t, int64(1), run.SpaceID)
	require.Equal(t, int64(2), run.CreatorID)
	require.Equal(t, "default", run.AssistantID)
	require.Equal(t, entity.RunStatusPending, run.Status)
	require.Equal(t, `{}`, run.Command)
	require.Equal(t, `{"messages":[{"role":"user","content":"hello"}]}`, run.Input)
	require.Equal(t, `{}`, run.Config)
	require.Equal(t, `{}`, run.Context)
	require.Equal(t, `{}`, run.Metadata)
	require.Equal(t, `["messages","updates"]`, run.StreamMode)
	require.Equal(t, "enqueue", run.MultitaskStrategy)
	require.Equal(t, "continue", run.OnDisconnect)
	require.Equal(t, "async", run.Durability)
	require.NotZero(t, run.CreatedAt)
	require.Equal(t, run.CreatedAt, run.UpdatedAt)
	require.Len(t, repo.runs[10], 1)
}

func TestListRunsNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 1, ThreadID: 10, Status: entity.RunStatusPending, CreatedAt: 1},
		{ID: 2, ThreadID: 10, Status: entity.RunStatusRunning, CreatedAt: 2},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	runs, total, err := svc.ListRuns(context.Background(), &ListRunsRequest{
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, runs, 2)
	require.Equal(t, int32(1), repo.lastRunListReq.Page)
	require.Equal(t, int32(20), repo.lastRunListReq.PageSize)
}

type memoryRepo struct {
	mu                 sync.Mutex
	threads            map[int64]*entity.Thread
	messages           map[int64][]*entity.Message
	runs               map[int64][]*entity.Run
	lastListReq        repository.ListThreadsRequest
	lastMessageListReq repository.ListMessagesRequest
	lastRunListReq     repository.ListRunsRequest
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		threads:  make(map[int64]*entity.Thread),
		messages: make(map[int64][]*entity.Message),
		runs:     make(map[int64][]*entity.Run),
	}
}

func (r *memoryRepo) CreateThread(ctx context.Context, thread *entity.Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.threads[thread.ID]; ok {
		return fmt.Errorf("thread %d already exists", thread.ID)
	}
	r.threads[thread.ID] = cloneThread(thread)
	return nil
}

func (r *memoryRepo) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	thread, ok := r.threads[id]
	if !ok {
		return nil, fmt.Errorf("thread %d not found", id)
	}
	return cloneThread(thread), nil
}

func (r *memoryRepo) ListThreads(ctx context.Context, req repository.ListThreadsRequest) ([]*entity.Thread, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastListReq = req

	threads := make([]*entity.Thread, 0, len(r.threads))
	for _, thread := range r.threads {
		if thread.SpaceID != req.SpaceID {
			continue
		}
		if req.UserID > 0 && thread.CreatorID != req.UserID {
			continue
		}
		if req.Status != nil && thread.Status != *req.Status {
			continue
		}
		threads = append(threads, cloneThread(thread))
	}

	sort.Slice(threads, func(i, j int) bool {
		if threads[i].UpdatedAt == threads[j].UpdatedAt {
			return threads[i].ID > threads[j].ID
		}
		return threads[i].UpdatedAt > threads[j].UpdatedAt
	})

	total := int64(len(threads))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(threads) {
		return []*entity.Thread{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(threads) {
		end = len(threads)
	}
	return threads[start:end], total, nil
}

func (r *memoryRepo) CreateMessage(ctx context.Context, message *entity.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages[message.ThreadID] = append(r.messages[message.ThreadID], cloneMessage(message))
	return nil
}

func (r *memoryRepo) ListMessages(ctx context.Context, req repository.ListMessagesRequest) ([]*entity.Message, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastMessageListReq = req

	messages := make([]*entity.Message, 0, len(r.messages[req.ThreadID]))
	for _, message := range r.messages[req.ThreadID] {
		messages = append(messages, cloneMessage(message))
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt == messages[j].CreatedAt {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt < messages[j].CreatedAt
	})

	total := int64(len(messages))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(messages) {
		return []*entity.Message{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(messages) {
		end = len(messages)
	}
	return messages[start:end], total, nil
}

func (r *memoryRepo) CreateRun(ctx context.Context, run *entity.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ThreadID] = append(r.runs[run.ThreadID], cloneRun(run))
	return nil
}

func (r *memoryRepo) GetRun(ctx context.Context, id int64) (*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID == id {
				return cloneRun(run), nil
			}
		}
	}

	return nil, fmt.Errorf("run %d not found", id)
}

func (r *memoryRepo) ListRuns(ctx context.Context, req repository.ListRunsRequest) ([]*entity.Run, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRunListReq = req

	runs := make([]*entity.Run, 0, len(r.runs[req.ThreadID]))
	for _, run := range r.runs[req.ThreadID] {
		if req.Status != nil && run.Status != *req.Status {
			continue
		}
		runs = append(runs, cloneRun(run))
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt == runs[j].CreatedAt {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].CreatedAt > runs[j].CreatedAt
	})

	total := int64(len(runs))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(runs) {
		return []*entity.Run{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(runs) {
		end = len(runs)
	}
	return runs[start:end], total, nil
}

func cloneThread(thread *entity.Thread) *entity.Thread {
	if thread == nil {
		return nil
	}
	cloned := *thread
	return &cloned
}

func cloneMessage(message *entity.Message) *entity.Message {
	if message == nil {
		return nil
	}
	cloned := *message
	return &cloned
}

func cloneRun(run *entity.Run) *entity.Run {
	if run == nil {
		return nil
	}
	cloned := *run
	return &cloned
}

type fixedIDGen struct {
	next int64
}

func (g fixedIDGen) GenID(ctx context.Context) (int64, error) {
	return g.next, nil
}

func (g fixedIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = g.next + int64(i)
	}
	return ids, nil
}

type sequenceIDGen struct {
	mu   sync.Mutex
	next int64
}

func newSequenceIDGen(next int64) *sequenceIDGen {
	return &sequenceIDGen{next: next}
}

func (g *sequenceIDGen) GenID(ctx context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := g.next
	g.next++
	return id, nil
}

func (g *sequenceIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}
