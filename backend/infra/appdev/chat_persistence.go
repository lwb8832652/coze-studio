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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
)

type ChatPersistence interface {
	BeginSession(ctx context.Context, req *appdevapp.ChatManagerRequest, session *appdevapp.ChatSession, message *appdevapp.ChatMessage) error
	FinishSession(ctx context.Context, req *appdevapp.ChatManagerRequest, status *appdevapp.ChatStatus, message *appdevapp.ChatMessage) error
	TouchSession(ctx context.Context, req *appdevapp.ChatManagerRequest, requestID string) error
	History(ctx context.Context, req *appdevapp.ChatManagerRequest) ([]*appdevapp.ChatMessage, error)
	Status(ctx context.Context, req *appdevapp.ChatManagerRequest) (*appdevapp.ChatStatus, error)
}

type PersistentChatRepository struct {
	db *gorm.DB
}

var appDevChatLeaseTimeout = 10 * time.Minute

type appDevChatSessionRecord struct {
	SpaceID   int64     `gorm:"column:space_id;primaryKey"`
	ProjectID string    `gorm:"column:project_id;size:64;primaryKey"`
	SessionID string    `gorm:"column:session_id;size:64;not null"`
	RequestID string    `gorm:"column:request_id;size:64;not null"`
	Running   bool      `gorm:"column:running;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (*appDevChatSessionRecord) TableName() string { return "appdev_chat_sessions" }

type appDevChatMessageRecord struct {
	ID              string    `gorm:"column:id;primaryKey;size:128"`
	SpaceID         int64     `gorm:"column:space_id;not null;index:idx_appdev_chat_messages_project,priority:1"`
	ProjectID       string    `gorm:"column:project_id;size:64;not null;index:idx_appdev_chat_messages_project,priority:2"`
	SessionID       string    `gorm:"column:session_id;size:64;not null"`
	RequestID       string    `gorm:"column:request_id;size:64;not null"`
	MessageType     string    `gorm:"column:message_type;size:32;not null"`
	Role            string    `gorm:"column:role;size:32"`
	Title           string    `gorm:"column:title;size:256"`
	Content         string    `gorm:"column:content;type:text;not null"`
	AttachmentsJSON []byte    `gorm:"column:attachments_json;type:json"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;index:idx_appdev_chat_messages_project,priority:3"`
}

func (*appDevChatMessageRecord) TableName() string { return "appdev_chat_messages" }

func NewPersistentChatRepository(db *gorm.DB) *PersistentChatRepository {
	return &PersistentChatRepository{db: db}
}

func (p *PersistentChatRepository) BeginSession(
	ctx context.Context,
	req *appdevapp.ChatManagerRequest,
	session *appdevapp.ChatSession,
	message *appdevapp.ChatMessage,
) error {
	if p == nil || p.db == nil || req == nil || session == nil || message == nil {
		return fmt.Errorf("persistent appdev chat is not configured")
	}
	spaceID, err := persistentChatSpaceID(req.SpaceID)
	if err != nil {
		return err
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := beginChatStatus(tx, spaceID, req.ProjectID, &appdevapp.ChatStatus{
			Running:   true,
			SessionID: session.SessionID,
			RequestID: session.RequestID,
		}); err != nil {
			return err
		}
		record, err := chatMessageRecord(spaceID, req.ProjectID, session.SessionID, session.RequestID, message)
		if err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error
	})
}

func (p *PersistentChatRepository) FinishSession(
	ctx context.Context,
	req *appdevapp.ChatManagerRequest,
	status *appdevapp.ChatStatus,
	message *appdevapp.ChatMessage,
) error {
	if p == nil || p.db == nil || req == nil || status == nil {
		return fmt.Errorf("persistent appdev chat is not configured")
	}
	spaceID, err := persistentChatSpaceID(req.SpaceID)
	if err != nil {
		return err
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := finishChatStatus(tx, spaceID, req.ProjectID, status); err != nil {
			return err
		}
		if message == nil {
			return nil
		}
		record, err := chatMessageRecord(spaceID, req.ProjectID, status.SessionID, status.RequestID, message)
		if err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error
	})
}

func (p *PersistentChatRepository) TouchSession(
	ctx context.Context,
	req *appdevapp.ChatManagerRequest,
	requestID string,
) error {
	if p == nil || p.db == nil || req == nil {
		return fmt.Errorf("persistent appdev chat is not configured")
	}
	spaceID, err := persistentChatSpaceID(req.SpaceID)
	if err != nil {
		return err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("appdev chat request_id is required")
	}

	result := p.db.WithContext(ctx).Model(&appDevChatSessionRecord{}).
		Where(
			"space_id = ? AND project_id = ? AND request_id = ? AND running = ?",
			spaceID,
			strings.TrimSpace(req.ProjectID),
			requestID,
			true,
		).
		Update("updated_at", time.Now().UTC())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("cannot renew stale appdev chat request")
	}
	return nil
}

func (p *PersistentChatRepository) History(ctx context.Context, req *appdevapp.ChatManagerRequest) ([]*appdevapp.ChatMessage, error) {
	if p == nil || p.db == nil || req == nil {
		return nil, fmt.Errorf("persistent appdev chat is not configured")
	}
	spaceID, err := persistentChatSpaceID(req.SpaceID)
	if err != nil {
		return nil, err
	}
	var records []*appDevChatMessageRecord
	if err := p.db.WithContext(ctx).
		Where("space_id = ? AND project_id = ?", spaceID, strings.TrimSpace(req.ProjectID)).
		Order("created_at ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	messages := make([]*appdevapp.ChatMessage, 0, len(records))
	for _, record := range records {
		var attachments []appdevapp.ChatAttachment
		if len(record.AttachmentsJSON) > 0 {
			if err := json.Unmarshal(record.AttachmentsJSON, &attachments); err != nil {
				return nil, fmt.Errorf("decode appdev chat attachments: %w", err)
			}
		}
		messages = append(messages, &appdevapp.ChatMessage{
			ID:          record.ID,
			Type:        record.MessageType,
			Role:        record.Role,
			Title:       record.Title,
			Content:     record.Content,
			Attachments: attachments,
			CreatedAt:   record.CreatedAt,
		})
	}
	return messages, nil
}

func (p *PersistentChatRepository) Status(ctx context.Context, req *appdevapp.ChatManagerRequest) (*appdevapp.ChatStatus, error) {
	if p == nil || p.db == nil || req == nil {
		return nil, fmt.Errorf("persistent appdev chat is not configured")
	}
	spaceID, err := persistentChatSpaceID(req.SpaceID)
	if err != nil {
		return nil, err
	}
	var record appDevChatSessionRecord
	if err := p.db.WithContext(ctx).
		Where("space_id = ? AND project_id = ?", spaceID, strings.TrimSpace(req.ProjectID)).
		Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &appdevapp.ChatStatus{Running: false}, nil
		}
		return nil, err
	}
	if record.Running && time.Since(record.UpdatedAt) > appDevChatLeaseTimeout {
		result := p.db.WithContext(ctx).Model(&appDevChatSessionRecord{}).
			Where(
				"space_id = ? AND project_id = ? AND request_id = ? AND running = ?",
				record.SpaceID,
				record.ProjectID,
				record.RequestID,
				true,
			).
			Updates(map[string]any{"running": false, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			record.Running = false
		}
	}
	return &appdevapp.ChatStatus{
		Running:   record.Running,
		SessionID: record.SessionID,
		RequestID: record.RequestID,
	}, nil
}

func beginChatStatus(tx *gorm.DB, spaceID int64, projectID string, status *appdevapp.ChatStatus) error {
	projectID = strings.TrimSpace(projectID)
	var existing appDevChatSessionRecord
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("space_id = ? AND project_id = ?", spaceID, projectID).
		Take(&existing).Error
	if err == nil {
		stale := existing.Running && time.Since(existing.UpdatedAt) > appDevChatLeaseTimeout
		if existing.Running && !stale {
			return fmt.Errorf("appdev chat task is already running")
		}
		result := tx.Model(&appDevChatSessionRecord{}).
			Where("space_id = ? AND project_id = ? AND request_id = ?", spaceID, projectID, existing.RequestID).
			Updates(map[string]any{
				"session_id": status.SessionID,
				"request_id": status.RequestID,
				"running":    true,
				"updated_at": time.Now().UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("appdev chat task is already running")
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	record := &appDevChatSessionRecord{
		SpaceID:   spaceID,
		ProjectID: projectID,
		SessionID: status.SessionID,
		RequestID: status.RequestID,
		Running:   status.Running,
		UpdatedAt: time.Now().UTC(),
	}
	if err := tx.Create(record).Error; err != nil {
		return fmt.Errorf("appdev chat task is already running: %w", err)
	}
	return nil
}

func finishChatStatus(tx *gorm.DB, spaceID int64, projectID string, status *appdevapp.ChatStatus) error {
	result := tx.Model(&appDevChatSessionRecord{}).
		Where(
			"space_id = ? AND project_id = ? AND request_id = ? AND running = ?",
			spaceID,
			strings.TrimSpace(projectID),
			status.RequestID,
			true,
		).
		Updates(map[string]any{
			"session_id": status.SessionID,
			"running":    status.Running,
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("cannot finish stale appdev chat request")
	}
	return nil
}

func chatMessageRecord(
	spaceID int64,
	projectID string,
	sessionID string,
	requestID string,
	message *appdevapp.ChatMessage,
) (*appDevChatMessageRecord, error) {
	attachments, err := json.Marshal(message.Attachments)
	if err != nil {
		return nil, err
	}
	createdAt := message.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return &appDevChatMessageRecord{
		ID:              message.ID,
		SpaceID:         spaceID,
		ProjectID:       strings.TrimSpace(projectID),
		SessionID:       sessionID,
		RequestID:       requestID,
		MessageType:     message.Type,
		Role:            message.Role,
		Title:           message.Title,
		Content:         message.Content,
		AttachmentsJSON: attachments,
		CreatedAt:       createdAt,
	}, nil
}

func persistentChatSpaceID(value string) (int64, error) {
	spaceID, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || spaceID <= 0 {
		return 0, fmt.Errorf("invalid appdev chat space_id")
	}
	return spaceID, nil
}
