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
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestApplicationCreateThreadReturnsTaskSummary(t *testing.T) {
	domainSVC := &recordingThreadService{
		created: &entity.Thread{
			ID:        10,
			SpaceID:   1,
			CreatorID: 2,
			AgentID:   3,
			Title:     "生成周报",
			Status:    entity.ThreadStatusIdle,
			Source:    entity.ThreadSourceIM,
			CreatedAt: 100,
			UpdatedAt: 100,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID:      1,
		UserID:       2,
		AgentID:      3,
		Title:        "生成周报",
		Source:       ThreadSourceIM,
		LegacyTaskID: 4,
		Metadata:     `{"channel":"lark"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), resp.Thread.ThreadID)
	require.Equal(t, "生成周报", domainSVC.createReq.Title)
	require.Equal(t, int64(2), domainSVC.createReq.UserID)
	require.Equal(t, entity.ThreadSourceIM, domainSVC.createReq.Source)
	require.Equal(t, int64(4), domainSVC.createReq.LegacyTaskID)
	require.Equal(t, `{"channel":"lark"}`, domainSVC.createReq.Metadata)
	require.Equal(t, ThreadStatusIdle, resp.Thread.Status)
	require.Equal(t, ThreadSourceIM, resp.Thread.Source)
}

func TestApplicationListThreadsMapsDomainThreads(t *testing.T) {
	domainSVC := &recordingThreadService{
		listed: []*entity.Thread{
			{
				ID:        10,
				SpaceID:   1,
				CreatorID: 2,
				Title:     "新任务",
				Status:    entity.ThreadStatusRunning,
				Source:    entity.ThreadSourceWeb,
				UpdatedAt: 200,
			},
		},
		total: 1,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	status := ThreadStatusRunning

	resp, err := app.ListThreads(context.Background(), &ListThreadsRequest{
		SpaceID:  1,
		UserID:   2,
		Status:   &status,
		Page:     2,
		PageSize: 5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Threads, 1)
	require.Equal(t, int64(10), resp.Threads[0].ThreadID)
	require.Equal(t, ThreadStatusRunning, resp.Threads[0].Status)
	require.Equal(t, int64(2), domainSVC.listReq.UserID)
	require.NotNil(t, domainSVC.listReq.Status)
	require.Equal(t, entity.ThreadStatusRunning, *domainSVC.listReq.Status)
	require.Equal(t, int32(2), domainSVC.listReq.Page)
	require.Equal(t, int32(5), domainSVC.listReq.PageSize)
}

func TestApplicationServiceRequiresThreadService(t *testing.T) {
	_, err := (*ApplicationService)(nil).CreateThread(context.Background(), &CreateThreadRequest{Title: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent thread service")

	_, err = (&ApplicationService{}).ListThreads(context.Background(), &ListThreadsRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent thread service")
}

func TestApplicationServiceRejectsNilRequests(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{}}

	_, err := app.CreateThread(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create thread request")

	_, err = app.ListThreads(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "list threads request")
}

func TestApplicationCreateThreadRejectsEmptyDomainThread(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{}}

	_, err := app.CreateThread(context.Background(), &CreateThreadRequest{Title: "空结果"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty thread")
}

func TestInitServiceBuildsUsableThreadService(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))

	app := InitService(&ServiceComponents{
		DB:    db,
		IDGen: fixedIDGen{},
	})
	resp, err := app.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "初始化任务",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Thread.ThreadID)
	require.Equal(t, "初始化任务", resp.Thread.Title)
}

type recordingThreadService struct {
	created   *entity.Thread
	listed    []*entity.Thread
	total     int64
	createReq *domainservice.CreateThreadRequest
	listReq   *domainservice.ListThreadsRequest
}

func migrateAgentThreadTableForTest(db *gorm.DB) error {
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

func (s *recordingThreadService) CreateThread(ctx context.Context, req *domainservice.CreateThreadRequest) (*entity.Thread, error) {
	s.createReq = req
	return s.created, nil
}

func (s *recordingThreadService) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	return nil, nil
}

func (s *recordingThreadService) ListThreads(ctx context.Context, req *domainservice.ListThreadsRequest) ([]*entity.Thread, int64, error) {
	s.listReq = req
	return s.listed, s.total, nil
}

type fixedIDGen struct{}

func (fixedIDGen) GenID(ctx context.Context) (int64, error) {
	return 1, nil
}

func (fixedIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	return ids, nil
}
