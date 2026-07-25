// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"context"
	"time"
)

type CreateCommand struct {
	ActorID       int64
	IdempotencyKey string
	RequestHash   string
	Draft         Draft
	Now           time.Time
}

type UpdateCommand struct {
	AnnouncementID int64
	ActorID         int64
	ExpectedVersion int64
	Draft           Draft
	Now             time.Time
}

type ScheduleCommand struct {
	AnnouncementID int64
	ActorID         int64
	ExpectedVersion int64
	ScheduledAt     time.Time
	Now             time.Time
}

type PublishCommand struct {
	AnnouncementID int64
	ActorID         int64
	ExpectedVersion int64
	IdempotencyKey  string
	RequestHash     string
	Now             time.Time
}

type CancelCommand struct {
	AnnouncementID int64
	ActorID         int64
	ExpectedVersion int64
	Now             time.Time
}

type MutationResult struct {
	Announcement *Announcement
	Replayed     bool
}

type AdvanceResult struct {
	Announcement *Announcement
	Done         bool
	Progressed   bool
}

type Repository interface {
	Create(context.Context, CreateCommand) (*MutationResult, error)
	Update(context.Context, UpdateCommand) (*Announcement, error)
	Schedule(context.Context, ScheduleCommand) (*Announcement, error)
	RequestPublish(context.Context, PublishCommand) (*MutationResult, error)
	Cancel(context.Context, CancelCommand) (*Announcement, error)
	Get(context.Context, int64) (*Announcement, error)
	List(context.Context, ListFilter) ([]*Announcement, int64, error)
	ListAuditEvents(context.Context, int64, int, int) ([]*AuditEvent, int64, error)
	ListReplayCandidates(context.Context, time.Time, int64, int) ([]int64, error)
	AdvancePublication(context.Context, int64, int64, time.Time, int) (*AdvanceResult, error)
	MarkProjectionFailed(context.Context, int64, int64, string, time.Time) (*Announcement, error)
}
