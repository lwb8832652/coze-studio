// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"errors"
	"time"
)

const ChannelTypeFeishu = "feishu"

type ReplyMode string

const (
	ReplyModeFinal  ReplyMode = "final"
	ReplyModeStream ReplyMode = "stream"
)

type GroupPolicy string

const (
	GroupPolicyMentionOnly GroupPolicy = "mention_only"
	GroupPolicyDisabled    GroupPolicy = "disabled"
)

type RuntimeStatus string

const (
	RuntimeStatusDisabled     RuntimeStatus = "disabled"
	RuntimeStatusPending      RuntimeStatus = "pending"
	RuntimeStatusConnecting   RuntimeStatus = "connecting"
	RuntimeStatusConnected    RuntimeStatus = "connected"
	RuntimeStatusReconnecting RuntimeStatus = "reconnecting"
	RuntimeStatusError        RuntimeStatus = "error"
)

type EventStatus string

const (
	EventStatusPending    EventStatus = "pending"
	EventStatusProcessing EventStatus = "processing"
	EventStatusSucceeded  EventStatus = "succeeded"
	EventStatusFailed     EventStatus = "failed"
)

var (
	ErrUnauthenticated          = errors.New("IM channel authentication is required")
	ErrForbidden                = errors.New("IM channel workspace access is forbidden")
	ErrInvalidInput             = errors.New("IM channel input is invalid")
	ErrNotFound                 = errors.New("IM channel configuration was not found")
	ErrConflict                 = errors.New("IM channel configuration conflicts with an existing record")
	ErrCredentialCodecMissing   = errors.New("IM channel credential encryption is not configured")
	ErrAgentUnavailable         = errors.New("IM channel target agent is unavailable")
	ErrConnectionTestFailed     = errors.New("IM channel connection test failed")
	ErrRuntimeUnavailable       = errors.New("IM channel runtime is unavailable")
	ErrAgentExecutionFailed     = errors.New("IM channel agent execution failed")
	ErrAgentResponseUnavailable = errors.New("IM channel agent response is unavailable")
)

type Config struct {
	ID                    int64
	SpaceID               int64
	CreatorID             int64
	UpdatedBy             int64
	AgentID               int64
	ChannelType           string
	Name                  string
	AppID                 string
	AppSecretCiphertext   string
	AppSecretFingerprint  string
	Enabled               bool
	ReplyMode             ReplyMode
	GroupPolicy           GroupPolicy
	RuntimeStatus         RuntimeStatus
	RuntimeError          string
	BotOpenID             string
	BotName               string
	LastConnectedAt       *time.Time
	LastTestedAt          *time.Time
	RuntimeOwner          string
	RuntimeLeaseExpiresAt *time.Time
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

type Session struct {
	ID             int64
	ConfigID       int64
	SpaceID        int64
	ChatID         string
	ChatType       string
	ExternalUserID string
	ThreadID       int64
	LastMessageID  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Resource struct {
	Type     string `json:"type"`
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name,omitempty"`
}

type InboundPayload struct {
	EventID        string     `json:"event_id"`
	MessageID      string     `json:"message_id"`
	ChatID         string     `json:"chat_id"`
	ChatType       string     `json:"chat_type"`
	UserID         string     `json:"user_id"`
	Content        string     `json:"content"`
	RawContentType string     `json:"raw_content_type"`
	Resources      []Resource `json:"resources,omitempty"`
	CreateTimeMs   int64      `json:"create_time_ms"`
}

type Event struct {
	ID                   int64
	ConfigID             int64
	EventKey             string
	MessageID            string
	PayloadJSON          string
	Status               EventStatus
	AttemptCount         int32
	NextRetryAt          *time.Time
	ProcessingOwner      string
	ProcessingLeaseUntil *time.Time
	LastError            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CompletedAt          *time.Time
}

type RuntimeState struct {
	Status          RuntimeStatus
	Error           string
	BotOpenID       string
	BotName         string
	ConnectedAt     *time.Time
	ClearConnection bool
}

func ValidReplyMode(value ReplyMode) bool {
	return value == ReplyModeFinal || value == ReplyModeStream
}

func ValidGroupPolicy(value GroupPolicy) bool {
	return value == GroupPolicyMentionOnly || value == GroupPolicyDisabled
}
