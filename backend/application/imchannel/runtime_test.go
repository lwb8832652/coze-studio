// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	"github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

func TestOfficialChannelFactoryConfiguresEventDispatcher(t *testing.T) {
	feishuChannel, err := (OfficialChannelFactory{}).New(
		&domain.Config{AppID: "cli_test"},
		"test-secret",
	)
	if err != nil {
		t.Fatalf("create official Feishu channel: %v", err)
	}

	channelValue := reflect.ValueOf(feishuChannel)
	if channelValue.Kind() != reflect.Pointer || channelValue.IsNil() {
		t.Fatalf("unexpected channel value: %T", feishuChannel)
	}
	wsClient := channelValue.Elem().FieldByName("wsClient")
	if !wsClient.IsValid() || wsClient.IsNil() {
		t.Fatal("official Feishu channel has no WebSocket client")
	}
	eventHandler := wsClient.Elem().FieldByName("eventHandler")
	if !eventHandler.IsValid() || eventHandler.IsNil() {
		t.Fatal("official Feishu WebSocket client has no event dispatcher")
	}
}

func TestRuntimeFailureNotificationRequiresStableThreshold(t *testing.T) {
	repo := newRuntimeRepositoryStub()
	manager := newRuntimeTestManager(repo)
	ctx := context.Background()

	manager.recordRuntimeFailure(ctx, 10, errors.New("temporary reconnect 1"))
	manager.recordRuntimeFailure(ctx, 10, errors.New("temporary reconnect 2"))

	if len(repo.notifications) != 0 {
		t.Fatalf("transient runtime failures should not notify, got %d", len(repo.notifications))
	}

	manager.recordRuntimeFailure(ctx, 10, errors.New("stable reconnect failure"))
	manager.recordRuntimeFailure(ctx, 10, errors.New("duplicate callback"))

	if len(repo.notifications) != 1 {
		t.Fatalf("stable incident should notify once, got %d", len(repo.notifications))
	}
	event := repo.notifications[0]
	if event.EventType != notification.EventIMChannelConnectionFailed {
		t.Fatalf("unexpected notification event type: %s", event.EventType)
	}
	wantRecipients := []int64{2, 7, 9}
	if !reflect.DeepEqual(event.Payload.ExplicitRecipientIDs, wantRecipients) {
		t.Fatalf("recipients = %v, want %v", event.Payload.ExplicitRecipientIDs, wantRecipients)
	}
	if event.Payload.ResourceDisplayName == "cli_secret" ||
		event.Payload.TargetID == "raw-provider-payload" {
		t.Fatalf("notification metadata contains sensitive provider data: %#v", event.Payload)
	}
}

func TestRuntimeFailureNotificationUsesCustomThreshold(t *testing.T) {
	repo := newRuntimeRepositoryStub()
	manager, err := NewRuntimeManager(
		repo,
		&runtimeIDGenStub{},
		nil,
		OfficialChannelFactory{},
		&AgentRunner{},
		WithRuntimeStableFailureThreshold(2),
	)
	if err != nil {
		t.Fatalf("create runtime manager: %v", err)
	}
	ctx := context.Background()

	manager.recordRuntimeFailure(ctx, 10, errors.New("temporary reconnect"))
	if len(repo.notifications) != 0 {
		t.Fatalf("first failure should not notify with threshold 2, got %d", len(repo.notifications))
	}
	manager.recordRuntimeFailure(ctx, 10, errors.New("stable reconnect failure"))
	if len(repo.notifications) != 1 {
		t.Fatalf("second failure should notify with threshold 2, got %d", len(repo.notifications))
	}
}

func TestRuntimeStableFailureThresholdFromEnv(t *testing.T) {
	threshold := RuntimeStableFailureThresholdFromEnv(func(key string) string {
		if key == RuntimeStableFailureThresholdEnv {
			return "5"
		}
		return ""
	})
	if threshold != 5 {
		t.Fatalf("threshold from env = %d, want 5", threshold)
	}

	defaulted := RuntimeStableFailureThresholdFromEnv(func(string) string {
		return "not-a-number"
	})
	if defaulted != domain.DefaultRuntimeStableFailureThreshold {
		t.Fatalf("invalid env threshold = %d, want default %d", defaulted, domain.DefaultRuntimeStableFailureThreshold)
	}
}

func TestRuntimeStableFailureThresholdForComponents(t *testing.T) {
	if got := runtimeStableFailureThresholdForComponents(&Components{RuntimeStableFailureThreshold: 6}); got != 6 {
		t.Fatalf("component threshold = %d, want 6", got)
	}
	if got := runtimeStableFailureThresholdForComponents(&Components{}); got != domain.DefaultRuntimeStableFailureThreshold {
		t.Fatalf("empty component threshold = %d, want default %d", got, domain.DefaultRuntimeStableFailureThreshold)
	}
}

func TestRuntimeTransientReconnectDoesNotNotifyRecovery(t *testing.T) {
	repo := newRuntimeRepositoryStub()
	manager := newRuntimeTestManager(repo)
	ctx := context.Background()

	manager.recordRuntimeFailure(ctx, 10, errors.New("temporary reconnect 1"))
	manager.recordRuntimeFailure(ctx, 10, errors.New("temporary reconnect 2"))
	now := time.Unix(200, 0)
	manager.recordRuntimeRecovery(ctx, 10, domain.RuntimeState{
		Status:      domain.RuntimeStatusConnected,
		ConnectedAt: &now,
	})

	if len(repo.notifications) != 0 {
		t.Fatalf("transient reconnect should not notify, got %d", len(repo.notifications))
	}
	if repo.config.RuntimeConsecutiveFailures != 0 || repo.config.RuntimeIncidentID != "" {
		t.Fatalf("transient recovery did not clear incident state: %#v", repo.config)
	}
}

func TestRuntimeRecoveryNotifiesOnceForStableIncident(t *testing.T) {
	repo := newRuntimeRepositoryStub()
	manager := newRuntimeTestManager(repo)
	ctx := context.Background()

	manager.recordRuntimeFailure(ctx, 10, errors.New("failure 1"))
	manager.recordRuntimeFailure(ctx, 10, errors.New("failure 2"))
	manager.recordRuntimeFailure(ctx, 10, errors.New("failure 3"))
	now := time.Unix(300, 0)
	state := domain.RuntimeState{Status: domain.RuntimeStatusConnected, ConnectedAt: &now}
	manager.recordRuntimeRecovery(ctx, 10, state)
	manager.recordRuntimeRecovery(ctx, 10, state)

	if len(repo.notifications) != 2 {
		t.Fatalf("stable failure and recovery should notify once each, got %d", len(repo.notifications))
	}
	if repo.notifications[1].EventType != notification.EventIMChannelRecovered {
		t.Fatalf("unexpected recovery event type: %s", repo.notifications[1].EventType)
	}
}

func TestRuntimeDeadLettersOnlyOnFinalRetry(t *testing.T) {
	repo := newRuntimeRepositoryStub()
	manager := newRuntimeTestManager(repo)
	channel := &runtimeChannelStub{}
	active := &activeChannel{config: repo.config, channel: channel}
	ctx := context.Background()

	manager.failEvent(ctx, active, &domain.Event{
		ID:           100,
		ConfigID:     10,
		EventKey:     "event-100",
		PayloadJSON:  `{"chat_id":"chat-1","message_id":"msg-1","content":"hello"}`,
		AttemptCount: domain.EventMaxAttempts - 2,
	}, errors.New("intermediate retry"))

	if len(repo.notifications) != 0 || repo.failedEvents != 1 {
		t.Fatalf("intermediate retry should only mark failed, notifications=%d failed=%d", len(repo.notifications), repo.failedEvents)
	}

	manager.failEvent(ctx, active, &domain.Event{
		ID:           101,
		ConfigID:     10,
		EventKey:     "event-101",
		PayloadJSON:  `{"chat_id":"chat-1","message_id":"msg-2","content":"do not expose"}`,
		AttemptCount: domain.EventMaxAttempts - 1,
	}, errors.New("final retry exhausted"))
	manager.failEvent(ctx, active, &domain.Event{
		ID:           101,
		ConfigID:     10,
		EventKey:     "event-101",
		PayloadJSON:  `{"chat_id":"chat-1","message_id":"msg-2","content":"do not expose"}`,
		AttemptCount: domain.EventMaxAttempts - 1,
	}, errors.New("duplicate final callback"))

	if len(repo.notifications) != 1 {
		t.Fatalf("dead-letter notification should be idempotent, got %d", len(repo.notifications))
	}
	if repo.notifications[0].EventType != notification.EventIMMessageDeadLettered {
		t.Fatalf("unexpected dead-letter event type: %s", repo.notifications[0].EventType)
	}
	if len(channel.sent) != 1 {
		t.Fatalf("user failure reply should be sent once, got %d", len(channel.sent))
	}
}

func TestRuntimeDuplicateInboundCallbackDoesNotCreateDuplicateEvent(t *testing.T) {
	repo := newRuntimeRepositoryStub()
	manager := newRuntimeTestManager(repo)
	ctx := context.Background()
	message := &channeltypes.NormalizedMessage{
		EventID:   "event-duplicate",
		MessageID: "message-duplicate",
		ChatID:    "chat-1",
		ChatType:  "p2p",
		UserID:    "ou-1",
		Content:   "hello",
	}

	if err := manager.persistInbound(ctx, repo.config, message); err != nil {
		t.Fatalf("persist first inbound: %v", err)
	}
	if err := manager.persistInbound(ctx, repo.config, message); err != nil {
		t.Fatalf("persist duplicate inbound: %v", err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("duplicate inbound callback inserted %d events", len(repo.events))
	}
}

func TestSafeRuntimeErrorRedactsSecretsAndProviderPayloads(t *testing.T) {
	message := safeRuntimeError(errors.New("app_secret=super-secret provider_body={raw_event_payload}"))
	for _, forbidden := range []string{"super-secret", "app_secret", "provider_body", "raw_event_payload"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("safe runtime error leaked %q in %q", forbidden, message)
		}
	}
}

type runtimeRepositoryStub struct {
	config        *domain.Config
	events        map[string]*domain.Event
	deadLetters   map[int64]struct{}
	notifications []notification.Event
	failedEvents  int
}

func newRuntimeRepositoryStub() *runtimeRepositoryStub {
	return &runtimeRepositoryStub{
		config: &domain.Config{
			ID:        10,
			SpaceID:   20,
			CreatorID: 7,
			UpdatedBy: 2,
			AgentID:   30,
			Name:      "飞书客服机器人",
			Enabled:   true,
			Version:   1,
		},
		events:      make(map[string]*domain.Event),
		deadLetters: make(map[int64]struct{}),
	}
}

func newRuntimeTestManager(repo *runtimeRepositoryStub) *RuntimeManager {
	return &RuntimeManager{
		repository: repo,
		idGen:      &runtimeIDGenStub{},
		owner:      "runtime-test",
		wake:       make(chan struct{}, 1),
		active:     make(map[int64]*activeChannel),
		processing: make(map[int64]bool),
		retryAfter: make(map[int64]time.Time),
	}
}

func (r *runtimeRepositoryStub) ListConfigs(context.Context, int64) ([]*domain.Config, error) {
	return []*domain.Config{r.config}, nil
}

func (r *runtimeRepositoryStub) GetConfig(context.Context, int64, int64) (*domain.Config, error) {
	return r.config, nil
}

func (r *runtimeRepositoryStub) GetConfigByID(context.Context, int64) (*domain.Config, error) {
	return r.config, nil
}

func (r *runtimeRepositoryStub) CreateConfig(context.Context, *domain.Config) error { return nil }

func (r *runtimeRepositoryStub) UpdateConfig(context.Context, *domain.Config) error { return nil }

func (r *runtimeRepositoryStub) SetEnabled(context.Context, int64, int64, int64, bool, domain.RuntimeStatus) error {
	return nil
}

func (r *runtimeRepositoryStub) DeleteConfig(context.Context, int64, int64, int64) error {
	return nil
}

func (r *runtimeRepositoryStub) MarkConnectionTest(context.Context, int64, string, string, time.Time) error {
	return nil
}

func (r *runtimeRepositoryStub) ListEnabledConfigs(context.Context) ([]*domain.Config, error) {
	return []*domain.Config{r.config}, nil
}

func (r *runtimeRepositoryStub) TryAcquireRuntimeLease(context.Context, int64, string, time.Time, time.Time) (bool, error) {
	return true, nil
}

func (r *runtimeRepositoryStub) RenewRuntimeLease(context.Context, int64, string, time.Time) (bool, error) {
	return true, nil
}

func (r *runtimeRepositoryStub) ReleaseRuntimeLease(context.Context, int64, string) error { return nil }

func (r *runtimeRepositoryStub) UpdateRuntimeState(_ context.Context, _ int64, state domain.RuntimeState) error {
	r.config.RuntimeStatus = state.Status
	r.config.RuntimeError = state.Error
	return nil
}

func (r *runtimeRepositoryStub) RecordRuntimeFailure(_ context.Context, _ int64, state domain.RuntimeState, threshold int) error {
	if threshold <= 0 {
		threshold = domain.DefaultRuntimeStableFailureThreshold
	}
	r.config.RuntimeStatus = state.Status
	r.config.RuntimeError = state.Error
	r.config.RuntimeConsecutiveFailures++
	if r.config.RuntimeIncidentID == "" {
		r.config.RuntimeIncidentID = state.IncidentID
		r.config.RuntimeRecoveryNotifiedAt = nil
		r.config.RuntimeLastRecoveredAt = nil
	}
	if int(r.config.RuntimeConsecutiveFailures) >= threshold &&
		r.config.RuntimeIncidentNotifiedAt == nil {
		now := time.Now()
		r.config.RuntimeIncidentNotifiedAt = &now
		r.notifications = append(r.notifications, notification.Event{
			EventID:          "im_channel:" + r.config.RuntimeIncidentID + ":failed",
			EventType:        notification.EventIMChannelConnectionFailed,
			AggregateType:    "im_channel",
			AggregateID:      r.config.RuntimeIncidentID,
			AggregateVersion: 1,
			OccurredAt:       now,
			ActorID:          r.config.UpdatedBy,
			SpaceID:          r.config.SpaceID,
			RecipientPolicy:  notification.RecipientExplicitInternalUsers,
			PayloadSchema:    notification.CurrentPayloadSchema,
			Payload: notification.EventPayload{
				ResourceDisplayName:  r.config.Name,
				StatusReasonCode:     notification.StatusReasonConnectionFailed,
				TargetID:             "20",
				ExplicitRecipientIDs: []int64{2, 7, 9},
			},
		})
	}
	return nil
}

func (r *runtimeRepositoryStub) RecordRuntimeRecovery(_ context.Context, _ int64, state domain.RuntimeState) error {
	shouldNotify := r.config.RuntimeIncidentID != "" &&
		r.config.RuntimeIncidentNotifiedAt != nil &&
		r.config.RuntimeRecoveryNotifiedAt == nil
	incidentID := r.config.RuntimeIncidentID
	r.config.RuntimeStatus = state.Status
	r.config.RuntimeError = ""
	r.config.RuntimeConsecutiveFailures = 0
	r.config.RuntimeIncidentID = ""
	r.config.RuntimeIncidentNotifiedAt = nil
	now := time.Now()
	r.config.RuntimeLastRecoveredAt = &now
	if shouldNotify {
		r.config.RuntimeRecoveryNotifiedAt = &now
		r.notifications = append(r.notifications, notification.Event{
			EventID:          "im_channel:" + incidentID + ":recovered",
			EventType:        notification.EventIMChannelRecovered,
			AggregateType:    "im_channel",
			AggregateID:      incidentID,
			AggregateVersion: 1,
			OccurredAt:       now,
			ActorID:          r.config.UpdatedBy,
			SpaceID:          r.config.SpaceID,
			RecipientPolicy:  notification.RecipientExplicitInternalUsers,
			PayloadSchema:    notification.CurrentPayloadSchema,
			Payload: notification.EventPayload{
				ResourceDisplayName:  r.config.Name,
				TargetID:             "20",
				ExplicitRecipientIDs: []int64{2, 7, 9},
			},
		})
	}
	return nil
}

func (r *runtimeRepositoryStub) InsertEvent(_ context.Context, event *domain.Event) (bool, error) {
	if _, exists := r.events[event.EventKey]; exists {
		return false, nil
	}
	r.events[event.EventKey] = event
	return true, nil
}

func (r *runtimeRepositoryStub) ClaimEvents(context.Context, int64, string, time.Time, time.Time, int) ([]*domain.Event, error) {
	return nil, nil
}

func (r *runtimeRepositoryStub) CompleteEvent(context.Context, int64, time.Time) error { return nil }

func (r *runtimeRepositoryStub) FailEvent(context.Context, int64, string, time.Time) error {
	r.failedEvents++
	return nil
}

func (r *runtimeRepositoryStub) DeadLetterEvent(_ context.Context, config *domain.Config, event *domain.Event, _ string, at time.Time) (bool, error) {
	if _, exists := r.deadLetters[event.ID]; exists {
		return false, nil
	}
	r.deadLetters[event.ID] = struct{}{}
	r.notifications = append(r.notifications, notification.Event{
		EventID:          "im_message:" + strconv.FormatInt(event.ID, 10) + ":dead_lettered",
		EventType:        notification.EventIMMessageDeadLettered,
		AggregateType:    "im_channel",
		AggregateID:      strconv.FormatInt(event.ID, 10),
		AggregateVersion: int64(event.AttemptCount),
		OccurredAt:       at,
		ActorID:          config.UpdatedBy,
		SpaceID:          config.SpaceID,
		RecipientPolicy:  notification.RecipientExplicitInternalUsers,
		PayloadSchema:    notification.CurrentPayloadSchema,
		Payload: notification.EventPayload{
			ResourceDisplayName:  config.Name,
			StatusReasonCode:     notification.StatusReasonRetryExhausted,
			TargetID:             "20",
			ExplicitRecipientIDs: []int64{2, 7, 9},
		},
	})
	return true, nil
}

func (r *runtimeRepositoryStub) CleanupEvents(context.Context, time.Time) error { return nil }

func (r *runtimeRepositoryStub) GetSession(context.Context, int64, string) (*domain.Session, error) {
	return nil, nil
}

func (r *runtimeRepositoryStub) SaveSession(context.Context, *domain.Session) error { return nil }

type runtimeIDGenStub struct {
	next int64
}

func (g *runtimeIDGenStub) GenID(context.Context) (int64, error) {
	g.next++
	return g.next, nil
}

func (g *runtimeIDGenStub) GenMultiIDs(_ context.Context, count int) ([]int64, error) {
	ids := make([]int64, count)
	for index := range ids {
		g.next++
		ids[index] = g.next
	}
	return ids, nil
}

var _ idgen.IDGenerator = (*runtimeIDGenStub)(nil)

type runtimeChannelStub struct {
	sent []*channeltypes.SendInput
}

func (c *runtimeChannelStub) Send(_ context.Context, input *channeltypes.SendInput) (*channeltypes.SendResult, error) {
	c.sent = append(c.sent, input)
	return &channeltypes.SendResult{}, nil
}

func (c *runtimeChannelStub) OnMessage(func(context.Context, *channeltypes.NormalizedMessage) error) {}
func (c *runtimeChannelStub) OnReaction(func(context.Context, *channeltypes.ReactionEvent) error) {}
func (c *runtimeChannelStub) OnComment(func(context.Context, *channeltypes.CommentEvent) error) {}
func (c *runtimeChannelStub) OnBotAdded(func(context.Context, *channeltypes.BotAddedEvent) error) {}
func (c *runtimeChannelStub) OnCardAction(func(context.Context, *channeltypes.CardActionEvent) error) {}
func (c *runtimeChannelStub) OnReject(func(context.Context, *channeltypes.RejectEvent) error) {}
func (c *runtimeChannelStub) DownloadFile(context.Context, string, string) ([]byte, error) { return nil, nil }
func (c *runtimeChannelStub) OnReady(func()) {}
func (c *runtimeChannelStub) OnError(func(error)) {}
func (c *runtimeChannelStub) OnReconnecting(func()) {}
func (c *runtimeChannelStub) OnReconnected(func()) {}
func (c *runtimeChannelStub) OnDisconnected(func()) {}
func (c *runtimeChannelStub) Start(context.Context) error { return nil }
func (c *runtimeChannelStub) Stream(context.Context, *channeltypes.SendInput) (channeltypes.StreamController, error) {
	return nil, nil
}
func (c *runtimeChannelStub) UpdatePolicy(channeltypes.PolicyConfig) {}
func (c *runtimeChannelStub) GetPolicy() channeltypes.PolicyConfig { return channeltypes.PolicyConfig{} }
func (c *runtimeChannelStub) GetBotIdentity(context.Context) *channeltypes.BotIdentity { return nil }
func (c *runtimeChannelStub) Stop(context.Context) error { return nil }
