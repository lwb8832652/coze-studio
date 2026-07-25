// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/pkg/safetext"
)

const (
	CurrentPayloadSchema        = int32(1)
	MaxNotificationTitleRunes   = 128
	MaxNotificationContentRunes = 512
	MaxDisplayNameRunes         = 64
	MaxTargetIDRunes            = 128
	MaxExplicitRecipients       = 2000
)

var (
	ErrInvalidEvent               = errors.New("invalid notification event")
	ErrUnsafePayload              = errors.New("unsafe notification payload")
	ErrUnknownEventType           = errors.New("unknown notification event type")
	ErrInvalidCursor              = errors.New("invalid notification cursor")
	ErrIdempotencyConflict        = errors.New("notification idempotency conflict")
	ErrLeaseLost                  = errors.New("notification outbox lease lost")
	ErrRecipientResolution        = errors.New("notification recipient resolution failed")
	ErrRecipientPolicyUnavailable = errors.New("notification recipient policy unavailable")
	ErrStorage                    = errors.New("notification storage failure")
)

const (
	ErrorCodeInvalidEvent               = "invalid_event"
	ErrorCodeUnsafePayload              = "unsafe_payload"
	ErrorCodeUnknownEvent               = "unknown_event"
	ErrorCodeRecipientResolution        = "recipient_resolution"
	ErrorCodeIdempotencyConflict        = "idempotency_conflict"
	ErrorCodeLeaseLost                  = "lease_lost"
	ErrorCodeRecipientPolicyUnavailable = "recipient_policy_unavailable"
	ErrorCodeStorage                    = "storage"
)

type EventType string

const (
	EventAgentRunAwaitingInput       EventType = "agent_run.awaiting_input"
	EventAgentRunSucceeded           EventType = "agent_run.succeeded"
	EventAgentRunFailed              EventType = "agent_run.failed"
	EventAgentRunCanceled            EventType = "agent_run.canceled"
	EventTaskCompleted               EventType = "task.completed"
	EventTaskFailed                  EventType = "task.failed"
	EventTaskCancelled               EventType = "task.cancelled"
	EventTaskAwaitingInput           EventType = "task.awaiting_input"
	EventScheduledExecutionSucceeded EventType = "scheduled_execution.succeeded"
	EventScheduledExecutionFailed    EventType = "scheduled_execution.failed"
	EventScheduledExecutionCanceled  EventType = "scheduled_execution.canceled"
	EventAppDevBuildSucceeded        EventType = "appdev_build.succeeded"
	EventAppDevBuildFailed           EventType = "appdev_build.failed"
	EventAppDevDeploySucceeded       EventType = "appdev_deploy.succeeded"
	EventAppDevDeployFailed          EventType = "appdev_deploy.failed"
	EventMCPDeploymentSucceeded      EventType = "mcp_deployment.succeeded"
	EventMCPDeploymentFailed         EventType = "mcp_deployment.failed"
	EventMCPConnectionDegraded       EventType = "mcp_connection.degraded"
	EventMCPConnectionRecovered      EventType = "mcp_connection.recovered"
	EventSkillOperationSucceeded     EventType = "skill_operation.succeeded"
	EventSkillOperationFailed        EventType = "skill_operation.failed"
	EventPluginOperationSucceeded    EventType = "plugin_operation.succeeded"
	EventPluginOperationFailed       EventType = "plugin_operation.failed"
	EventResourceOperationSucceeded  EventType = "resource_operation.succeeded"
	EventResourceOperationFailed     EventType = "resource_operation.failed"
	EventWorkspaceMembershipChanged  EventType = "workspace.membership_changed"
	EventWorkspaceRoleChanged        EventType = "workspace.role_changed"
	EventIMChannelConnectionFailed   EventType = "im_channel.connection_failed"
	EventIMChannelRecovered          EventType = "im_channel.connection_recovered"
	EventIMMessageDeadLettered       EventType = "im_message.dead_lettered"
	EventBillingPaymentSucceeded     EventType = "billing.payment_succeeded"
	EventBillingPaymentFailed        EventType = "billing.payment_failed"
	EventBillingOrderTimedOut        EventType = "billing.order_timed_out"
	EventBillingSubscriptionActivated EventType = "billing.subscription_activated"
	EventBillingSubscriptionChanged  EventType = "billing.subscription_changed"
	EventBillingSubscriptionExpiring EventType = "billing.subscription_expiring"
	EventBillingSubscriptionExpired  EventType = "billing.subscription_expired"
	EventBillingCreditAdjusted       EventType = "billing.credit_adjusted"
	EventBillingCreditLow            EventType = "billing.credit_low"
	EventSystemAnnouncement          EventType = "system.announcement"
	EventSystemProviderUnavailable   EventType = "system.provider_unavailable"
	EventSystemProviderRecovered     EventType = "system.provider_recovered"
	EventSystemOutboxBacklog         EventType = "system.notification_backlog"
)

type RecipientPolicy string

const (
	RecipientActor                 RecipientPolicy = "actor"
	RecipientResourceOwner         RecipientPolicy = "resource_owner"
	RecipientWorkspaceOwnersAdmins RecipientPolicy = "workspace_owners_admins"
	RecipientWorkspaceMembers      RecipientPolicy = "workspace_members"
	RecipientSystemAdmins          RecipientPolicy = "system_admins"
	RecipientExplicitInternalUsers RecipientPolicy = "explicit_internal_users"
)

type Scope string

const (
	ScopePersonal  Scope = "personal"
	ScopeWorkspace Scope = "workspace"
	ScopeSystem    Scope = "system"
)

type Category string

const (
	CategoryTask          Category = "task"
	CategoryScheduledTask Category = "scheduled_task"
	CategoryAppDev        Category = "appdev"
	CategoryMCP           Category = "mcp"
	CategoryResource      Category = "resource"
	CategoryWorkspace     Category = "workspace"
	CategoryIM            Category = "im"
	CategoryBilling       Category = "billing"
	CategorySystem        Category = "system"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeveritySuccess Severity = "success"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type TargetType string

const (
	TargetNone                TargetType = "none"
	TargetTaskThread          TargetType = "task_thread"
	TargetScheduledTaskCenter TargetType = "scheduled_task_center"
	TargetAppDev              TargetType = "appdev"
	TargetSkill               TargetType = "skill"
	TargetWorkspace           TargetType = "workspace"
	TargetBilling             TargetType = "billing"
	TargetSystemAnnouncements TargetType = "system_announcements"
	// TargetInternalRoute is read-only compatibility for already persisted
	// notifications. New producers must use a structured target.
	TargetInternalRoute       TargetType = "internal_route"
)

type AnnouncementRouteType string

const (
	AnnouncementRouteNone                AnnouncementRouteType = "none"
	AnnouncementRouteWorkspaceHome       AnnouncementRouteType = "workspace_home"
	AnnouncementRouteSystemAnnouncements AnnouncementRouteType = "system_announcements"
)

type AnnouncementRoute struct {
	Type    AnnouncementRouteType `json:"type"`
	SpaceID int64                 `json:"space_id,omitempty"`
}

type WorkspaceMemberAction string

const (
	WorkspaceMemberAdded                WorkspaceMemberAction = "added"
	WorkspaceMemberRemoved              WorkspaceMemberAction = "removed"
	WorkspaceMemberRoleChanged          WorkspaceMemberAction = "role_changed"
	WorkspaceMemberOwnershipTransferred WorkspaceMemberAction = "ownership_transferred"
)

type WorkspaceMemberAudience string

const (
	WorkspaceMemberAudienceTarget WorkspaceMemberAudience = "target"
	WorkspaceMemberAudienceAdmins WorkspaceMemberAudience = "admins"
)

type StatusReasonCode string

const (
	StatusReasonNone                 StatusReasonCode = ""
	StatusReasonActionRequired       StatusReasonCode = "action_required"
	StatusReasonConfigurationInvalid StatusReasonCode = "configuration_invalid"
	StatusReasonPermissionDenied     StatusReasonCode = "permission_denied"
	StatusReasonQuotaInsufficient    StatusReasonCode = "quota_insufficient"
	StatusReasonProviderUnavailable  StatusReasonCode = "provider_unavailable"
	StatusReasonConnectionFailed     StatusReasonCode = "connection_failed"
	StatusReasonRetryExhausted       StatusReasonCode = "retry_exhausted"
	StatusReasonCanceledByUser       StatusReasonCode = "canceled_by_user"
)

type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxProcessing OutboxStatus = "processing"
	OutboxDelivered  OutboxStatus = "delivered"
	OutboxDead       OutboxStatus = "dead"
)

type EventPayload struct {
	ResourceDisplayName  string           `json:"resource_display_name,omitempty"`
	ActorDisplayName     string           `json:"actor_display_name,omitempty"`
	StatusReasonCode     StatusReasonCode `json:"status_reason_code,omitempty"`
	TargetID             string           `json:"target_id,omitempty"`
	AnnouncementTitle    string                  `json:"announcement_title,omitempty"`
	AnnouncementBody     string                  `json:"announcement_body,omitempty"`
	AnnouncementSeverity Severity                `json:"announcement_severity,omitempty"`
	AnnouncementRoute    *AnnouncementRoute      `json:"announcement_route,omitempty"`
	WorkspaceAction      WorkspaceMemberAction   `json:"workspace_action,omitempty"`
	WorkspaceAudience    WorkspaceMemberAudience `json:"workspace_audience,omitempty"`
	SubjectDisplayName   string                  `json:"subject_display_name,omitempty"`
	ExplicitRecipientIDs []int64                 `json:"explicit_recipient_ids,omitempty"`
}

type Event struct {
	EventID          string
	EventType        EventType
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	OccurredAt       time.Time
	ActorID          int64
	SpaceID          int64
	RecipientPolicy  RecipientPolicy
	PayloadSchema    int32
	Payload          EventPayload
}

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]*$`)

func (e Event) Validate() error {
	if !validIdentifier(e.EventID, 128) {
		return fmt.Errorf("%w: event ID is invalid", ErrInvalidEvent)
	}
	if !validIdentifier(string(e.EventType), 64) {
		return fmt.Errorf("%w: event type is invalid", ErrInvalidEvent)
	}
	if !validIdentifier(e.AggregateType, 64) ||
		!validIdentifier(e.AggregateID, 128) ||
		e.AggregateVersion <= 0 {
		return fmt.Errorf("%w: aggregate identity is invalid", ErrInvalidEvent)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurred time is required", ErrInvalidEvent)
	}
	if e.ActorID < 0 || e.SpaceID < 0 {
		return fmt.Errorf("%w: actor and space IDs cannot be negative", ErrInvalidEvent)
	}
	if !e.RecipientPolicy.valid() {
		return fmt.Errorf("%w: recipient policy is invalid", ErrInvalidEvent)
	}
	if e.RecipientPolicy == RecipientActor && e.ActorID <= 0 {
		return fmt.Errorf("%w: actor recipient requires actor ID", ErrInvalidEvent)
	}
	if e.PayloadSchema != CurrentPayloadSchema {
		return fmt.Errorf("%w: unsupported payload schema", ErrInvalidEvent)
	}
	if err := validateDisplayName(e.Payload.ResourceDisplayName); err != nil {
		return err
	}
	if err := validateDisplayName(e.Payload.ActorDisplayName); err != nil {
		return err
	}
	if err := validateDisplayName(e.Payload.SubjectDisplayName); err != nil {
		return err
	}
	if !e.Payload.StatusReasonCode.valid() {
		return fmt.Errorf("%w: status reason code is invalid", ErrInvalidEvent)
	}
	if e.Payload.TargetID != "" && !validIdentifier(e.Payload.TargetID, MaxTargetIDRunes) {
		return fmt.Errorf("%w: target ID is invalid", ErrInvalidEvent)
	}
	if err := validateAnnouncementPayload(e); err != nil {
		return err
	}
	if err := validateWorkspacePayload(e); err != nil {
		return err
	}
	if e.RecipientPolicy != RecipientExplicitInternalUsers &&
		len(e.Payload.ExplicitRecipientIDs) > 0 {
		return fmt.Errorf("%w: explicit recipients require the internal policy", ErrInvalidEvent)
	}
	if e.RecipientPolicy == RecipientExplicitInternalUsers {
		if err := ValidateRecipientIDs(e.Payload.ExplicitRecipientIDs); err != nil {
			return err
		}
	}
	return nil
}

func (e Event) IdempotencyKey() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		string(e.EventType),
		e.AggregateType,
		e.AggregateID,
		fmt.Sprintf("%d", e.AggregateVersion),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func CanonicalizeEvent(event Event) (Event, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.EventType = EventType(strings.TrimSpace(string(event.EventType)))
	event.AggregateType = strings.TrimSpace(event.AggregateType)
	event.AggregateID = strings.TrimSpace(event.AggregateID)
	event.RecipientPolicy = RecipientPolicy(
		strings.ToLower(strings.TrimSpace(string(event.RecipientPolicy))),
	)
	event.Payload.ResourceDisplayName = strings.TrimSpace(
		event.Payload.ResourceDisplayName,
	)
	event.Payload.ActorDisplayName = strings.TrimSpace(
		event.Payload.ActorDisplayName,
	)
	event.Payload.StatusReasonCode = StatusReasonCode(
		strings.ToLower(strings.TrimSpace(string(event.Payload.StatusReasonCode))),
	)
	event.Payload.TargetID = strings.TrimSpace(event.Payload.TargetID)
	event.Payload.AnnouncementTitle = strings.TrimSpace(
		event.Payload.AnnouncementTitle,
	)
	event.Payload.AnnouncementBody = strings.TrimSpace(
		event.Payload.AnnouncementBody,
	)
	event.Payload.AnnouncementSeverity = Severity(
		strings.ToLower(strings.TrimSpace(
			string(event.Payload.AnnouncementSeverity),
		)),
	)
	if event.Payload.AnnouncementRoute != nil {
		event.Payload.AnnouncementRoute.Type = AnnouncementRouteType(
			strings.ToLower(strings.TrimSpace(
				string(event.Payload.AnnouncementRoute.Type),
			)),
		)
	}
	event.Payload.WorkspaceAction = WorkspaceMemberAction(
		strings.ToLower(strings.TrimSpace(
			string(event.Payload.WorkspaceAction),
		)),
	)
	event.Payload.WorkspaceAudience = WorkspaceMemberAudience(
		strings.ToLower(strings.TrimSpace(
			string(event.Payload.WorkspaceAudience),
		)),
	)
	event.Payload.SubjectDisplayName = strings.TrimSpace(
		event.Payload.SubjectDisplayName,
	)
	if len(event.Payload.ExplicitRecipientIDs) > 0 {
		recipients, err := canonicalRecipientIDs(
			event.Payload.ExplicitRecipientIDs,
			ErrInvalidEvent,
		)
		if err != nil {
			return Event{}, err
		}
		event.Payload.ExplicitRecipientIDs = recipients
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func ValidateRecipientIDs(ids []int64) error {
	if len(ids) == 0 || len(ids) > MaxExplicitRecipients {
		return fmt.Errorf("%w: recipient count is invalid", ErrInvalidEvent)
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("%w: recipient ID must be positive", ErrInvalidEvent)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: duplicate recipient ID", ErrInvalidEvent)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func NormalizeRecipientIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > MaxExplicitRecipients {
		return nil, fmt.Errorf("%w: recipient count is invalid", ErrRecipientResolution)
	}
	return canonicalRecipientIDs(ids, ErrRecipientResolution)
}

func canonicalRecipientIDs(ids []int64, baseError error) ([]int64, error) {
	if len(ids) == 0 || len(ids) > MaxExplicitRecipients {
		return nil, fmt.Errorf("%w: recipient count is invalid", baseError)
	}
	result := append([]int64(nil), ids...)
	for _, id := range result {
		if id <= 0 {
			return nil, fmt.Errorf("%w: recipient ID is invalid", baseError)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left] < result[right]
	})
	writeIndex := 1
	for readIndex := 1; readIndex < len(result); readIndex++ {
		if result[readIndex] == result[writeIndex-1] {
			continue
		}
		result[writeIndex] = result[readIndex]
		writeIndex++
	}
	return result[:writeIndex], nil
}

func StableErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrUnsafePayload):
		return ErrorCodeUnsafePayload
	case errors.Is(err, ErrUnknownEventType):
		return ErrorCodeUnknownEvent
	case errors.Is(err, ErrInvalidEvent), errors.Is(err, ErrInvalidCursor):
		return ErrorCodeInvalidEvent
	case errors.Is(err, ErrRecipientResolution):
		return ErrorCodeRecipientResolution
	case errors.Is(err, ErrRecipientPolicyUnavailable):
		return ErrorCodeRecipientPolicyUnavailable
	case errors.Is(err, ErrIdempotencyConflict):
		return ErrorCodeIdempotencyConflict
	case errors.Is(err, ErrLeaseLost):
		return ErrorCodeLeaseLost
	default:
		return ErrorCodeStorage
	}
}

func IsRetryableErrorCode(code string) bool {
	switch code {
	case ErrorCodeStorage,
		ErrorCodeLeaseLost,
		ErrorCodeRecipientResolution:
		return true
	default:
		return false
	}
}

func validIdentifier(value string, maxRunes int) bool {
	value = strings.TrimSpace(value)
	return value != "" &&
		utf8.RuneCountInString(value) <= maxRunes &&
		safeIdentifier.MatchString(value)
}

func validateDisplayName(value string) error {
	_, err := safetext.Normalize(value, safetext.Rules{
		MaxRunes: MaxDisplayNameRunes,
	})
	if errors.Is(err, safetext.ErrUnsafe) {
		return ErrUnsafePayload
	}
	if err != nil {
		return fmt.Errorf("%w: display name is invalid", ErrInvalidEvent)
	}
	return nil
}

func validateAnnouncementPayload(event Event) error {
	payload := event.Payload
	hasAnnouncementPayload := payload.AnnouncementTitle != "" ||
		payload.AnnouncementBody != "" ||
		payload.AnnouncementSeverity != "" ||
		payload.AnnouncementRoute != nil
	if !hasAnnouncementPayload {
		return nil
	}
	if event.EventType != EventSystemAnnouncement {
		return fmt.Errorf(
			"%w: announcement payload is restricted to system announcements",
			ErrInvalidEvent,
		)
	}
	if err := validateAnnouncementText(
		payload.AnnouncementTitle,
		MaxNotificationTitleRunes,
		false,
	); err != nil {
		return err
	}
	if err := validateAnnouncementText(
		payload.AnnouncementBody,
		MaxNotificationContentRunes,
		true,
	); err != nil {
		return err
	}
	if !payload.AnnouncementSeverity.valid() {
		return fmt.Errorf("%w: announcement severity is invalid", ErrInvalidEvent)
	}
	if payload.AnnouncementRoute == nil {
		return fmt.Errorf("%w: announcement route is required", ErrInvalidEvent)
	}
	if _, err := NormalizeAnnouncementRoute(*payload.AnnouncementRoute); err != nil {
		return err
	}
	return nil
}

func validateAnnouncementText(value string, maxRunes int, multiline bool) error {
	_, err := safetext.Normalize(value, safetext.Rules{
		MaxRunes:  maxRunes,
		Multiline: multiline,
		Required:  true,
	})
	if errors.Is(err, safetext.ErrUnsafe) {
		return ErrUnsafePayload
	}
	if err != nil {
		return fmt.Errorf("%w: announcement text is invalid", ErrInvalidEvent)
	}
	return nil
}

func NormalizeAnnouncementRoute(route AnnouncementRoute) (AnnouncementRoute, error) {
	route.Type = AnnouncementRouteType(
		strings.ToLower(strings.TrimSpace(string(route.Type))),
	)
	if route.Type == "" {
		route.Type = AnnouncementRouteNone
	}
	switch route.Type {
	case AnnouncementRouteNone, AnnouncementRouteSystemAnnouncements:
		if route.SpaceID != 0 {
			return AnnouncementRoute{}, fmt.Errorf(
				"%w: announcement route has unexpected workspace",
				ErrInvalidEvent,
			)
		}
	case AnnouncementRouteWorkspaceHome:
		if route.SpaceID <= 0 {
			return AnnouncementRoute{}, fmt.Errorf(
				"%w: announcement workspace route is invalid",
				ErrInvalidEvent,
			)
		}
	default:
		return AnnouncementRoute{}, fmt.Errorf(
			"%w: announcement route type is invalid",
			ErrInvalidEvent,
		)
	}
	return route, nil
}

func validateWorkspacePayload(event Event) error {
	payload := event.Payload
	hasWorkspacePayload := payload.WorkspaceAction != "" ||
		payload.WorkspaceAudience != "" ||
		payload.SubjectDisplayName != ""
	isWorkspaceEvent := event.EventType == EventWorkspaceMembershipChanged ||
		event.EventType == EventWorkspaceRoleChanged
	if !hasWorkspacePayload {
		if isWorkspaceEvent {
			return fmt.Errorf(
				"%w: workspace notification facts are required",
				ErrInvalidEvent,
			)
		}
		return nil
	}
	if !isWorkspaceEvent ||
		!payload.WorkspaceAction.valid() ||
		!payload.WorkspaceAudience.valid() ||
		strings.TrimSpace(payload.SubjectDisplayName) == "" {
		return fmt.Errorf(
			"%w: workspace notification facts are invalid",
			ErrInvalidEvent,
		)
	}
	return nil
}

func (p RecipientPolicy) valid() bool {
	switch p {
	case RecipientActor,
		RecipientResourceOwner,
		RecipientWorkspaceOwnersAdmins,
		RecipientWorkspaceMembers,
		RecipientSystemAdmins,
		RecipientExplicitInternalUsers:
		return true
	default:
		return false
	}
}

func (c StatusReasonCode) valid() bool {
	switch c {
	case StatusReasonNone,
		StatusReasonActionRequired,
		StatusReasonConfigurationInvalid,
		StatusReasonPermissionDenied,
		StatusReasonQuotaInsufficient,
		StatusReasonProviderUnavailable,
		StatusReasonConnectionFailed,
		StatusReasonRetryExhausted,
		StatusReasonCanceledByUser:
		return true
	default:
		return false
	}
}

func (s Severity) valid() bool {
	switch s {
	case SeverityInfo, SeveritySuccess, SeverityWarning, SeverityError:
		return true
	default:
		return false
	}
}

func (a WorkspaceMemberAction) valid() bool {
	switch a {
	case WorkspaceMemberAdded,
		WorkspaceMemberRemoved,
		WorkspaceMemberRoleChanged,
		WorkspaceMemberOwnershipTransferred:
		return true
	default:
		return false
	}
}

func (a WorkspaceMemberAudience) valid() bool {
	switch a {
	case WorkspaceMemberAudienceTarget, WorkspaceMemberAudienceAdmins:
		return true
	default:
		return false
	}
}
