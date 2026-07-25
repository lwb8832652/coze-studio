// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventValidationAndIdempotency(t *testing.T) {
	event := validTestEvent()

	require.NoError(t, event.Validate())
	require.Equal(t, event.IdempotencyKey(), event.IdempotencyKey())

	changed := event
	changed.AggregateVersion++
	require.NotEqual(t, event.IdempotencyKey(), changed.IdempotencyKey())
}

func TestEventValidationRejectsUnsafePayloadAndUnknownTemplate(t *testing.T) {
	registry := DefaultTemplateRegistry()

	event := validTestEvent()
	event.Payload.ResourceDisplayName = "provider_body=https://object.example/private"
	require.ErrorIs(t, event.Validate(), ErrUnsafePayload)

	event = validTestEvent()
	event.EventType = "unknown.event"
	_, err := registry.Render(event)
	require.ErrorIs(t, err, ErrUnknownEventType)

	event = validTestEvent()
	event.Payload.ActorDisplayName = strings.Repeat("a", MaxDisplayNameRunes+1)
	require.ErrorIs(t, event.Validate(), ErrInvalidEvent)

	event = validTestEvent()
	event.Payload.StatusReasonCode = "raw_provider_failure"
	require.ErrorIs(t, event.Validate(), ErrInvalidEvent)
}

func TestSystemAnnouncementContractAllowsOrdinaryRuntimeTerms(t *testing.T) {
	event := validTestEvent()
	event.EventID = "announcement-100-batch-1"
	event.EventType = EventSystemAnnouncement
	event.AggregateType = "announcement"
	event.AggregateID = "100"
	event.ActorID = 42
	event.SpaceID = 0
	event.RecipientPolicy = RecipientExplicitInternalUsers
	event.Payload = EventPayload{
		AnnouncementTitle:    "Prompt 配置维护",
		AnnouncementBody:     "checkpoint 状态将在维护结束后继续保留。",
		AnnouncementSeverity: SeverityInfo,
		AnnouncementRoute: &AnnouncementRoute{
			Type: AnnouncementRouteSystemAnnouncements,
		},
		ExplicitRecipientIDs: []int64{101},
	}

	require.NoError(t, DefaultTemplateRegistry().ValidateAppendable(event))
	draft, err := DefaultTemplateRegistry().Render(event)
	require.NoError(t, err)
	require.Equal(t, TargetSystemAnnouncements, draft.TargetType)
	require.Empty(t, draft.TargetID)
}

func TestWorkspaceMemberTemplatesAreAudienceSpecificAndRemovedTargetHasNoRoute(
	t *testing.T,
) {
	target := validTestEvent()
	target.EventID = "workspace-member:removed:target:202:101:1"
	target.EventType = EventWorkspaceMembershipChanged
	target.AggregateType = "workspace_member.removed"
	target.AggregateID = "202:101"
	target.SpaceID = 202
	target.RecipientPolicy = RecipientExplicitInternalUsers
	target.Payload = EventPayload{
		ResourceDisplayName:  "研发空间",
		WorkspaceAction:      WorkspaceMemberRemoved,
		WorkspaceAudience:    WorkspaceMemberAudienceTarget,
		SubjectDisplayName:   "成员 101",
		ExplicitRecipientIDs: []int64{101},
	}

	targetDraft, err := DefaultTemplateRegistry().Render(target)
	require.NoError(t, err)
	require.Equal(t, TargetNone, targetDraft.TargetType)
	require.Contains(t, targetDraft.Content, "你已被移出")

	admins := target
	admins.EventID = "workspace-member:removed:admins:202:101:1"
	admins.Payload.WorkspaceAudience = WorkspaceMemberAudienceAdmins
	admins.Payload.ExplicitRecipientIDs = []int64{202}
	adminDraft, err := DefaultTemplateRegistry().Render(admins)
	require.NoError(t, err)
	require.Equal(t, TargetWorkspace, adminDraft.TargetType)
	require.Contains(t, adminDraft.Content, "成员「成员 101」已移出")
	require.NotContains(t, adminDraft.Content, "你的关系")
}

func TestEventValidationRejectsClientShapedRecipientData(t *testing.T) {
	event := validTestEvent()
	event.RecipientPolicy = RecipientWorkspaceMembers
	event.Payload.ExplicitRecipientIDs = []int64{101}
	require.ErrorIs(t, event.Validate(), ErrInvalidEvent)

	event = validTestEvent()
	event.RecipientPolicy = RecipientExplicitInternalUsers
	event.Payload.ExplicitRecipientIDs = []int64{101, 101}
	require.ErrorIs(t, event.Validate(), ErrInvalidEvent)
}

func TestCursorRoundTripAndInvalidInput(t *testing.T) {
	cursor := Cursor{
		CreatedAt:      1_721_000_000_123,
		SequenceNo:     9001,
		SnapshotCutoff: 9900,
	}
	encoded := EncodeCursor(cursor)

	decoded, err := DecodeCursor(encoded)
	require.NoError(t, err)
	require.Equal(t, cursor, decoded)

	_, err = DecodeCursor("not-a-valid-cursor")
	require.ErrorIs(t, err, ErrInvalidCursor)

	_, err = DecodeCursor(strings.Repeat("A", maxEncodedCursorBytes+1))
	require.ErrorIs(t, err, ErrInvalidCursor)
}

func TestTemplateRendersOnlyControlledFields(t *testing.T) {
	event := validTestEvent()
	event.Payload.ResourceDisplayName = "季度报告"
	event.Payload.ActorDisplayName = "刘文波"
	event.Payload.StatusReasonCode = StatusReasonConfigurationInvalid
	event.Payload.TargetID = "thread-100"

	draft, err := DefaultTemplateRegistry().Render(event)
	require.NoError(t, err)
	require.Equal(t, "任务已完成", draft.Title)
	require.Equal(
		t,
		"季度报告：任务已完成，可查看结果。 操作人：刘文波。 原因：配置无效。",
		draft.Content,
	)
	require.Equal(t, TargetTaskThread, draft.TargetType)
	require.Equal(t, "thread-100", draft.TargetID)
	require.NotContains(t, draft.Content, event.EventID)
	require.NotContains(t, draft.Content, "provider")
}

func TestAppendValidationRejectsUnavailableRecipientPolicy(t *testing.T) {
	event := validTestEvent()
	event.EventType = EventAppDevBuildSucceeded
	event.RecipientPolicy = RecipientResourceOwner

	err := DefaultTemplateRegistry().ValidateAppendable(event)

	require.ErrorIs(t, err, ErrRecipientPolicyUnavailable)
	require.Equal(t, ErrorCodeRecipientPolicyUnavailable, StableErrorCode(err))
}

func validTestEvent() Event {
	return Event{
		EventID:          "evt-100",
		EventType:        EventAgentRunSucceeded,
		AggregateType:    "agent_run",
		AggregateID:      "run-100",
		AggregateVersion: 1,
		OccurredAt:       time.UnixMilli(1_721_000_000_000),
		ActorID:          101,
		SpaceID:          202,
		RecipientPolicy:  RecipientActor,
		PayloadSchema:    CurrentPayloadSchema,
		Payload: EventPayload{
			TargetID: "thread-100",
		},
	}
}

func TestStableErrorCodeNeverIncludesRawError(t *testing.T) {
	raw := errors.New("api_key=secret provider response")
	require.Equal(t, ErrorCodeStorage, StableErrorCode(raw))
	require.NotContains(t, StableErrorCode(raw), raw.Error())
}
