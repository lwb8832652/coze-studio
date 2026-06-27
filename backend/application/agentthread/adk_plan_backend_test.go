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
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/stretchr/testify/require"
)

func TestADKPlanBackendTranslatesEinoFilesToDurablePlan(t *testing.T) {
	store := newMemoryADKPlanStore()
	events := &recordingRunEventSink{}
	backend, err := NewADKPlanBackend(&RunSummary{
		RunID:     21,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}, store, events)
	require.NoError(t, err)

	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content: `{
			"id":"1",
			"subject":"Run tests",
			"description":"Run focused tests.",
			"status":"pending",
			"blocks":[],
			"blockedBy":[],
			"activeForm":"Running tests",
			"metadata":{"source":"agent"}
		}`,
	}))

	files, err := backend.LsInfo(context.Background(), &plantask.LsInfoRequest{
		Path: "/plans",
	})
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "/plans/.highwatermark", files[0].Path)
	require.Equal(t, "/plans/1.json", files[1].Path)

	content, err := backend.Read(context.Background(), &plantask.ReadRequest{
		FilePath: "/plans/1.json",
	})
	require.NoError(t, err)
	var task map[string]any
	require.NoError(t, json.Unmarshal([]byte(content.Content), &task))
	require.Equal(t, "1", task["id"])
	require.Equal(t, "Run tests", task["subject"])
	require.Equal(t, "pending", task["status"])
	require.Equal(t, []string{"plan.task.created"}, events.eventTypes())
	require.Contains(t, events.events[0].Payload, `"plan_scope_run_id":21`)
	require.Contains(t, events.events[0].Payload, `"plan_task_id":"1"`)
	require.NotContains(t, events.events[0].Payload, "Run focused tests")
	require.NotContains(t, events.events[0].Payload, `"metadata"`)
}

func TestADKPlanBackendArchivesCompletedCleanupWithoutDuplicateEvent(t *testing.T) {
	store := newMemoryADKPlanStore()
	events := &recordingRunEventSink{}
	backend, err := NewADKPlanBackend(&RunSummary{
		RunID:     21,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}, store, events)
	require.NoError(t, err)
	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Run tests","description":"","status":"pending","blocks":[],"blockedBy":[]}`,
	}))
	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Run tests","description":"","status":"completed","blocks":[],"blockedBy":[]}`,
	}))

	require.NoError(t, backend.Delete(context.Background(), &plantask.DeleteRequest{
		FilePath: "/plans/1.json",
	}))

	require.Equal(t, []string{
		"plan.task.created",
		"plan.task.completed",
	}, events.eventTypes())
	task := store.task(1)
	require.NotNil(t, task)
	require.Equal(t, "completed", task.Status)
	require.False(t, task.Active)
	files, err := backend.LsInfo(context.Background(), &plantask.LsInfoRequest{
		Path: "/plans",
	})
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestADKPlanBackendRejectsInvalidPathsAndStaleReservations(t *testing.T) {
	store := newMemoryADKPlanStore()
	backend, err := NewADKPlanBackend(&RunSummary{
		RunID:     21,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}, store, &recordingRunEventSink{})
	require.NoError(t, err)

	for _, path := range []string{
		"/other/.highwatermark",
		"/plans/../1.json",
		"/plans/01.json",
		"/plans/1.txt",
		`/plans\1.json`,
	} {
		require.Error(t, backend.Write(context.Background(), &plantask.WriteRequest{
			FilePath: path,
			Content:  "1",
		}), path)
	}

	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	err = backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reservation")
}

func TestADKPlanBackendUsesSourceRunPlanScope(t *testing.T) {
	store := newMemoryADKPlanStore()
	backend, err := NewADKPlanBackend(&RunSummary{
		RunID:          21,
		PlanScopeRunID: 20,
		ThreadID:       10,
		SpaceID:        30,
		CreatorID:      40,
	}, store, &recordingRunEventSink{})
	require.NoError(t, err)

	require.NoError(t, backend.Write(context.Background(), &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))

	require.Equal(t, int64(20), store.lastScope.ScopeRunID)
	require.Equal(t, int64(21), store.lastScope.ActiveRunID)
}

func TestADKPlanBackendAllowsOnlyOneConcurrentReservation(t *testing.T) {
	store := newMemoryADKPlanStore()
	backends := make([]*ADKPlanBackend, 20)
	for index := range backends {
		backend, err := NewADKPlanBackend(&RunSummary{
			RunID:     21,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		}, store, &recordingRunEventSink{})
		require.NoError(t, err)
		backends[index] = backend
	}

	var (
		wg        sync.WaitGroup
		successes int
		mu        sync.Mutex
	)
	for index := range backends {
		wg.Add(1)
		go func(backend *ADKPlanBackend) {
			defer wg.Done()
			err := backend.Write(
				context.Background(),
				&plantask.WriteRequest{
					FilePath: "/plans/.highwatermark",
					Content:  "1",
				},
			)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(backends[index])
	}
	wg.Wait()

	require.Equal(t, 1, successes)
	require.Equal(t, int64(1), store.highWatermark)
}

type memoryADKPlanStore struct {
	mu            sync.Mutex
	highWatermark int64
	revision      int64
	tasks         map[int64]*ADKPlanTask
	lastScope     ADKPlanScope
}

func newMemoryADKPlanStore() *memoryADKPlanStore {
	return &memoryADKPlanStore{tasks: make(map[int64]*ADKPlanTask)}
}

func (s *memoryADKPlanStore) OpenPlan(
	_ context.Context,
	scope ADKPlanScope,
) (*ADKPlanSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScope = scope
	return s.snapshot(), nil
}

func (s *memoryADKPlanStore) GetPlanTask(
	_ context.Context,
	scope ADKPlanScope,
	taskID int64,
) (*ADKPlanTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScope = scope
	task := s.tasks[taskID]
	if task == nil {
		return nil, errors.New("plan task not found")
	}
	cloned := *task
	return &cloned, nil
}

func (s *memoryADKPlanStore) ReservePlanTaskID(
	_ context.Context,
	scope ADKPlanScope,
	expected int64,
	next int64,
) (*ADKPlanSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScope = scope
	if s.highWatermark != expected || next != expected+1 {
		return nil, errors.New("plan reservation conflict")
	}
	s.highWatermark = next
	s.revision++
	return s.snapshot(), nil
}

func (s *memoryADKPlanStore) UpsertPlanTask(
	_ context.Context,
	scope ADKPlanScope,
	task *ADKPlanTask,
) (*ADKPlanMutation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScope = scope
	if task.TaskID > s.highWatermark {
		return nil, errors.New("task exceeds high watermark")
	}
	var previous *ADKPlanTask
	if current := s.tasks[task.TaskID]; current != nil {
		cloned := *current
		previous = &cloned
	}
	cloned := *task
	cloned.Active = true
	s.tasks[task.TaskID] = &cloned
	s.revision++
	return &ADKPlanMutation{
		Snapshot: s.snapshot(),
		Task:     cloneADKPlanTask(&cloned),
		Previous: previous,
		Created:  previous == nil,
	}, nil
}

func (s *memoryADKPlanStore) ArchivePlanTask(
	_ context.Context,
	scope ADKPlanScope,
	taskID int64,
) (*ADKPlanMutation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScope = scope
	current := s.tasks[taskID]
	if current == nil {
		return nil, errors.New("plan task not found")
	}
	previous := *current
	current.Active = false
	if current.Status != "completed" {
		current.Status = "deleted"
	}
	s.revision++
	return &ADKPlanMutation{
		Snapshot: s.snapshot(),
		Task:     cloneADKPlanTask(current),
		Previous: &previous,
	}, nil
}

func (s *memoryADKPlanStore) snapshot() *ADKPlanSnapshot {
	tasks := make([]*ADKPlanTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.Active {
			tasks = append(tasks, cloneADKPlanTask(task))
		}
	}
	return &ADKPlanSnapshot{
		HighWatermark: s.highWatermark,
		Revision:      s.revision,
		Tasks:         tasks,
	}
}

func (s *memoryADKPlanStore) task(taskID int64) *ADKPlanTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneADKPlanTask(s.tasks[taskID])
}

func cloneADKPlanTask(task *ADKPlanTask) *ADKPlanTask {
	if task == nil {
		return nil
	}
	cloned := *task
	cloned.Blocks = append([]string(nil), task.Blocks...)
	cloned.BlockedBy = append([]string(nil), task.BlockedBy...)
	if task.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(task.Metadata))
		for key, value := range task.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}
