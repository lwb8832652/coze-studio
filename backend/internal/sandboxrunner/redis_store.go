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
	"io"
	"strconv"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const maxRedisQueueDepth = 4096

var ErrExecutionConflict = errors.New("sandbox runner execution conflict")

type RedisStoreConfig struct {
	DeploymentID       string
	ActiveKeyID        string
	Keys               map[string]string
	MaxQueueDepth      int
	PerSpaceQueueDepth int
	PerUserQueueDepth  int
	RecordTTL          time.Duration
}

type RedisStore struct {
	client             cache.Cmdable
	deploymentHash     string
	keys               redisEncryptionKeyring
	maxQueueDepth      int
	perSpaceQueueDepth int
	perUserQueueDepth  int
	recordTTL          time.Duration
	now                func() time.Time
	random             io.Reader
}

type redisEncryptionKeyring struct {
	ActiveKeyID string
	Keys        map[string][]byte
}

type encryptedExecutionEnvelope struct {
	Schema       string `json:"schema"`
	KeyID        string `json:"key_id"`
	DeploymentID string `json:"deployment_id"`
	Nonce        string `json:"nonce"`
	Ciphertext   string `json:"ciphertext"`
}

type persistedExecution struct {
	Schema          string                       `json:"schema"`
	ExecutionID     string                       `json:"execution_id"`
	OperationHash   string                       `json:"operation_hash"`
	RequestDigest   string                       `json:"request_digest"`
	IdentityDigest  string                       `json:"identity_digest"`
	Scope           string                       `json:"scope"`
	WorkloadKind    string                       `json:"workload_kind"`
	DeadlineUnixMS  int64                        `json:"deadline_unix_ms"`
	State           infrasandbox.ExecutionStatus `json:"state"`
	AcceptedUnixMS  int64                        `json:"accepted_unix_ms"`
	UpdatedUnixMS   int64                        `json:"updated_unix_ms"`
	RequestEnvelope string                       `json:"request_envelope"`
	Identity        string                       `json:"identity"`
}

func NewRedisStore(client cache.Cmdable, config RedisStoreConfig) (*RedisStore, error) {
	if client == nil || !validKeyID(config.DeploymentID) || !validKeyID(config.ActiveKeyID) ||
		config.MaxQueueDepth < 1 || config.MaxQueueDepth > maxRedisQueueDepth || config.PerSpaceQueueDepth < 1 || config.PerSpaceQueueDepth > config.MaxQueueDepth ||
		config.PerUserQueueDepth < 1 || config.PerUserQueueDepth > config.PerSpaceQueueDepth || config.RecordTTL <= 0 || config.RecordTTL > 24*time.Hour {
		return nil, ErrConfiguration
	}
	keys := make(map[string][]byte, len(config.Keys))
	for keyID, value := range config.Keys {
		if !validKeyID(keyID) || len(value) < 16 {
			return nil, ErrConfiguration
		}
		digest := sha256.Sum256([]byte(value))
		keys[keyID] = digest[:]
	}
	if len(keys[config.ActiveKeyID]) != sha256.Size {
		return nil, ErrConfiguration
	}
	return &RedisStore{
		client: client, deploymentHash: redisHash(config.DeploymentID),
		keys:          redisEncryptionKeyring{ActiveKeyID: config.ActiveKeyID, Keys: keys},
		maxQueueDepth: config.MaxQueueDepth, perSpaceQueueDepth: config.PerSpaceQueueDepth, perUserQueueDepth: config.PerUserQueueDepth, recordTTL: config.RecordTTL,
		now: func() time.Time { return time.Now().UTC() }, random: rand.Reader,
	}, nil
}

func (store *RedisStore) Accept(ctx context.Context, command ExecuteCommand) (StoredExecution, bool, error) {
	if store == nil || store.client == nil || ctx == nil || !validIdentifier(command.IdempotencyKey) || command.Identity.SpaceID <= 0 || command.Identity.UserID <= 0 ||
		len(command.RawBody) == 0 || len(command.RawBody) > maxRequestBytes || command.Deadline.IsZero() || !command.Deadline.After(store.now()) {
		return StoredExecution{}, false, ErrProtocol
	}
	digest := sha256.Sum256(command.RawBody)
	requestDigest := hex.EncodeToString(digest[:])
	operationHash := redisHash(command.IdempotencyKey)
	operationKey := store.operationKey(operationHash)
	acceptedAt := store.now().UTC()
	recordTTL := store.recordTTL
	if remaining := command.Deadline.UTC().Sub(acceptedAt); remaining < recordTTL {
		recordTTL = remaining
	}
	if recordTTL <= 0 {
		return StoredExecution{}, false, ErrProtocol
	}
	executionID, err := store.newExecutionID()
	if err != nil {
		return StoredExecution{}, false, ErrUnavailable
	}
	identity := encodedIdentity(command.Identity)
	if identity == "" {
		return StoredExecution{}, false, ErrUnavailable
	}
	payload := persistedExecution{Schema: "coze.sandbox.runner_execution.v1", ExecutionID: executionID,
		OperationHash: operationHash, RequestDigest: requestDigest, Scope: string(command.Scope), WorkloadKind: string(command.WorkloadKind),
		DeadlineUnixMS: command.Deadline.UTC().UnixMilli(), State: ExecutionStateAccepted, AcceptedUnixMS: acceptedAt.UnixMilli(), UpdatedUnixMS: acceptedAt.UnixMilli(),
		RequestEnvelope: base64.RawURLEncoding.EncodeToString(command.RawBody), Identity: identity, IdentityDigest: redisHash(identity)}
	sealed, err := store.seal(payload)
	if err != nil {
		return StoredExecution{}, false, ErrUnavailable
	}
	result, err := store.client.RunScript(ctx, redisAcceptExecutionScript,
		[]string{operationKey, store.executionKey(executionID), store.queueKey(), store.spaceQueueKey(command.Identity.SpaceID), store.userQueueKey(command.Identity.SpaceID, command.Identity.UserID), store.activeQueueKey()},
		store.maxQueueDepth, store.perSpaceQueueDepth, store.perUserQueueDepth, sealed, durationMillisecondsCeil(recordTTL), executionID).Result()
	if err != nil {
		return StoredExecution{}, false, ErrUnavailable
	}
	status, replayExecutionID, ok := parseRedisAcceptResult(result)
	if !ok {
		return StoredExecution{}, false, ErrUnavailable
	}
	switch status {
	case "accepted":
		return storedProjection(payload), false, nil
	case "replay":
		record, err := store.Get(ctx, replayExecutionID)
		if err != nil || record.RequestDigest != requestDigest {
			return StoredExecution{}, false, ErrExecutionConflict
		}
		return record, true, nil
	case "capacity":
		return StoredExecution{}, false, ErrUnavailable
	default:
		return StoredExecution{}, false, ErrUnavailable
	}
}

func (store *RedisStore) Get(ctx context.Context, executionID string) (StoredExecution, error) {
	if store == nil || store.client == nil || ctx == nil || !validExecutionID(executionID) {
		return StoredExecution{}, ErrProtocol
	}
	value, err := store.client.Get(ctx, store.executionKey(executionID)).Result()
	if err != nil {
		if errors.Is(err, cache.Nil) {
			return StoredExecution{}, ErrUnavailable
		}
		return StoredExecution{}, ErrUnavailable
	}
	payload, err := store.open(value)
	if err != nil || payload.ExecutionID != executionID || payload.Schema != "coze.sandbox.runner_execution.v1" || !validPersistedExecution(payload) {
		return StoredExecution{}, ErrUnavailable
	}
	return storedProjection(payload), nil
}

func (store *RedisStore) Transition(ctx context.Context, executionID string, next infrasandbox.ExecutionStatus) (StoredExecution, error) {
	if store == nil || store.client == nil || ctx == nil || !validExecutionID(executionID) || !validExecutionState(next) {
		return StoredExecution{}, ErrProtocol
	}
	key := store.executionKey(executionID)
	for attempts := 0; attempts < 3; attempts++ {
		currentCiphertext, err := store.client.Get(ctx, key).Result()
		if err != nil {
			return StoredExecution{}, ErrUnavailable
		}
		payload, err := store.open(currentCiphertext)
		if err != nil || payload.ExecutionID != executionID || !canTransition(payload.State, next) {
			return StoredExecution{}, ErrExecutionConflict
		}
		identity, err := decodeIdentity(payload.Identity)
		if err != nil || identity.SpaceID <= 0 || identity.UserID <= 0 {
			return StoredExecution{}, ErrUnavailable
		}
		payload.State, payload.UpdatedUnixMS = next, store.now().UTC().UnixMilli()
		nextCiphertext, err := store.seal(payload)
		if err != nil {
			return StoredExecution{}, ErrUnavailable
		}
		recordTTL := store.recordTTL
		if remaining := time.UnixMilli(payload.DeadlineUnixMS).UTC().Sub(store.now().UTC()); remaining < recordTTL {
			recordTTL = remaining
		}
		if recordTTL <= 0 {
			return StoredExecution{}, ErrExecutionConflict
		}
		terminal := ""
		if isTerminalExecutionState(next) {
			terminal = "terminal"
		}
		result, err := store.client.RunScript(ctx, redisTransitionExecutionScript,
			[]string{key, store.queueKey(), store.spaceQueueKey(identity.SpaceID), store.userQueueKey(identity.SpaceID, identity.UserID), store.activeQueueKey()}, currentCiphertext, nextCiphertext, durationMillisecondsCeil(recordTTL), terminal, executionID).Result()
		if err != nil {
			return StoredExecution{}, ErrUnavailable
		}
		status, ok := parseRedisTransitionResult(result)
		switch {
		case !ok:
			return StoredExecution{}, ErrUnavailable
		case status == "updated":
			return storedProjection(payload), nil
		case status == "stale":
			continue
		default:
			return StoredExecution{}, ErrExecutionConflict
		}
	}
	return StoredExecution{}, ErrExecutionConflict
}

func (store *RedisStore) Status(ctx context.Context, executionID string) (infrasandbox.ExecuteResult, error) {
	stored, err := store.Get(ctx, executionID)
	if err != nil {
		return infrasandbox.ExecuteResult{}, err
	}
	return infrasandbox.ExecuteResult{ExecutionID: stored.ExecutionID, Status: stored.State}, nil
}

func (store *RedisStore) Lookup(ctx context.Context, request infrasandbox.ExecutionLookupRequest) (infrasandbox.ExecutionLookupResult, error) {
	normalized, err := infrasandbox.NormalizeExecutionLookupRequest(request)
	if err != nil {
		return infrasandbox.ExecutionLookupResult{}, ErrProtocol
	}
	if normalized.Legacy {
		return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, nil
	}
	executionID, err := store.client.Get(ctx, store.operationKey(redisHash(normalized.OperationID))).Result()
	if err != nil {
		if errors.Is(err, cache.Nil) {
			return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupNotFound}, nil
		}
		return infrasandbox.ExecutionLookupResult{}, ErrUnavailable
	}
	stored, err := store.Get(ctx, executionID)
	if err != nil {
		return infrasandbox.ExecutionLookupResult{}, err
	}
	if !constantTimeEqual(stored.RequestDigest, normalized.RequestDigest.Hex()) || stored.Scope != string(normalized.Scope) || stored.WorkloadKind != string(normalized.WorkloadKind) {
		return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, nil
	}
	return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupFound, Execution: infrasandbox.ExecuteResult{ExecutionID: stored.ExecutionID, Status: stored.State}}, nil
}

func (store *RedisStore) QueueStatus(ctx context.Context, executionID string) (infrasandbox.QueueStatus, error) {
	stored, err := store.Get(ctx, executionID)
	if err != nil {
		return infrasandbox.QueueStatus{}, err
	}
	waiting := stored.State == ExecutionStateAccepted
	return infrasandbox.QueueStatus{Schema: infrasandbox.QueueStatusSchemaV1, Waiting: waiting, ApproximatePosition: 0,
		EstimatedWaitSeconds: 0, DeadlineUnixMilli: stored.Deadline.UnixMilli(), Cancelable: waiting, ReasonCode: ""}, nil
}

func (store *RedisStore) KeepAlive(context.Context, string) error { return nil }

func (store *RedisStore) Cancel(ctx context.Context, executionID string) error {
	if store == nil || ctx == nil || !validExecutionID(executionID) {
		return ErrProtocol
	}
	for attempts := 0; attempts < 3; attempts++ {
		stored, err := store.Get(ctx, executionID)
		if err != nil {
			return err
		}
		if stored.State == infrasandbox.ExecutionStatusCanceled {
			return nil
		}
		if isTerminalExecutionState(stored.State) {
			return ErrExecutionConflict
		}
		if _, err := store.Transition(ctx, executionID, infrasandbox.ExecutionStatusCanceled); err == nil {
			return nil
		} else if !errors.Is(err, ErrExecutionConflict) {
			return err
		}
	}
	return ErrExecutionConflict
}

func (store *RedisStore) Recover(ctx context.Context) ([]StoredExecution, error) {
	recovered, err := store.RecoverExecutions(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]StoredExecution, 0, len(recovered))
	for _, execution := range recovered {
		values = append(values, execution.Stored)
	}
	return values, nil
}

func (store *RedisStore) RecoverExecutions(ctx context.Context) ([]RecoveredExecution, error) {
	if store == nil || store.client == nil || ctx == nil {
		return nil, ErrProtocol
	}
	executionIDs, err := store.client.LRange(ctx, store.activeQueueKey(), 0, -1).Result()
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(executionIDs) > store.maxQueueDepth {
		return nil, ErrUnavailable
	}
	recovered := make([]RecoveredExecution, 0, len(executionIDs))
	seen := make(map[string]struct{}, len(executionIDs))
	for _, executionID := range executionIDs {
		if !validExecutionID(executionID) {
			return nil, ErrUnavailable
		}
		if _, exists := seen[executionID]; exists {
			return nil, ErrUnavailable
		}
		seen[executionID] = struct{}{}
		payload, err := store.loadPersisted(ctx, executionID)
		if err != nil {
			// A separately expired record is stale index data. It is not evidence
			// to create a new logical execution, so it is skipped fail-closed.
			continue
		}
		if payload.State == ExecutionStateAccepted || payload.State == ExecutionStateRunning {
			command, err := recoveredCommand(payload)
			if err != nil {
				return nil, ErrUnavailable
			}
			recovered = append(recovered, RecoveredExecution{Stored: storedProjection(payload), Command: command})
		}
	}
	return recovered, nil
}

func (store *RedisStore) loadPersisted(ctx context.Context, executionID string) (persistedExecution, error) {
	value, err := store.client.Get(ctx, store.executionKey(executionID)).Result()
	if err != nil {
		return persistedExecution{}, ErrUnavailable
	}
	payload, err := store.open(value)
	if err != nil || payload.ExecutionID != executionID || payload.Schema != "coze.sandbox.runner_execution.v1" || !validPersistedExecution(payload) {
		return persistedExecution{}, ErrUnavailable
	}
	return payload, nil
}

func recoveredCommand(payload persistedExecution) (ExecuteCommand, error) {
	raw, err := base64.RawURLEncoding.DecodeString(payload.RequestEnvelope)
	if err != nil {
		return ExecuteCommand{}, err
	}
	command, err := parseExecute(raw)
	if err != nil || command.Scope != domainsandbox.Scope(payload.Scope) || command.WorkloadKind != infrasandbox.WorkloadKind(payload.WorkloadKind) || command.Deadline.UnixMilli() != payload.DeadlineUnixMS {
		return ExecuteCommand{}, ErrProtocol
	}
	identity, err := decodeIdentity(payload.Identity)
	if err != nil || redisHash(payload.Identity) != payload.IdentityDigest {
		return ExecuteCommand{}, ErrProtocol
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != payload.RequestDigest {
		return ExecuteCommand{}, ErrProtocol
	}
	command.Identity = identity
	return command, nil
}

func (store *RedisStore) seal(payload persistedExecution) (string, error) {
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
	aad := []byte("coze.sandbox.runner_execution.v1|" + store.keys.ActiveKeyID + "|" + store.deploymentHash)
	envelope := encryptedExecutionEnvelope{Schema: "coze.sandbox.runner_encrypted_execution.v1", KeyID: store.keys.ActiveKeyID, DeploymentID: store.deploymentHash,
		Nonce: base64.RawURLEncoding.EncodeToString(nonce), Ciphertext: base64.RawURLEncoding.EncodeToString(gcm.Seal(nil, nonce, body, aad))}
	encoded, err := json.Marshal(envelope)
	return string(encoded), err
}

func (store *RedisStore) open(value string) (persistedExecution, error) {
	var envelope encryptedExecutionEnvelope
	if err := decodeStrictJSON([]byte(value), &envelope); err != nil || envelope.Schema != "coze.sandbox.runner_encrypted_execution.v1" ||
		envelope.DeploymentID != store.deploymentHash || !validKeyID(envelope.KeyID) {
		return persistedExecution{}, ErrProtocol
	}
	key := store.keys.Keys[envelope.KeyID]
	if len(key) != sha256.Size {
		return persistedExecution{}, ErrProtocol
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return persistedExecution{}, err
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return persistedExecution{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return persistedExecution{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return persistedExecution{}, ErrProtocol
	}
	aad := []byte("coze.sandbox.runner_execution.v1|" + envelope.KeyID + "|" + store.deploymentHash)
	body, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return persistedExecution{}, err
	}
	var payload persistedExecution
	if err := decodeStrictJSON(body, &payload); err != nil {
		return persistedExecution{}, err
	}
	return payload, nil
}

func (store *RedisStore) newExecutionID() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := io.ReadFull(store.random, randomBytes); err != nil {
		return "", err
	}
	return "exec-" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func (store *RedisStore) executionKey(executionID string) string {
	return "sandbox:runner:" + store.deploymentHash + ":execution:" + executionID
}
func (store *RedisStore) operationKey(hash string) string {
	return "sandbox:runner:" + store.deploymentHash + ":operation:" + hash
}
func (store *RedisStore) queueKey() string {
	return "sandbox:runner:" + store.deploymentHash + ":queue-depth"
}
func (store *RedisStore) activeQueueKey() string {
	return "sandbox:runner:" + store.deploymentHash + ":active-executions"
}
func (store *RedisStore) spaceQueueKey(spaceID int64) string {
	return "sandbox:runner:" + store.deploymentHash + ":space:" + redisHash(strconv.FormatInt(spaceID, 10)) + ":queue-depth"
}
func (store *RedisStore) userQueueKey(spaceID, userID int64) string {
	return "sandbox:runner:" + store.deploymentHash + ":space:" + redisHash(strconv.FormatInt(spaceID, 10)) + ":user:" + redisHash(strconv.FormatInt(userID, 10)) + ":queue-depth"
}
func redisHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func storedProjection(payload persistedExecution) StoredExecution {
	return StoredExecution{ExecutionID: payload.ExecutionID, RequestDigest: payload.RequestDigest, Scope: payload.Scope, WorkloadKind: payload.WorkloadKind,
		Deadline: time.UnixMilli(payload.DeadlineUnixMS).UTC(), State: payload.State, AcceptedAt: time.UnixMilli(payload.AcceptedUnixMS).UTC(), UpdatedAt: time.UnixMilli(payload.UpdatedUnixMS).UTC()}
}

func validPersistedExecution(value persistedExecution) bool {
	return value.Schema == "coze.sandbox.runner_execution.v1" && validExecutionID(value.ExecutionID) && len(value.OperationHash) == sha256.Size*2 &&
		len(value.RequestDigest) == sha256.Size*2 && len(value.IdentityDigest) == sha256.Size*2 && validIdentifier(value.Scope) && validIdentifier(value.WorkloadKind) && value.DeadlineUnixMS > 0 &&
		value.AcceptedUnixMS > 0 && value.UpdatedUnixMS >= value.AcceptedUnixMS && validExecutionState(value.State) &&
		value.RequestEnvelope != "" && value.Identity != ""
}

func parseRedisAcceptResult(value any) (string, string, bool) {
	items, ok := value.([]interface{})
	if !ok || len(items) < 1 || len(items) > 2 {
		return "", "", false
	}
	status, ok := items[0].(string)
	if !ok || (status != "accepted" && status != "replay" && status != "capacity") {
		return "", "", false
	}
	if status == "capacity" {
		return status, "", len(items) == 1
	}
	identifier, ok := items[1].(string)
	return status, identifier, ok && validExecutionID(identifier)
}

func parseRedisTransitionResult(value any) (string, bool) {
	items, ok := value.([]interface{})
	if !ok || len(items) != 1 {
		return "", false
	}
	status, ok := items[0].(string)
	return status, ok && (status == "updated" || status == "stale" || status == "missing")
}

func validExecutionState(value infrasandbox.ExecutionStatus) bool {
	switch value {
	case ExecutionStateAccepted, ExecutionStateRunning, infrasandbox.ExecutionStatusSucceeded, infrasandbox.ExecutionStatusFailed,
		infrasandbox.ExecutionStatusCanceled, infrasandbox.ExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}

func isTerminalExecutionState(value infrasandbox.ExecutionStatus) bool {
	return value == infrasandbox.ExecutionStatusSucceeded || value == infrasandbox.ExecutionStatusFailed ||
		value == infrasandbox.ExecutionStatusCanceled || value == infrasandbox.ExecutionStatusTimedOut
}

func canTransition(current, next infrasandbox.ExecutionStatus) bool {
	if isTerminalExecutionState(current) || !validExecutionState(next) || current == next {
		return false
	}
	return (current == ExecutionStateAccepted && (next == ExecutionStateRunning || isTerminalExecutionState(next))) ||
		(current == ExecutionStateRunning && isTerminalExecutionState(next))
}

func constantTimeEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func encodedIdentity(identity sandboxidentity.Request) string {
	encoded, err := json.Marshal(identity)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeIdentity(value string) (sandboxidentity.Request, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return sandboxidentity.Request{}, err
	}
	var identity sandboxidentity.Request
	if err := json.Unmarshal(decoded, &identity); err != nil {
		return sandboxidentity.Request{}, err
	}
	return identity, nil
}

func durationMillisecondsCeil(value time.Duration) int64 {
	return (value.Microseconds() + 999) / 1000
}
