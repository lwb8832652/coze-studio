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

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
)

func TestCreateTaskStartsCreated(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 201}})

	task, err := svc.Create(context.Background(), &CreateRequest{SpaceID: 1, CreatorID: 2, Title: "Generate report", Input: `{"topic":"weekly"}`})

	require.NoError(t, err)
	require.Equal(t, int64(201), task.ID)
	require.Equal(t, entity.StatusCreated, task.Status)
	require.Equal(t, int32(0), task.Progress)
}

func TestRetryFailedTaskQueuesIt(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 202}})
	task, err := svc.Create(context.Background(), &CreateRequest{SpaceID: 1, CreatorID: 2, Title: "x"})
	require.NoError(t, err)
	require.NoError(t, svc.Fail(context.Background(), task.ID, "boom"))

	retried, err := svc.Retry(context.Background(), task.ID)

	require.NoError(t, err)
	require.Equal(t, entity.StatusQueued, retried.Status)
}

func TestCancelCreatedTaskMarksCanceled(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 203}})
	task, err := svc.Create(context.Background(), &CreateRequest{SpaceID: 1, CreatorID: 2, Title: "x"})
	require.NoError(t, err)

	canceled, err := svc.Cancel(context.Background(), task.ID)

	require.NoError(t, err)
	require.Equal(t, entity.StatusCanceled, canceled.Status)
}

func TestCompleteRequiresRunningTask(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 204}})
	task, err := svc.Create(context.Background(), &CreateRequest{SpaceID: 1, CreatorID: 2, Title: "x"})
	require.NoError(t, err)

	err = svc.Complete(context.Background(), task.ID, `{"ok":true}`)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot transition")
}

func TestFailSupportsCreatedQueuedRunningAndCanceling(t *testing.T) {
	ctx := context.Background()
	for _, status := range []entity.Status{
		entity.StatusCreated,
		entity.StatusQueued,
		entity.StatusRunning,
		entity.StatusCanceling,
	} {
		t.Run(string(status), func(t *testing.T) {
			repo := newMemoryRepo()
			svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 205}})
			require.NoError(t, repo.Create(ctx, &entity.Task{ID: 1, SpaceID: 1, CreatorID: 2, Title: "x", Status: status}))

			err := svc.Fail(ctx, 1, "boom")

			require.NoError(t, err)
			got, err := svc.Get(ctx, 1)
			require.NoError(t, err)
			require.Equal(t, entity.StatusFailed, got.Status)
			require.Equal(t, "boom", got.Error)
		})
	}
}

func TestRetryOnlySupportsFailedTask(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 206}})
	task, err := svc.Create(context.Background(), &CreateRequest{SpaceID: 1, CreatorID: 2, Title: "x"})
	require.NoError(t, err)

	_, err = svc.Retry(context.Background(), task.ID)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot transition")
}

type memoryRepo struct {
	mu     sync.Mutex
	tasks  map[int64]*entity.Task
	events map[int64][]*entity.Event
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		tasks:  make(map[int64]*entity.Task),
		events: make(map[int64][]*entity.Event),
	}
}

func (r *memoryRepo) Create(ctx context.Context, task *entity.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[task.ID]; ok {
		return fmt.Errorf("task %d already exists", task.ID)
	}
	r.tasks[task.ID] = cloneTask(task)
	return nil
}

func (r *memoryRepo) Get(ctx context.Context, id int64) (*entity.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %d not found", id)
	}
	return cloneTask(task), nil
}

func (r *memoryRepo) List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tasks := make([]*entity.Task, 0)
	for _, task := range r.tasks {
		if task.SpaceID != spaceID {
			continue
		}
		if status != nil && task.Status != *status {
			continue
		}
		tasks = append(tasks, cloneTask(task))
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].UpdatedAt == tasks[j].UpdatedAt {
			return tasks[i].ID > tasks[j].ID
		}
		return tasks[i].UpdatedAt > tasks[j].UpdatedAt
	})
	total := int64(len(tasks))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := int((page - 1) * pageSize)
	if start >= len(tasks) {
		return []*entity.Task{}, total, nil
	}
	end := start + int(pageSize)
	if end > len(tasks) {
		end = len(tasks)
	}
	return tasks[start:end], total, nil
}

func (r *memoryRepo) UpdateStatus(ctx context.Context, id int64, from, to entity.Status, progress int32, result, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return fmt.Errorf("task %d not found", id)
	}
	if task.Status != from {
		return fmt.Errorf("update task status failed: task %d is not in status %s", id, from)
	}
	task.Status = to
	task.Progress = progress
	task.Result = result
	task.Error = errMsg
	return nil
}

func (r *memoryRepo) CreateEvent(ctx context.Context, event *entity.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event.TaskID] = append(r.events[event.TaskID], cloneEvent(event))
	return nil
}

func (r *memoryRepo) ListEvents(ctx context.Context, taskID int64) ([]*entity.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]*entity.Event, 0, len(r.events[taskID]))
	for _, event := range r.events[taskID] {
		events = append(events, cloneEvent(event))
	}
	return events, nil
}

func cloneTask(task *entity.Task) *entity.Task {
	if task == nil {
		return nil
	}
	cloned := *task
	return &cloned
}

func cloneEvent(event *entity.Event) *entity.Event {
	if event == nil {
		return nil
	}
	cloned := *event
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
