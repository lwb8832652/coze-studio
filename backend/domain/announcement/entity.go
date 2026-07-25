// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/pkg/safetext"
)

const (
	MaxTitleRunes      = 128
	MaxBodyRunes       = 512
	MaxAudienceTargets = 1000
	MaxPageSize        = 100
	MaxAuditPageSize   = 100
	MaxScheduleHorizon = 366 * 24 * time.Hour
)

var (
	ErrPermissionDenied       = errors.New("announcement permission denied")
	ErrInvalidInput           = errors.New("invalid announcement input")
	ErrUnsafeContent          = errors.New("unsafe announcement content")
	ErrNotFound               = errors.New("announcement not found")
	ErrAudienceTargetNotFound = errors.New("announcement audience target not found")
	ErrAudienceTooLarge       = errors.New("announcement audience too large")
	ErrStateConflict          = errors.New("announcement state conflict")
	ErrVersionConflict        = errors.New("announcement version conflict")
	ErrIdempotencyConflict    = errors.New("announcement idempotency conflict")
	ErrStorage                = errors.New("announcement storage failure")
)

const (
	ErrorCodePermissionDenied       = "permission_denied"
	ErrorCodeInvalidInput           = "invalid_input"
	ErrorCodeUnsafeContent          = "unsafe_content"
	ErrorCodeNotFound               = "not_found"
	ErrorCodeAudienceTargetNotFound = "audience_target_not_found"
	ErrorCodeAudienceTooLarge       = "audience_too_large"
	ErrorCodeStateConflict          = "state_conflict"
	ErrorCodeVersionConflict        = "version_conflict"
	ErrorCodeIdempotencyConflict    = "idempotency_conflict"
	ErrorCodeStorage                = "storage"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusScheduled Status = "scheduled"
	StatusPublished Status = "published"
	StatusCancelled Status = "cancelled"
)

type ProjectionStatus string

const (
	ProjectionIdle         ProjectionStatus = "idle"
	ProjectionSnapshotting ProjectionStatus = "snapshotting"
	ProjectionProjecting   ProjectionStatus = "projecting"
	ProjectionFailed       ProjectionStatus = "failed"
	ProjectionCompleted    ProjectionStatus = "completed"
)

type AudienceType string

const (
	AudienceAll        AudienceType = "all"
	AudienceUsers      AudienceType = "users"
	AudienceWorkspaces AudienceType = "workspaces"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeveritySuccess Severity = "success"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Audience struct {
	Type      AudienceType
	TargetIDs []int64
}

type Draft struct {
	Title    string
	Body     string
	Severity Severity
	Route    domainnotification.AnnouncementRoute
	Audience Audience
}

type Announcement struct {
	ID                 int64
	Draft              Draft
	Status             Status
	ProjectionStatus   ProjectionStatus
	ScheduledAt        int64
	PublishRequestedAt int64
	SnapshotAt         int64
	PublishedAt        int64
	CancelledAt        int64
	CreatedBy          int64
	UpdatedBy          int64
	PublishActorID     int64
	RecipientCount     int64
	ProjectedCount     int64
	LastErrorCode      string
	Version            int64
	CreatedAt          int64
	UpdatedAt          int64
}

type AuditEvent struct {
	ID               int64
	AnnouncementID   int64
	ActorID           int64
	Action            string
	FromStatus        Status
	ToStatus          Status
	ProjectionStatus  ProjectionStatus
	Result            string
	ErrorCode         string
	RecipientCount    int64
	ProjectedCount    int64
	CreatedAt         int64
}

type ListFilter struct {
	Status Status
	Offset int
	Limit  int
}

var idempotencyKeyPattern = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9._:-]{7,63}$`,
)

func NormalizeDraft(input Draft) (Draft, error) {
	title, err := normalizeText(input.Title, MaxTitleRunes, false)
	if err != nil {
		return Draft{}, fmt.Errorf("%w: title", err)
	}
	body, err := normalizeText(input.Body, MaxBodyRunes, true)
	if err != nil {
		return Draft{}, fmt.Errorf("%w: body", err)
	}
	severity := Severity(strings.ToLower(strings.TrimSpace(string(input.Severity))))
	if !severity.valid() {
		return Draft{}, fmt.Errorf("%w: severity", ErrInvalidInput)
	}
	route, err := domainnotification.NormalizeAnnouncementRoute(input.Route)
	if err != nil {
		return Draft{}, fmt.Errorf("%w: route", ErrInvalidInput)
	}
	audience, err := NormalizeAudience(input.Audience)
	if err != nil {
		return Draft{}, err
	}
	return Draft{
		Title:    title,
		Body:     body,
		Severity: severity,
		Route:    route,
		Audience: audience,
	}, nil
}

func NormalizeAudience(input Audience) (Audience, error) {
	audienceType := AudienceType(
		strings.ToLower(strings.TrimSpace(string(input.Type))),
	)
	if !audienceType.valid() {
		return Audience{}, fmt.Errorf("%w: audience type", ErrInvalidInput)
	}
	targets := append([]int64(nil), input.TargetIDs...)
	for _, targetID := range targets {
		if targetID <= 0 {
			return Audience{}, fmt.Errorf("%w: audience target", ErrInvalidInput)
		}
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left] < targets[right]
	})
	writeIndex := 0
	for _, targetID := range targets {
		if writeIndex > 0 && targets[writeIndex-1] == targetID {
			continue
		}
		targets[writeIndex] = targetID
		writeIndex++
	}
	targets = targets[:writeIndex]
	if audienceType == AudienceAll {
		if len(targets) != 0 {
			return Audience{}, fmt.Errorf(
				"%w: all-users audience cannot contain targets",
				ErrInvalidInput,
			)
		}
		return Audience{Type: audienceType}, nil
	}
	if len(targets) == 0 || len(targets) > MaxAudienceTargets {
		return Audience{}, fmt.Errorf(
			"%w: audience target count",
			ErrInvalidInput,
		)
	}
	return Audience{Type: audienceType, TargetIDs: targets}, nil
}

func NormalizeIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !idempotencyKeyPattern.MatchString(value) {
		return "", fmt.Errorf("%w: idempotency key", ErrInvalidInput)
	}
	return value, nil
}

func ValidateScheduleTime(now time.Time, scheduledAt time.Time) error {
	if now.IsZero() || scheduledAt.IsZero() {
		return fmt.Errorf("%w: schedule time", ErrInvalidInput)
	}
	if !scheduledAt.After(now) ||
		scheduledAt.After(now.Add(MaxScheduleHorizon)) {
		return fmt.Errorf("%w: schedule time", ErrInvalidInput)
	}
	return nil
}

func StableErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPermissionDenied):
		return ErrorCodePermissionDenied
	case errors.Is(err, ErrUnsafeContent):
		return ErrorCodeUnsafeContent
	case errors.Is(err, ErrInvalidInput):
		return ErrorCodeInvalidInput
	case errors.Is(err, ErrAudienceTargetNotFound):
		return ErrorCodeAudienceTargetNotFound
	case errors.Is(err, ErrAudienceTooLarge):
		return ErrorCodeAudienceTooLarge
	case errors.Is(err, ErrNotFound):
		return ErrorCodeNotFound
	case errors.Is(err, ErrStateConflict):
		return ErrorCodeStateConflict
	case errors.Is(err, ErrVersionConflict):
		return ErrorCodeVersionConflict
	case errors.Is(err, ErrIdempotencyConflict):
		return ErrorCodeIdempotencyConflict
	default:
		return ErrorCodeStorage
	}
}

func normalizeText(value string, maxRunes int, multiline bool) (string, error) {
	value, err := safetext.Normalize(value, safetext.Rules{
		MaxRunes:  maxRunes,
		Multiline: multiline,
		Required:  true,
	})
	if errors.Is(err, safetext.ErrUnsafe) {
		return "", ErrUnsafeContent
	}
	if err != nil {
		return "", ErrInvalidInput
	}
	return value, nil
}

func (value Status) Valid() bool {
	switch value {
	case StatusDraft, StatusScheduled, StatusPublished, StatusCancelled:
		return true
	default:
		return false
	}
}

func (value ProjectionStatus) Valid() bool {
	switch value {
	case ProjectionIdle,
		ProjectionSnapshotting,
		ProjectionProjecting,
		ProjectionFailed,
		ProjectionCompleted:
		return true
	default:
		return false
	}
}

func (value AudienceType) valid() bool {
	switch value {
	case AudienceAll, AudienceUsers, AudienceWorkspaces:
		return true
	default:
		return false
	}
}

func (value Severity) valid() bool {
	switch value {
	case SeverityInfo, SeveritySuccess, SeverityWarning, SeverityError:
		return true
	default:
		return false
	}
}
