// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"strconv"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

const (
	MinCreditThresholdMicros      int64 = 1
	MaxCreditThresholdMicros      int64 = 1_000_000_000_000_000
	MinCreditRecoveryMarginMicros int64 = 1
	MaxCreditRecoveryMarginMicros int64 = 1_000_000_000_000_000
)

// CreditThresholdConfig is disabled only when all numeric values are zero.
// A missing persisted subject-specific and subject-type default row resolves
// to DefaultCreditThresholdConfig, so deployments fail safe without sending
// an unexpected first wave of low-balance notifications.
type CreditThresholdConfig struct {
	Enabled              bool
	ThresholdMicros      int64
	RecoveryMarginMicros int64
}

func DefaultCreditThresholdConfig() CreditThresholdConfig {
	return CreditThresholdConfig{}
}

func (c CreditThresholdConfig) Validate() error {
	if !c.Enabled {
		if c.ThresholdMicros != 0 || c.RecoveryMarginMicros != 0 {
			return ErrInvalidInput
		}
		return nil
	}
	if c.ThresholdMicros < MinCreditThresholdMicros ||
		c.ThresholdMicros > MaxCreditThresholdMicros ||
		c.RecoveryMarginMicros < MinCreditRecoveryMarginMicros ||
		c.RecoveryMarginMicros > MaxCreditRecoveryMarginMicros ||
		c.ThresholdMicros > MaxCreditThresholdMicros-c.RecoveryMarginMicros {
		return ErrInvalidInput
	}
	return nil
}

type CreditThresholdEpisode struct {
	AccountID             int64
	EpisodeNo             int64
	Active                bool
	ThresholdMicros       int64
	RecoveryMarginMicros  int64
	Version               int64
	OpenedAt              time.Time
	RecoveredAt           *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (e CreditThresholdEpisode) Validate() error {
	config := CreditThresholdConfig{
		Enabled:              true,
		ThresholdMicros:      e.ThresholdMicros,
		RecoveryMarginMicros: e.RecoveryMarginMicros,
	}
	if e.AccountID <= 0 ||
		e.EpisodeNo <= 0 ||
		e.Version <= 0 ||
		e.OpenedAt.IsZero() ||
		e.CreatedAt.IsZero() ||
		e.UpdatedAt.IsZero() ||
		config.Validate() != nil {
		return ErrInvalidInput
	}
	if e.Active && e.RecoveredAt != nil {
		return ErrInvalidInput
	}
	if !e.Active && e.RecoveredAt == nil {
		return ErrInvalidInput
	}
	if e.RecoveredAt != nil && e.RecoveredAt.Before(e.OpenedAt) {
		return ErrInvalidInput
	}
	return nil
}

type CreditThresholdDecision struct {
	Episode *CreditThresholdEpisode
	Changed bool
	Notify  bool
}

// EvaluateCreditThreshold is a pure state transition over settled balances
// observed under the same account lock. A new episode opens only on a true
// downward crossing. An active episode is reset only at threshold + recovery
// margin; recovery never creates an event.
func EvaluateCreditThreshold(
	config CreditThresholdConfig,
	accountID int64,
	settledBalanceBeforeMicros int64,
	settledBalanceAfterMicros int64,
	current *CreditThresholdEpisode,
	now time.Time,
) (CreditThresholdDecision, error) {
	if config.Validate() != nil ||
		accountID <= 0 ||
		settledBalanceBeforeMicros < 0 ||
		settledBalanceAfterMicros < 0 ||
		now.IsZero() {
		return CreditThresholdDecision{}, ErrInvalidInput
	}
	now = now.UTC()
	episode, err := cloneCreditThresholdEpisode(current)
	if err != nil {
		return CreditThresholdDecision{}, err
	}
	if episode != nil && episode.AccountID != accountID {
		return CreditThresholdDecision{}, ErrInvalidInput
	}

	if !config.Enabled {
		if episode == nil || !episode.Active {
			return CreditThresholdDecision{Episode: episode}, nil
		}
		recoveredAt := now
		episode.Active = false
		episode.RecoveredAt = &recoveredAt
		episode.Version++
		episode.UpdatedAt = now
		return CreditThresholdDecision{Episode: episode, Changed: true}, nil
	}

	if episode == nil {
		if !isDownwardCreditThresholdCrossing(
			config,
			settledBalanceBeforeMicros,
			settledBalanceAfterMicros,
		) {
			return CreditThresholdDecision{}, nil
		}
		return CreditThresholdDecision{
			Episode: &CreditThresholdEpisode{
				AccountID:             accountID,
				EpisodeNo:             1,
				Active:                true,
				ThresholdMicros:       config.ThresholdMicros,
				RecoveryMarginMicros:  config.RecoveryMarginMicros,
				Version:               InitialVersion,
				OpenedAt:              now,
				CreatedAt:             now,
				UpdatedAt:             now,
			},
			Changed: true,
			Notify:  true,
		}, nil
	}

	if episode.Active {
		recoveryBoundary := config.ThresholdMicros + config.RecoveryMarginMicros
		if settledBalanceAfterMicros >= recoveryBoundary {
			recoveredAt := now
			episode.Active = false
			episode.ThresholdMicros = config.ThresholdMicros
			episode.RecoveryMarginMicros = config.RecoveryMarginMicros
			episode.RecoveredAt = &recoveredAt
			episode.Version++
			episode.UpdatedAt = now
			return CreditThresholdDecision{Episode: episode, Changed: true}, nil
		}
		if episode.ThresholdMicros != config.ThresholdMicros ||
			episode.RecoveryMarginMicros != config.RecoveryMarginMicros {
			episode.ThresholdMicros = config.ThresholdMicros
			episode.RecoveryMarginMicros = config.RecoveryMarginMicros
			episode.Version++
			episode.UpdatedAt = now
			return CreditThresholdDecision{Episode: episode, Changed: true}, nil
		}
		return CreditThresholdDecision{Episode: episode}, nil
	}

	if !isDownwardCreditThresholdCrossing(
		config,
		settledBalanceBeforeMicros,
		settledBalanceAfterMicros,
	) {
		return CreditThresholdDecision{Episode: episode}, nil
	}
	episode.EpisodeNo++
	episode.Active = true
	episode.ThresholdMicros = config.ThresholdMicros
	episode.RecoveryMarginMicros = config.RecoveryMarginMicros
	episode.OpenedAt = now
	episode.RecoveredAt = nil
	episode.Version++
	episode.UpdatedAt = now
	return CreditThresholdDecision{Episode: episode, Changed: true, Notify: true}, nil
}

func isDownwardCreditThresholdCrossing(
	config CreditThresholdConfig,
	settledBalanceBeforeMicros int64,
	settledBalanceAfterMicros int64,
) bool {
	return settledBalanceBeforeMicros >= config.ThresholdMicros &&
		settledBalanceAfterMicros < config.ThresholdMicros &&
		settledBalanceAfterMicros < settledBalanceBeforeMicros
}

type CreditThresholdEvaluation struct {
	Account                     Account
	SettledBalanceBeforeMicros int64
	SettledBalanceAfterMicros  int64
	ActorUserID                 int64
	OccurredAt                  time.Time
}

func (e CreditThresholdEvaluation) Validate() error {
	if e.Account.ID <= 0 ||
		e.Account.Subject.Validate() != nil ||
		e.Account.AvailableMicros < 0 ||
		e.Account.ReservedMicros < 0 ||
		e.Account.Version <= 0 ||
		e.SettledBalanceBeforeMicros < 0 ||
		e.SettledBalanceAfterMicros < 0 ||
		e.SettledBalanceBeforeMicros == e.SettledBalanceAfterMicros ||
		e.ActorUserID < 0 ||
		e.OccurredAt.IsZero() {
		return ErrInvalidInput
	}
	settledBalanceAfter, err := settledCreditBalanceMicros(&e.Account)
	if err != nil {
		return err
	}
	if settledBalanceAfter != e.SettledBalanceAfterMicros {
		return ErrInvalidInput
	}
	return nil
}

func CreditAdjustmentNotificationEvent(
	account Account,
	businessNo string,
	recipients []int64,
	actorUserID int64,
	occurredAt time.Time,
) (domainnotification.Event, error) {
	eventID, err := CreditAdjustmentNotificationEventID(account.ID, businessNo)
	if err != nil ||
		validateCreditNotificationAccount(account) != nil ||
		actorUserID <= 0 ||
		occurredAt.IsZero() {
		return domainnotification.Event{}, ErrInvalidInput
	}
	recipients, err = domainnotification.NormalizeRecipientIDs(recipients)
	if err != nil {
		return domainnotification.Event{}, err
	}
	return canonicalCreditNotificationEvent(domainnotification.Event{
		EventID:          eventID,
		EventType:        domainnotification.EventBillingCreditAdjusted,
		AggregateType:    "billing_credit_adjustment",
		AggregateID:      eventID,
		AggregateVersion: InitialVersion,
		OccurredAt:       occurredAt.UTC(),
		ActorID:          actorUserID,
		SpaceID:          billingNotificationSpaceID(account.Subject),
		RecipientPolicy:  domainnotification.RecipientExplicitInternalUsers,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName:  creditBalanceDisplayName(account.Subject),
			TargetID:             strconv.FormatInt(account.Subject.ID, 10),
			ExplicitRecipientIDs: recipients,
		},
	})
}

func CreditAdjustmentNotificationEventID(
	accountID int64,
	businessNo string,
) (string, error) {
	businessNo, err := normalizeBusinessNo(businessNo)
	if err != nil || accountID <= 0 {
		return "", ErrInvalidInput
	}
	return stableBillingEventID(
		string(domainnotification.EventBillingCreditAdjusted),
		strconv.FormatInt(accountID, 10),
		businessNo,
	), nil
}

func CreditLowNotificationEvent(
	account Account,
	episode CreditThresholdEpisode,
	recipients []int64,
	actorUserID int64,
) (domainnotification.Event, error) {
	if validateCreditNotificationAccount(account) != nil ||
		episode.Validate() != nil ||
		episode.AccountID != account.ID ||
		!episode.Active ||
		actorUserID < 0 {
		return domainnotification.Event{}, ErrInvalidInput
	}
	recipients, err := domainnotification.NormalizeRecipientIDs(recipients)
	if err != nil {
		return domainnotification.Event{}, err
	}
	accountID := strconv.FormatInt(account.ID, 10)
	episodeNo := strconv.FormatInt(episode.EpisodeNo, 10)
	return canonicalCreditNotificationEvent(domainnotification.Event{
		EventID: stableBillingEventID(
			string(domainnotification.EventBillingCreditLow),
			accountID,
			episodeNo,
		),
		EventType:        domainnotification.EventBillingCreditLow,
		AggregateType:    "billing_credit_threshold_episode",
		AggregateID:      accountID,
		AggregateVersion: episode.EpisodeNo,
		OccurredAt:       episode.OpenedAt.UTC(),
		ActorID:          actorUserID,
		SpaceID:          billingNotificationSpaceID(account.Subject),
		RecipientPolicy:  domainnotification.RecipientExplicitInternalUsers,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName:  creditBalanceDisplayName(account.Subject),
			StatusReasonCode:     domainnotification.StatusReasonQuotaInsufficient,
			TargetID:             strconv.FormatInt(account.Subject.ID, 10),
			ExplicitRecipientIDs: recipients,
		},
	})
}

func settledCreditBalanceMicros(account *Account) (int64, error) {
	if account == nil || account.AvailableMicros < 0 || account.ReservedMicros < 0 {
		return 0, ErrInvalidInput
	}
	return checkedAdd(account.AvailableMicros, account.ReservedMicros)
}

func applyCreditThresholdAfterSettledBalanceChange(
	ctx context.Context,
	repository Repository,
	account *Account,
	settledBalanceBefore int64,
	actorUserID int64,
	occurredAt time.Time,
) error {
	settledBalanceAfter, err := settledCreditBalanceMicros(account)
	if err != nil {
		return err
	}
	if settledBalanceAfter == settledBalanceBefore {
		return nil
	}
	evaluation := CreditThresholdEvaluation{
		Account:                     *account,
		SettledBalanceBeforeMicros: settledBalanceBefore,
		SettledBalanceAfterMicros:  settledBalanceAfter,
		ActorUserID:                 actorUserID,
		OccurredAt:                  occurredAt,
	}
	if err = evaluation.Validate(); err != nil {
		return err
	}
	return repository.ApplyCreditThreshold(ctx, evaluation)
}

func cloneCreditThresholdEpisode(
	episode *CreditThresholdEpisode,
) (*CreditThresholdEpisode, error) {
	if episode == nil {
		return nil, nil
	}
	if err := episode.Validate(); err != nil {
		return nil, err
	}
	cloned := *episode
	if episode.RecoveredAt != nil {
		recoveredAt := episode.RecoveredAt.UTC()
		cloned.RecoveredAt = &recoveredAt
	}
	return &cloned, nil
}

func validateCreditNotificationAccount(account Account) error {
	if account.ID <= 0 ||
		account.Subject.Validate() != nil ||
		account.Version <= 0 {
		return ErrInvalidInput
	}
	return nil
}

func canonicalCreditNotificationEvent(
	event domainnotification.Event,
) (domainnotification.Event, error) {
	event, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return domainnotification.Event{}, err
	}
	if err = domainnotification.DefaultTemplateRegistry().ValidateAppendable(event); err != nil {
		return domainnotification.Event{}, err
	}
	return event, nil
}

func creditBalanceDisplayName(subject Subject) string {
	if subject.Type == SubjectTypeWorkspace {
		return "Workspace credit balance"
	}
	return "Personal credit balance"
}
