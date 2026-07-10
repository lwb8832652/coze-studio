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

package appdev

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
)

func TestPersistentChatCanBeCancelledFromAnotherBroker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:appdev-chat?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&appDevChatSessionRecord{},
		&appDevChatMessageRecord{},
	))

	ctx := context.Background()
	persistence := NewPersistentChatRepository(db)
	req := &appdevapp.ChatManagerRequest{
		SpaceID:   "1001",
		ProjectID: "project-1",
		UserID:    42,
	}
	session := &appdevapp.ChatSession{
		SessionID: "session-1",
		RequestID: "request-1",
		Running:   true,
	}
	require.NoError(t, persistence.BeginSession(ctx, req, session, &appdevapp.ChatMessage{
		ID:        "user_request-1",
		Type:      "user",
		Role:      "user",
		Content:   "build a page",
		CreatedAt: time.Now().UTC(),
	}))

	broker := NewChatBrokerWithPersistence(persistence)
	require.NoError(t, broker.Cancel(ctx, req))

	status, err := broker.Status(ctx, req)
	require.NoError(t, err)
	require.False(t, status.Running)
	require.Equal(t, session.RequestID, status.RequestID)

	history, err := broker.History(ctx, req)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "任务已取消，未修改项目文件。", history[1].Content)

	secondSession := &appdevapp.ChatSession{
		SessionID: "session-2",
		RequestID: "request-2",
		Running:   true,
	}
	require.NoError(t, persistence.BeginSession(ctx, req, secondSession, &appdevapp.ChatMessage{
		ID:        "user_request-2",
		Type:      "user",
		Role:      "user",
		Content:   "continue",
		CreatedAt: time.Now().UTC(),
	}))

	err = persistence.FinishSession(ctx, req, &appdevapp.ChatStatus{
		Running:   false,
		SessionID: session.SessionID,
		RequestID: session.RequestID,
	}, nil)
	require.ErrorContains(t, err, "stale appdev chat request")

	currentStatus, err := persistence.Status(ctx, req)
	require.NoError(t, err)
	require.True(t, currentStatus.Running)
	require.Equal(t, secondSession.RequestID, currentStatus.RequestID)

	thirdSession := &appdevapp.ChatSession{
		SessionID: "session-3",
		RequestID: "request-3",
		Running:   true,
	}
	err = persistence.BeginSession(ctx, req, thirdSession, &appdevapp.ChatMessage{
		ID:        "user_request-3",
		Type:      "user",
		Role:      "user",
		Content:   "must be rejected",
		CreatedAt: time.Now().UTC(),
	})
	require.ErrorContains(t, err, "already running")

	require.NoError(t, db.Model(&appDevChatSessionRecord{}).
		Where("space_id = ? AND project_id = ?", 1001, req.ProjectID).
		Update("updated_at", time.Now().UTC().Add(-appDevChatLeaseTimeout-time.Minute)).Error)
	recoveredStatus, err := persistence.Status(ctx, req)
	require.NoError(t, err)
	require.False(t, recoveredStatus.Running)
}

func TestPersistentChatTouchSessionRenewsOnlyTheActiveRequest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:appdev-chat-touch?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&appDevChatSessionRecord{},
		&appDevChatMessageRecord{},
	))

	ctx := context.Background()
	persistence := NewPersistentChatRepository(db)
	req := &appdevapp.ChatManagerRequest{SpaceID: "1001", ProjectID: "project-touch", UserID: 42}
	session := &appdevapp.ChatSession{SessionID: "session-touch", RequestID: "request-touch", Running: true}
	require.NoError(t, persistence.BeginSession(ctx, req, session, &appdevapp.ChatMessage{
		ID:        "user_request-touch",
		Type:      "user",
		Role:      "user",
		Content:   "build a page",
		CreatedAt: time.Now().UTC(),
	}))

	staleAt := time.Now().UTC().Add(-2 * appDevChatLeaseTimeout)
	require.NoError(t, db.Model(&appDevChatSessionRecord{}).
		Where("space_id = ? AND project_id = ?", int64(1001), req.ProjectID).
		Update("updated_at", staleAt).Error)
	require.ErrorContains(t, persistence.TouchSession(ctx, req, "request-replaced"), "stale")
	require.NoError(t, persistence.TouchSession(ctx, req, session.RequestID))

	var record appDevChatSessionRecord
	require.NoError(t, db.Where("space_id = ? AND project_id = ?", int64(1001), req.ProjectID).Take(&record).Error)
	require.True(t, record.Running)
	require.WithinDuration(t, time.Now().UTC(), record.UpdatedAt, time.Second)
}
