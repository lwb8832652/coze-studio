// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestRetryIdentityIgnoresDeliveryMetadataAndCanonicalizesRecipients(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	event := testEvent("evt-original", 1)
	event.EventType = domainnotification.EventWorkspaceRoleChanged
	event.AggregateType = "workspace_member"
	event.AggregateID = "member-101"
	event.RecipientPolicy = domainnotification.RecipientExplicitInternalUsers
	event.Payload.ExplicitRecipientIDs = []int64{202, 101, 202}
	event.Payload.WorkspaceAction = domainnotification.WorkspaceMemberRoleChanged
	event.Payload.WorkspaceAudience = domainnotification.WorkspaceMemberAudienceAdmins
	event.Payload.SubjectDisplayName = "成员 101"

	require.NoError(t, repository.Append(ctx, event))

	retry := event
	retry.EventID = "evt-retry"
	retry.OccurredAt = event.OccurredAt.Add(time.Minute)
	retry.Payload.ExplicitRecipientIDs = []int64{101, 202}
	require.NoError(t, repository.Append(ctx, retry))

	var count int64
	require.NoError(t, db.Model(&outboxPO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var stored outboxPO
	require.NoError(t, db.First(&stored).Error)
	payload, err := decodeEventPayload(stored.PayloadJSON)
	require.NoError(t, err)
	require.Equal(t, []int64{101, 202}, payload.ExplicitRecipientIDs)

	conflict := retry
	conflict.Payload.ResourceDisplayName = "不同资源"
	require.ErrorIs(
		t,
		repository.Append(ctx, conflict),
		domainnotification.ErrIdempotencyConflict,
	)
}

func TestSameOutboxIdentityMatchesCanonicalMySQLJSONSemantics(t *testing.T) {
	// MySQL JSON may normalize whitespace, key ordering, and integral number
	// spelling. Retry identity must compare the controlled payload semantically.
	left := &outboxPO{
		EventID:          "evt-first",
		IdempotencyKey:   strings.Repeat("c", 64),
		EventType:        string(domainnotification.EventWorkspaceRoleChanged),
		AggregateType:    "workspace_member",
		AggregateID:      "member-101",
		AggregateVersion: 1,
		OccurredAt:       1_721_000_000_000,
		SpaceID:          202,
		ActorID:          101,
		RecipientPolicy:  string(domainnotification.RecipientExplicitInternalUsers),
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		PayloadJSON: []byte(`{
			"target_id": "member-101",
			"workspace_action": "role_changed",
			"workspace_audience": "admins",
			"subject_display_name": "成员 101",
			"explicit_recipient_ids": [202, 101, 202]
		}`),
	}
	right := *left
	right.EventID = "evt-retry"
	right.OccurredAt += 60_000
	right.PayloadJSON = []byte(
		`{"explicit_recipient_ids":[1.01e2,202.0],"subject_display_name":"成员 101","target_id":"member-101","workspace_action":"role_changed","workspace_audience":"admins"}`,
	)

	require.True(t, sameOutboxIdentity(left, &right))

	right.AggregateVersion++
	require.False(t, sameOutboxIdentity(left, &right))
}

func TestClaimReturnsLeaseExpiryFromRepositoryLockTime(t *testing.T) {
	_, repository := newTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repository.Append(ctx, testEvent("evt-lease-expiry", 1)))
	lease := 30 * time.Second
	logicalNow := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)

	claims, err := repository.ClaimOutboxBatch(
		ctx,
		"worker-lease-expiry",
		logicalNow,
		lease,
		1,
	)

	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, logicalNow.Add(lease), claims[0].LeaseExpiresAt)
}
