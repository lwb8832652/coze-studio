/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
)

const (
	journalLeaseTokenBytes = 16
	journalLimiterMinTTL   = 15 * time.Second
	journalLimiterMaxTTL   = time.Hour
)

const journalTokenBucketScript = `
local now_parts = redis.call('TIME')
local now_ms = (tonumber(now_parts[1]) * 1000) + math.floor(tonumber(now_parts[2]) / 1000)
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local ttl_ms = tonumber(ARGV[3])
if not rate or rate < 1 or rate > 100000 or not burst or burst < rate or burst > 200000 or
   not ttl_ms or ttl_ms < 15000 or ttl_ms > 3600000 then
  return {-1, 0}
end
local tokens = tonumber(redis.call('HGET', KEYS[1], 'tokens'))
local last_ms = tonumber(redis.call('HGET', KEYS[1], 'last_ms'))
if not tokens or not last_ms then
  tokens = burst
  last_ms = now_ms
else
  local elapsed = math.max(0, now_ms - last_ms)
  tokens = math.min(burst, tokens + (elapsed * rate / 1000))
end
if tokens < 1 then
  local retry_ms = math.ceil((1 - tokens) * 1000 / rate)
  redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'last_ms', tostring(now_ms))
  redis.call('PEXPIRE', KEYS[1], ttl_ms)
  return {0, retry_ms}
end
tokens = tokens - 1
redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'last_ms', tostring(now_ms))
redis.call('PEXPIRE', KEYS[1], ttl_ms)
return {1, 0}
`

const journalAcquireStreamLeaseScript = `
local now_parts = redis.call('TIME')
local now_ms = (tonumber(now_parts[1]) * 1000) + math.floor(tonumber(now_parts[2]) / 1000)
local tenant_cap = tonumber(ARGV[1])
local cluster_cap = tonumber(ARGV[2])
local ttl_ms = tonumber(ARGV[3])
local token = ARGV[4]
if not tenant_cap or tenant_cap < 1 or tenant_cap > 10000 or
   not cluster_cap or cluster_cap < tenant_cap or cluster_cap > 1000000 or
   not ttl_ms or ttl_ms < 15000 or ttl_ms > 3600000 or
   not token or string.len(token) ~= 22 then
  return {-1, 0}
end
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now_ms)
local tenant_existing = redis.call('ZSCORE', KEYS[1], token)
local cluster_existing = redis.call('ZSCORE', KEYS[2], token)
if tenant_existing and cluster_existing then
  return {1, tonumber(tenant_existing)}
end
if redis.call('ZCARD', KEYS[1]) >= tenant_cap or redis.call('ZCARD', KEYS[2]) >= cluster_cap then
  local earliest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
  local retry_ms = 1000
  if #earliest > 0 then retry_ms = math.max(1, tonumber(earliest[2]) - now_ms) end
  return {0, retry_ms}
end
local expiry_ms = now_ms + ttl_ms
if redis.call('ZADD', KEYS[1], 'NX', expiry_ms, token) ~= 1 then return {-1, 0} end
if redis.call('ZADD', KEYS[2], 'NX', expiry_ms, token) ~= 1 then
  redis.call('ZREM', KEYS[1], token)
  return {-1, 0}
end
redis.call('PEXPIRE', KEYS[1], ttl_ms)
redis.call('PEXPIRE', KEYS[2], ttl_ms)
return {1, expiry_ms}
`

const journalRenewStreamLeaseScript = `
local now_parts = redis.call('TIME')
local now_ms = (tonumber(now_parts[1]) * 1000) + math.floor(tonumber(now_parts[2]) / 1000)
local ttl_ms = tonumber(ARGV[1])
local token = ARGV[2]
if not ttl_ms or ttl_ms < 15000 or ttl_ms > 3600000 or not token or string.len(token) ~= 22 then
  return -1
end
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now_ms)
if not redis.call('ZSCORE', KEYS[1], token) or not redis.call('ZSCORE', KEYS[2], token) then
  return 0
end
local expiry_ms = now_ms + ttl_ms
redis.call('ZADD', KEYS[1], 'XX', expiry_ms, token)
redis.call('ZADD', KEYS[2], 'XX', expiry_ms, token)
redis.call('PEXPIRE', KEYS[1], ttl_ms)
redis.call('PEXPIRE', KEYS[2], ttl_ms)
return 1
`

const journalReleaseStreamLeaseScript = `
local token = ARGV[1]
if not token or string.len(token) ~= 22 then return -1 end
local tenant_removed = redis.call('ZREM', KEYS[1], token)
local cluster_removed = redis.call('ZREM', KEYS[2], token)
if tenant_removed == 0 and cluster_removed == 0 then return 0 end
return 1
`

type JournalLimiterConfigurationProvider interface {
	GetBaseConfig(context.Context) (*adminconfig.BasicConfiguration, error)
}

type RedisJournalRateLimiterOptions struct {
	Environment    string
	TokenGenerator func() (string, error)
}

type RedisJournalRateLimiter struct {
	client         cache.ScriptCmdable
	provider       JournalLimiterConfigurationProvider
	environment    string
	tokenGenerator func() (string, error)
}

func NewRedisJournalRateLimiter(
	client cache.ScriptCmdable,
	provider JournalLimiterConfigurationProvider,
	options RedisJournalRateLimiterOptions,
) (*RedisJournalRateLimiter, error) {
	environment := canonicalJournalLimiterEnvironment(options.Environment)
	if client == nil || provider == nil || environment == "" {
		return nil, errors.New("journal rate limiter dependencies are invalid")
	}
	tokenGenerator := options.TokenGenerator
	if tokenGenerator == nil {
		tokenGenerator = newJournalLeaseToken
	}
	return &RedisJournalRateLimiter{
		client: client, provider: provider, environment: environment,
		tokenGenerator: tokenGenerator,
	}, nil
}

func (l *RedisJournalRateLimiter) Acquire(
	ctx context.Context,
	req appagentthread.JournalAdmissionRequest,
) (appagentthread.JournalAdmissionLease, error) {
	if l == nil || l.client == nil || l.provider == nil || ctx == nil ||
		req.SpaceID <= 0 || req.ViewerID <= 0 || !validJournalAdmissionKind(req.Kind) {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	configuration, err := l.runtimeConfiguration(ctx)
	if err != nil {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	if req.Kind != appagentthread.JournalAdmissionKindStream {
		return l.acquireShortRequest(ctx, req, configuration)
	}
	return l.acquireStream(ctx, req, configuration)
}

func (l *RedisJournalRateLimiter) acquireShortRequest(
	ctx context.Context,
	req appagentthread.JournalAdmissionRequest,
	configuration *adminconfig.JournalRuntimeConfiguration,
) (appagentthread.JournalAdmissionLease, error) {
	key := fmt.Sprintf(
		"journal:{%s}:space:%d:short:%s",
		l.environment,
		req.SpaceID,
		req.Kind,
	)
	result, err := l.client.RunScript(
		ctx,
		journalTokenBucketScript,
		[]string{key},
		configuration.ShortRequestQPS,
		configuration.ShortRequestBurst,
		time.Duration(configuration.LeaseTTLSeconds)*time.Second/time.Millisecond,
	).Result()
	if err != nil {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	status, detail, err := parseJournalScriptPair(result)
	if err != nil || status < 0 {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	if status == 0 {
		return nil, &appagentthread.JournalRateLimitError{RetryAfter: time.Duration(detail) * time.Millisecond}
	}
	return journalNoopAdmissionLease{}, nil
}

func (l *RedisJournalRateLimiter) acquireStream(
	ctx context.Context,
	req appagentthread.JournalAdmissionRequest,
	configuration *adminconfig.JournalRuntimeConfiguration,
) (appagentthread.JournalAdmissionLease, error) {
	token, err := l.tokenGenerator()
	if err != nil || !validJournalLeaseToken(token) {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	keys := l.streamKeys(req.SpaceID)
	ttl := time.Duration(configuration.LeaseTTLSeconds) * time.Second
	result, err := l.client.RunScript(
		ctx,
		journalAcquireStreamLeaseScript,
		keys,
		configuration.SseTenantConnectionCap,
		configuration.SseClusterConnectionCap,
		ttl/time.Millisecond,
		token,
	).Result()
	if err != nil {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	status, detail, err := parseJournalScriptPair(result)
	if err != nil || status < 0 {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	if status == 0 {
		return nil, &appagentthread.JournalRateLimitError{RetryAfter: time.Duration(detail) * time.Millisecond}
	}
	return &redisJournalAdmissionLease{
		client: l.client, keys: keys, token: token, ttl: ttl,
	}, nil
}

func (l *RedisJournalRateLimiter) runtimeConfiguration(
	ctx context.Context,
) (*adminconfig.JournalRuntimeConfiguration, error) {
	base, err := l.provider.GetBaseConfig(ctx)
	if err != nil || base == nil || base.JournalRuntimeConfiguration == nil {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	configuration := base.JournalRuntimeConfiguration
	ttl := time.Duration(configuration.LeaseTTLSeconds) * time.Second
	if configuration.SseTenantConnectionCap < 1 || configuration.SseTenantConnectionCap > 10000 ||
		configuration.SseClusterConnectionCap < configuration.SseTenantConnectionCap ||
		configuration.SseClusterConnectionCap > 1000000 ||
		configuration.ShortRequestQPS < 1 || configuration.ShortRequestQPS > 100000 ||
		configuration.ShortRequestBurst < configuration.ShortRequestQPS ||
		configuration.ShortRequestBurst > 200000 || ttl < journalLimiterMinTTL ||
		ttl > journalLimiterMaxTTL {
		return nil, appagentthread.ErrJournalAdmissionUnavailable
	}
	cloned := *configuration
	return &cloned, nil
}

func (l *RedisJournalRateLimiter) streamKeys(spaceID int64) []string {
	hashTag := l.environment
	return []string{
		fmt.Sprintf("journal:{%s}:space:%d:sse", hashTag, spaceID),
		fmt.Sprintf("journal:{%s}:cluster:sse", hashTag),
	}
}

type journalNoopAdmissionLease struct{}

func (journalNoopAdmissionLease) Renew(context.Context) error   { return nil }
func (journalNoopAdmissionLease) Release(context.Context) error { return nil }

type redisJournalAdmissionLease struct {
	client cache.ScriptCmdable
	keys   []string
	token  string
	ttl    time.Duration
}

func (l *redisJournalAdmissionLease) Renew(ctx context.Context) error {
	if l == nil || l.client == nil || ctx == nil || len(l.keys) != 2 ||
		!validJournalLeaseToken(l.token) || l.ttl < journalLimiterMinTTL || l.ttl > journalLimiterMaxTTL {
		return appagentthread.ErrJournalAdmissionUnavailable
	}
	result, err := l.client.RunScript(
		ctx,
		journalRenewStreamLeaseScript,
		l.keys,
		l.ttl/time.Millisecond,
		l.token,
	).Result()
	if err != nil {
		return appagentthread.ErrJournalAdmissionUnavailable
	}
	status, err := parseJournalScriptInt(result)
	if err != nil || status != 1 {
		return appagentthread.ErrJournalAdmissionUnavailable
	}
	return nil
}

func (l *redisJournalAdmissionLease) Release(ctx context.Context) error {
	if l == nil || l.client == nil || ctx == nil || len(l.keys) != 2 || !validJournalLeaseToken(l.token) {
		return appagentthread.ErrJournalAdmissionUnavailable
	}
	result, err := l.client.RunScript(
		ctx,
		journalReleaseStreamLeaseScript,
		l.keys,
		l.token,
	).Result()
	if err != nil {
		return appagentthread.ErrJournalAdmissionUnavailable
	}
	status, err := parseJournalScriptInt(result)
	if err != nil || status < 0 {
		return appagentthread.ErrJournalAdmissionUnavailable
	}
	return nil
}

func validJournalAdmissionKind(kind appagentthread.JournalAdmissionKind) bool {
	switch kind {
	case appagentthread.JournalAdmissionKindBootstrap,
		appagentthread.JournalAdmissionKindSnapshot,
		appagentthread.JournalAdmissionKindAction,
		appagentthread.JournalAdmissionKindStream:
		return true
	default:
		return false
	}
}

func canonicalJournalLimiterEnvironment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '-' || current == '_' {
			continue
		}
		return ""
	}
	return strings.ToLower(value)
}

func newJournalLeaseToken() (string, error) {
	raw := make([]byte, journalLeaseTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validJournalLeaseToken(value string) bool {
	if len(value) != 22 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == journalLeaseTokenBytes
}

func parseJournalScriptPair(value interface{}) (int64, int64, error) {
	items, ok := value.([]interface{})
	if !ok || len(items) != 2 {
		return 0, 0, errors.New("journal limiter script returned an invalid pair")
	}
	first, err := parseJournalScriptInt(items[0])
	if err != nil {
		return 0, 0, err
	}
	second, err := parseJournalScriptInt(items[1])
	if err != nil {
		return 0, 0, err
	}
	return first, second, nil
}

func parseJournalScriptInt(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("journal limiter script returned %T", value)
	}
}
