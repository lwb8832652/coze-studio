// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
)

const (
	defaultArtifactGrantRedisNamespace = "appdev:artifact-grant:v1"
	artifactGrantRedisProtocolVersion  = "artifact-grant/v1"
	maxArtifactGrantRedisReplyFields   = 16
	maxArtifactGrantRedisReplyBytes    = 8 * 1024
)

const artifactGrantIssueScript = `
local protocol = "artifact-grant/v1"
if redis.call("EXISTS", KEYS[1]) ~= 0 then
  return {protocol, "conflict"}
end
local now = redis.call("TIME")
local now_us = tonumber(now[1]) * 1000000 + tonumber(now[2])
local expires_us = now_us + tonumber(ARGV[2])
redis.call("HSET", KEYS[1],
  "schema_version", "1",
  "state", "issued",
  "token_hash", ARGV[3],
  "space_id", ARGV[4],
  "project_id", ARGV[5],
  "provider_key", ARGV[6],
  "provider_scope", ARGV[7],
  "operation", ARGV[8],
  "direction", ARGV[9],
  "object_key", ARGV[10],
  "digest", ARGV[11],
  "size", ARGV[12],
  "max_size", ARGV[13],
  "issued_at_us", tostring(now_us),
  "expires_at_us", tostring(expires_us))
redis.call("PEXPIRE", KEYS[1], ARGV[1])
return {protocol, "ok", tostring(now_us), tostring(expires_us)}
`

const artifactGrantResolveAudienceScript = `
local protocol = "artifact-grant/v1"
if redis.call("EXISTS", KEYS[1]) == 0 then
  return {protocol, "not_found"}
end
if redis.call("HGET", KEYS[1], "schema_version") ~= "1" or redis.call("PTTL", KEYS[1]) <= 0 then
  return {protocol, "expired"}
end
return {
  protocol,
  "ok",
  redis.call("HGET", KEYS[1], "space_id"),
  redis.call("HGET", KEYS[1], "project_id"),
  redis.call("HGET", KEYS[1], "provider_key"),
  redis.call("HGET", KEYS[1], "provider_scope"),
  redis.call("HGET", KEYS[1], "operation")
}
`

const artifactGrantTokenHashCompareScript = `
local function artifact_grant_token_hash_equal(left, right)
  if type(left) ~= "string" or type(right) ~= "string" or
     string.len(left) ~= 64 or string.len(right) ~= 64 then
    return false
  end
  local difference = 0
  local invalid = 0
  for index = 1, 64 do
    local left_byte = string.byte(left, index)
    local right_byte = string.byte(right, index)
    difference = difference + math.abs(left_byte - right_byte)
    local left_hex = (left_byte >= 48 and left_byte <= 57) or (left_byte >= 97 and left_byte <= 102)
    local right_hex = (right_byte >= 48 and right_byte <= 57) or (right_byte >= 97 and right_byte <= 102)
    if not left_hex then
      invalid = invalid + 1
    end
    if not right_hex then
      invalid = invalid + 1
    end
  end
  return difference == 0 and invalid == 0
end
`

const artifactGrantConsumeScript = artifactGrantTokenHashCompareScript + `
local protocol = "artifact-grant/v1"
if redis.call("EXISTS", KEYS[1]) == 0 then
  return {protocol, "not_found"}
end
if redis.call("HGET", KEYS[1], "schema_version") ~= "1" then
  return {protocol, "malformed"}
end
local now = redis.call("TIME")
local now_us = tonumber(now[1]) * 1000000 + tonumber(now[2])
local expires_us = tonumber(redis.call("HGET", KEYS[1], "expires_at_us"))
if expires_us == nil then
  return {protocol, "malformed"}
end
if expires_us <= now_us then
  redis.call("DEL", KEYS[1])
  return {protocol, "expired"}
end
if not artifact_grant_token_hash_equal(redis.call("HGET", KEYS[1], "token_hash"), ARGV[1]) then
  return {protocol, "token"}
end
if redis.call("HGET", KEYS[1], "space_id") ~= ARGV[2] or
   redis.call("HGET", KEYS[1], "project_id") ~= ARGV[3] or
   redis.call("HGET", KEYS[1], "provider_key") ~= ARGV[4] or
   redis.call("HGET", KEYS[1], "provider_scope") ~= ARGV[5] or
   redis.call("HGET", KEYS[1], "operation") ~= ARGV[6] then
  return {protocol, "audience"}
end
if redis.call("HGET", KEYS[1], "direction") ~= ARGV[7] then
  return {protocol, "direction"}
end
local state = redis.call("HGET", KEYS[1], "state")
if state == "revoked" then
  return {protocol, "revoked"}
end
if state == "consuming" or state == "consumed" then
  return {protocol, "consumed"}
end
if state ~= "issued" then
  return {protocol, "malformed"}
end
redis.call("HSET", KEYS[1], "state", "consumed")
return {protocol, "ok", "consumed",
  redis.call("HGET", KEYS[1], "space_id"),
  redis.call("HGET", KEYS[1], "project_id"),
  redis.call("HGET", KEYS[1], "provider_key"),
  redis.call("HGET", KEYS[1], "provider_scope"),
  redis.call("HGET", KEYS[1], "operation"),
  redis.call("HGET", KEYS[1], "direction"),
  redis.call("HGET", KEYS[1], "object_key"),
  redis.call("HGET", KEYS[1], "digest"),
  redis.call("HGET", KEYS[1], "size"),
  redis.call("HGET", KEYS[1], "max_size"),
  redis.call("HGET", KEYS[1], "issued_at_us"),
  redis.call("HGET", KEYS[1], "expires_at_us")}
`

const artifactGrantRevokeScript = artifactGrantTokenHashCompareScript + `
local protocol = "artifact-grant/v1"
if redis.call("EXISTS", KEYS[1]) == 0 then
  return {protocol, "not_found"}
end
if redis.call("HGET", KEYS[1], "schema_version") ~= "1" then
  return {protocol, "malformed"}
end
local now = redis.call("TIME")
local now_us = tonumber(now[1]) * 1000000 + tonumber(now[2])
local expires_us = tonumber(redis.call("HGET", KEYS[1], "expires_at_us"))
if expires_us == nil then
  return {protocol, "malformed"}
end
if expires_us <= now_us then
  redis.call("DEL", KEYS[1])
  return {protocol, "expired"}
end
if not artifact_grant_token_hash_equal(redis.call("HGET", KEYS[1], "token_hash"), ARGV[1]) then
  return {protocol, "token"}
end
if redis.call("HGET", KEYS[1], "space_id") ~= ARGV[2] or
   redis.call("HGET", KEYS[1], "project_id") ~= ARGV[3] or
   redis.call("HGET", KEYS[1], "provider_key") ~= ARGV[4] or
   redis.call("HGET", KEYS[1], "provider_scope") ~= ARGV[5] or
   redis.call("HGET", KEYS[1], "operation") ~= ARGV[6] then
  return {protocol, "audience"}
end
if redis.call("HGET", KEYS[1], "direction") ~= ARGV[7] then
  return {protocol, "direction"}
end
local state = redis.call("HGET", KEYS[1], "state")
if state == "consuming" or state == "consumed" then
  return {protocol, "consumed"}
end
if state ~= "issued" and state ~= "revoked" then
  return {protocol, "malformed"}
end
if state == "issued" then
  redis.call("HSET", KEYS[1], "state", "revoked")
end
return {protocol, "ok", "revoked",
  redis.call("HGET", KEYS[1], "space_id"),
  redis.call("HGET", KEYS[1], "project_id"),
  redis.call("HGET", KEYS[1], "provider_key"),
  redis.call("HGET", KEYS[1], "provider_scope"),
  redis.call("HGET", KEYS[1], "operation"),
  redis.call("HGET", KEYS[1], "direction"),
  redis.call("HGET", KEYS[1], "object_key"),
  redis.call("HGET", KEYS[1], "digest"),
  redis.call("HGET", KEYS[1], "size"),
  redis.call("HGET", KEYS[1], "max_size"),
  redis.call("HGET", KEYS[1], "issued_at_us"),
  redis.call("HGET", KEYS[1], "expires_at_us")}
`

type ArtifactGrantRedisRepositoryOption func(*RedisArtifactGrantRepository) error

func WithArtifactGrantRedisNamespace(namespace string) ArtifactGrantRedisRepositoryOption {
	return func(repository *RedisArtifactGrantRepository) error {
		if !validArtifactGrantRedisNamespace(namespace) {
			return domainappdev.ErrArtifactGrantInvalid
		}
		repository.namespace = namespace
		return nil
	}
}

type RedisArtifactGrantRepository struct {
	client    cache.ScriptCmdable
	namespace string
}

func NewRedisArtifactGrantRepository(client cache.ScriptCmdable, options ...ArtifactGrantRedisRepositoryOption) (*RedisArtifactGrantRepository, error) {
	if client == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	repository := &RedisArtifactGrantRepository{client: client, namespace: defaultArtifactGrantRedisNamespace}
	for _, option := range options {
		if option == nil || option(repository) != nil {
			return nil, domainappdev.ErrArtifactGrantInvalid
		}
	}
	return repository, nil
}

// ResolveArtifactGrantAudience returns only the bounded audience needed by the
// independent provider-authentication boundary. It never returns the token,
// object key, digest, or transfer limits.
func (repository *RedisArtifactGrantRepository) ResolveArtifactGrantAudience(
	ctx context.Context,
	grantID domainappdev.ArtifactGrantID,
) (domainappdev.ArtifactGrantAudience, error) {
	if repository == nil || grantID.IsZero() {
		return domainappdev.ArtifactGrantAudience{}, domainappdev.ErrArtifactGrantInvalid
	}
	if err := artifactGrantRedisContextError(ctx); err != nil {
		return domainappdev.ArtifactGrantAudience{}, err
	}
	result, runErr := repository.client.RunScript(
		ctx,
		artifactGrantResolveAudienceScript,
		[]string{artifactGrantRedisKey(repository.namespace, grantID)},
	).Result()
	if runErr != nil {
		return domainappdev.ArtifactGrantAudience{}, domainappdev.ErrArtifactGrantUnavailable
	}
	reply, err := parseArtifactGrantRedisReply(result)
	if err != nil || len(reply) < 2 || reply[0] != artifactGrantRedisProtocolVersion {
		return domainappdev.ArtifactGrantAudience{}, domainappdev.ErrArtifactGrantUnavailable
	}
	if reply[1] != "ok" {
		return domainappdev.ArtifactGrantAudience{}, artifactGrantRedisCodeError(reply)
	}
	if len(reply) != 7 {
		return domainappdev.ArtifactGrantAudience{}, domainappdev.ErrArtifactGrantUnavailable
	}
	audience := domainappdev.ArtifactGrantAudience{
		SpaceID: reply[2], ProjectID: reply[3], ProviderKey: reply[4],
		ProviderScope: domainsandbox.Scope(reply[5]), Operation: reply[6],
	}
	if domainappdev.ValidateArtifactGrantAudience(audience) != nil {
		return domainappdev.ArtifactGrantAudience{}, domainappdev.ErrArtifactGrantUnavailable
	}
	return audience, nil
}

func (repository *RedisArtifactGrantRepository) Issue(ctx context.Context, input domainappdev.IssueArtifactGrantRepositoryInput) (*domainappdev.ArtifactGrantRecord, error) {
	if contextErr := artifactGrantRedisContextError(ctx); contextErr != nil {
		return nil, contextErr
	}
	if input.GrantID.IsZero() || input.TokenHash.IsZero() ||
		domainappdev.ValidateArtifactGrantTTL(input.TTL) != nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	normalized, err := domainappdev.NormalizeArtifactGrantSpec(input.Spec)
	if err != nil || normalized != input.Spec {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	ttlMicros := input.TTL.Microseconds()
	if ttlMicros <= 0 {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	ttlMillis := (ttlMicros + 999) / 1000
	result, runErr := repository.client.RunScript(ctx, artifactGrantIssueScript,
		[]string{artifactGrantRedisKey(repository.namespace, input.GrantID)},
		ttlMillis, ttlMicros, hex.EncodeToString(input.TokenHash.Bytes()),
		input.Spec.Audience.SpaceID, input.Spec.Audience.ProjectID, input.Spec.Audience.ProviderKey,
		string(input.Spec.Audience.ProviderScope), input.Spec.Audience.Operation, string(input.Spec.Direction),
		input.Spec.ObjectKey, input.Spec.Digest.String(), strconv.FormatInt(input.Spec.Size, 10), strconv.FormatInt(input.Spec.MaxSize, 10),
	).Result()
	if runErr != nil {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	reply, parseErr := parseArtifactGrantRedisReply(result)
	if parseErr != nil {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	if reply[1] != "ok" {
		return nil, artifactGrantRedisCodeError(reply)
	}
	if len(reply) != 4 {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	issuedMicros, ok := parseArtifactGrantRedisInt64(reply[2])
	if !ok {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	expiresMicros, ok := parseArtifactGrantRedisInt64(reply[3])
	if !ok {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	record := &domainappdev.ArtifactGrantRecord{
		GrantID: input.GrantID, Spec: input.Spec, State: domainappdev.ArtifactGrantStateIssued,
		IssuedAt: time.UnixMicro(issuedMicros).UTC(), ExpiresAt: time.UnixMicro(expiresMicros).UTC(),
	}
	if !validArtifactGrantRedisRecord(record, input.GrantID, input.Spec.Audience, input.Spec.Direction, domainappdev.ArtifactGrantStateIssued) ||
		record.ExpiresAt.Sub(record.IssuedAt) > input.TTL {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return record, nil
}

func (repository *RedisArtifactGrantRepository) Consume(ctx context.Context, input domainappdev.ConsumeArtifactGrantRepositoryInput) (*domainappdev.ArtifactGrantRecord, error) {
	if err := validateArtifactGrantRedisCapability(ctx, input.GrantID, input.TokenHash, input.Audience, input.Direction); err != nil {
		return nil, err
	}
	result, runErr := repository.client.RunScript(ctx, artifactGrantConsumeScript,
		[]string{artifactGrantRedisKey(repository.namespace, input.GrantID)}, artifactGrantRedisCapabilityArgs(input.TokenHash, input.Audience, input.Direction)...).Result()
	if runErr != nil {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return repository.parseCapabilityResult(result, input.GrantID, input.Audience, input.Direction, domainappdev.ArtifactGrantStateConsumed)
}

func (repository *RedisArtifactGrantRepository) Revoke(ctx context.Context, input domainappdev.RevokeArtifactGrantRepositoryInput) (*domainappdev.ArtifactGrantRecord, error) {
	if err := validateArtifactGrantRedisCapability(ctx, input.GrantID, input.TokenHash, input.Audience, input.Direction); err != nil {
		return nil, err
	}
	result, runErr := repository.client.RunScript(ctx, artifactGrantRevokeScript,
		[]string{artifactGrantRedisKey(repository.namespace, input.GrantID)}, artifactGrantRedisCapabilityArgs(input.TokenHash, input.Audience, input.Direction)...).Result()
	if runErr != nil {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return repository.parseCapabilityResult(result, input.GrantID, input.Audience, input.Direction, domainappdev.ArtifactGrantStateRevoked)
}

func (repository *RedisArtifactGrantRepository) parseCapabilityResult(result any, grantID domainappdev.ArtifactGrantID, audience domainappdev.ArtifactGrantAudience, direction domainappdev.ArtifactGrantDirection, expectedState domainappdev.ArtifactGrantState) (*domainappdev.ArtifactGrantRecord, error) {
	reply, err := parseArtifactGrantRedisReply(result)
	if err != nil {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	if reply[1] != "ok" {
		return nil, artifactGrantRedisCodeError(reply)
	}
	if len(reply) != 15 || reply[2] != string(expectedState) {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	digest, err := domainappdev.ParseArtifactGrantDigest(reply[10])
	if err != nil {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	size, sizeOK := parseArtifactGrantRedisInt64(reply[11])
	maxSize, maxSizeOK := parseArtifactGrantRedisInt64(reply[12])
	issuedMicros, issuedOK := parseArtifactGrantRedisInt64(reply[13])
	expiresMicros, expiresOK := parseArtifactGrantRedisInt64(reply[14])
	if !sizeOK || !maxSizeOK || !issuedOK || !expiresOK {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	record := &domainappdev.ArtifactGrantRecord{
		GrantID: grantID,
		Spec: domainappdev.ArtifactGrantSpec{
			Audience: domainappdev.ArtifactGrantAudience{
				SpaceID: reply[3], ProjectID: reply[4], ProviderKey: reply[5],
				ProviderScope: domainsandbox.Scope(reply[6]), Operation: reply[7],
			},
			Direction: domainappdev.ArtifactGrantDirection(reply[8]), ObjectKey: reply[9],
			Digest: digest, Size: size, MaxSize: maxSize,
		},
		State: expectedState, IssuedAt: time.UnixMicro(issuedMicros).UTC(), ExpiresAt: time.UnixMicro(expiresMicros).UTC(),
	}
	if !validArtifactGrantRedisRecord(record, grantID, audience, direction, expectedState) {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return record, nil
}

func artifactGrantRedisKey(namespace string, grantID domainappdev.ArtifactGrantID) string {
	return namespace + ":{" + grantID.Encoded() + "}"
}

func artifactGrantRedisCapabilityArgs(hash domainappdev.ArtifactGrantTokenHash, audience domainappdev.ArtifactGrantAudience, direction domainappdev.ArtifactGrantDirection) []interface{} {
	return []interface{}{
		hex.EncodeToString(hash.Bytes()), audience.SpaceID, audience.ProjectID, audience.ProviderKey,
		string(audience.ProviderScope), audience.Operation, string(direction),
	}
}

func validateArtifactGrantRedisCapability(ctx context.Context, grantID domainappdev.ArtifactGrantID, hash domainappdev.ArtifactGrantTokenHash, audience domainappdev.ArtifactGrantAudience, direction domainappdev.ArtifactGrantDirection) error {
	if contextErr := artifactGrantRedisContextError(ctx); contextErr != nil {
		return contextErr
	}
	if grantID.IsZero() || hash.IsZero() ||
		domainappdev.ValidateArtifactGrantAudience(audience) != nil || domainappdev.ValidateArtifactGrantDirection(direction) != nil {
		return domainappdev.ErrArtifactGrantInvalid
	}
	return nil
}

func artifactGrantRedisContextError(ctx context.Context) error {
	if ctx == nil {
		return domainappdev.ErrArtifactGrantInvalid
	}
	if ctx.Err() != nil {
		return domainappdev.ErrArtifactGrantUnavailable
	}
	return nil
}

func parseArtifactGrantRedisReply(result any) ([]string, error) {
	items, ok := result.([]interface{})
	if !ok || len(items) < 2 || len(items) > maxArtifactGrantRedisReplyFields {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	reply := make([]string, len(items))
	total := 0
	for index, item := range items {
		value, ok := item.(string)
		if !ok || !utf8.ValidString(value) {
			return nil, domainappdev.ErrArtifactGrantUnavailable
		}
		total += len(value)
		if total > maxArtifactGrantRedisReplyBytes {
			return nil, domainappdev.ErrArtifactGrantUnavailable
		}
		reply[index] = value
	}
	if reply[0] != artifactGrantRedisProtocolVersion {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return reply, nil
}

func artifactGrantRedisCodeError(reply []string) error {
	if len(reply) != 2 {
		return domainappdev.ErrArtifactGrantUnavailable
	}
	switch reply[1] {
	case "conflict":
		return domainappdev.ErrArtifactGrantConflict
	case "audience":
		return domainappdev.ErrArtifactGrantAudienceMismatch
	case "direction":
		return domainappdev.ErrArtifactGrantDirectionMismatch
	case "revoked":
		return domainappdev.ErrArtifactGrantRevoked
	case "consumed":
		return domainappdev.ErrArtifactGrantConsumed
	case "not_found", "expired", "token":
		return domainappdev.ErrArtifactGrantDenied
	default:
		return domainappdev.ErrArtifactGrantUnavailable
	}
}

func parseArtifactGrantRedisInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && strconv.FormatInt(parsed, 10) == value
}

func validArtifactGrantRedisRecord(record *domainappdev.ArtifactGrantRecord, grantID domainappdev.ArtifactGrantID, audience domainappdev.ArtifactGrantAudience, direction domainappdev.ArtifactGrantDirection, state domainappdev.ArtifactGrantState) bool {
	if record == nil || !record.GrantID.Equal(grantID) || record.Spec.Audience != audience || record.Spec.Direction != direction ||
		record.State != state || record.IssuedAt.IsZero() || !record.ExpiresAt.After(record.IssuedAt) ||
		record.ExpiresAt.Sub(record.IssuedAt) > domainappdev.MaxArtifactGrantTTL {
		return false
	}
	normalized, err := domainappdev.NormalizeArtifactGrantSpec(record.Spec)
	return err == nil && normalized == record.Spec
}

func validArtifactGrantRedisNamespace(value string) bool {
	if value == "" || len(value) > 160 || strings.ContainsAny(value, "{} \\/") {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == ':' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
