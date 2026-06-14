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

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestListTaskThreadsHandlerReturnsAgentThreads(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/task_threads", ListTaskThreads)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads?space_id=1&page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"msg":"success"`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"title":"任务列表"`)
}

func TestGetTaskThreadHandlerReturnsAgentThread(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/task_threads/:thread_id", GetTaskThread)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"title":"任务列表"`)
}

func TestListTaskThreadMessagesHandlerReturnsMessages(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/task_threads/:thread_id/messages", ListTaskThreadMessages)
	installAgentThreadTestService(t)

	_, err := appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		Role:     appagentthread.MessageRoleUser,
		Content:  "请分析客户反馈",
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "客户反馈集中在响应速度。",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/messages?page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"role":"user"`)
	require.Contains(t, body, `"content":"请分析客户反馈"`)
	require.Contains(t, body, `"role":"assistant"`)
	require.Contains(t, body, `"content":"客户反馈集中在响应速度。"`)
}

func TestAppendTaskThreadMessageHandlerCreatesMessage(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/task_threads/:thread_id/messages", AppendTaskThreadMessage)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"role":     "user",
		"content":  "请生成行动计划",
		"metadata": `{"source":"test"}`,
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/messages",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"role":"user"`)
	require.Contains(t, body, `"content":"请生成行动计划"`)

	resp, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Equal(t, "请生成行动计划", resp.Messages[0].Content)
	require.Equal(t, `{"source":"test"}`, resp.Messages[0].Metadata)
}

func TestCreateTaskThreadRunHandlerCreatesPendingRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/task_threads/:thread_id/runs", CreateTaskThreadRun)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"input":           `{"messages":[{"role":"user","content":"请追加行动建议"}]}`,
		"config":          `{"mode":"Auto"}`,
		"metadata":        `{"source":"test"}`,
		"idempotency_key": "thread-only-1-msg-1",
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"input":"{\"messages\":[{\"role\":\"user\",\"content\":\"请追加行动建议\"}]}"`)

	resp, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Equal(t, appagentthread.RunStatusPending, resp.Runs[0].Status)
	require.Equal(t, `{"mode":"Auto"}`, resp.Runs[0].Config)
	require.Equal(t, "thread-only-1-msg-1", resp.Runs[0].IdempotencyKey)
}

func TestListTaskThreadRunsHandlerReturnsRuns(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/task_threads/:thread_id/runs", ListTaskThreadRuns)
	installAgentThreadTestService(t)

	_, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"第一轮"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"第二轮"}]}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/runs?page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"input":"{\"messages\":[{\"role\":\"user\",\"content\":\"第二轮\"}]}"`)
	require.Contains(t, body, `"input":"{\"messages\":[{\"role\":\"user\",\"content\":\"第一轮\"}]}"`)
}

func TestListTaskThreadsHandlerRejectsInvalidQuery(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/task_threads", ListTaskThreads)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads?space_id=bad", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func installAgentThreadTestService(t *testing.T) {
	t.Helper()
	prev := appagentthread.SVC.ThreadSVC
	t.Cleanup(func() {
		appagentthread.SVC.ThreadSVC = prev
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadHandlerTableForTest(db))
	appagentthread.InitService(&appagentthread.ServiceComponents{DB: db, IDGen: &sequentialIDGen{next: 1}})
	_, err = appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:      1,
		UserID:       2,
		Title:        "任务列表",
		Source:       appagentthread.ThreadSourceWeb,
		LegacyTaskID: 100,
		Metadata:     `{"message":"hello"}`,
	})
	require.NoError(t, err)
}

func migrateAgentThreadHandlerTableForTest(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE agent_threads (
			id integer PRIMARY KEY,
			space_id integer,
			creator_id integer,
			agent_id integer,
			title text,
			status text,
			source text,
			legacy_task_id integer,
			metadata json,
			created_at integer,
			updated_at integer,
			last_message_at integer
		);
		CREATE TABLE agent_thread_messages (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			role text,
			content text,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_runs (
			id integer PRIMARY KEY,
			thread_id integer,
			space_id integer,
			creator_id integer,
			assistant_id text,
			status text,
			command json,
			input json,
			config json,
			context json,
			metadata json,
			stream_mode json,
			multitask_strategy text,
			on_disconnect text,
			durability text,
			idempotency_key text,
			worker_id text,
			error_code text,
			error_message text,
			started_at integer,
			ended_at integer,
			created_at integer,
			updated_at integer
		)
	`).Error
}

type sequentialIDGen struct {
	mu   sync.Mutex
	next int64
}

func (g *sequentialIDGen) GenID(ctx context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.next <= 0 {
		g.next = 2
		return 1, nil
	}

	id := g.next
	g.next++
	return id, nil
}

func (g *sequentialIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
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
