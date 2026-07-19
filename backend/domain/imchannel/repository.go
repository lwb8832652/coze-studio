// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"time"
)

type Repository interface {
	ListConfigs(ctx context.Context, spaceID int64) ([]*Config, error)
	GetConfig(ctx context.Context, spaceID, configID int64) (*Config, error)
	GetConfigByID(ctx context.Context, configID int64) (*Config, error)
	CreateConfig(ctx context.Context, config *Config) error
	UpdateConfig(ctx context.Context, config *Config) error
	SetEnabled(ctx context.Context, spaceID, configID, actorID int64, enabled bool, status RuntimeStatus) error
	DeleteConfig(ctx context.Context, spaceID, configID, actorID int64) error
	MarkConnectionTest(ctx context.Context, configID int64, botOpenID, botName string, testedAt time.Time) error

	ListEnabledConfigs(ctx context.Context) ([]*Config, error)
	TryAcquireRuntimeLease(ctx context.Context, configID int64, owner string, now, expiresAt time.Time) (bool, error)
	RenewRuntimeLease(ctx context.Context, configID int64, owner string, expiresAt time.Time) (bool, error)
	ReleaseRuntimeLease(ctx context.Context, configID int64, owner string) error
	UpdateRuntimeState(ctx context.Context, configID int64, state RuntimeState) error

	InsertEvent(ctx context.Context, event *Event) (bool, error)
	ClaimEvents(ctx context.Context, configID int64, owner string, now, leaseUntil time.Time, limit int) ([]*Event, error)
	CompleteEvent(ctx context.Context, eventID int64, completedAt time.Time) error
	FailEvent(ctx context.Context, eventID int64, message string, nextRetryAt time.Time) error
	CleanupEvents(ctx context.Context, completedBefore time.Time) error

	GetSession(ctx context.Context, configID int64, chatID string) (*Session, error)
	SaveSession(ctx context.Context, session *Session) error
}
