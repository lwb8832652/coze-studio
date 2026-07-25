// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationContentLimitRemainsACompileTimeContract(t *testing.T) {
	require.Positive(t, MaxNotificationContentRunes)
}

func TestCanonicalizeEventSortsAndDeduplicatesExplicitRecipients(t *testing.T) {
	event := validTestEvent()
	event.EventType = EventWorkspaceRoleChanged
	event.AggregateType = "workspace_member"
	event.AggregateID = "member-101"
	event.RecipientPolicy = RecipientExplicitInternalUsers
	event.Payload.ExplicitRecipientIDs = []int64{202, 101, 202}
	event.Payload.WorkspaceAction = WorkspaceMemberRoleChanged
	event.Payload.WorkspaceAudience = WorkspaceMemberAudienceAdmins
	event.Payload.SubjectDisplayName = "成员 101"

	canonical, err := CanonicalizeEvent(event)

	require.NoError(t, err)
	require.Equal(t, []int64{101, 202}, canonical.Payload.ExplicitRecipientIDs)
}

func TestEveryNotificationTaxonomyDeclaresAllowedPayloadFields(t *testing.T) {
	registry := DefaultTemplateRegistry()

	require.NotEmpty(t, registry.templates)
	for eventType, definition := range registry.templates {
		require.NotNil(
			t,
			definition.AllowedPayloadFields,
			"event %s must explicitly declare payload fields",
			eventType,
		)
	}
}

func TestTemplateAllowsOnlyFieldsDeclaredByTheEventTaxonomy(t *testing.T) {
	registry := DefaultTemplateRegistry()
	allowed := validTestEvent()
	allowed.EventType = EventWorkspaceRoleChanged
	allowed.AggregateType = "workspace_member"
	allowed.AggregateID = "member-101"
	allowed.RecipientPolicy = RecipientExplicitInternalUsers
	allowed.Payload = EventPayload{
		ResourceDisplayName:  "研发空间",
		WorkspaceAction:      WorkspaceMemberRoleChanged,
		WorkspaceAudience:    WorkspaceMemberAudienceAdmins,
		SubjectDisplayName:   "成员 101",
		ExplicitRecipientIDs: []int64{101},
	}

	_, err := registry.Render(allowed)
	require.NoError(t, err)

	rejected := validTestEvent()
	rejected.EventType = EventSystemAnnouncement
	rejected.AggregateType = "system_announcement"
	rejected.AggregateID = "announcement-1"
	rejected.ActorID = 0
	rejected.SpaceID = 0
	rejected.RecipientPolicy = RecipientSystemAdmins
	rejected.Payload = EventPayload{ActorDisplayName: "provider raw message"}

	_, err = registry.Render(rejected)
	require.ErrorIs(t, err, ErrInvalidEvent)
}
