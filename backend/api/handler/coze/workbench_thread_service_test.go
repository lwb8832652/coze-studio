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
	"context"
	"net/http"
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
	appagentthread.InitService(&appagentthread.ServiceComponents{DB: db, IDGen: sequentialIDGen{}})
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
		)
	`).Error
}

type sequentialIDGen struct {
	next int64
}

func (g sequentialIDGen) GenID(ctx context.Context) (int64, error) {
	if g.next == 0 {
		return 1, nil
	}

	return g.next, nil
}

func (g sequentialIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	return ids, nil
}
