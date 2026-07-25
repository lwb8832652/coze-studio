// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestCreditThresholdConfigHasExplicitDisabledDefaultAndSafeBounds(t *testing.T) {
	defaultConfig := DefaultCreditThresholdConfig()
	if defaultConfig.Enabled || defaultConfig.ThresholdMicros != 0 || defaultConfig.RecoveryMarginMicros != 0 {
		t.Fatalf("DefaultCreditThresholdConfig() = %+v, want explicit disabled config", defaultConfig)
	}
	if err := defaultConfig.Validate(); err != nil {
		t.Fatalf("disabled default Validate() error = %v", err)
	}

	valid := CreditThresholdConfig{
		Enabled:              true,
		ThresholdMicros:      MinCreditThresholdMicros,
		RecoveryMarginMicros: MinCreditRecoveryMarginMicros,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("minimum enabled config Validate() error = %v", err)
	}

	tests := []CreditThresholdConfig{
		{Enabled: false, ThresholdMicros: 1},
		{Enabled: false, RecoveryMarginMicros: 1},
		{Enabled: true, ThresholdMicros: 0, RecoveryMarginMicros: 1},
		{Enabled: true, ThresholdMicros: 1, RecoveryMarginMicros: 0},
		{Enabled: true, ThresholdMicros: MaxCreditThresholdMicros + 1, RecoveryMarginMicros: 1},
		{Enabled: true, ThresholdMicros: MaxCreditThresholdMicros, RecoveryMarginMicros: 1},
		{Enabled: true, ThresholdMicros: 1, RecoveryMarginMicros: MaxCreditRecoveryMarginMicros + 1},
	}
	for _, test := range tests {
		if err := test.Validate(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("CreditThresholdConfig%+v.Validate() error = %v, want ErrInvalidInput", test, err)
		}
	}
}

func TestEvaluateCreditThresholdEpisodeLifecycleUsesHysteresis(t *testing.T) {
	config := CreditThresholdConfig{
		Enabled:              true,
		ThresholdMicros:      100,
		RecoveryMarginMicros: 20,
	}
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)

	first, err := EvaluateCreditThreshold(config, 77, 120, 99, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Changed || !first.Notify || first.Episode == nil || !first.Episode.Active ||
		first.Episode.EpisodeNo != 1 || first.Episode.Version != InitialVersion {
		t.Fatalf("first crossing decision = %+v, want active first episode and notification", first)
	}

	repeated, err := EvaluateCreditThreshold(config, 77, 99, 80, first.Episode, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Changed || repeated.Notify {
		t.Fatalf("repeated low decision = %+v, want no duplicate", repeated)
	}

	nearThreshold, err := EvaluateCreditThreshold(config, 77, 80, 119, first.Episode, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if nearThreshold.Changed || nearThreshold.Notify || !nearThreshold.Episode.Active {
		t.Fatalf("hysteresis decision = %+v, want episode to remain active", nearThreshold)
	}

	recovered, err := EvaluateCreditThreshold(config, 77, 119, 120, first.Episode, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Changed || recovered.Notify || recovered.Episode.Active ||
		recovered.Episode.EpisodeNo != 1 || recovered.Episode.RecoveredAt == nil {
		t.Fatalf("recovery decision = %+v, want inactive episode without recovery notification", recovered)
	}

	atThreshold, err := EvaluateCreditThreshold(config, 77, 120, 100, recovered.Episode, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if atThreshold.Changed || atThreshold.Notify {
		t.Fatalf("at-threshold decision = %+v, want no crossing", atThreshold)
	}

	second, err := EvaluateCreditThreshold(config, 77, 100, 99, recovered.Episode, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Changed || !second.Notify || !second.Episode.Active ||
		second.Episode.EpisodeNo != 2 || second.Episode.Version != recovered.Episode.Version+1 {
		t.Fatalf("second crossing decision = %+v, want a new episode", second)
	}

	restarted, err := EvaluateCreditThreshold(config, 77, 99, 50, second.Episode, now.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Changed || restarted.Notify || restarted.Episode.EpisodeNo != 2 {
		t.Fatalf("restart/replay decision = %+v, want persisted episode deduplication", restarted)
	}
}

func TestEvaluateCreditThresholdRequiresDownwardCrossingToOpenEpisode(t *testing.T) {
	config := CreditThresholdConfig{
		Enabled:              true,
		ThresholdMicros:      100,
		RecoveryMarginMicros: 20,
	}
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		before int64
		after  int64
		notify bool
	}{
		{name: "zero to low positive grant", before: 0, after: 50},
		{name: "low positive increase without recovery", before: 50, after: 90},
		{name: "initial low account continues downward", before: 90, after: 80},
		{name: "downward move stops at threshold", before: 120, after: 100},
		{name: "true downward crossing", before: 120, after: 99, notify: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := EvaluateCreditThreshold(
				config,
				77,
				test.before,
				test.after,
				nil,
				now,
			)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Notify != test.notify || decision.Changed != test.notify {
				t.Fatalf(
					"EvaluateCreditThreshold(%d -> %d) = %+v, want notify=%v",
					test.before,
					test.after,
					decision,
					test.notify,
				)
			}
			if test.notify {
				if decision.Episode == nil || !decision.Episode.Active ||
					decision.Episode.EpisodeNo != 1 {
					t.Fatalf("crossing episode = %+v, want active episode 1", decision.Episode)
				}
			} else if decision.Episode != nil {
				t.Fatalf("non-crossing episode = %+v, want no backfilled episode", decision.Episode)
			}
		})
	}
}

func TestCreditThresholdEvaluationRequiresLockedBeforeAndMatchingAfterBalance(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	evaluation := CreditThresholdEvaluation{
		Account: Account{
			ID:              77,
			Subject:         Subject{Type: SubjectTypeUser, ID: 88},
			AvailableMicros: 90,
			ReservedMicros:  10,
			Version:         InitialVersion,
		},
		SettledBalanceBeforeMicros: 120,
		SettledBalanceAfterMicros:  100,
		OccurredAt:                 now,
	}
	if err := evaluation.Validate(); err != nil {
		t.Fatalf("valid locked balance transition error = %v", err)
	}

	negativeBefore := evaluation
	negativeBefore.SettledBalanceBeforeMicros = -1
	if err := negativeBefore.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative before balance error = %v, want ErrInvalidInput", err)
	}
	mismatchedAfter := evaluation
	mismatchedAfter.SettledBalanceAfterMicros = 99
	if err := mismatchedAfter.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("mismatched after balance error = %v, want ErrInvalidInput", err)
	}
}

func TestDisabledCreditThresholdResetsAnActiveEpisodeWithoutNotification(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	active := &CreditThresholdEpisode{
		AccountID:             88,
		EpisodeNo:            3,
		Active:               true,
		ThresholdMicros:      100,
		RecoveryMarginMicros: 20,
		Version:              5,
		OpenedAt:             now.Add(-time.Hour),
		CreatedAt:            now.Add(-time.Hour),
		UpdatedAt:            now.Add(-time.Hour),
	}
	decision, err := EvaluateCreditThreshold(
		DefaultCreditThresholdConfig(),
		active.AccountID,
		10,
		9,
		active,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Changed || decision.Notify || decision.Episode.Active ||
		decision.Episode.RecoveredAt == nil || decision.Episode.Version != active.Version+1 {
		t.Fatalf("disabled decision = %+v, want reset without notification", decision)
	}
}

func TestCreditNotificationEventsUseBoundedPersistedRoutes(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	userAccount := Account{
		ID:      501,
		Subject: Subject{Type: SubjectTypeUser, ID: 101},
		Version: 4,
	}
	adjusted, err := CreditAdjustmentNotificationEvent(
		userAccount,
		"admin-credit:admin-adjustment/unsafe-for-an-identifier",
		[]int64{userAccount.Subject.ID},
		9001,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	adjustmentEventID, err := CreditAdjustmentNotificationEventID(
		userAccount.ID,
		"admin-credit:admin-adjustment/unsafe-for-an-identifier",
	)
	if err != nil {
		t.Fatal(err)
	}
	if adjustmentEventID != adjusted.EventID {
		t.Fatalf(
			"adjustment event identity helper = %q, event = %q",
			adjustmentEventID,
			adjusted.EventID,
		)
	}
	replayed, err := CreditAdjustmentNotificationEvent(
		userAccount,
		"admin-credit:admin-adjustment/unsafe-for-an-identifier",
		[]int64{userAccount.Subject.ID},
		9001,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if adjusted.EventID != replayed.EventID || adjusted.IdempotencyKey() != replayed.IdempotencyKey() {
		t.Fatalf("adjustment identity is not stable: %q / %q", adjusted.EventID, replayed.EventID)
	}
	debited, err := CreditAdjustmentNotificationEvent(
		userAccount,
		"admin-debit:admin-adjustment/unsafe-for-an-identifier",
		[]int64{userAccount.Subject.ID},
		9001,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if adjusted.EventID == debited.EventID ||
		adjusted.IdempotencyKey() == debited.IdempotencyKey() {
		t.Fatalf(
			"opposite adjustment directions share identity: credit=%q debit=%q",
			adjusted.EventID,
			debited.EventID,
		)
	}
	debitEventID, err := CreditAdjustmentNotificationEventID(
		userAccount.ID,
		"admin-debit:admin-adjustment/unsafe-for-an-identifier",
	)
	if err != nil {
		t.Fatal(err)
	}
	if debitEventID != debited.EventID || debitEventID == adjustmentEventID {
		t.Fatalf(
			"directional adjustment identities = credit %q debit %q",
			adjustmentEventID,
			debitEventID,
		)
	}
	if adjusted.EventType != domainnotification.EventBillingCreditAdjusted ||
		adjusted.ActorID != 9001 ||
		adjusted.SpaceID != 0 ||
		adjusted.RecipientPolicy != domainnotification.RecipientExplicitInternalUsers ||
		!reflect.DeepEqual(adjusted.Payload.ExplicitRecipientIDs, []int64{101}) {
		t.Fatalf("user adjustment event = %+v, want persisted user route", adjusted)
	}
	if adjusted.Payload.StatusReasonCode != domainnotification.StatusReasonNone {
		t.Fatalf("adjustment payload status = %q, want no balance details", adjusted.Payload.StatusReasonCode)
	}

	workspaceAccount := Account{
		ID:      502,
		Subject: Subject{Type: SubjectTypeWorkspace, ID: 202},
		Version: 8,
	}
	episode := CreditThresholdEpisode{
		AccountID:             workspaceAccount.ID,
		EpisodeNo:            2,
		Active:               true,
		ThresholdMicros:      100,
		RecoveryMarginMicros: 20,
		Version:              3,
		OpenedAt:             now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	low, err := CreditLowNotificationEvent(
		workspaceAccount,
		episode,
		[]int64{302, 301, 302},
		9001,
	)
	if err != nil {
		t.Fatal(err)
	}
	if low.EventType != domainnotification.EventBillingCreditLow ||
		low.SpaceID != workspaceAccount.Subject.ID ||
		low.ActorID != 9001 ||
		low.RecipientPolicy != domainnotification.RecipientExplicitInternalUsers ||
		!reflect.DeepEqual(low.Payload.ExplicitRecipientIDs, []int64{301, 302}) ||
		low.Payload.StatusReasonCode != domainnotification.StatusReasonQuotaInsufficient {
		t.Fatalf("workspace low-credit event = %+v, want bounded workspace route", low)
	}
	if strings.Contains(strings.ToLower(low.Payload.ResourceDisplayName), "ledger") ||
		strings.Contains(strings.ToLower(low.Payload.ResourceDisplayName), "provider") {
		t.Fatalf("unsafe low-credit display payload = %q", low.Payload.ResourceDisplayName)
	}
	if err = domainnotification.DefaultTemplateRegistry().ValidateAppendable(low); err != nil {
		t.Fatalf("low-credit event is not appendable: %v", err)
	}
}
