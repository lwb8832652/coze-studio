// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"fmt"
	"strconv"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

const MaxMaintenanceIssueSummaries = 8

type MaintenanceIssueReason string

const (
	MaintenanceIssueMissingAccount                  MaintenanceIssueReason = "missing_account"
	MaintenanceIssueMissingSourceOrder              MaintenanceIssueReason = "missing_source_order"
	MaintenanceIssueSourceOrderAccountMismatch      MaintenanceIssueReason = "source_order_account_mismatch"
	MaintenanceIssueInvalidSubject                  MaintenanceIssueReason = "invalid_subject"
	MaintenanceIssueInvalidActor                    MaintenanceIssueReason = "invalid_actor"
	MaintenanceIssueUserSubjectOrderMismatch        MaintenanceIssueReason = "user_subject_order_mismatch"
)

type MaintenanceIssueSummary struct {
	Reason MaintenanceIssueReason `json:"reason"`
	Count  int64                  `json:"count"`
}

type MaintenanceErrorSummary struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

type MaintenanceSubscriptionCandidate struct {
	ID               int64
	Version          int64
	AccountID        int64
	SourceOrderID    int64
	CurrentPeriodEnd time.Time
	Route            BillingNotificationRoute
}

type MaintenanceOrderCandidate struct {
	ID        int64
	Version   int64
	AccountID int64
	ExpiresAt time.Time
	Route     BillingNotificationRoute
}

type MaintenanceSubscriptionBatch struct {
	Candidates []MaintenanceSubscriptionCandidate
	Issues     []MaintenanceIssueSummary
}

type MaintenanceOrderBatch struct {
	Candidates []MaintenanceOrderCandidate
	Issues     []MaintenanceIssueSummary
}

type MaintenanceProjectionOutcome struct {
	Transitioned         bool
	NotificationInserted bool
	Stale                bool
}

type MaintenanceStageResult struct {
	Scanned              int64                     `json:"scanned"`
	Succeeded            int64                     `json:"succeeded"`
	NewProjections       int64                     `json:"new_projections"`
	DuplicateProjections int64                     `json:"duplicate_projections"`
	InvalidCandidates    int64                     `json:"invalid_candidates"`
	StaleCandidates      int64                     `json:"stale_candidates"`
	Failed               int64                     `json:"failed"`
	Issues               []MaintenanceIssueSummary `json:"issues,omitempty"`
	Errors               []MaintenanceErrorSummary `json:"errors,omitempty"`
}

type MaintenanceProjectionResult struct {
	ExpiringSubscriptions int64                  `json:"expiring_subscriptions"`
	ExpiredSubscriptions  int64                  `json:"expired_subscriptions"`
	ClosedOrders          int64                  `json:"closed_orders"`
	Expiring              MaintenanceStageResult `json:"expiring"`
	Expired               MaintenanceStageResult `json:"expired"`
	Orders                MaintenanceStageResult `json:"orders"`
}

type MaintenanceProjectionRepository interface {
	ScanExpiringSubscriptions(context.Context, time.Time, time.Duration, int) (MaintenanceSubscriptionBatch, error)
	ProjectExpiringSubscription(context.Context, MaintenanceSubscriptionCandidate, time.Time, time.Duration) (MaintenanceProjectionOutcome, error)
	ScanDueSubscriptions(context.Context, time.Time, int) (MaintenanceSubscriptionBatch, error)
	ExpireDueSubscriptionProjection(context.Context, MaintenanceSubscriptionCandidate, time.Time) (MaintenanceProjectionOutcome, error)
	ScanExpiredOrders(context.Context, time.Time, int) (MaintenanceOrderBatch, error)
	CloseExpiredOrderProjection(context.Context, MaintenanceOrderCandidate, time.Time) (MaintenanceProjectionOutcome, error)
}

// BillingNotificationRoute is derived only from persisted billing account and
// source-order facts. It is intentionally not accepted from API callers.
type BillingNotificationRoute struct {
	Subject     Subject
	ActorUserID int64
}

func NewBillingNotificationRoute(subjectType SubjectType, subjectID, actorUserID int64) (BillingNotificationRoute, error) {
	route := BillingNotificationRoute{
		Subject:     Subject{Type: subjectType, ID: subjectID},
		ActorUserID: actorUserID,
	}
	if err := route.Validate(); err != nil {
		return BillingNotificationRoute{}, err
	}
	return route, nil
}

func (r BillingNotificationRoute) Validate() error {
	if r.Subject.Validate() != nil || r.ActorUserID <= 0 {
		return fmt.Errorf("%w: persisted billing notification route is invalid", domainnotification.ErrRecipientResolution)
	}
	if r.Subject.Type == SubjectTypeUser && r.Subject.ID != r.ActorUserID {
		return fmt.Errorf("%w: billing user subject does not match source order user", domainnotification.ErrRecipientResolution)
	}
	return nil
}

// UTCNaiveWallClockUnixMilli canonicalizes a MySQL DATETIME wall value. The
// location attached by a DSN is deliberately ignored: only the persisted
// year/month/day/hour/minute/second/millisecond components define identity.
func UTCNaiveWallClockUnixMilli(boundary time.Time) (int64, error) {
	if boundary.IsZero() {
		return 0, ErrInvalidInput
	}
	canonical := time.Date(
		boundary.Year(),
		boundary.Month(),
		boundary.Day(),
		boundary.Hour(),
		boundary.Minute(),
		boundary.Second(),
		boundary.Nanosecond(),
		time.UTC,
	)
	value := canonical.UnixMilli()
	if value <= 0 {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func SubscriptionExpiringNotificationEvent(
	subscriptionID int64,
	periodEnd time.Time,
	occurredAt time.Time,
	route BillingNotificationRoute,
) (domainnotification.Event, error) {
	if periodEnd.IsZero() || occurredAt.IsZero() {
		return domainnotification.Event{}, ErrInvalidInput
	}
	return newBillingMaintenanceNotificationEvent(
		domainnotification.EventBillingSubscriptionExpiring,
		"billing_subscription_period",
		subscriptionID,
		periodEnd,
		occurredAt,
		route,
		"Subscription",
	)
}

func SubscriptionExpiredNotificationEvent(
	subscriptionID int64,
	periodEnd time.Time,
	occurredAt time.Time,
	route BillingNotificationRoute,
) (domainnotification.Event, error) {
	if periodEnd.IsZero() || occurredAt.IsZero() {
		return domainnotification.Event{}, ErrInvalidInput
	}
	return newBillingMaintenanceNotificationEvent(
		domainnotification.EventBillingSubscriptionExpired,
		"billing_subscription_period",
		subscriptionID,
		periodEnd,
		occurredAt,
		route,
		"Subscription",
	)
}

func OrderTimedOutNotificationEvent(
	orderID int64,
	expiresAt time.Time,
	occurredAt time.Time,
	route BillingNotificationRoute,
) (domainnotification.Event, error) {
	if expiresAt.IsZero() || occurredAt.IsZero() {
		return domainnotification.Event{}, ErrInvalidInput
	}
	return newBillingMaintenanceNotificationEvent(
		domainnotification.EventBillingOrderTimedOut,
		"billing_order_timeout",
		orderID,
		expiresAt,
		occurredAt,
		route,
		"Billing order",
	)
}

func newBillingMaintenanceNotificationEvent(
	eventType domainnotification.EventType,
	aggregateType string,
	aggregateID int64,
	boundary time.Time,
	occurredAt time.Time,
	route BillingNotificationRoute,
	displayName string,
) (domainnotification.Event, error) {
	if aggregateID <= 0 || route.Validate() != nil {
		return domainnotification.Event{}, fmt.Errorf("%w: invalid billing notification facts", domainnotification.ErrRecipientResolution)
	}
	projectionVersion, err := UTCNaiveWallClockUnixMilli(boundary)
	if err != nil {
		return domainnotification.Event{}, err
	}
	aggregateIDValue := strconv.FormatInt(aggregateID, 10)
	projectionVersionValue := strconv.FormatInt(projectionVersion, 10)
	return domainnotification.Event{
		EventID: stableBillingEventID(
			string(eventType),
			aggregateType,
			aggregateIDValue,
			projectionVersionValue,
		),
		EventType:        eventType,
		AggregateType:    aggregateType,
		AggregateID:      aggregateIDValue,
		AggregateVersion: projectionVersion,
		OccurredAt:       occurredAt.UTC(),
		ActorID:          route.ActorUserID,
		SpaceID:          billingNotificationSpaceID(route.Subject),
		RecipientPolicy:  domainnotification.RecipientActor,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName: displayName,
			TargetID:            aggregateIDValue,
		},
	}, nil
}
