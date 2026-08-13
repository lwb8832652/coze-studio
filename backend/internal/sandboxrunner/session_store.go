// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/infra/cache"
)

const (
	maxSessionOperationEnvelopeBytes = 16 << 10
	// The wire contract permits an inline response up to
	// maxSessionInlineResultBytes. AES-GCM adds one authentication tag and the
	// Redis envelope base64-encodes the ciphertext, so reserve the encoded
	// upper bound plus a small fixed JSON envelope allowance.
	maxSessionResultEnvelopeBytes  = (maxSessionInlineResultBytes+16+2)/3*4 + 1024
	maxSessionOperationReasonBytes = 64
	maxSessionOperationQueueDepth  = 4096
	maxSessionLeaseTTL             = 10 * time.Minute
	maxSessionRecordTTL            = 24 * time.Hour
)

var (
	ErrSessionOperationConflict = errors.New("sandbox runner session operation conflict")
	ErrSessionLeaseHeld         = errors.New("sandbox runner session lease is held")
	ErrSessionLeaseLost         = errors.New("sandbox runner session lease lost")
)

// SessionOperationInput deliberately carries only a digest and non-sensitive
// scheduling metadata. Request bodies remain in the authenticated request's
// synchronous call stack and are never persisted for replay.
type SessionOperationInput struct {
	SessionID     string
	OperationID   string
	UserID        int64
	Kind          SessionOperationKind
	RequestDigest []byte
	Weight        int
	Deadline      time.Time
}

func (SessionOperationInput) String() string {
	return "sandboxrunner.SessionOperationInput{operation:<redacted>}"
}
func (SessionOperationInput) GoString() string { return SessionOperationInput{}.String() }
func (SessionOperationInput) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, SessionOperationInput{}.String())
}

type SessionOperationRecord struct {
	SessionID       string
	OperationID     string
	Kind            SessionOperationKind
	State           SessionOperationState
	CancelRequested bool
	AcceptedAt      time.Time
	UpdatedAt       time.Time
	Deadline        time.Time
	ResultDigest    []byte
	Result          []byte
	ReasonCode      string
}

func (SessionOperationRecord) String() string {
	return "sandboxrunner.SessionOperationRecord{operation:<redacted>}"
}
func (SessionOperationRecord) GoString() string { return SessionOperationRecord{}.String() }
func (SessionOperationRecord) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, SessionOperationRecord{}.String())
}

type SessionOperationCompletion struct {
	State        SessionOperationState
	ResultDigest []byte
	Result       []byte
	ReasonCode   string
}

func (SessionOperationCompletion) String() string {
	return "sandboxrunner.SessionOperationCompletion{result:<redacted>}"
}
func (SessionOperationCompletion) GoString() string { return SessionOperationCompletion{}.String() }
func (SessionOperationCompletion) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, SessionOperationCompletion{}.String())
}

type SessionOperationStore interface {
	Accept(context.Context, SessionOperationInput) (SessionOperationRecord, bool, error)
	Get(context.Context, string, string) (SessionOperationRecord, error)
	MarkQueued(context.Context, string, string) (SessionOperationRecord, error)
	ClaimQueued(context.Context, string, string, int, int) (SessionOperationRecord, bool, error)
	Complete(context.Context, string, string, SessionOperationCompletion) (SessionOperationRecord, error)
	RequestCancel(context.Context, string, string) (SessionOperationRecord, bool, error)
	CancelQueued(context.Context, string, string) (SessionOperationRecord, bool, error)
	RecoverOperations(context.Context) ([]SessionOperationRecord, error)
}

type RedisSessionStoreConfig struct {
	DeploymentID  string
	ActiveKeyID   string
	Keys          map[string]string
	RecordTTL     time.Duration
	LeaseTTL      time.Duration
	MaxQueueDepth int
}

type RedisSessionStore struct {
	client         cache.Cmdable
	deploymentID   string
	deploymentHash string
	keys           redisEncryptionKeyring
	recordTTL      time.Duration
	leaseTTL       time.Duration
	maxQueueDepth  int
	now            func() time.Time
	random         io.Reader
}

type persistedSessionOperation struct {
	Schema          string                `json:"schema"`
	RecordID        string                `json:"record_id"`
	SessionID       string                `json:"session_id"`
	OperationID     string                `json:"operation_id"`
	UserHash        string                `json:"user_hash"`
	Kind            SessionOperationKind  `json:"kind"`
	RequestDigest   string                `json:"request_digest"`
	Weight          int                   `json:"weight"`
	State           SessionOperationState `json:"state"`
	AcceptedUnixMS  int64                 `json:"accepted_unix_ms"`
	UpdatedUnixMS   int64                 `json:"updated_unix_ms"`
	DeadlineUnixMS  int64                 `json:"deadline_unix_ms"`
	ResultDigest    string                `json:"result_digest,omitempty"`
	ReasonCode      string                `json:"reason_code,omitempty"`
	CancelRequested bool                  `json:"cancel_requested,omitempty"`
}

type encryptedSessionOperationEnvelope struct {
	Schema       string `json:"schema"`
	KeyID        string `json:"key_id"`
	DeploymentID string `json:"deployment_id"`
	Nonce        string `json:"nonce"`
	Ciphertext   string `json:"ciphertext"`
}

type encryptedSessionResultEnvelope struct {
	Schema       string `json:"schema"`
	KeyID        string `json:"key_id"`
	DeploymentID string `json:"deployment_id"`
	RecordID     string `json:"record_id"`
	Nonce        string `json:"nonce"`
	Ciphertext   string `json:"ciphertext"`
}

func NewRedisSessionStore(client cache.Cmdable, config RedisSessionStoreConfig) (*RedisSessionStore, error) {
	if client == nil || !validKeyID(config.DeploymentID) || !validKeyID(config.ActiveKeyID) ||
		config.RecordTTL <= 0 || config.RecordTTL > maxSessionRecordTTL ||
		config.LeaseTTL <= 0 || config.LeaseTTL > maxSessionLeaseTTL ||
		config.MaxQueueDepth < 1 || config.MaxQueueDepth > maxSessionOperationQueueDepth {
		return nil, ErrConfiguration
	}
	keys := make(map[string][]byte, len(config.Keys))
	for keyID, value := range config.Keys {
		if !validKeyID(keyID) || len(value) < 16 {
			return nil, ErrConfiguration
		}
		digest := sha256.Sum256([]byte(value))
		keys[keyID] = append([]byte(nil), digest[:]...)
	}
	if len(keys[config.ActiveKeyID]) != sha256.Size {
		return nil, ErrConfiguration
	}
	return &RedisSessionStore{
		client: client, deploymentID: config.DeploymentID, deploymentHash: redisHash(config.DeploymentID),
		keys:      redisEncryptionKeyring{ActiveKeyID: config.ActiveKeyID, Keys: keys},
		recordTTL: config.RecordTTL, leaseTTL: config.LeaseTTL, maxQueueDepth: config.MaxQueueDepth,
		now: func() time.Time { return time.Now().UTC() }, random: rand.Reader,
	}, nil
}

func (store *RedisSessionStore) Accept(ctx context.Context, input SessionOperationInput) (SessionOperationRecord, bool, error) {
	if store == nil || store.client == nil || ctx == nil || !validSessionOperationInput(input, store.now()) {
		return SessionOperationRecord{}, false, ErrProtocol
	}
	now := store.now().UTC()
	ttl := store.operationTTL(input.Deadline, now)
	if ttl <= 0 {
		return SessionOperationRecord{}, false, ErrProtocol
	}
	recordID, err := store.newOpaqueID()
	if err != nil {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	payload := persistedSessionOperation{
		Schema: "coze.sandbox.core_operation.v1", RecordID: recordID,
		SessionID: input.SessionID, OperationID: input.OperationID,
		UserHash: redisHash(strconv.FormatInt(input.UserID, 10)), Kind: input.Kind,
		RequestDigest: hex.EncodeToString(input.RequestDigest), Weight: input.Weight,
		State: SessionOperationAccepted, AcceptedUnixMS: now.UnixMilli(), UpdatedUnixMS: now.UnixMilli(),
		DeadlineUnixMS: input.Deadline.UTC().UnixMilli(),
	}
	sealed, err := store.sealOperation(payload)
	if err != nil {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	result, err := store.client.RunScript(ctx, redisAcceptSessionOperationScript,
		[]string{store.operationLookupKey(input.SessionID, input.OperationID), store.recordKey(recordID), store.activeOperationsKey()},
		recordID, sealed, durationMillisecondsCeil(ttl), store.maxQueueDepth).Result()
	if err != nil {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	status, existingID, ok := parseSessionScriptPair(result)
	if !ok {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	switch status {
	case "accepted":
		return sessionOperationProjection(payload), false, nil
	case "replay":
		existing, _, err := store.loadOperation(ctx, input.SessionID, input.OperationID)
		if err != nil || !constantTimeEqual(existing.RequestDigest, payload.RequestDigest) || existing.Kind != input.Kind {
			return SessionOperationRecord{}, false, ErrSessionOperationConflict
		}
		if existingID != existing.RecordID {
			return SessionOperationRecord{}, false, ErrUnavailable
		}
		projection, err := store.projectOperation(ctx, existing)
		if err != nil {
			return SessionOperationRecord{}, false, err
		}
		return projection, true, nil
	case "capacity":
		return SessionOperationRecord{}, false, ErrUnavailable
	default:
		return SessionOperationRecord{}, false, ErrUnavailable
	}
}

func (store *RedisSessionStore) Get(ctx context.Context, sessionID, operationID string) (SessionOperationRecord, error) {
	payload, _, err := store.loadOperation(ctx, sessionID, operationID)
	if err != nil {
		return SessionOperationRecord{}, err
	}
	return store.projectOperation(ctx, payload)
}

func (store *RedisSessionStore) MarkQueued(ctx context.Context, sessionID, operationID string) (SessionOperationRecord, error) {
	return store.transitionOperation(ctx, sessionID, operationID, func(payload *persistedSessionOperation) (bool, error) {
		if payload.State == SessionOperationQueued {
			return false, nil
		}
		if payload.State != SessionOperationAccepted {
			return false, ErrSessionOperationConflict
		}
		payload.State = SessionOperationQueued
		return true, nil
	}, sessionTransitionQueued)
}

func (store *RedisSessionStore) ClaimQueued(ctx context.Context, sessionID, operationID string, totalWeight, perUserLimit int) (SessionOperationRecord, bool, error) {
	if totalWeight < 1 || totalWeight > 64 || perUserLimit < 1 || perUserLimit > 4096 {
		return SessionOperationRecord{}, false, ErrProtocol
	}
	payload, current, err := store.loadOperation(ctx, sessionID, operationID)
	if err != nil {
		return SessionOperationRecord{}, false, err
	}
	if payload.State != SessionOperationQueued {
		return SessionOperationRecord{}, false, ErrSessionOperationConflict
	}
	claimable, err := store.prepareFairQueueClaim(ctx, payload.RecordID, totalWeight, perUserLimit)
	if err != nil {
		return SessionOperationRecord{}, false, err
	}
	if !claimable {
		return sessionOperationProjection(payload), false, nil
	}
	payload.State = SessionOperationRunning
	payload.UpdatedUnixMS = store.now().UTC().UnixMilli()
	sealed, ttl, err := store.sealOperationWithTTL(payload)
	if err != nil {
		return SessionOperationRecord{}, false, err
	}
	result, err := store.client.RunScript(ctx, redisClaimSessionOperationScript,
		[]string{store.recordKey(payload.RecordID), store.queueKey(), store.activeWeightKey(), store.activeUserKey(payload.UserHash), store.activeSessionKey(payload.SessionID)},
		current, sealed, durationMillisecondsCeil(ttl), payload.RecordID, payload.Weight, totalWeight, perUserLimit).Result()
	if err != nil {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	status, ok := parseSessionScriptStatus(result)
	switch {
	case !ok:
		return SessionOperationRecord{}, false, ErrUnavailable
	case status == "started":
		return sessionOperationProjection(payload), true, nil
	case status == "capacity":
		return sessionOperationProjection(withSessionOperationState(payload, SessionOperationQueued)), false, nil
	case status == "waiting":
		return sessionOperationProjection(withSessionOperationState(payload, SessionOperationQueued)), false, nil
	case status == "stale":
		return SessionOperationRecord{}, false, ErrSessionOperationConflict
	default:
		return SessionOperationRecord{}, false, ErrUnavailable
	}
}

// prepareFairQueueClaim only lets the earliest capacity-eligible operation
// claim. A head blocked by its user or Session is rotated so another tenant or
// Session may use otherwise-idle global capacity. A globally blocked head is
// retained because every later operation has the same Core weight contract.
func (store *RedisSessionStore) prepareFairQueueClaim(ctx context.Context, targetRecordID string, totalWeight, perUserLimit int) (bool, error) {
	for attempt := 0; attempt < store.maxQueueDepth; attempt++ {
		head, err := store.client.LRange(ctx, store.queueKey(), 0, 0).Result()
		if err != nil {
			if errors.Is(err, cache.Nil) {
				return false, ErrSessionOperationConflict
			}
			return false, ErrUnavailable
		}
		if len(head) != 1 {
			return false, ErrSessionOperationConflict
		}
		if head[0] == targetRecordID {
			return true, nil
		}
		candidate, _, err := store.loadOperationByRecordID(ctx, head[0])
		if errors.Is(err, ErrUnavailable) {
			// A record can expire between queue maintenance steps. Only prune it
			// when Redis atomically proves the exact queue head is still present
			// and its record key is absent. A present but undecryptable record is
			// retained and fails closed instead of being mistaken for expiry.
			value, scriptErr := store.client.RunScript(ctx, redisPruneMissingSessionQueueHeadScript,
				[]string{store.queueKey(), store.recordKey(head[0]), store.activeOperationsKey()}, head[0]).Result()
			if scriptErr != nil {
				return false, ErrUnavailable
			}
			status, ok := parseSessionScriptStatus(value)
			if !ok {
				return false, ErrUnavailable
			}
			switch status {
			case "removed", "changed":
				continue
			case "present":
				return false, ErrUnavailable
			default:
				return false, ErrUnavailable
			}
		}
		if err != nil {
			return false, ErrUnavailable
		}
		if candidate.State != SessionOperationQueued {
			continue
		}
		statusValue, err := store.client.RunScript(ctx, redisRotateBlockedSessionOperationScript,
			[]string{store.queueKey(), store.activeWeightKey(), store.activeUserKey(candidate.UserHash), store.activeSessionKey(candidate.SessionID)},
			candidate.RecordID, candidate.Weight, totalWeight, perUserLimit).Result()
		if err != nil {
			return false, ErrUnavailable
		}
		status, ok := parseSessionScriptStatus(statusValue)
		if !ok {
			return false, ErrUnavailable
		}
		switch status {
		case "rotated":
			continue
		case "changed":
			attempt--
			continue
		case "eligible", "global":
			return false, nil
		default:
			return false, ErrUnavailable
		}
	}
	return false, ErrUnavailable
}

func (store *RedisSessionStore) Complete(ctx context.Context, sessionID, operationID string, completion SessionOperationCompletion) (SessionOperationRecord, error) {
	if !validSessionOperationCompletion(completion) {
		return SessionOperationRecord{}, ErrProtocol
	}
	payload, current, err := store.loadOperation(ctx, sessionID, operationID)
	if err != nil {
		return SessionOperationRecord{}, err
	}
	if payload.State != SessionOperationRunning {
		return SessionOperationRecord{}, ErrSessionOperationConflict
	}
	if payload.CancelRequested && completion.State == SessionOperationSucceeded {
		completion = SessionOperationCompletion{State: SessionOperationCanceled, ReasonCode: "SANDBOX_OPERATION_CANCELED"}
	}
	payload.State, payload.UpdatedUnixMS = completion.State, store.now().UTC().UnixMilli()
	payload.ResultDigest, payload.ReasonCode = hex.EncodeToString(completion.ResultDigest), completion.ReasonCode
	sealed, err := store.sealOperation(payload)
	if err != nil {
		return SessionOperationRecord{}, ErrUnavailable
	}
	ttl := store.recordTTL
	sealedResult := ""
	if len(completion.Result) != 0 {
		sealedResult, err = store.sealResult(payload.RecordID, completion.Result)
		if err != nil {
			return SessionOperationRecord{}, ErrUnavailable
		}
	}
	result, err := store.client.RunScript(ctx, redisFinishSessionOperationScript,
		[]string{store.recordKey(payload.RecordID), store.activeOperationsKey(), store.activeWeightKey(), store.activeUserKey(payload.UserHash), store.activeSessionKey(payload.SessionID), store.resultKey(payload.RecordID), store.operationLookupKey(payload.SessionID, payload.OperationID)},
		current, sealed, durationMillisecondsCeil(ttl), payload.RecordID, payload.Weight, sealedResult, payload.RecordID).Result()
	if err != nil {
		return SessionOperationRecord{}, ErrUnavailable
	}
	status, ok := parseSessionScriptStatus(result)
	if !ok || status != "finished" {
		if status == "stale" || status == "lookup_stale" {
			return SessionOperationRecord{}, ErrSessionOperationConflict
		}
		return SessionOperationRecord{}, ErrUnavailable
	}
	projection := sessionOperationProjection(payload)
	projection.Result = append([]byte(nil), completion.Result...)
	return projection, nil
}

// RequestCancel persists cancellation before signaling any local worker. A
// queued operation is terminal immediately; a running operation remains
// capacity-accounted until its worker observes CancelRequested and finishes.
func (store *RedisSessionStore) RequestCancel(ctx context.Context, sessionID, operationID string) (SessionOperationRecord, bool, error) {
	for attempt := 0; attempt < 2; attempt++ {
		payload, current, err := store.loadOperation(ctx, sessionID, operationID)
		if err != nil {
			return SessionOperationRecord{}, false, err
		}
		if isTerminalSessionOperationState(payload.State) {
			return sessionOperationProjection(payload), false, nil
		}
		immediate := payload.State == SessionOperationAccepted || payload.State == SessionOperationQueued
		if !immediate && payload.State != SessionOperationRunning {
			return SessionOperationRecord{}, false, ErrSessionOperationConflict
		}
		if payload.CancelRequested {
			return sessionOperationProjection(payload), false, nil
		}
		if immediate {
			payload.State = SessionOperationCanceled
		} else {
			payload.CancelRequested = true
		}
		payload.UpdatedUnixMS = store.now().UTC().UnixMilli()
		var sealed string
		var ttl time.Duration
		if immediate {
			sealed, err = store.sealOperation(payload)
			ttl = store.recordTTL
		} else {
			sealed, ttl, err = store.sealOperationWithTTL(payload)
		}
		if err != nil {
			return SessionOperationRecord{}, false, err
		}
		result, err := store.client.RunScript(ctx, redisRequestCancelSessionOperationScript,
			[]string{store.recordKey(payload.RecordID), store.queueKey(), store.activeOperationsKey(), store.operationLookupKey(payload.SessionID, payload.OperationID)},
			current, sealed, durationMillisecondsCeil(ttl), payload.RecordID, strconv.FormatBool(immediate), payload.RecordID).Result()
		if err != nil {
			return SessionOperationRecord{}, false, ErrUnavailable
		}
		status, ok := parseSessionScriptStatus(result)
		if !ok {
			return SessionOperationRecord{}, false, ErrUnavailable
		}
		switch status {
		case "canceled":
			return sessionOperationProjection(payload), true, nil
		case "requested":
			return sessionOperationProjection(payload), false, nil
		case "stale":
			continue
		case "lookup_stale":
			return SessionOperationRecord{}, false, ErrSessionOperationConflict
		default:
			return SessionOperationRecord{}, false, ErrUnavailable
		}
	}
	return SessionOperationRecord{}, false, ErrSessionOperationConflict
}

func (store *RedisSessionStore) CancelQueued(ctx context.Context, sessionID, operationID string) (SessionOperationRecord, bool, error) {
	payload, current, err := store.loadOperation(ctx, sessionID, operationID)
	if err != nil {
		return SessionOperationRecord{}, false, err
	}
	if payload.State == SessionOperationCanceled {
		return sessionOperationProjection(payload), false, nil
	}
	if payload.State != SessionOperationAccepted && payload.State != SessionOperationQueued {
		return SessionOperationRecord{}, false, ErrSessionOperationConflict
	}
	payload.State, payload.UpdatedUnixMS = SessionOperationCanceled, store.now().UTC().UnixMilli()
	sealed, err := store.sealOperation(payload)
	if err != nil {
		return SessionOperationRecord{}, false, err
	}
	ttl := store.recordTTL
	result, err := store.client.RunScript(ctx, redisCancelQueuedSessionOperationScript,
		[]string{store.recordKey(payload.RecordID), store.queueKey(), store.activeOperationsKey(), store.operationLookupKey(payload.SessionID, payload.OperationID)},
		current, sealed, durationMillisecondsCeil(ttl), payload.RecordID, payload.RecordID).Result()
	if err != nil {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	status, ok := parseSessionScriptStatus(result)
	if !ok || status != "canceled" {
		if status == "stale" || status == "lookup_stale" {
			return SessionOperationRecord{}, false, ErrSessionOperationConflict
		}
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	return sessionOperationProjection(payload), true, nil
}

// RecoverOperations fences every pre-crash active operation unknown. Request
// bodies are deliberately absent, so accepted/queued work cannot be safely
// replayed any more than running work can. Missing or undecryptable active
// metadata blocks readiness instead of silently discarding uncertain state.
func (store *RedisSessionStore) RecoverOperations(ctx context.Context) ([]SessionOperationRecord, error) {
	if store == nil || store.client == nil || ctx == nil {
		return nil, ErrProtocol
	}
	recordIDs, err := store.client.LRange(ctx, store.activeOperationsKey(), 0, int64(store.maxQueueDepth)).Result()
	if err != nil && !errors.Is(err, cache.Nil) {
		return nil, ErrUnavailable
	}
	if len(recordIDs) > store.maxQueueDepth {
		return nil, ErrUnavailable
	}
	for _, recordID := range recordIDs {
		payload, current, err := store.loadOperationByRecordID(ctx, recordID)
		if err != nil {
			return nil, ErrUnavailable
		}
		switch payload.State {
		case SessionOperationAccepted, SessionOperationQueued, SessionOperationRunning:
			wasRunning := payload.State == SessionOperationRunning
			payload.State, payload.UpdatedUnixMS = SessionOperationUnknown, store.now().UTC().UnixMilli()
			sealed, err := store.sealOperation(payload)
			if err != nil {
				return nil, ErrUnavailable
			}
			ttl := store.recordTTL
			value, err := store.client.RunScript(ctx, redisFenceActiveSessionOperationScript,
				[]string{store.recordKey(recordID), store.activeOperationsKey(), store.activeWeightKey(), store.activeUserKey(payload.UserHash), store.activeSessionKey(payload.SessionID), store.queueKey(), store.operationLookupKey(payload.SessionID, payload.OperationID)},
				current, sealed, durationMillisecondsCeil(ttl), recordID, payload.Weight, strconv.FormatBool(wasRunning), recordID).Result()
			if err != nil {
				return nil, ErrUnavailable
			}
			status, ok := parseSessionScriptStatus(value)
			if !ok || status != "fenced" {
				if status == "stale" || status == "lookup_stale" {
					return nil, ErrSessionOperationConflict
				}
				return nil, ErrUnavailable
			}
		default:
			value, err := store.client.RunScript(ctx, redisRemoveSessionOperationIndexScript,
				[]string{store.activeOperationsKey(), store.queueKey()}, recordID).Result()
			status, ok := parseSessionScriptStatus(value)
			if err != nil || !ok || status != "removed" {
				return nil, ErrUnavailable
			}
		}
	}
	return nil, nil
}

// Consume implements the Session v2 replay boundary. Neither key ID nor nonce
// is retained in plaintext; all Redis and expiry errors fail closed.
func (store *RedisSessionStore) Consume(ctx context.Context, keyID, nonce string, expiresAt time.Time) (bool, error) {
	if store == nil || store.client == nil || ctx == nil || !validSessionStoreIdentifier(keyID) ||
		!validSessionStoreIdentifier(nonce) || expiresAt.IsZero() {
		return false, ErrProtocol
	}
	ttl := expiresAt.UTC().Sub(store.now().UTC())
	if ttl <= 0 || ttl > maxSessionRecordTTL {
		return false, ErrProtocol
	}
	value, err := store.client.RunScript(ctx, redisConsumeSessionNonceScript,
		[]string{store.nonceKey(keyID, nonce)}, durationMillisecondsCeil(ttl)).Result()
	if err != nil {
		return false, ErrUnavailable
	}
	status, ok := parseSessionScriptStatus(value)
	if !ok {
		return false, ErrUnavailable
	}
	return status == "consumed", nil
}

func (store *RedisSessionStore) Acquire(ctx context.Context, deploymentID, resourceID string) (SessionLeaseHandle, error) {
	if store == nil || store.client == nil || ctx == nil || deploymentID != store.deploymentID ||
		!validSessionStoreIdentifier(resourceID) {
		return nil, ErrProtocol
	}
	ownerBytes := make([]byte, 32)
	if _, err := io.ReadFull(store.random, ownerBytes); err != nil {
		return nil, ErrUnavailable
	}
	owner := base64.RawURLEncoding.EncodeToString(ownerBytes)
	key := store.leaseKey(resourceID)
	value, err := store.client.RunScript(ctx, redisAcquireSessionLeaseScript, []string{key}, owner, durationMillisecondsCeil(store.leaseTTL)).Result()
	if err != nil {
		return nil, ErrSessionLeaseLost
	}
	status, ok := parseSessionScriptStatus(value)
	if !ok {
		return nil, ErrSessionLeaseLost
	}
	if status != "acquired" {
		return nil, ErrSessionLeaseHeld
	}
	handleCtx, cancel := context.WithCancel(ctx)
	handle := &redisSessionLeaseHandle{
		store: store, key: key, owner: owner, ctx: handleCtx, cancel: cancel,
		renewDone: make(chan struct{}),
	}
	go handle.renewUntilDone()
	return handle, nil
}

type redisSessionLeaseHandle struct {
	store     *RedisSessionStore
	key       string
	owner     string
	ctx       context.Context
	cancel    context.CancelFunc
	once      sync.Once
	renewDone chan struct{}
}

func (handle *redisSessionLeaseHandle) Context() context.Context {
	if handle == nil || handle.ctx == nil {
		return context.Background()
	}
	return handle.ctx
}

func (handle *redisSessionLeaseHandle) Renew(ctx context.Context) error {
	return handle.compareOwner(ctx, "renew")
}

func (handle *redisSessionLeaseHandle) Owned(ctx context.Context) error {
	return handle.compareOwner(ctx, "owned")
}

func (handle *redisSessionLeaseHandle) Release(ctx context.Context) error {
	if handle == nil || handle.store == nil || ctx == nil {
		return ErrSessionLeaseLost
	}
	value, err := handle.store.client.RunScript(ctx, redisReleaseSessionLeaseScript, []string{handle.key}, handle.owner).Result()
	status, ok := parseSessionScriptStatus(value)
	if err != nil || !ok || status != "released" {
		handle.lose()
		return ErrSessionLeaseLost
	}
	handle.once.Do(handle.cancel)
	return nil
}

func (handle *redisSessionLeaseHandle) compareOwner(ctx context.Context, operation string) error {
	if handle == nil || handle.store == nil || ctx == nil {
		return ErrSessionLeaseLost
	}
	if err := handle.ctx.Err(); err != nil {
		return ErrSessionLeaseLost
	}
	script := redisOwnSessionLeaseScript
	args := []any{handle.owner}
	if operation == "renew" {
		script = redisRenewSessionLeaseScript
		args = append(args, durationMillisecondsCeil(handle.store.leaseTTL))
	}
	value, err := handle.store.client.RunScript(ctx, script, []string{handle.key}, args...).Result()
	status, ok := parseSessionScriptStatus(value)
	if err != nil || !ok || status != operation {
		handle.lose()
		return ErrSessionLeaseLost
	}
	return nil
}

func (handle *redisSessionLeaseHandle) lose() { handle.once.Do(handle.cancel) }

func (handle *redisSessionLeaseHandle) renewUntilDone() {
	defer close(handle.renewDone)
	interval := handle.store.leaseTTL / 3
	if interval <= 0 {
		handle.lose()
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-handle.ctx.Done():
			return
		case <-ticker.C:
			deadline := time.Now().Add(interval)
			renewCtx, cancel := context.WithDeadline(context.Background(), deadline)
			err := handle.Renew(renewCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (store *RedisSessionStore) transitionOperation(
	ctx context.Context,
	sessionID, operationID string,
	mutate func(*persistedSessionOperation) (bool, error),
	transition string,
) (SessionOperationRecord, error) {
	payload, current, err := store.loadOperation(ctx, sessionID, operationID)
	if err != nil {
		return SessionOperationRecord{}, err
	}
	changed, err := mutate(&payload)
	if err != nil {
		return SessionOperationRecord{}, err
	}
	if !changed {
		return sessionOperationProjection(payload), nil
	}
	payload.UpdatedUnixMS = store.now().UTC().UnixMilli()
	sealed, ttl, err := store.sealOperationWithTTL(payload)
	if err != nil {
		return SessionOperationRecord{}, err
	}
	value, err := store.client.RunScript(ctx, redisTransitionSessionOperationScript,
		[]string{store.recordKey(payload.RecordID), store.queueKey()}, current, sealed,
		durationMillisecondsCeil(ttl), payload.RecordID, transition).Result()
	if err != nil {
		return SessionOperationRecord{}, ErrUnavailable
	}
	status, ok := parseSessionScriptStatus(value)
	if !ok || status != "updated" {
		if status == "stale" {
			return SessionOperationRecord{}, ErrSessionOperationConflict
		}
		return SessionOperationRecord{}, ErrUnavailable
	}
	return sessionOperationProjection(payload), nil
}

func (store *RedisSessionStore) loadOperation(ctx context.Context, sessionID, operationID string) (persistedSessionOperation, string, error) {
	if store == nil || store.client == nil || ctx == nil || !validSessionStoreIdentifier(sessionID) || !validSessionStoreIdentifier(operationID) {
		return persistedSessionOperation{}, "", ErrProtocol
	}
	recordID, err := store.client.Get(ctx, store.operationLookupKey(sessionID, operationID)).Result()
	if err != nil || !validOpaqueRecordID(recordID) {
		return persistedSessionOperation{}, "", ErrUnavailable
	}
	payload, current, err := store.loadOperationByRecordID(ctx, recordID)
	if err != nil || payload.SessionID != sessionID || payload.OperationID != operationID {
		return persistedSessionOperation{}, "", ErrUnavailable
	}
	return payload, current, nil
}

func (store *RedisSessionStore) loadOperationByRecordID(ctx context.Context, recordID string) (persistedSessionOperation, string, error) {
	if !validOpaqueRecordID(recordID) {
		return persistedSessionOperation{}, "", ErrUnavailable
	}
	current, err := store.client.Get(ctx, store.recordKey(recordID)).Result()
	if err != nil {
		return persistedSessionOperation{}, "", ErrUnavailable
	}
	payload, err := store.openOperation(current)
	if err != nil || payload.RecordID != recordID || !validPersistedSessionOperation(payload) {
		return persistedSessionOperation{}, "", ErrUnavailable
	}
	return payload, current, nil
}

func (store *RedisSessionStore) projectOperation(ctx context.Context, payload persistedSessionOperation) (SessionOperationRecord, error) {
	projection := sessionOperationProjection(payload)
	if payload.State != SessionOperationSucceeded || payload.ResultDigest == "" {
		return projection, nil
	}
	current, err := store.client.Get(ctx, store.resultKey(payload.RecordID)).Result()
	if err != nil {
		return SessionOperationRecord{}, ErrUnavailable
	}
	result, err := store.openResult(payload.RecordID, current)
	if err != nil {
		return SessionOperationRecord{}, ErrUnavailable
	}
	digest := sha256.Sum256(result)
	if len(projection.ResultDigest) != sha256.Size || subtle.ConstantTimeCompare(digest[:], projection.ResultDigest) != 1 {
		return SessionOperationRecord{}, ErrUnavailable
	}
	projection.Result = result
	return projection, nil
}

func (store *RedisSessionStore) sealOperationWithTTL(payload persistedSessionOperation) (string, time.Duration, error) {
	ttl := store.operationTTL(time.UnixMilli(payload.DeadlineUnixMS), store.now().UTC())
	if ttl <= 0 {
		return "", 0, ErrSessionOperationConflict
	}
	sealed, err := store.sealOperation(payload)
	if err != nil {
		return "", 0, ErrUnavailable
	}
	return sealed, ttl, nil
}

func (store *RedisSessionStore) sealOperation(payload persistedSessionOperation) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(store.keys.Keys[store.keys.ActiveKeyID])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(store.random, nonce); err != nil {
		return "", err
	}
	aad := []byte("coze.sandbox.core_operation.v1|" + store.keys.ActiveKeyID + "|" + store.deploymentHash)
	envelope := encryptedSessionOperationEnvelope{
		Schema: "coze.sandbox.encrypted_core_operation.v1", KeyID: store.keys.ActiveKeyID,
		DeploymentID: store.deploymentHash, Nonce: base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(gcm.Seal(nil, nonce, body, aad)),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > maxSessionOperationEnvelopeBytes {
		return "", ErrProtocol
	}
	return string(encoded), nil
}

func (store *RedisSessionStore) openOperation(value string) (persistedSessionOperation, error) {
	if len(value) == 0 || len(value) > maxSessionOperationEnvelopeBytes {
		return persistedSessionOperation{}, ErrProtocol
	}
	var envelope encryptedSessionOperationEnvelope
	if decodeStrictJSON([]byte(value), &envelope) != nil || envelope.Schema != "coze.sandbox.encrypted_core_operation.v1" ||
		envelope.DeploymentID != store.deploymentHash || !validKeyID(envelope.KeyID) {
		return persistedSessionOperation{}, ErrProtocol
	}
	key := store.keys.Keys[envelope.KeyID]
	if len(key) != sha256.Size {
		return persistedSessionOperation{}, ErrProtocol
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return persistedSessionOperation{}, ErrProtocol
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return persistedSessionOperation{}, ErrProtocol
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return persistedSessionOperation{}, ErrProtocol
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return persistedSessionOperation{}, ErrProtocol
	}
	aad := []byte("coze.sandbox.core_operation.v1|" + envelope.KeyID + "|" + store.deploymentHash)
	body, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return persistedSessionOperation{}, ErrProtocol
	}
	var payload persistedSessionOperation
	if decodeStrictJSON(body, &payload) != nil {
		return persistedSessionOperation{}, ErrProtocol
	}
	return payload, nil
}

func (store *RedisSessionStore) sealResult(recordID string, result []byte) (string, error) {
	if !validOpaqueRecordID(recordID) || len(result) == 0 || len(result) > maxSessionInlineResultBytes {
		return "", ErrProtocol
	}
	block, err := aes.NewCipher(store.keys.Keys[store.keys.ActiveKeyID])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(store.random, nonce); err != nil {
		return "", err
	}
	aad := []byte("coze.sandbox.core_operation_result.v1|" + store.keys.ActiveKeyID + "|" + store.deploymentHash + "|" + recordID)
	envelope := encryptedSessionResultEnvelope{
		Schema: "coze.sandbox.encrypted_core_operation_result.v1", KeyID: store.keys.ActiveKeyID,
		DeploymentID: store.deploymentHash, RecordID: recordID,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(gcm.Seal(nil, nonce, result, aad)),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > maxSessionResultEnvelopeBytes {
		return "", ErrProtocol
	}
	return string(encoded), nil
}

func (store *RedisSessionStore) openResult(recordID, value string) ([]byte, error) {
	if !validOpaqueRecordID(recordID) || len(value) == 0 || len(value) > maxSessionResultEnvelopeBytes {
		return nil, ErrProtocol
	}
	var envelope encryptedSessionResultEnvelope
	if decodeStrictJSON([]byte(value), &envelope) != nil || envelope.Schema != "coze.sandbox.encrypted_core_operation_result.v1" ||
		envelope.DeploymentID != store.deploymentHash || envelope.RecordID != recordID || !validKeyID(envelope.KeyID) {
		return nil, ErrProtocol
	}
	key := store.keys.Keys[envelope.KeyID]
	if len(key) != sha256.Size {
		return nil, ErrProtocol
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, ErrProtocol
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) > maxSessionInlineResultBytes+32 {
		return nil, ErrProtocol
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrProtocol
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, ErrProtocol
	}
	aad := []byte("coze.sandbox.core_operation_result.v1|" + envelope.KeyID + "|" + store.deploymentHash + "|" + recordID)
	result, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil || len(result) == 0 || len(result) > maxSessionInlineResultBytes {
		return nil, ErrProtocol
	}
	return append([]byte(nil), result...), nil
}

func (store *RedisSessionStore) operationTTL(deadline, now time.Time) time.Duration {
	ttl := store.recordTTL
	if remaining := deadline.UTC().Sub(now.UTC()); remaining < ttl {
		ttl = remaining
	}
	return ttl
}

func (store *RedisSessionStore) newOpaqueID() (string, error) {
	value := make([]byte, 24)
	if _, err := io.ReadFull(store.random, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (store *RedisSessionStore) prefix() string {
	return "coze:sandbox:core:" + store.deploymentHash + ":"
}
func (store *RedisSessionStore) operationLookupKey(sessionID, operationID string) string {
	return store.prefix() + "operation:" + redisHash(sessionID+"\x00"+operationID)
}
func (store *RedisSessionStore) recordKey(recordID string) string {
	return store.prefix() + "record:" + redisHash(recordID)
}
func (store *RedisSessionStore) resultKey(recordID string) string {
	return store.prefix() + "result:" + redisHash(recordID)
}
func (store *RedisSessionStore) activeOperationsKey() string { return store.prefix() + "active" }
func (store *RedisSessionStore) queueKey() string            { return store.prefix() + "queue" }
func (store *RedisSessionStore) activeWeightKey() string     { return store.prefix() + "active-weight" }
func (store *RedisSessionStore) activeUserKey(userHash string) string {
	return store.prefix() + "user:" + userHash + ":active"
}
func (store *RedisSessionStore) activeSessionKey(sessionID string) string {
	return store.prefix() + "session:" + redisHash(sessionID) + ":active"
}
func (store *RedisSessionStore) leaseKey(resourceID string) string {
	return store.prefix() + "lease:" + redisHash(resourceID)
}
func (store *RedisSessionStore) nonceKey(keyID, nonce string) string {
	return store.prefix() + "nonce:" + redisHash(keyID+"\x00"+nonce)
}

func sessionOperationProjection(payload persistedSessionOperation) SessionOperationRecord {
	resultDigest, _ := hex.DecodeString(payload.ResultDigest)
	return SessionOperationRecord{
		SessionID: payload.SessionID, OperationID: payload.OperationID, Kind: payload.Kind, State: payload.State,
		CancelRequested: payload.CancelRequested,
		AcceptedAt:      time.UnixMilli(payload.AcceptedUnixMS).UTC(), UpdatedAt: time.UnixMilli(payload.UpdatedUnixMS).UTC(),
		Deadline: time.UnixMilli(payload.DeadlineUnixMS).UTC(), ResultDigest: resultDigest, ReasonCode: payload.ReasonCode,
	}
}

func validSessionOperationInput(input SessionOperationInput, now time.Time) bool {
	return validSessionStoreIdentifier(input.SessionID) && validSessionStoreIdentifier(input.OperationID) && input.UserID > 0 &&
		validSessionOperationKind(input.Kind) && len(input.RequestDigest) == sha256.Size && !allZeroBytes(input.RequestDigest) &&
		input.Weight >= 1 && input.Weight <= 2 && !input.Deadline.IsZero() && input.Deadline.After(now)
}

func validPersistedSessionOperation(payload persistedSessionOperation) bool {
	requestDigest, err := hex.DecodeString(payload.RequestDigest)
	resultDigest, resultErr := hex.DecodeString(payload.ResultDigest)
	return payload.Schema == "coze.sandbox.core_operation.v1" && validOpaqueRecordID(payload.RecordID) &&
		validSessionStoreIdentifier(payload.SessionID) && validSessionStoreIdentifier(payload.OperationID) &&
		len(payload.UserHash) == sha256.Size*2 && validSessionOperationKind(payload.Kind) &&
		err == nil && len(requestDigest) == sha256.Size && !allZeroBytes(requestDigest) &&
		payload.Weight >= 1 && payload.Weight <= 2 && validSessionOperationState(payload.State) &&
		payload.AcceptedUnixMS > 0 && payload.UpdatedUnixMS >= payload.AcceptedUnixMS && payload.DeadlineUnixMS > payload.AcceptedUnixMS &&
		(resultErr == nil && (len(resultDigest) == 0 || len(resultDigest) == sha256.Size)) && validSessionReason(payload.ReasonCode)
}

func validSessionOperationCompletion(completion SessionOperationCompletion) bool {
	if !isTerminalSessionOperationState(completion.State) || !validSessionReason(completion.ReasonCode) {
		return false
	}
	if len(completion.Result) > maxSessionInlineResultBytes {
		return false
	}
	if completion.State != SessionOperationSucceeded {
		return len(completion.Result) == 0 && len(completion.ResultDigest) == 0
	}
	if len(completion.Result) == 0 {
		return false
	}
	if len(completion.ResultDigest) != sha256.Size || allZeroBytes(completion.ResultDigest) {
		return false
	}
	digest := sha256.Sum256(completion.Result)
	return subtle.ConstantTimeCompare(digest[:], completion.ResultDigest) == 1
}

func isTerminalSessionOperationState(state SessionOperationState) bool {
	return terminalSessionOperationState(state)
}

func validSessionReason(reason string) bool {
	if len(reason) > maxSessionOperationReasonBytes || !utf8.ValidString(reason) {
		return false
	}
	for index := range reason {
		character := reason[index]
		if !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '_' {
			return false
		}
	}
	return true
}

func validSessionStoreIdentifier(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' && character != ':' && character != '+' {
			return false
		}
	}
	return true
}

func validOpaqueRecordID(value string) bool {
	return len(value) == 32 && validSessionStoreIdentifier(value)
}

func allZeroBytes(value []byte) bool {
	if len(value) == 0 {
		return true
	}
	return subtle.ConstantTimeCompare(value, make([]byte, len(value))) == 1
}

func withSessionOperationState(payload persistedSessionOperation, state SessionOperationState) persistedSessionOperation {
	payload.State = state
	return payload
}

func parseSessionScriptStatus(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	default:
		return "", false
	}
}

func parseSessionScriptPair(value any) (string, string, bool) {
	values, ok := value.([]any)
	if !ok || len(values) != 2 {
		return "", "", false
	}
	left, leftOK := parseSessionScriptStatus(values[0])
	right, rightOK := parseSessionScriptStatus(values[1])
	return left, right, leftOK && rightOK
}

const sessionTransitionQueued = "queued"

const redisAcceptSessionOperationScript = `
local function extend_ttl(key, ttl_ms)
  local created = redis.call('PEXPIRE', key, ttl_ms, 'NX')
  if created == 0 then redis.call('PEXPIRE', key, ttl_ms, 'GT') end
end
local existing = redis.call('GET', KEYS[1])
if existing then return {'replay', existing} end
if redis.call('EXISTS', KEYS[2]) == 1 then return {'collision', ''} end
if redis.call('LLEN', KEYS[3]) >= tonumber(ARGV[4]) then return {'capacity', ''} end
redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
redis.call('RPUSH', KEYS[3], ARGV[1])
extend_ttl(KEYS[3], ARGV[3])
return {'accepted', ARGV[1]}
`

const redisTransitionSessionOperationScript = `
local function extend_ttl(key, ttl_ms)
  local created = redis.call('PEXPIRE', key, ttl_ms, 'NX')
  if created == 0 then redis.call('PEXPIRE', key, ttl_ms, 'GT') end
end
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 'stale' end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
if ARGV[5] == 'queued' then
  redis.call('RPUSH', KEYS[2], ARGV[4])
  extend_ttl(KEYS[2], ARGV[3])
end
return 'updated'
`

const redisClaimSessionOperationScript = `
local function extend_ttl(key, ttl_ms)
  local created = redis.call('PEXPIRE', key, ttl_ms, 'NX')
  if created == 0 then redis.call('PEXPIRE', key, ttl_ms, 'GT') end
end
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 'stale' end
local head = redis.call('LINDEX', KEYS[2], 0)
if not head or head ~= ARGV[4] then return 'waiting' end
local weight = tonumber(redis.call('GET', KEYS[3]) or '0')
local user = tonumber(redis.call('GET', KEYS[4]) or '0')
local session = tonumber(redis.call('GET', KEYS[5]) or '0')
if weight + tonumber(ARGV[5]) > tonumber(ARGV[6]) or user >= tonumber(ARGV[7]) or session >= 1 then return 'capacity' end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
redis.call('LPOP', KEYS[2])
redis.call('INCRBY', KEYS[3], ARGV[5])
redis.call('INCR', KEYS[4])
redis.call('INCR', KEYS[5])
extend_ttl(KEYS[3], ARGV[3])
extend_ttl(KEYS[4], ARGV[3])
extend_ttl(KEYS[5], ARGV[3])
return 'started'
`

const redisRotateBlockedSessionOperationScript = `
if redis.call('LINDEX', KEYS[1], 0) ~= ARGV[1] then return 'changed' end
local weight = tonumber(redis.call('GET', KEYS[2]) or '0')
local user = tonumber(redis.call('GET', KEYS[3]) or '0')
local session = tonumber(redis.call('GET', KEYS[4]) or '0')
if weight + tonumber(ARGV[2]) > tonumber(ARGV[3]) then return 'global' end
if user < tonumber(ARGV[4]) and session < 1 then return 'eligible' end
redis.call('LPOP', KEYS[1])
redis.call('RPUSH', KEYS[1], ARGV[1])
return 'rotated'
`

const redisPruneMissingSessionQueueHeadScript = `
if redis.call('LINDEX', KEYS[1], 0) ~= ARGV[1] then return 'changed' end
if redis.call('EXISTS', KEYS[2]) == 1 then return 'present' end
redis.call('LPOP', KEYS[1])
redis.call('LREM', KEYS[3], 0, ARGV[1])
return 'removed'
`

const redisFinishSessionOperationScript = `
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 'stale' end
if redis.call('GET', KEYS[7]) ~= ARGV[7] then return 'lookup_stale' end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
redis.call('PEXPIRE', KEYS[7], ARGV[3])
if ARGV[6] ~= '' then
  redis.call('SET', KEYS[6], ARGV[6], 'PX', ARGV[3])
else
  redis.call('DEL', KEYS[6])
end
redis.call('LREM', KEYS[2], 0, ARGV[4])
local weight = tonumber(redis.call('GET', KEYS[3]) or '0')
local user = tonumber(redis.call('GET', KEYS[4]) or '0')
if weight <= tonumber(ARGV[5]) then redis.call('DEL', KEYS[3]) else redis.call('DECRBY', KEYS[3], ARGV[5]) end
if user <= 1 then redis.call('DEL', KEYS[4]) else redis.call('DECR', KEYS[4]) end
local session = tonumber(redis.call('GET', KEYS[5]) or '0')
if session <= 1 then redis.call('DEL', KEYS[5]) else redis.call('DECR', KEYS[5]) end
return 'finished'
`

const redisRequestCancelSessionOperationScript = `
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 'stale' end
if ARGV[5] == 'true' and redis.call('GET', KEYS[4]) ~= ARGV[6] then return 'lookup_stale' end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
if ARGV[5] == 'true' then
  redis.call('PEXPIRE', KEYS[4], ARGV[3])
  redis.call('LREM', KEYS[2], 0, ARGV[4])
  redis.call('LREM', KEYS[3], 0, ARGV[4])
  return 'canceled'
end
return 'requested'
`

const redisCancelQueuedSessionOperationScript = `
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 'stale' end
if redis.call('GET', KEYS[4]) ~= ARGV[5] then return 'lookup_stale' end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
redis.call('PEXPIRE', KEYS[4], ARGV[3])
redis.call('LREM', KEYS[2], 0, ARGV[4])
redis.call('LREM', KEYS[3], 0, ARGV[4])
return 'canceled'
`

const redisFenceActiveSessionOperationScript = `
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 'stale' end
if redis.call('GET', KEYS[7]) ~= ARGV[7] then return 'lookup_stale' end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
redis.call('PEXPIRE', KEYS[7], ARGV[3])
redis.call('LREM', KEYS[2], 0, ARGV[4])
redis.call('LREM', KEYS[6], 0, ARGV[4])
if ARGV[6] ~= 'true' then return 'fenced' end
local weight = tonumber(redis.call('GET', KEYS[3]) or '0')
local user = tonumber(redis.call('GET', KEYS[4]) or '0')
if weight <= tonumber(ARGV[5]) then redis.call('DEL', KEYS[3]) else redis.call('DECRBY', KEYS[3], ARGV[5]) end
if user <= 1 then redis.call('DEL', KEYS[4]) else redis.call('DECR', KEYS[4]) end
local session = tonumber(redis.call('GET', KEYS[5]) or '0')
if session <= 1 then redis.call('DEL', KEYS[5]) else redis.call('DECR', KEYS[5]) end
return 'fenced'
`

const redisRemoveSessionOperationIndexScript = `
redis.call('LREM', KEYS[1], 0, ARGV[1])
redis.call('LREM', KEYS[2], 0, ARGV[1])
return 'removed'
`

const redisConsumeSessionNonceScript = `
local result = redis.call('SET', KEYS[1], '1', 'PX', ARGV[1], 'NX')
if result then return 'consumed' end
return 'replay'
`

const redisAcquireSessionLeaseScript = `
local result = redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2], 'NX')
if result then return 'acquired' end
return 'held'
`

const redisRenewSessionLeaseScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 'lost' end
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return 'renew'
`

const redisOwnSessionLeaseScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 'lost' end
return 'owned'
`

const redisReleaseSessionLeaseScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 'lost' end
redis.call('DEL', KEYS[1])
return 'released'
`

var _ SessionOperationStore = (*RedisSessionStore)(nil)
