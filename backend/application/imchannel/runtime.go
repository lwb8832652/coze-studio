// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/channel"
	channeltypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	runtimeReconcileInterval = 10 * time.Second
	runtimeLeaseDuration     = 35 * time.Second
	eventProcessingLease     = 6 * time.Minute
	eventRetention           = 7 * 24 * time.Hour
)

type ChannelFactory interface {
	New(config *domain.Config, appSecret string) (channeltypes.Channel, error)
}

type OfficialChannelFactory struct{}

func (OfficialChannelFactory) New(config *domain.Config, appSecret string) (channeltypes.Channel, error) {
	if config == nil || strings.TrimSpace(config.AppID) == "" || strings.TrimSpace(appSecret) == "" {
		return nil, domain.ErrInvalidInput
	}
	client := lark.NewClient(
		config.AppID,
		appSecret,
		lark.WithLogLevel(larkcore.LogLevelWarn),
	)
	eventDispatcher := larkdispatcher.NewEventDispatcher("", "")
	wsClient := larkws.NewClient(
		config.AppID,
		appSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogLevel(larkcore.LogLevelWarn),
	)
	requireMention := true
	respondToMentionAll := false
	policy := channeltypes.PolicyConfig{
		RequireMention:      &requireMention,
		RespondToMentionAll: &respondToMentionAll,
		DMMode:              "open",
	}
	if config.GroupPolicy == domain.GroupPolicyDisabled {
		policy.GroupAllowlist = []string{"__coze_no_group_is_allowed__"}
	}
	return channel.NewChannel(
		client,
		wsClient,
		channeltypes.WithPolicyConfig(policy),
	), nil
}

type OfficialConnectionTester struct{}

func (OfficialConnectionTester) Test(ctx context.Context, appID, appSecret string) (*BotIdentity, error) {
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(appSecret) == "" {
		return nil, domain.ErrInvalidInput
	}
	client := lark.NewClient(
		appID,
		appSecret,
		lark.WithLogLevel(larkcore.LogLevelWarn),
	)
	response, err := client.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, err
	}
	if response == nil || response.StatusCode != 200 {
		return nil, domain.ErrConnectionTestFailed
	}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(response.RawBody, &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 || payload.Bot.OpenID == "" {
		return nil, domain.ErrConnectionTestFailed
	}
	return &BotIdentity{OpenID: payload.Bot.OpenID, Name: payload.Bot.AppName}, nil
}

type activeChannel struct {
	config  *domain.Config
	channel channeltypes.Channel
	cancel  context.CancelFunc
}

type RuntimeManager struct {
	repository domain.Repository
	idGen      idgen.IDGenerator
	codec      CredentialCodec
	factory    ChannelFactory
	runner     *AgentRunner
	owner      string
	stableFailureThreshold int
	wake       chan struct{}

	mu         sync.Mutex
	active     map[int64]*activeChannel
	processing map[int64]bool
	retryAfter map[int64]time.Time
}

func NewRuntimeManager(
	repository domain.Repository,
	idGenerator idgen.IDGenerator,
	codec CredentialCodec,
	factory ChannelFactory,
	runner *AgentRunner,
	options ...RuntimeManagerOption,
) (*RuntimeManager, error) {
	if repository == nil || idGenerator == nil || factory == nil || runner == nil {
		return nil, domain.ErrRuntimeUnavailable
	}
	manager := &RuntimeManager{
		repository: repository,
		idGen:      idGenerator,
		codec:      codec,
		factory:    factory,
		runner:     runner,
		owner:      runtimeOwner(),
		stableFailureThreshold: domain.DefaultRuntimeStableFailureThreshold,
		wake:       make(chan struct{}, 1),
		active:     make(map[int64]*activeChannel),
		processing: make(map[int64]bool),
		retryAfter: make(map[int64]time.Time),
	}
	for _, option := range options {
		if option != nil {
			option(manager)
		}
	}
	if manager.stableFailureThreshold <= 0 {
		manager.stableFailureThreshold = domain.DefaultRuntimeStableFailureThreshold
	}
	return manager, nil
}

type RuntimeManagerOption func(*RuntimeManager)

func WithRuntimeStableFailureThreshold(threshold int) RuntimeManagerOption {
	return func(manager *RuntimeManager) {
		if manager != nil && threshold > 0 {
			manager.stableFailureThreshold = threshold
		}
	}
}

func (m *RuntimeManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go m.loop(ctx)
}

func (m *RuntimeManager) Wake() {
	if m == nil {
		return
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *RuntimeManager) loop(ctx context.Context) {
	ticker := time.NewTicker(runtimeReconcileInterval)
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(time.Hour)
	defer cleanupTicker.Stop()
	m.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			m.stopAll(context.Background())
			return
		case <-m.wake:
			m.reconcile(ctx)
		case <-ticker.C:
			m.reconcile(ctx)
		case <-cleanupTicker.C:
			if err := m.repository.CleanupEvents(ctx, time.Now().Add(-eventRetention)); err != nil {
				logs.CtxWarnf(ctx, "cleanup Feishu IM events failed: %v", err)
			}
		}
	}
}

func (m *RuntimeManager) reconcile(ctx context.Context) {
	configs, err := m.repository.ListEnabledConfigs(ctx)
	if err != nil {
		logs.CtxWarnf(ctx, "list enabled Feishu IM channels failed: %v", err)
		return
	}
	enabled := make(map[int64]*domain.Config, len(configs))
	for _, config := range configs {
		if config != nil {
			enabled[config.ID] = config
		}
	}

	for configID, active := range m.activeSnapshot() {
		config := enabled[configID]
		if config == nil || config.Version != active.config.Version {
			m.stopActive(ctx, configID)
			continue
		}
		renewed, renewErr := m.repository.RenewRuntimeLease(
			ctx,
			configID,
			m.owner,
			time.Now().Add(runtimeLeaseDuration),
		)
		if renewErr != nil || !renewed {
			m.stopActive(ctx, configID)
			continue
		}
		m.processPending(ctx, active)
	}

	for _, config := range configs {
		if config == nil || m.getActive(config.ID) != nil || m.shouldBackoff(config.ID) {
			continue
		}
		acquired, acquireErr := m.repository.TryAcquireRuntimeLease(
			ctx,
			config.ID,
			m.owner,
			time.Now(),
			time.Now().Add(runtimeLeaseDuration),
		)
		if acquireErr != nil {
			logs.CtxWarnf(ctx, "acquire Feishu IM channel lease failed: %v", acquireErr)
			continue
		}
		if acquired {
			m.startConfig(ctx, config)
		}
	}
}

func (m *RuntimeManager) startConfig(root context.Context, config *domain.Config) {
	if config == nil {
		return
	}
	if m.codec == nil {
		m.recordStartFailure(root, config.ID, domain.ErrCredentialCodecMissing)
		return
	}
	secret, err := m.codec.Decrypt(config.ID, config.AppSecretCiphertext)
	if err != nil {
		m.recordStartFailure(root, config.ID, err)
		return
	}
	feishuChannel, err := m.factory.New(config, secret)
	if err != nil {
		m.recordStartFailure(root, config.ID, err)
		return
	}
	channelCtx, cancel := context.WithCancel(root)
	active := &activeChannel{config: config, channel: feishuChannel, cancel: cancel}
	m.mu.Lock()
	if existing := m.active[config.ID]; existing != nil {
		m.mu.Unlock()
		cancel()
		_ = feishuChannel.Stop(context.Background())
		return
	}
	m.active[config.ID] = active
	delete(m.retryAfter, config.ID)
	m.mu.Unlock()

	_ = m.repository.UpdateRuntimeState(root, config.ID, domain.RuntimeState{
		Status: domain.RuntimeStatusConnecting,
	})
	feishuChannel.OnReady(func() {
		now := time.Now()
		identity := feishuChannel.GetBotIdentity(channelCtx)
		state := domain.RuntimeState{
			Status:      domain.RuntimeStatusConnected,
			ConnectedAt: &now,
		}
		if identity != nil {
			state.BotOpenID = identity.OpenID
			state.BotName = identity.Name
		}
		m.recordRuntimeRecovery(channelCtx, config.ID, state)
	})
	feishuChannel.OnReconnecting(func() {
		_ = m.repository.UpdateRuntimeState(channelCtx, config.ID, domain.RuntimeState{
			Status: domain.RuntimeStatusReconnecting,
		})
	})
	feishuChannel.OnReconnected(func() {
		now := time.Now()
		m.recordRuntimeRecovery(channelCtx, config.ID, domain.RuntimeState{
			Status:      domain.RuntimeStatusConnected,
			ConnectedAt: &now,
		})
	})
	feishuChannel.OnDisconnected(func() {
		if channelCtx.Err() == nil {
			_ = m.repository.UpdateRuntimeState(channelCtx, config.ID, domain.RuntimeState{
				Status: domain.RuntimeStatusReconnecting,
			})
		}
	})
	feishuChannel.OnError(func(channelErr error) {
		m.recordRuntimeFailure(channelCtx, config.ID, channelErr)
	})
	feishuChannel.OnMessage(func(messageCtx context.Context, message *channeltypes.NormalizedMessage) error {
		return m.persistInbound(messageCtx, config, message)
	})

	go func() {
		startErr := feishuChannel.Start(channelCtx)
		m.removeActive(config.ID, active)
		_ = m.repository.ReleaseRuntimeLease(context.Background(), config.ID, m.owner)
		if channelCtx.Err() == nil {
			m.setBackoff(config.ID, time.Now().Add(30*time.Second))
			m.recordRuntimeFailure(context.Background(), config.ID, startErr)
		}
	}()
}

func (m *RuntimeManager) persistInbound(
	ctx context.Context,
	config *domain.Config,
	message *channeltypes.NormalizedMessage,
) error {
	if config == nil || message == nil {
		return nil
	}
	eventKey := strings.TrimSpace(message.EventID)
	if eventKey == "" {
		eventKey = strings.TrimSpace(message.MessageID)
	}
	if eventKey == "" || message.ChatID == "" {
		return nil
	}
	resources := make([]domain.Resource, 0, len(message.Resources))
	for _, resource := range message.Resources {
		resources = append(resources, domain.Resource{
			Type:     boundedMessage(resource.Type, 32),
			FileKey:  boundedMessage(resource.FileKey, 256),
			FileName: boundedMessage(resource.FileName, 256),
		})
	}
	payload := domain.InboundPayload{
		EventID:        boundedMessage(message.EventID, 256),
		MessageID:      boundedMessage(message.MessageID, 256),
		ChatID:         boundedMessage(message.ChatID, 256),
		ChatType:       boundedMessage(message.ChatType, 32),
		UserID:         boundedMessage(message.UserID, 256),
		Content:        boundedMessage(message.Content, maxInboundPromptBytes),
		RawContentType: boundedMessage(message.RawContentType, 64),
		Resources:      resources,
		CreateTimeMs:   message.CreateTimeMs,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID, err := m.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	inserted, err := m.repository.InsertEvent(ctx, &domain.Event{
		ID:           eventID,
		ConfigID:     config.ID,
		EventKey:     eventKey,
		MessageID:    payload.MessageID,
		PayloadJSON:  string(payloadJSON),
		Status:       domain.EventStatusPending,
		AttemptCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return err
	}
	if inserted {
		m.Wake()
	}
	return nil
}

func (m *RuntimeManager) processPending(ctx context.Context, active *activeChannel) {
	if active == nil || active.config == nil {
		return
	}
	configID := active.config.ID
	m.mu.Lock()
	if m.processing[configID] {
		m.mu.Unlock()
		return
	}
	m.processing[configID] = true
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.processing, configID)
			m.mu.Unlock()
		}()
		for {
			now := time.Now()
			events, err := m.repository.ClaimEvents(
				ctx,
				configID,
				m.owner,
				now,
				now.Add(eventProcessingLease),
				20,
			)
			if err != nil {
				logs.CtxWarnf(ctx, "claim Feishu IM events failed: %v", err)
				return
			}
			if len(events) == 0 {
				return
			}
			for _, event := range events {
				if event.Status == domain.EventStatusFailed &&
					event.AttemptCount >= domain.EventMaxAttempts {
					if err := m.deadLetterEvent(ctx, active, event, errors.New(event.LastError)); err != nil {
						_ = m.repository.FailEvent(
							ctx,
							event.ID,
							safeRuntimeError(err),
							time.Now().Add(30*time.Second),
						)
					}
					continue
				}
				m.processEvent(ctx, active, event)
			}
		}
	}()
}

func (m *RuntimeManager) processEvent(ctx context.Context, active *activeChannel, event *domain.Event) {
	if active == nil || event == nil {
		return
	}
	var payload domain.InboundPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		m.failEvent(ctx, active, event, err)
		return
	}
	answer, err := m.runner.Execute(ctx, active.config, event.EventKey, payload)
	if err != nil {
		m.failEvent(ctx, active, event, err)
		return
	}
	if err := sendAnswer(ctx, active.channel, active.config, payload, answer); err != nil {
		m.failEvent(ctx, active, event, err)
		return
	}
	if err := m.repository.CompleteEvent(ctx, event.ID, time.Now()); err != nil {
		logs.CtxWarnf(ctx, "complete Feishu IM event failed: %v", err)
	}
}

func (m *RuntimeManager) failEvent(
	ctx context.Context,
	active *activeChannel,
	event *domain.Event,
	eventErr error,
) {
	message := safeRuntimeError(eventErr)
	delay := time.Duration(1<<minInt(int(event.AttemptCount), 5)) * 5 * time.Second
	if event.AttemptCount+1 >= domain.EventMaxAttempts {
		if err := m.deadLetterEvent(ctx, active, event, eventErr); err != nil {
			_ = m.repository.FailEvent(
				ctx,
				event.ID,
				message,
				time.Now().Add(delay),
			)
		}
		return
	}
	_ = m.repository.FailEvent(
		ctx,
		event.ID,
		message,
		time.Now().Add(delay),
	)
}

func (m *RuntimeManager) deadLetterEvent(
	ctx context.Context,
	active *activeChannel,
	event *domain.Event,
	eventErr error,
) error {
	if active == nil || active.config == nil || event == nil {
		return domain.ErrInvalidInput
	}
	message := safeRuntimeError(eventErr)
	deadLettered, err := m.repository.DeadLetterEvent(ctx, active.config, event, message, time.Now())
	if err != nil {
		logs.CtxWarnf(ctx, "dead-letter Feishu IM event failed: %s", safeRuntimeError(err))
		return err
	}
	if deadLettered && active.channel != nil {
		var payload domain.InboundPayload
		if json.Unmarshal([]byte(event.PayloadJSON), &payload) == nil {
			_, _ = active.channel.Send(ctx, &channeltypes.SendInput{
				ChatID:         payload.ChatID,
				ReplyMessageID: payload.MessageID,
				Text:           "消息处理失败，请稍后重试或联系工作空间管理员。",
			})
		}
	}
	return nil
}

func sendAnswer(
	ctx context.Context,
	feishuChannel channeltypes.Channel,
	config *domain.Config,
	payload domain.InboundPayload,
	answer string,
) error {
	answer = strings.TrimSpace(answer)
	if feishuChannel == nil || config == nil || payload.ChatID == "" || answer == "" {
		return domain.ErrAgentResponseUnavailable
	}
	input := &channeltypes.SendInput{
		ChatID:         payload.ChatID,
		ReplyMessageID: payload.MessageID,
	}
	if config.ReplyMode != domain.ReplyModeStream {
		input.Markdown = answer
		_, err := feishuChannel.Send(ctx, input)
		return err
	}
	first, rest := splitReply(answer, 800)
	input.Markdown = first
	controller, err := feishuChannel.Stream(ctx, input)
	if err != nil {
		return err
	}
	for rest != "" {
		var chunk string
		chunk, rest = splitReply(rest, 800)
		if err := controller.Append(ctx, chunk); err != nil {
			return err
		}
	}
	if err := controller.Flush(ctx); err != nil {
		return err
	}
	return controller.Close(ctx)
}

func (m *RuntimeManager) recordStartFailure(ctx context.Context, configID int64, err error) {
	m.setBackoff(configID, time.Now().Add(30*time.Second))
	m.recordRuntimeFailure(ctx, configID, err)
	_ = m.repository.ReleaseRuntimeLease(ctx, configID, m.owner)
}

func (m *RuntimeManager) recordRuntimeFailure(ctx context.Context, configID int64, err error) {
	if m == nil || m.repository == nil {
		return
	}
	recordErr := m.repository.RecordRuntimeFailure(ctx, configID, domain.RuntimeState{
		Status: domain.RuntimeStatusError,
		Error:  safeRuntimeError(err),
		IncidentID: m.newRuntimeIncidentID(ctx, configID),
	}, m.runtimeStableFailureThreshold())
	if recordErr != nil {
		logs.CtxWarnf(ctx, "record Feishu IM runtime failure failed: %s", safeRuntimeError(recordErr))
	}
}

func (m *RuntimeManager) recordRuntimeRecovery(ctx context.Context, configID int64, state domain.RuntimeState) {
	if m == nil || m.repository == nil {
		return
	}
	if state.Status == "" {
		state.Status = domain.RuntimeStatusConnected
	}
	if err := m.repository.RecordRuntimeRecovery(ctx, configID, state); err != nil {
		logs.CtxWarnf(ctx, "record Feishu IM runtime recovery failed: %s", safeRuntimeError(err))
	}
}

func (m *RuntimeManager) newRuntimeIncidentID(ctx context.Context, configID int64) string {
	if m != nil && m.idGen != nil {
		if id, err := m.idGen.GenID(ctx); err == nil && id > 0 {
			return fmt.Sprintf("feishu-im:%d:%d", configID, id)
		}
	}
	return fmt.Sprintf("feishu-im:%d:%d", configID, time.Now().UnixNano())
}

func (m *RuntimeManager) runtimeStableFailureThreshold() int {
	if m != nil && m.stableFailureThreshold > 0 {
		return m.stableFailureThreshold
	}
	return domain.DefaultRuntimeStableFailureThreshold
}

func (m *RuntimeManager) activeSnapshot() map[int64]*activeChannel {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := make(map[int64]*activeChannel, len(m.active))
	for id, active := range m.active {
		snapshot[id] = active
	}
	return snapshot
}

func (m *RuntimeManager) getActive(configID int64) *activeChannel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[configID]
}

func (m *RuntimeManager) removeActive(configID int64, expected *activeChannel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[configID] == expected {
		delete(m.active, configID)
	}
}

func (m *RuntimeManager) stopActive(ctx context.Context, configID int64) {
	m.mu.Lock()
	active := m.active[configID]
	if active != nil {
		delete(m.active, configID)
	}
	m.mu.Unlock()
	if active == nil {
		return
	}
	active.cancel()
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = active.channel.Stop(stopCtx)
	_ = m.repository.ReleaseRuntimeLease(ctx, configID, m.owner)
}

func (m *RuntimeManager) stopAll(ctx context.Context) {
	for configID := range m.activeSnapshot() {
		m.stopActive(ctx, configID)
	}
}

func (m *RuntimeManager) setBackoff(configID int64, until time.Time) {
	m.mu.Lock()
	m.retryAfter[configID] = until
	m.mu.Unlock()
}

func (m *RuntimeManager) shouldBackoff(configID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	until := m.retryAfter[configID]
	if until.IsZero() || time.Now().After(until) {
		delete(m.retryAfter, configID)
		return false
	}
	return true
}

func runtimeOwner() string {
	var random [8]byte
	_, _ = rand.Read(random[:])
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%d-%s", boundedMessage(host, 40), os.Getpid(), hex.EncodeToString(random[:]))
}

func safeRuntimeError(err error) string {
	if err == nil {
		return "connection_failed"
	}
	if errors.Is(err, context.Canceled) {
		return "runtime_stopped"
	}
	switch {
	case errors.Is(err, domain.ErrCredentialCodecMissing),
		errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrConnectionTestFailed):
		return "configuration_invalid"
	case errors.Is(err, domain.ErrAgentUnavailable),
		errors.Is(err, domain.ErrAgentExecutionFailed),
		errors.Is(err, domain.ErrAgentResponseUnavailable):
		return "agent_execution_failed"
	case errors.Is(err, domain.ErrRuntimeUnavailable):
		return "runtime_unavailable"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "connection_failed"
	}
	if isStableRuntimeErrorCode(message) {
		return message
	}
	return "provider_unavailable"
}

func isStableRuntimeErrorCode(value string) bool {
	switch value {
	case "connection_failed",
		"runtime_stopped",
		"configuration_invalid",
		"agent_execution_failed",
		"runtime_unavailable",
		"provider_unavailable",
		"retry_exhausted":
		return true
	default:
		return false
	}
}

func splitReply(value string, size int) (string, string) {
	runes := []rune(value)
	if size <= 0 || len(runes) <= size {
		return value, ""
	}
	return string(runes[:size]), string(runes[size:])
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
