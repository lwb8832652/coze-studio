// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

type healthMonitorIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *healthMonitorIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *healthMonitorIDGenerator) GenMultiIDs(
	ctx context.Context,
	count int,
) ([]int64, error) {
	ids := make([]int64, count)
	for index := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

type failingHealthNotificationOutbox struct {
	err error
}

func (f failingHealthNotificationOutbox) AppendInTransaction(
	context.Context,
	*gorm.DB,
	domainnotification.Event,
) error {
	return f.err
}

type selectiveHealthNotificationOutbox struct {
	delegate   HealthNotificationOutbox
	failEvents map[string]error
}

func (s selectiveHealthNotificationOutbox) AppendInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	if err := s.failEvents[event.EventID]; err != nil {
		return err
	}
	return s.delegate.AppendInTransaction(ctx, tx, event)
}

func TestMySQLHealthMonitorClaimCooldownRestartRecoveryAndNewIncident(t *testing.T) {
	ctx := context.Background()
	db, providerRepository, outbox := newHealthMonitorRepositoryTest(t)
	provider := createEnabledHealthMonitorProvider(t, ctx, providerRepository, "health-monitor-lifecycle")
	repository := NewMySQLHealthMonitorRepository(db, outbox)
	base := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)

	first := claimHealthMonitorProvider(t, ctx, repository, "worker-a", base)
	if competing, err := repository.ClaimHealthChecks(ctx, domainsandbox.HealthMonitorClaimRequest{
		WorkerID: "worker-b",
		Now: base,
		Lease: time.Minute,
		Limit: 1,
	}); err != nil || len(competing) != 0 {
		t.Fatalf("competing claim = %#v, %v", competing, err)
	}
	completeHealthMonitorCheck(
		t,
		ctx,
		repository,
		first,
		unhealthyHealthSnapshot(base),
		base.Add(time.Minute),
	)
	if early, err := repository.ClaimHealthChecks(ctx, domainsandbox.HealthMonitorClaimRequest{
		WorkerID: "worker-b",
		Now: base.Add(30 * time.Second),
		Lease: time.Minute,
		Limit: 1,
	}); err != nil || len(early) != 0 {
		t.Fatalf("cooldown claim = %#v, %v", early, err)
	}

	secondRepository := NewMySQLHealthMonitorRepository(db, outbox)
	second := claimHealthMonitorProvider(t, ctx, secondRepository, "worker-b", base.Add(time.Minute))
	completeHealthMonitorCheck(
		t,
		ctx,
		secondRepository,
		second,
		unhealthyHealthSnapshot(base.Add(time.Minute)),
		base.Add(2*time.Minute),
	)
	third := claimHealthMonitorProvider(t, ctx, repository, "worker-a", base.Add(2*time.Minute))
	completion := completeHealthMonitorCheck(
		t,
		ctx,
		repository,
		third,
		unhealthyHealthSnapshot(base.Add(2*time.Minute)),
		base.Add(3*time.Minute),
	)
	if completion.Notification != domainsandbox.HealthIncidentNotificationUnhealthy ||
		completion.Episode.IncidentID != "sandbox-incident-"+providerIDText(provider.ID)+"-1" {
		t.Fatalf("stable completion = %#v", completion)
	}
	firstUnhealthyEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":1:unhealthy"
	assertHealthProjectionStatus(
		t,
		db,
		firstUnhealthyEventID,
		healthProjectionStatusPending,
	)
	assertNotificationOutboxEventCount(t, db, firstUnhealthyEventID, 0)
	projectPendingHealthNotifications(
		t,
		ctx,
		repository,
		"projector-a",
		base.Add(2*time.Minute+time.Second),
		1,
	)
	assertHealthProjectionStatus(
		t,
		db,
		firstUnhealthyEventID,
		healthProjectionStatusProjected,
	)
	assertHealthNotificationOutbox(
		t,
		db,
		1,
		domainnotification.EventSystemProviderUnavailable,
		provider,
	)

	restartedRepository := NewMySQLHealthMonitorRepository(db, outbox)
	repeated := claimHealthMonitorProvider(
		t,
		ctx,
		restartedRepository,
		"worker-a",
		base.Add(3*time.Minute),
	)
	completion = completeHealthMonitorCheck(
		t,
		ctx,
		restartedRepository,
		repeated,
		unhealthyHealthSnapshot(base.Add(3*time.Minute)),
		base.Add(4*time.Minute),
	)
	if completion.Notification != domainsandbox.HealthIncidentNotificationNone {
		t.Fatalf("repeated unhealthy completion = %#v", completion)
	}
	recoveryClaim := claimHealthMonitorProvider(
		t,
		ctx,
		restartedRepository,
		"worker-a",
		base.Add(4*time.Minute),
	)
	completion = completeHealthMonitorCheck(
		t,
		ctx,
		restartedRepository,
		recoveryClaim,
		healthyHealthSnapshot(base.Add(4*time.Minute)),
		base.Add(5*time.Minute),
	)
	if completion.Notification != domainsandbox.HealthIncidentNotificationRecovered {
		t.Fatalf("recovery completion = %#v", completion)
	}
	recoveredEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":1:recovered"
	assertHealthProjectionStatus(
		t,
		db,
		recoveredEventID,
		healthProjectionStatusPending,
	)
	assertNotificationOutboxEventCount(t, db, recoveredEventID, 0)
	projectPendingHealthNotifications(
		t,
		ctx,
		restartedRepository,
		"projector-b",
		base.Add(4*time.Minute+time.Second),
		1,
	)
	assertHealthProjectionStatus(
		t,
		db,
		recoveredEventID,
		healthProjectionStatusProjected,
	)
	assertHealthNotificationOutbox(
		t,
		db,
		2,
		domainnotification.EventSystemProviderRecovered,
		provider,
	)

	for index := 5; index < 8; index++ {
		checkedAt := base.Add(time.Duration(index) * time.Minute)
		claim := claimHealthMonitorProvider(
			t,
			ctx,
			restartedRepository,
			"worker-a",
			checkedAt,
		)
		completion = completeHealthMonitorCheck(
			t,
			ctx,
			restartedRepository,
			claim,
			unhealthyHealthSnapshot(checkedAt),
			checkedAt.Add(time.Minute),
		)
	}
	if completion.Notification != domainsandbox.HealthIncidentNotificationUnhealthy ||
		completion.Episode.IncidentSequence != 2 {
		t.Fatalf("second incident completion = %#v", completion)
	}
	secondUnhealthyEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":2:unhealthy"
	assertHealthProjectionStatus(
		t,
		db,
		secondUnhealthyEventID,
		healthProjectionStatusPending,
	)
	assertNotificationOutboxEventCount(t, db, secondUnhealthyEventID, 0)
	projectPendingHealthNotifications(
		t,
		ctx,
		restartedRepository,
		"projector-b",
		base.Add(7*time.Minute+time.Second),
		1,
	)
	assertHealthProjectionStatus(
		t,
		db,
		secondUnhealthyEventID,
		healthProjectionStatusProjected,
	)
	assertHealthNotificationOutbox(
		t,
		db,
		3,
		domainnotification.EventSystemProviderUnavailable,
		provider,
	)
}

func TestMySQLHealthMonitorOutboxFailureKeepsAuthoritativeHealthAndRetriesProjection(t *testing.T) {
	ctx := context.Background()
	db, providerRepository, outbox := newHealthMonitorRepositoryTest(t)
	provider := createEnabledHealthMonitorProvider(t, ctx, providerRepository, "health-monitor-projection")
	failure := errors.New("outbox unavailable with endpoint token and raw provider body")
	failingRepository := NewMySQLHealthMonitorRepository(
		db,
		failingHealthNotificationOutbox{err: failure},
	)
	base := time.Date(2026, 7, 25, 11, 0, 0, 0, time.UTC)
	var completion domainsandbox.HealthMonitorCheckCompletion
	for index := 0; index < 3; index++ {
		checkedAt := base.Add(time.Duration(index) * time.Minute)
		claim := claimHealthMonitorProvider(t, ctx, failingRepository, "worker-a", checkedAt)
		completion = completeHealthMonitorCheck(
			t,
			ctx,
			failingRepository,
			claim,
			unhealthyHealthSnapshot(checkedAt),
			checkedAt.Add(time.Minute),
		)
	}
	if completion.Notification != domainsandbox.HealthIncidentNotificationUnhealthy {
		t.Fatalf("unhealthy transition = %#v", completion)
	}
	var episode providerHealthEpisodePO
	if err := db.Where("provider_id = ?", provider.ID).Take(&episode).Error; err != nil {
		t.Fatalf("read committed episode: %v", err)
	}
	if episode.IncidentStatus != string(domainsandbox.HealthIncidentStatusOpen) ||
		episode.IncidentSequence != 1 ||
		episode.LeaseToken != "" {
		t.Fatalf("authoritative unhealthy episode = %#v", episode)
	}
	var stored providerPO
	if err := db.Where("id = ?", provider.ID).Take(&stored).Error; err != nil {
		t.Fatalf("read committed provider: %v", err)
	}
	if stored.HealthStatus != string(domainsandbox.HealthStatusUnhealthy) ||
		stored.LastHealthAt == nil ||
		!stored.LastHealthAt.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("authoritative unhealthy provider = %#v", stored)
	}
	assertHealthProjectionStatus(
		t,
		db,
		"sandbox-health:"+providerIDText(provider.ID)+":1:unhealthy",
		healthProjectionStatusPending,
	)
	var outboxCount int64
	if err := db.Table("notification_outbox").Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox before projection: %v", err)
	}
	if outboxCount != 0 {
		t.Fatalf("outbox count before projection = %d", outboxCount)
	}

	_, err := failingRepository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-a", base.Add(3*time.Minute)),
	)
	if !errors.Is(err, domainsandbox.ErrHealthMonitorOutbox) ||
		strings.Contains(err.Error(), "endpoint token") ||
		strings.Contains(err.Error(), "provider body") {
		t.Fatalf("failed unhealthy projection error = %v", err)
	}
	assertHealthProjectionStatus(
		t,
		db,
		"sandbox-health:"+providerIDText(provider.ID)+":1:unhealthy",
		healthProjectionStatusPending,
	)

	restartedRepository := NewMySQLHealthMonitorRepository(db, outbox)
	result, err := restartedRepository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", base.Add(3*time.Minute+10*time.Second)),
	)
	if err != nil || result.Projected != 1 {
		t.Fatalf("restarted unhealthy projection = %#v, %v", result, err)
	}
	assertHealthNotificationOutbox(
		t,
		db,
		1,
		domainnotification.EventSystemProviderUnavailable,
		provider,
	)

	recoveryRepository := NewMySQLHealthMonitorRepository(
		db,
		failingHealthNotificationOutbox{err: failure},
	)
	recoveryAt := base.Add(4 * time.Minute)
	recoveryClaim := claimHealthMonitorProvider(
		t,
		ctx,
		recoveryRepository,
		"worker-a",
		recoveryAt,
	)
	recovery := completeHealthMonitorCheck(
		t,
		ctx,
		recoveryRepository,
		recoveryClaim,
		healthyHealthSnapshot(recoveryAt),
		recoveryAt.Add(time.Minute),
	)
	if recovery.Notification != domainsandbox.HealthIncidentNotificationRecovered {
		t.Fatalf("recovery transition = %#v", recovery)
	}
	if err := db.Where("id = ?", provider.ID).Take(&stored).Error; err != nil {
		t.Fatalf("read recovered provider: %v", err)
	}
	if stored.HealthStatus != string(domainsandbox.HealthStatusHealthy) ||
		stored.LastHealthAt == nil ||
		!stored.LastHealthAt.Equal(recoveryAt) {
		t.Fatalf("recovery did not commit before projection: %#v", stored)
	}
	if err := db.Where("provider_id = ?", provider.ID).Take(&episode).Error; err != nil {
		t.Fatalf("read recovered episode: %v", err)
	}
	if episode.IncidentStatus != string(domainsandbox.HealthIncidentStatusRecovered) {
		t.Fatalf("recovery episode = %#v", episode)
	}
	_, err = recoveryRepository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-a", recoveryAt.Add(time.Second)),
	)
	if !errors.Is(err, domainsandbox.ErrHealthMonitorOutbox) {
		t.Fatalf("failed recovery projection error = %v", err)
	}
	assertHealthProjectionStatus(
		t,
		db,
		"sandbox-health:"+providerIDText(provider.ID)+":1:recovered",
		healthProjectionStatusPending,
	)

	result, err = restartedRepository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", recoveryAt.Add(10*time.Second)),
	)
	if err != nil || result.Projected != 1 {
		t.Fatalf("restarted recovery projection = %#v, %v", result, err)
	}
	assertHealthNotificationOutbox(
		t,
		db,
		2,
		domainnotification.EventSystemProviderRecovered,
		provider,
	)
	result, err = restartedRepository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", recoveryAt.Add(20*time.Second)),
	)
	if err != nil || result.Projected != 0 {
		t.Fatalf("duplicate projection retry = %#v, %v", result, err)
	}
	assertHealthNotificationOutbox(
		t,
		db,
		2,
		domainnotification.EventSystemProviderRecovered,
		provider,
	)
}

func TestMySQLHealthMonitorStaleProviderVersionReturnsExplicitConflict(t *testing.T) {
	ctx := context.Background()
	db, providerRepository, outbox := newHealthMonitorRepositoryTest(t)
	provider := createEnabledHealthMonitorProvider(t, ctx, providerRepository, "health-monitor-stale-version")
	repository := NewMySQLHealthMonitorRepository(db, outbox)
	base := time.Date(2026, 7, 25, 11, 30, 0, 0, time.UTC)
	claim := claimHealthMonitorProvider(t, ctx, repository, "worker-a", base)
	if err := db.Model(&providerPO{}).
		Where("id = ?", provider.ID).
		Update("version", gorm.Expr("version + 1")).Error; err != nil {
		t.Fatalf("advance provider version: %v", err)
	}
	_, err := repository.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: claim,
			Health: healthyHealthSnapshot(base),
			CheckedAt: base,
			NextCheckAt: base.Add(time.Minute),
			FailureThreshold: 3,
		},
	)
	if !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale provider completion error = %v", err)
	}
	var episode providerHealthEpisodePO
	if err := db.Where("provider_id = ?", provider.ID).Take(&episode).Error; err != nil {
		t.Fatalf("read released stale episode: %v", err)
	}
	if episode.LeaseOwner != "" ||
		episode.LeaseToken != "" ||
		episode.LeaseExpiresAt != 0 {
		t.Fatalf("stale provider claim was not explicitly released: %#v", episode)
	}
}

func TestMySQLHealthProjectionRequiresProjectedUnhealthyBeforeRecoveryClaim(
	t *testing.T,
) {
	ctx := context.Background()
	db, providerRepository, outbox := newHealthMonitorRepositoryTest(t)
	provider := createEnabledHealthMonitorProvider(
		t,
		ctx,
		providerRepository,
		"health-monitor-causal-projection",
	)
	failure := errors.New("temporary outbox failure with raw provider body")
	repository := NewMySQLHealthMonitorRepository(
		db,
		failingHealthNotificationOutbox{err: failure},
	)
	base := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		checkedAt := base.Add(time.Duration(index) * time.Minute)
		claim := claimHealthMonitorProvider(
			t,
			ctx,
			repository,
			"worker-a",
			checkedAt,
		)
		completeHealthMonitorCheck(
			t,
			ctx,
			repository,
			claim,
			unhealthyHealthSnapshot(checkedAt),
			checkedAt.Add(time.Minute),
		)
	}
	recoveryAt := base.Add(3 * time.Minute)
	recoveryClaim := claimHealthMonitorProvider(
		t,
		ctx,
		repository,
		"worker-a",
		recoveryAt,
	)
	recovery := completeHealthMonitorCheck(
		t,
		ctx,
		repository,
		recoveryClaim,
		healthyHealthSnapshot(recoveryAt),
		recoveryAt.Add(time.Minute),
	)
	if recovery.Notification != domainsandbox.HealthIncidentNotificationRecovered {
		t.Fatalf("recovery transition = %#v", recovery)
	}
	var stored providerPO
	if err := db.Where("id = ?", provider.ID).Take(&stored).Error; err != nil {
		t.Fatalf("read recovered provider: %v", err)
	}
	if stored.HealthStatus != string(domainsandbox.HealthStatusHealthy) ||
		stored.LastHealthAt == nil ||
		!stored.LastHealthAt.Equal(recoveryAt) {
		t.Fatalf("health recovery was not committed: %#v", stored)
	}

	result, err := repository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-a", recoveryAt.Add(time.Minute)),
	)
	if !errors.Is(err, domainsandbox.ErrHealthMonitorOutbox) ||
		result.Claimed != 1 ||
		result.Projected != 0 {
		t.Fatalf("blocked unhealthy projection = %#v, %v", result, err)
	}
	unhealthyEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":1:unhealthy"
	recoveredEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":1:recovered"
	var recoveryProjection providerHealthNotificationProjectionPO
	if err := db.Where("event_id = ?", recoveredEventID).
		Take(&recoveryProjection).Error; err != nil {
		t.Fatalf("read blocked recovery projection: %v", err)
	}
	if recoveryProjection.Status != healthProjectionStatusPending ||
		recoveryProjection.LeaseToken != "" ||
		recoveryProjection.AttemptCount != 0 {
		t.Fatalf(
			"recovery was claimed before unhealthy projected: %#v",
			recoveryProjection,
		)
	}

	restarted := NewMySQLHealthMonitorRepository(db, outbox)
	result, err = restarted.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", recoveryAt.Add(time.Minute+10*time.Second)),
	)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("unhealthy retry projection = %#v, %v", result, err)
	}
	assertHealthProjectionStatus(
		t,
		db,
		unhealthyEventID,
		healthProjectionStatusProjected,
	)
	assertHealthProjectionStatus(
		t,
		db,
		recoveredEventID,
		healthProjectionStatusPending,
	)
	result, err = restarted.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", recoveryAt.Add(time.Minute+11*time.Second)),
	)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("recovery projection after unhealthy = %#v, %v", result, err)
	}
	assertHealthNotificationOutbox(
		t,
		db,
		2,
		domainnotification.EventSystemProviderRecovered,
		provider,
	)
}

func TestMySQLHealthProjectionOrdersIncidentsIndependently(t *testing.T) {
	ctx := context.Background()
	db, _, outbox := newHealthMonitorRepositoryTest(t)
	base := time.Date(2026, 7, 25, 17, 0, 0, 0, time.UTC)
	rows := []providerHealthNotificationProjectionPO{
		healthProjectionTestRow(9101, 1, domainsandbox.HealthIncidentNotificationUnhealthy, base),
		healthProjectionTestRow(9101, 1, domainsandbox.HealthIncidentNotificationRecovered, base.Add(time.Minute)),
		healthProjectionTestRow(9102, 1, domainsandbox.HealthIncidentNotificationUnhealthy, base),
		healthProjectionTestRow(9102, 1, domainsandbox.HealthIncidentNotificationRecovered, base.Add(time.Minute)),
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed independent health projections: %v", err)
	}
	failingEventID := "sandbox-health:9101:1:unhealthy"
	repository := NewMySQLHealthMonitorRepository(
		db,
		selectiveHealthNotificationOutbox{
			delegate: outbox,
			failEvents: map[string]error{
				failingEventID: errors.New("temporary incident A outbox failure"),
			},
		},
	)
	result, err := repository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-a", base.Add(2*time.Minute)),
	)
	if !errors.Is(err, domainsandbox.ErrHealthMonitorOutbox) ||
		result.Claimed != 2 ||
		result.Projected != 1 {
		t.Fatalf("first independent projection = %#v, %v", result, err)
	}
	result, err = repository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-a", base.Add(2*time.Minute+time.Second)),
	)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("incident B recovery projection = %#v, %v", result, err)
	}
	assertNotificationOutboxEventCount(t, db, "sandbox-health:9102:1:unhealthy", 1)
	assertNotificationOutboxEventCount(t, db, "sandbox-health:9102:1:recovered", 1)
	assertNotificationOutboxEventCount(t, db, failingEventID, 0)
	assertNotificationOutboxEventCount(t, db, "sandbox-health:9101:1:recovered", 0)

	restarted := NewMySQLHealthMonitorRepository(db, outbox)
	result, err = restarted.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", base.Add(2*time.Minute+10*time.Second)),
	)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("incident A unhealthy retry = %#v, %v", result, err)
	}
	result, err = restarted.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest("projector-b", base.Add(2*time.Minute+11*time.Second)),
	)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("incident A recovery retry = %#v, %v", result, err)
	}
	assertNotificationOutboxEventCount(t, db, failingEventID, 1)
	assertNotificationOutboxEventCount(t, db, "sandbox-health:9101:1:recovered", 1)
}

func TestMySQLHealthMonitorExpiredLeaseCanBeReclaimedByAnotherInstance(t *testing.T) {
	ctx := context.Background()
	db, providerRepository, outbox := newHealthMonitorRepositoryTest(t)
	provider := createEnabledHealthMonitorProvider(
		t,
		ctx,
		providerRepository,
		"health-monitor-replay",
	)
	repositoryA := NewMySQLHealthMonitorRepository(db, outbox)
	repositoryB := NewMySQLHealthMonitorRepository(db, outbox)
	base := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	databaseNow := base
	databaseClock := func(
		context.Context,
		*gorm.DB,
		time.Time,
	) (time.Time, error) {
		return databaseNow, nil
	}
	repositoryA.databaseClock = databaseClock
	repositoryB.databaseClock = databaseClock
	first := claimHealthMonitorProvider(t, ctx, repositoryA, "worker-a", base)

	databaseNow = base.Add(30 * time.Second)
	if claims, err := repositoryB.ClaimHealthChecks(ctx, domainsandbox.HealthMonitorClaimRequest{
		WorkerID: "worker-b",
		Now: base.Add(30 * time.Second),
		Lease: time.Minute,
		Limit: 1,
	}); err != nil || len(claims) != 0 {
		t.Fatalf("claim before lease expiry = %#v, %v", claims, err)
	}

	var providerBefore providerPO
	if err := db.Where("id = ?", provider.ID).Take(&providerBefore).Error; err != nil {
		t.Fatalf("read provider before expired completion: %v", err)
	}
	var episodeBefore providerHealthEpisodePO
	if err := db.Where("provider_id = ?", provider.ID).Take(&episodeBefore).Error; err != nil {
		t.Fatalf("read episode before expired completion: %v", err)
	}
	var outboxCountBefore int64
	if err := db.Table("notification_outbox").Count(&outboxCountBefore).Error; err != nil {
		t.Fatalf("count outbox before expired completion: %v", err)
	}

	databaseNow = first.LeaseExpiresAt
	_, err := repositoryA.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: first,
			Health: unhealthyHealthSnapshot(base.Add(30 * time.Second)),
			CheckedAt: base.Add(30 * time.Second),
			NextCheckAt: base.Add(90 * time.Second),
			FailureThreshold: 1,
		},
	)
	if !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("expired CompleteHealthCheck() error = %v", err)
	}

	var providerAfter providerPO
	if err := db.Where("id = ?", provider.ID).Take(&providerAfter).Error; err != nil {
		t.Fatalf("read provider after expired completion: %v", err)
	}
	var episodeAfter providerHealthEpisodePO
	if err := db.Where("provider_id = ?", provider.ID).Take(&episodeAfter).Error; err != nil {
		t.Fatalf("read episode after expired completion: %v", err)
	}
	var outboxCountAfter int64
	if err := db.Table("notification_outbox").Count(&outboxCountAfter).Error; err != nil {
		t.Fatalf("count outbox after expired completion: %v", err)
	}
	if providerAfter.Version != providerBefore.Version ||
		providerAfter.LastHealthAt != providerBefore.LastHealthAt {
		t.Fatalf(
			"expired claim changed provider health: before=%#v after=%#v",
			providerBefore,
			providerAfter,
		)
	}
	if episodeAfter != episodeBefore {
		t.Fatalf(
			"expired claim changed episode: before=%#v after=%#v",
			episodeBefore,
			episodeAfter,
		)
	}
	if outboxCountAfter != outboxCountBefore {
		t.Fatalf(
			"expired claim changed outbox count: before=%d after=%d",
			outboxCountBefore,
			outboxCountAfter,
		)
	}

	replayed := claimHealthMonitorProvider(t, ctx, repositoryB, "worker-b", base.Add(time.Minute))
	if replayed.LeaseOwner != "worker-b" || replayed.LeaseToken == first.LeaseToken {
		t.Fatalf("replayed lease owner = %q", replayed.LeaseOwner)
	}
	databaseNow = base.Add(time.Minute + time.Millisecond)
	completion, err := repositoryB.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: replayed,
			Health: unhealthyHealthSnapshot(databaseNow),
			CheckedAt: databaseNow,
			NextCheckAt: databaseNow.Add(time.Minute),
			FailureThreshold: 1,
		},
	)
	if err != nil {
		t.Fatalf("new owner CompleteHealthCheck() error = %v", err)
	}
	if !completion.Applied ||
		completion.Notification != domainsandbox.HealthIncidentNotificationUnhealthy {
		t.Fatalf("new owner completion = %#v", completion)
	}
	unhealthyEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":1:unhealthy"
	assertHealthProjectionStatus(
		t,
		db,
		unhealthyEventID,
		healthProjectionStatusPending,
	)
	assertNotificationOutboxEventCount(t, db, unhealthyEventID, 0)
	projectPendingHealthNotifications(
		t,
		ctx,
		repositoryB,
		"projector-b",
		databaseNow.Add(time.Second),
		1,
	)
	assertHealthProjectionStatus(
		t,
		db,
		unhealthyEventID,
		healthProjectionStatusProjected,
	)
	assertHealthNotificationOutbox(
		t,
		db,
		1,
		domainnotification.EventSystemProviderUnavailable,
		provider,
	)
}

func TestMySQLHealthMonitorDatabaseClockFailureIsSanitized(t *testing.T) {
	ctx := context.Background()
	db, providerRepository, outbox := newHealthMonitorRepositoryTest(t)
	provider := createEnabledHealthMonitorProvider(
		t,
		ctx,
		providerRepository,
		"health-monitor-clock-failure",
	)
	repository := NewMySQLHealthMonitorRepository(db, outbox)
	base := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	rawClockError := errors.New(
		"raw database error with SQL, DSN, endpoint token and provider body",
	)
	failClock := false
	repository.databaseClock = func(
		context.Context,
		*gorm.DB,
		time.Time,
	) (time.Time, error) {
		if failClock {
			return time.Time{}, rawClockError
		}
		return base, nil
	}

	claim := claimHealthMonitorProvider(t, ctx, repository, "worker-a", base)
	var providerBefore providerPO
	if err := db.Where("id = ?", provider.ID).Take(&providerBefore).Error; err != nil {
		t.Fatalf("read provider before clock failure: %v", err)
	}
	var episodeBefore providerHealthEpisodePO
	if err := db.Where("provider_id = ?", provider.ID).Take(&episodeBefore).Error; err != nil {
		t.Fatalf("read episode before clock failure: %v", err)
	}
	var outboxCountBefore int64
	if err := db.Table("notification_outbox").Count(&outboxCountBefore).Error; err != nil {
		t.Fatalf("count outbox before clock failure: %v", err)
	}

	failClock = true
	_, err := repository.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: claim,
			Health: unhealthyHealthSnapshot(base),
			CheckedAt: base,
			NextCheckAt: base.Add(time.Minute),
			FailureThreshold: 1,
		},
	)
	if !errors.Is(err, ErrHealthMonitorInfrastructure) {
		t.Fatalf("CompleteHealthCheck(clock failure) error = %v", err)
	}
	if errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("clock failure was misclassified as version conflict: %v", err)
	}
	for _, prohibited := range []string{
		"SQL",
		"DSN",
		"endpoint token",
		"provider body",
	} {
		if strings.Contains(err.Error(), prohibited) {
			t.Fatalf("database clock error leaked %q: %v", prohibited, err)
		}
	}

	var providerAfter providerPO
	if err := db.Where("id = ?", provider.ID).Take(&providerAfter).Error; err != nil {
		t.Fatalf("read provider after clock failure: %v", err)
	}
	var episodeAfter providerHealthEpisodePO
	if err := db.Where("provider_id = ?", provider.ID).Take(&episodeAfter).Error; err != nil {
		t.Fatalf("read episode after clock failure: %v", err)
	}
	var outboxCountAfter int64
	if err := db.Table("notification_outbox").Count(&outboxCountAfter).Error; err != nil {
		t.Fatalf("count outbox after clock failure: %v", err)
	}
	sameLastHealthAt := providerBefore.LastHealthAt == nil &&
		providerAfter.LastHealthAt == nil
	if providerBefore.LastHealthAt != nil && providerAfter.LastHealthAt != nil {
		sameLastHealthAt = providerBefore.LastHealthAt.Equal(*providerAfter.LastHealthAt)
	}
	if providerAfter.Version != providerBefore.Version || !sameLastHealthAt {
		t.Fatalf(
			"clock failure changed provider health: before=%#v after=%#v",
			providerBefore,
			providerAfter,
		)
	}
	if episodeAfter != episodeBefore {
		t.Fatalf(
			"clock failure changed episode: before=%#v after=%#v",
			episodeBefore,
			episodeAfter,
		)
	}
	if outboxCountAfter != outboxCountBefore {
		t.Fatalf("database clock error leaked raw details: %v", err)
	}
}

func newHealthMonitorRepositoryTest(
	t *testing.T,
) (*gorm.DB, *MySQLRepository, *infranotification.MySQLRepository) {
	t.Helper()
	providerRepository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(
		&providerHealthEpisodePO{},
		&providerHealthNotificationProjectionPO{},
	); err != nil {
		t.Fatalf("migrate health monitor state: %v", err)
	}
	if err := db.Exec(`
CREATE TABLE notification_outbox (
  id INTEGER PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  event_type TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  aggregate_version INTEGER NOT NULL,
  occurred_at INTEGER NOT NULL,
  space_id INTEGER NOT NULL DEFAULT 0,
  actor_id INTEGER NOT NULL DEFAULT 0,
  recipient_policy TEXT NOT NULL,
  payload_schema INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  available_at INTEGER NOT NULL,
  locked_at INTEGER NOT NULL DEFAULT 0,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error_code TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  delivered_at INTEGER NOT NULL DEFAULT 0
)`).Error; err != nil {
		t.Fatalf("create notification outbox: %v", err)
	}
	outbox := infranotification.NewMySQLRepository(
		db,
		&healthMonitorIDGenerator{next: 10_000},
	)
	return db, providerRepository, outbox
}

func healthProjectionRequest(
	workerID string,
	now time.Time,
) domainsandbox.HealthNotificationProjectionRequest {
	return domainsandbox.HealthNotificationProjectionRequest{
		WorkerID: workerID,
		Now: now,
		Lease: time.Minute,
		Limit: 4,
	}
}

func projectPendingHealthNotifications(
	t *testing.T,
	ctx context.Context,
	repository *MySQLHealthMonitorRepository,
	workerID string,
	now time.Time,
	want int,
) {
	t.Helper()
	result, err := repository.ProjectPendingHealthNotifications(
		ctx,
		healthProjectionRequest(workerID, now),
	)
	if err != nil || result.Claimed != want || result.Projected != want {
		t.Fatalf(
			"ProjectPendingHealthNotifications() = %#v, %v, want %d projected",
			result,
			err,
			want,
		)
	}
}

func assertHealthProjectionStatus(
	t *testing.T,
	db *gorm.DB,
	eventID string,
	wantStatus string,
) {
	t.Helper()
	var projection providerHealthNotificationProjectionPO
	if err := db.Where("event_id = ?", eventID).Take(&projection).Error; err != nil {
		t.Fatalf("read health projection %q: %v", eventID, err)
	}
	if projection.Status != wantStatus {
		t.Fatalf(
			"health projection %q status = %q, want %q",
			eventID,
			projection.Status,
			wantStatus,
		)
	}
}

func healthProjectionTestRow(
	providerID uint64,
	incidentSequence uint64,
	notification domainsandbox.HealthIncidentNotification,
	occurredAt time.Time,
) providerHealthNotificationProjectionPO {
	suffix := string(notification)
	incidentID := "sandbox-incident-" +
		strconv.FormatUint(providerID, 10) +
		"-" +
		strconv.FormatUint(incidentSequence, 10)
	return providerHealthNotificationProjectionPO{
		EventID: "sandbox-health:" +
			strconv.FormatUint(providerID, 10) +
			":" +
			strconv.FormatUint(incidentSequence, 10) +
			":" +
			suffix,
		ProviderID: providerID,
		IncidentID: incidentID,
		IncidentSequence: incidentSequence,
		NotificationType: string(notification),
		OccurredAt: occurredAt.UnixMilli(),
		Status: healthProjectionStatusPending,
		NextAttemptAt: occurredAt.UnixMilli(),
		Version: domainsandbox.InitialVersion,
		CreatedAt: occurredAt.UnixMilli(),
		UpdatedAt: occurredAt.UnixMilli(),
	}
}

func assertNotificationOutboxEventCount(
	t *testing.T,
	db *gorm.DB,
	eventID string,
	want int64,
) {
	t.Helper()
	var count int64
	if err := db.Table("notification_outbox").
		Where("event_id = ?", eventID).
		Count(&count).Error; err != nil {
		t.Fatalf("count notification outbox event %q: %v", eventID, err)
	}
	if count != want {
		t.Fatalf(
			"notification outbox event %q count = %d, want %d",
			eventID,
			count,
			want,
		)
	}
}

func createEnabledHealthMonitorProvider(
	t *testing.T,
	ctx context.Context,
	repository *MySQLRepository,
	key string,
) *domainsandbox.Provider {
	t.Helper()
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput(key))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	version, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: provider.ID,
		ExpectedVersion: provider.Version,
		Status: domainsandbox.ProviderStatusEnabled,
		ActorUserID: 9001,
	})
	if err != nil {
		t.Fatalf("UpdateProviderStatus() error = %v", err)
	}
	provider.Version = version
	provider.Status = domainsandbox.ProviderStatusEnabled
	return provider
}

func claimHealthMonitorProvider(
	t *testing.T,
	ctx context.Context,
	repository *MySQLHealthMonitorRepository,
	workerID string,
	now time.Time,
) domainsandbox.HealthMonitorClaim {
	t.Helper()
	claims, err := repository.ClaimHealthChecks(ctx, domainsandbox.HealthMonitorClaimRequest{
		WorkerID: workerID,
		Now: now,
		Lease: time.Minute,
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ClaimHealthChecks() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("ClaimHealthChecks() claims = %#v", claims)
	}
	return claims[0]
}

func completeHealthMonitorCheck(
	t *testing.T,
	ctx context.Context,
	repository *MySQLHealthMonitorRepository,
	claim domainsandbox.HealthMonitorClaim,
	health domainsandbox.HealthSnapshot,
	nextCheckAt time.Time,
) domainsandbox.HealthMonitorCheckCompletion {
	t.Helper()
	completion, err := repository.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: claim,
			Health: health,
			CheckedAt: health.CheckedAt,
			NextCheckAt: nextCheckAt,
			FailureThreshold: 3,
		},
	)
	if err != nil {
		t.Fatalf("CompleteHealthCheck() error = %v", err)
	}
	return completion
}

func unhealthyHealthSnapshot(checkedAt time.Time) domainsandbox.HealthSnapshot {
	return domainsandbox.HealthSnapshot{
		Status: domainsandbox.HealthStatusUnhealthy,
		Capabilities: []domainsandbox.Scope{},
		ReasonCode: "UNHEALTHY",
		Message: "health probe reported unhealthy",
		CheckedAt: checkedAt,
	}
}

func healthyHealthSnapshot(checkedAt time.Time) domainsandbox.HealthSnapshot {
	return domainsandbox.HealthSnapshot{
		Status: domainsandbox.HealthStatusHealthy,
		Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		ReasonCode: "HEALTHY",
		CheckedAt: checkedAt,
	}
}

func assertHealthNotificationOutbox(
	t *testing.T,
	db *gorm.DB,
	wantCount int64,
	wantLatestType domainnotification.EventType,
	provider *domainsandbox.Provider,
) {
	t.Helper()
	var count int64
	if err := db.Table("notification_outbox").Count(&count).Error; err != nil {
		t.Fatalf("count notification outbox: %v", err)
	}
	if count != wantCount {
		t.Fatalf("notification outbox count = %d, want %d", count, wantCount)
	}
	var row struct {
		EventID          string `gorm:"column:event_id"`
		EventType       string `gorm:"column:event_type"`
		AggregateType   string `gorm:"column:aggregate_type"`
		AggregateID     string `gorm:"column:aggregate_id"`
		AggregateVersion int64 `gorm:"column:aggregate_version"`
		SpaceID          int64 `gorm:"column:space_id"`
		ActorID          int64 `gorm:"column:actor_id"`
		RecipientPolicy string `gorm:"column:recipient_policy"`
		PayloadJSON     string `gorm:"column:payload_json"`
	}
	if err := db.Table("notification_outbox").
		Order("id DESC").
		Take(&row).Error; err != nil {
		t.Fatalf("read notification outbox: %v", err)
	}
	if row.EventType != string(wantLatestType) ||
		row.RecipientPolicy != string(domainnotification.RecipientSystemAdmins) {
		t.Fatalf("notification route = %#v", row)
	}
	incidentSequence := int64(1)
	if wantCount >= 3 {
		incidentSequence = 2
	}
	suffix := "unhealthy"
	wantAggregateVersion := int64(1)
	if wantLatestType == domainnotification.EventSystemProviderRecovered {
		suffix = "recovered"
		wantAggregateVersion = 2
	}
	wantIncidentID := "sandbox-incident-" +
		providerIDText(provider.ID) +
		"-" +
		strconv.FormatInt(incidentSequence, 10)
	wantEventID := "sandbox-health:" +
		providerIDText(provider.ID) +
		":" +
		strconv.FormatInt(incidentSequence, 10) +
		":" +
		suffix
	if row.EventID != wantEventID ||
		row.AggregateType != "sandbox_provider_health_incident" ||
		row.AggregateID != wantIncidentID ||
		row.AggregateVersion != wantAggregateVersion ||
		row.SpaceID != 0 ||
		row.ActorID != 0 {
		t.Fatalf("notification incident envelope = %#v", row)
	}
	if wantCount >= 2 {
		var incidentRows []struct {
			AggregateID      string `gorm:"column:aggregate_id"`
			AggregateVersion int64  `gorm:"column:aggregate_version"`
		}
		if err := db.Table("notification_outbox").
			Order("id ASC").
			Limit(int(wantCount)).
			Find(&incidentRows).Error; err != nil {
			t.Fatalf("read notification incident envelopes: %v", err)
		}
		if len(incidentRows) < 2 ||
			incidentRows[0].AggregateID != incidentRows[1].AggregateID ||
			incidentRows[0].AggregateVersion != 1 ||
			incidentRows[1].AggregateVersion != 2 {
			t.Fatalf("unhealthy/recovery aggregate versions = %#v", incidentRows)
		}
		if wantCount >= 3 &&
			(incidentRows[2].AggregateID == incidentRows[0].AggregateID ||
				incidentRows[2].AggregateVersion != 1) {
			t.Fatalf("new incident aggregate = %#v", incidentRows)
		}
	}
	for _, prohibited := range []string{
		provider.EndpointSecret,
		provider.CredentialSecret,
		provider.EndpointHint,
		"token",
		"credential",
		"provider_body",
		"raw provider body",
		"stack",
	} {
		if prohibited != "" && strings.Contains(
			strings.ToLower(row.PayloadJSON),
			strings.ToLower(prohibited),
		) {
			t.Fatalf("notification payload leaked %q: %s", prohibited, row.PayloadJSON)
		}
	}
}

func providerIDText(providerID int64) string {
	return strconv.FormatInt(providerID, 10)
}
