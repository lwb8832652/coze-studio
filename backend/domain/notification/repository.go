// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

const maxEncodedCursorBytes = 4 * 1024

type OutboxClaim struct {
	OutboxID       int64
	LockedBy       string
	LeaseExpiresAt time.Time
	AttemptCount   int
	ClaimErrorCode string
	Event          Event
}

type Message struct {
	ID         int64
	EventID    string
	Scope      Scope
	SpaceID    int64
	SenderID   int64
	Category   Category
	Severity   Severity
	EventType  EventType
	Title      string
	Content    string
	TargetType TargetType
	TargetID   string
	CreatedAt  int64
}

type RecipientMessage struct {
	RecipientID int64
	SequenceNo  int64
	UserID      int64
	ReadAt      int64
	Message     Message
}

type Cursor struct {
	CreatedAt      int64 `json:"t"`
	SequenceNo     int64 `json:"q"`
	SnapshotCutoff int64 `json:"s"`
}

type ListFilter struct {
	UserID     int64
	Cursor     Cursor
	HasCursor  bool
	Limit      int
	UnreadOnly bool
}

type ListPage struct {
	Items          []RecipientMessage
	NextCursor     Cursor
	HasMore        bool
	SnapshotCutoff int64
}

func EncodeCursor(cursor Cursor) string {
	if cursor.CreatedAt <= 0 ||
		cursor.SequenceNo <= 0 ||
		cursor.SnapshotCutoff <= 0 {
		return ""
	}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeCursor(value string) (Cursor, error) {
	if value == "" || value == "0" {
		return Cursor{}, nil
	}
	if len(value) > maxEncodedCursorBytes {
		return Cursor{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var cursor Cursor
	if err := json.Unmarshal(raw, &cursor); err != nil ||
		cursor.CreatedAt <= 0 ||
		cursor.SequenceNo <= 0 ||
		cursor.SnapshotCutoff <= 0 {
		return Cursor{}, ErrInvalidCursor
	}
	return cursor, nil
}
