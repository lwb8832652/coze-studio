// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
)

const (
	capacityRedisKeyPrefix     = "sandbox:capacity:"
	capacityLeaseTokenBytes    = 16
	MinCapacityLeaseDuration   = time.Second
	MaxCapacityLeaseDuration   = time.Hour
	MaxCapacityReplayRetention = time.Hour
	// MaxCapacityLifecycleGenerationRetention bounds idle active-generation
	// ownership. Drain gates remain persistent until an explicit lifecycle
	// transition, while an idle active generation expires fail-closed to stale.
	MaxCapacityLifecycleGenerationRetention = time.Hour
	MaxCapacityTombstones                   = 4096
)

const acquireCapacityScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end

local function refresh_key_expiry(key, now_ms)
  local latest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #latest == 0 then
    redis.call('DEL', key)
    return
  end
  local ttl_ms = math.ceil(tonumber(latest[2]) - now_ms)
  if ttl_ms <= 0 then
    redis.call('DEL', key)
    return
  end
  redis.call('PEXPIRE', key, ttl_ms)
end

local capacity = tonumber(ARGV[1])
local lease_ms = tonumber(ARGV[2])
local token = ARGV[3]
local now_ms = redis_now_ms()
if not capacity or capacity < 1 or capacity > 1024 or
   not lease_ms or lease_ms < 1000 or lease_ms > 3600000 or
   not token or string.len(token) ~= 22 then
  return {-1, now_ms}
end

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
local existing_expiry = redis.call('ZSCORE', KEYS[1], token)
if existing_expiry then
  refresh_key_expiry(KEYS[1], now_ms)
  return {1, tonumber(existing_expiry)}
end
if redis.call('ZCARD', KEYS[1]) >= capacity then
  refresh_key_expiry(KEYS[1], now_ms)
  return {0, now_ms}
end

local expiry_ms = now_ms + lease_ms
if redis.call('ZADD', KEYS[1], 'NX', expiry_ms, token) ~= 1 then
  refresh_key_expiry(KEYS[1], now_ms)
  return {-1, now_ms}
end
refresh_key_expiry(KEYS[1], now_ms)
return {1, expiry_ms}
`

const renewCapacityScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end

local function refresh_key_expiry(key, now_ms)
  local latest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #latest == 0 then
    redis.call('DEL', key)
    return
  end
  local ttl_ms = math.ceil(tonumber(latest[2]) - now_ms)
  if ttl_ms <= 0 then
    redis.call('DEL', key)
    return
  end
  redis.call('PEXPIRE', key, ttl_ms)
end

local lease_ms = tonumber(ARGV[1])
local token = ARGV[2]
local now_ms = redis_now_ms()
if not lease_ms or lease_ms < 1000 or lease_ms > 3600000 or
   not token or string.len(token) ~= 22 then
  return {-1, now_ms}
end

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
if not redis.call('ZSCORE', KEYS[1], token) then
  refresh_key_expiry(KEYS[1], now_ms)
  return {0, now_ms}
end

local expiry_ms = now_ms + lease_ms
redis.call('ZADD', KEYS[1], 'XX', expiry_ms, token)
refresh_key_expiry(KEYS[1], now_ms)
return {1, expiry_ms}
`

const releaseCapacityScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end

local function refresh_key_expiry(key, now_ms)
  local latest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #latest == 0 then
    redis.call('DEL', key)
    return
  end
  local ttl_ms = math.ceil(tonumber(latest[2]) - now_ms)
  if ttl_ms <= 0 then
    redis.call('DEL', key)
    return
  end
  redis.call('PEXPIRE', key, ttl_ms)
end

local token = ARGV[1]
local now_ms = redis_now_ms()
if not token or string.len(token) ~= 22 then
  return -1
end

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_ms)
local removed = redis.call('ZREM', KEYS[1], token)
refresh_key_expiry(KEYS[1], now_ms)
return removed
`

const acquireFencedCapacityScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end

local function refresh_key_expiry(key, now_ms)
  local expires_ms = 0
  local highest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #highest > 0 and tonumber(highest[2]) > 0 then expires_ms = tonumber(highest[2]) end
  local lowest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  if #lowest > 0 and tonumber(lowest[2]) < 0 and -tonumber(lowest[2]) > expires_ms then
    expires_ms = -tonumber(lowest[2])
  end
  if expires_ms <= now_ms then redis.call('DEL', key) else redis.call('PEXPIRE', key, math.ceil(expires_ms - now_ms)) end
end

local function cleanup(key, now_ms)
  redis.call('ZREMRANGEBYSCORE', key, -now_ms, -1)
  local expired = redis.call('ZRANGEBYSCORE', key, 1, now_ms)
  for _, member in ipairs(expired) do
    redis.call('ZREM', key, member)
    if string.sub(member, 1, 2) == 'a:' and string.len(member) == 70 then
      local expired_token = string.sub(member, 3, 24)
      redis.call('ZADD', key, -(now_ms + 3600000), 't:' .. expired_token)
    end
  end
end

local function find_active(key, token, fence)
  local prefix = 'a:' .. token .. ':' .. fence .. ':'
  local entries = redis.call('ZRANGEBYSCORE', key, 1, '+inf', 'WITHSCORES')
  for index = 1, #entries, 2 do
    if string.sub(entries[index], 1, string.len(prefix)) == prefix then
      return entries[index], tonumber(entries[index + 1])
    end
  end
  return nil, nil
end

local function token_is_active(key, token)
  local prefix = 'a:' .. token .. ':'
  local entries = redis.call('ZRANGEBYSCORE', key, 1, '+inf')
  for _, member in ipairs(entries) do
    if string.sub(member, 1, string.len(prefix)) == prefix then return true end
  end
  return false
end

local capacity = tonumber(ARGV[1])
local lease_ms = tonumber(ARGV[2])
local token = ARGV[3]
local fence = ARGV[4]
local now_ms = redis_now_ms()
if not capacity or capacity < 1 or capacity > 1024 or not lease_ms or lease_ms < 1000 or lease_ms > 3600000 or
   not token or string.len(token) ~= 22 or not fence or string.len(fence) ~= 22 then return {-1, now_ms} end
cleanup(KEYS[1], now_ms)
if redis.call('ZSCORE', KEYS[1], 't:' .. token) then refresh_key_expiry(KEYS[1], now_ms); return {-2, now_ms} end
local existing, existing_expiry = find_active(KEYS[1], token, fence)
if existing then refresh_key_expiry(KEYS[1], now_ms); return {1, existing_expiry} end
if token_is_active(KEYS[1], token) then refresh_key_expiry(KEYS[1], now_ms); return {-2, now_ms} end
if redis.call('ZCOUNT', KEYS[1], 1, '+inf') >= capacity then refresh_key_expiry(KEYS[1], now_ms); return {0, now_ms} end
local expiry_ms = now_ms + lease_ms
local member = 'a:' .. token .. ':' .. fence .. ':' .. token
if redis.call('ZADD', KEYS[1], 'NX', expiry_ms, member) ~= 1 then return {-1, now_ms} end
refresh_key_expiry(KEYS[1], now_ms)
return {1, expiry_ms}
`

const renewFencedCapacityScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end
local function refresh_key_expiry(key, now_ms)
  local expires_ms = 0
  local highest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #highest > 0 and tonumber(highest[2]) > 0 then expires_ms = tonumber(highest[2]) end
  local lowest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  if #lowest > 0 and tonumber(lowest[2]) < 0 and -tonumber(lowest[2]) > expires_ms then expires_ms = -tonumber(lowest[2]) end
  if expires_ms <= now_ms then redis.call('DEL', key) else redis.call('PEXPIRE', key, math.ceil(expires_ms - now_ms)) end
end
local function cleanup(key, now_ms)
  redis.call('ZREMRANGEBYSCORE', key, -now_ms, -1)
  local expired = redis.call('ZRANGEBYSCORE', key, 1, now_ms)
  for _, member in ipairs(expired) do
    redis.call('ZREM', key, member)
    if string.sub(member, 1, 2) == 'a:' and string.len(member) == 70 then
      redis.call('ZADD', key, -(now_ms + 3600000), 't:' .. string.sub(member, 3, 24))
    end
  end
end
local function find_active(key, token, fence)
  local prefix = 'a:' .. token .. ':' .. fence .. ':'
  local entries = redis.call('ZRANGEBYSCORE', key, 1, '+inf', 'WITHSCORES')
  for index = 1, #entries, 2 do
    if string.sub(entries[index], 1, string.len(prefix)) == prefix then return entries[index], tonumber(entries[index + 1]) end
  end
  return nil, nil
end
local lease_ms = tonumber(ARGV[1])
local token = ARGV[2]
local fence = ARGV[3]
local renewal_id = ARGV[4]
local now_ms = redis_now_ms()
if not lease_ms or lease_ms < 1000 or lease_ms > 3600000 or not token or string.len(token) ~= 22 or
   not fence or string.len(fence) ~= 22 or not renewal_id or string.len(renewal_id) ~= 22 then return {-1, now_ms} end
cleanup(KEYS[1], now_ms)
local member, current_expiry = find_active(KEYS[1], token, fence)
if not member then refresh_key_expiry(KEYS[1], now_ms); return {0, now_ms} end
if string.sub(member, 49, 70) == renewal_id then refresh_key_expiry(KEYS[1], now_ms); return {1, current_expiry} end
local expiry_ms = now_ms + lease_ms
local next_member = 'a:' .. token .. ':' .. fence .. ':' .. renewal_id
redis.call('ZREM', KEYS[1], member)
redis.call('ZADD', KEYS[1], expiry_ms, next_member)
refresh_key_expiry(KEYS[1], now_ms)
return {1, expiry_ms}
`

const releaseFencedCapacityScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end
local function refresh_key_expiry(key, now_ms)
  local expires_ms = 0
  local highest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #highest > 0 and tonumber(highest[2]) > 0 then expires_ms = tonumber(highest[2]) end
  local lowest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  if #lowest > 0 and tonumber(lowest[2]) < 0 and -tonumber(lowest[2]) > expires_ms then expires_ms = -tonumber(lowest[2]) end
  if expires_ms <= now_ms then redis.call('DEL', key) else redis.call('PEXPIRE', key, math.ceil(expires_ms - now_ms)) end
end
local function cleanup(key, now_ms)
  redis.call('ZREMRANGEBYSCORE', key, -now_ms, -1)
  local expired = redis.call('ZRANGEBYSCORE', key, 1, now_ms)
  for _, member in ipairs(expired) do
    redis.call('ZREM', key, member)
    if string.sub(member, 1, 2) == 'a:' and string.len(member) == 70 then redis.call('ZADD', key, -(now_ms + 3600000), 't:' .. string.sub(member, 3, 24)) end
  end
end
local function find_active(key, token, fence)
  local prefix = 'a:' .. token .. ':' .. fence .. ':'
  local entries = redis.call('ZRANGEBYSCORE', key, 1, '+inf')
  for _, member in ipairs(entries) do
    if string.sub(member, 1, string.len(prefix)) == prefix then return member end
  end
  return nil
end
local token = ARGV[1]
local fence = ARGV[2]
local now_ms = redis_now_ms()
if not token or string.len(token) ~= 22 or not fence or string.len(fence) ~= 22 then return -1 end
cleanup(KEYS[1], now_ms)
local member = find_active(KEYS[1], token, fence)
if not member then refresh_key_expiry(KEYS[1], now_ms); return 0 end
redis.call('ZREM', KEYS[1], member)
redis.call('ZADD', KEYS[1], -(now_ms + 3600000), 't:' .. token)
refresh_key_expiry(KEYS[1], now_ms)
return 1
`

const hasActiveCapacityLeasesScript = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end
local function valid_token(value)
  return string.len(value) == 22 and string.match(value, '^[-A-Za-z0-9_]+$') ~= nil
end
local function valid_active(member)
  return string.len(member) == 70 and string.sub(member, 1, 2) == 'a:' and
    string.sub(member, 25, 25) == ':' and string.sub(member, 48, 48) == ':' and
    valid_token(string.sub(member, 3, 24)) and valid_token(string.sub(member, 26, 47)) and
    valid_token(string.sub(member, 49, 70))
end
local function valid_tombstone(member)
  return string.len(member) == 24 and string.sub(member, 1, 2) == 't:' and
    valid_token(string.sub(member, 3, 24))
end
local function refresh_key_expiry(key, now_ms)
  local expires_ms = 0
  local highest = redis.call('ZREVRANGE', key, 0, 0, 'WITHSCORES')
  if #highest > 0 and tonumber(highest[2]) > 0 then expires_ms = tonumber(highest[2]) end
  local lowest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  if #lowest > 0 and tonumber(lowest[2]) < 0 and -tonumber(lowest[2]) > expires_ms then
    expires_ms = -tonumber(lowest[2])
  end
  if expires_ms <= now_ms then
    redis.call('DEL', key)
  else
    redis.call('PEXPIRE', key, math.ceil(expires_ms - now_ms))
  end
end

local now_ms = redis_now_ms()
local entries = redis.call('ZRANGE', KEYS[1], 0, -1, 'WITHSCORES')
for index = 1, #entries, 2 do
  local member = entries[index]
  local score = tonumber(entries[index + 1])
  if not score or score == 0 or
     (score > 0 and not valid_active(member)) or
     (score < 0 and not valid_tombstone(member)) then
    return -1
  end
end

redis.call('ZREMRANGEBYSCORE', KEYS[1], -now_ms, -1)
local expired = redis.call('ZRANGEBYSCORE', KEYS[1], 1, now_ms)
for _, member in ipairs(expired) do
  redis.call('ZREM', KEYS[1], member)
  redis.call('ZADD', KEYS[1], -(now_ms + 3600000), 't:' .. string.sub(member, 3, 24))
end
local active = redis.call('ZCOUNT', KEYS[1], 1, '+inf')
refresh_key_expiry(KEYS[1], now_ms)
if active > 0 then return 1 end
return 0
`

const capacityLifecycleScriptPrelude = `
local function redis_now_ms()
  local parts = redis.call('TIME')
  return (tonumber(parts[1]) * 1000) + math.floor(tonumber(parts[2]) / 1000)
end

local function valid_token(value)
  return string.len(value) == 22 and string.match(value, '^[-A-Za-z0-9_]+$') ~= nil
end

local function valid_active(member)
  return string.len(member) == 70 and string.sub(member, 1, 2) == 'a:' and
    string.sub(member, 25, 25) == ':' and string.sub(member, 48, 48) == ':' and
    valid_token(string.sub(member, 3, 24)) and valid_token(string.sub(member, 26, 47)) and
    valid_token(string.sub(member, 49, 70))
end

local function valid_tombstone(member)
  return string.len(member) == 24 and string.sub(member, 1, 2) == 't:' and
    valid_token(string.sub(member, 3, 24))
end

local function valid_gate(member)
  return string.len(member) == 24 and string.sub(member, 1, 2) == 'g:' and
    valid_token(string.sub(member, 3, 24))
end

local function valid_generation(member)
  return string.len(member) == 24 and string.sub(member, 1, 2) == 'l:' and
    valid_token(string.sub(member, 3, 24))
end

local function add_tombstone(key, token, now_ms)
  -- Redis TIME has millisecond precision and several releases may share one
  -- tick. A bounded fractional sequence preserves oldest-first eviction
  -- without changing the millisecond replay-retention contract.
  local sequence = redis.call('ZCOUNT', key, '-inf', -1)
  local score = -(now_ms + 3600000) - (math.min(sequence, 4096) / 1000)
  redis.call('ZADD', key, score, 't:' .. token)
end

local function trim_tombstones(key)
  local count = redis.call('ZCOUNT', key, '-inf', -1)
  if count <= 4096 then return true end
  local victims = redis.call('ZREVRANGEBYSCORE', key, -1, '-inf', 'LIMIT', 0, count - 4096)
  if #victims ~= count - 4096 then return false end
  for _, member in ipairs(victims) do
    if not valid_tombstone(member) then return false end
    redis.call('ZREM', key, member)
  end
  return redis.call('ZCOUNT', key, '-inf', -1) <= 4096
end

local function cleanup(key, now_ms)
  redis.call('ZREMRANGEBYSCORE', key, -now_ms, -1)
  local expired = redis.call('ZRANGEBYSCORE', key, 1, now_ms, 'LIMIT', 0, 1025)
  if #expired > 1024 then return false end
  for _, member in ipairs(expired) do
    if not valid_active(member) then return false end
    redis.call('ZREM', key, member)
    add_tombstone(key, string.sub(member, 3, 24), now_ms)
  end
  return trim_tombstones(key)
end

local function lifecycle_state(key)
  local entries = redis.call('ZRANGEBYSCORE', key, 0, 0, 'LIMIT', 0, 3)
  if #entries > 2 then return nil end
  local gate = ''
  local generation = ''
  for _, member in ipairs(entries) do
    if valid_gate(member) then
      if gate ~= '' then return nil end
      gate = member
    elseif valid_generation(member) then
      if generation ~= '' then return nil end
      generation = member
    else
      return nil
    end
  end
  return {gate, generation}
end

local function gates(key)
  local state = lifecycle_state(key)
  if not state then return nil end
  if state[1] == '' then return {} end
  return {state[1]}
end

local function active_entries(key)
  local count = redis.call('ZCOUNT', key, 1, '+inf')
  if count > 1024 then return nil end
  local entries = redis.call('ZRANGEBYSCORE', key, 1, '+inf', 'WITHSCORES', 'LIMIT', 0, 1025)
  if #entries ~= count * 2 then return nil end
  for index = 1, #entries, 2 do
    if not valid_active(entries[index]) then return nil end
  end
  return entries
end

local function refresh_key_expiry(key, now_ms)
  local state = lifecycle_state(key)
  if not state then return false end
  if state[1] ~= '' then
    redis.call('PERSIST', key)
    return true
  end
	local expires_ms = 0
	if state[2] ~= '' then expires_ms = now_ms + 3600000 end
	local latest_active = redis.call('ZREVRANGEBYSCORE', key, '+inf', 1, 'WITHSCORES', 'LIMIT', 0, 1)
	if #latest_active == 2 and tonumber(latest_active[2]) > expires_ms then
	  expires_ms = tonumber(latest_active[2])
	end
  local latest_tombstone = redis.call('ZRANGEBYSCORE', key, '-inf', -1, 'WITHSCORES', 'LIMIT', 0, 1)
  if #latest_tombstone == 2 and -tonumber(latest_tombstone[2]) > expires_ms then
    expires_ms = -tonumber(latest_tombstone[2])
  end
  if expires_ms <= now_ms then
    redis.call('DEL', key)
  else
    redis.call('PEXPIRE', key, math.ceil(expires_ms - now_ms))
  end
  return true
end
`

var acquireLifecycleCapacityScript = capacityLifecycleScriptPrelude + `
local capacity = tonumber(ARGV[1])
local lease_ms = tonumber(ARGV[2])
local token = ARGV[3]
local fence = ARGV[4]
local now_ms = redis_now_ms()
if not capacity or capacity < 1 or capacity > 1024 or not lease_ms or lease_ms < 1000 or lease_ms > 3600000 or
   not token or not valid_token(token) or not fence or not valid_token(fence) then return {-1, now_ms} end
if not cleanup(KEYS[1], now_ms) then return {-1, now_ms} end
local gate_entries = gates(KEYS[1])
if not gate_entries then return {-1, now_ms} end
if #gate_entries > 0 then refresh_key_expiry(KEYS[1], now_ms); return {-3, now_ms} end
if redis.call('ZSCORE', KEYS[1], 't:' .. token) then refresh_key_expiry(KEYS[1], now_ms); return {-2, now_ms} end
local entries = active_entries(KEYS[1])
if not entries then return {-1, now_ms} end
local prefix = 'a:' .. token .. ':'
local exact_prefix = prefix .. fence .. ':'
for index = 1, #entries, 2 do
  local member = entries[index]
  if string.sub(member, 1, string.len(exact_prefix)) == exact_prefix then
    refresh_key_expiry(KEYS[1], now_ms)
    return {1, tonumber(entries[index + 1])}
  end
  if string.sub(member, 1, string.len(prefix)) == prefix then
    refresh_key_expiry(KEYS[1], now_ms)
    return {-2, now_ms}
  end
end
if redis.call('ZCOUNT', KEYS[1], 1, '+inf') >= capacity then refresh_key_expiry(KEYS[1], now_ms); return {0, now_ms} end
local expiry_ms = now_ms + lease_ms
local member = 'a:' .. token .. ':' .. fence .. ':' .. token
if redis.call('ZADD', KEYS[1], 'NX', expiry_ms, member) ~= 1 then return {-1, now_ms} end
if not refresh_key_expiry(KEYS[1], now_ms) then return {-1, now_ms} end
return {1, expiry_ms}
`

var renewLifecycleCapacityScript = capacityLifecycleScriptPrelude + `
local lease_ms = tonumber(ARGV[1])
local token = ARGV[2]
local fence = ARGV[3]
local renewal_id = ARGV[4]
local now_ms = redis_now_ms()
if not lease_ms or lease_ms < 1000 or lease_ms > 3600000 or not token or not valid_token(token) or
   not fence or not valid_token(fence) or not renewal_id or not valid_token(renewal_id) then return {-1, now_ms} end
if not cleanup(KEYS[1], now_ms) then return {-1, now_ms} end
local entries = active_entries(KEYS[1])
if not entries then return {-1, now_ms} end
local prefix = 'a:' .. token .. ':' .. fence .. ':'
for index = 1, #entries, 2 do
  local member = entries[index]
  if string.sub(member, 1, string.len(prefix)) == prefix then
    local current_expiry = tonumber(entries[index + 1])
    if string.sub(member, 49, 70) == renewal_id then
      refresh_key_expiry(KEYS[1], now_ms)
      return {1, current_expiry}
    end
    local expiry_ms = now_ms + lease_ms
    redis.call('ZREM', KEYS[1], member)
    redis.call('ZADD', KEYS[1], expiry_ms, 'a:' .. token .. ':' .. fence .. ':' .. renewal_id)
    refresh_key_expiry(KEYS[1], now_ms)
    return {1, expiry_ms}
  end
end
refresh_key_expiry(KEYS[1], now_ms)
return {0, now_ms}
`

var releaseLifecycleCapacityScript = capacityLifecycleScriptPrelude + `
local token = ARGV[1]
local fence = ARGV[2]
local now_ms = redis_now_ms()
if not token or not valid_token(token) or not fence or not valid_token(fence) then return -1 end
if not cleanup(KEYS[1], now_ms) then return -1 end
local entries = active_entries(KEYS[1])
if not entries then return -1 end
local prefix = 'a:' .. token .. ':' .. fence .. ':'
for index = 1, #entries, 2 do
  local member = entries[index]
  if string.sub(member, 1, string.len(prefix)) == prefix then
    redis.call('ZREM', KEYS[1], member)
    add_tombstone(KEYS[1], token, now_ms)
    if not trim_tombstones(KEYS[1]) or not refresh_key_expiry(KEYS[1], now_ms) then return -1 end
    return 1
  end
end
refresh_key_expiry(KEYS[1], now_ms)
return 0
`

var hasActiveLifecycleCapacityScript = capacityLifecycleScriptPrelude + `
local now_ms = redis_now_ms()
if not cleanup(KEYS[1], now_ms) then return -1 end
local gate_entries = gates(KEYS[1])
if not gate_entries then return -1 end
local entries = active_entries(KEYS[1])
if not entries then return -1 end
if not refresh_key_expiry(KEYS[1], now_ms) then return -1 end
if redis.call('ZCOUNT', KEYS[1], 1, '+inf') > 0 then return 1 end
return 0
`

var beginDrainCapacityScript = capacityLifecycleScriptPrelude + `
local fence = ARGV[1]
local now_ms = redis_now_ms()
if not fence or not valid_token(fence) then return {-1, 0, ''} end
if not cleanup(KEYS[1], now_ms) then return {-1, 0, ''} end
local gate_entries = gates(KEYS[1])
if not gate_entries then return {-1, 0, ''} end
local entries = active_entries(KEYS[1])
if not entries then return {-1, 0, ''} end
local active_count = redis.call('ZCOUNT', KEYS[1], 1, '+inf')
local previous = ''
if #gate_entries > 0 then
  previous = string.sub(gate_entries[1], 3, 24)
  if previous == fence then return {-1, 0, ''} end
  redis.call('ZREM', KEYS[1], gate_entries[1])
end
if redis.call('ZADD', KEYS[1], 'NX', 0, 'g:' .. fence) ~= 1 then return {-1, 0, ''} end
redis.call('PERSIST', KEYS[1])
return {1, active_count, previous}
`

var restoreActiveCapacityScript = capacityLifecycleScriptPrelude + `
local current = ARGV[1]
local now_ms = redis_now_ms()
if not current or not valid_token(current) then return -1 end
if not cleanup(KEYS[1], now_ms) then return -1 end
local state = lifecycle_state(KEYS[1])
if not state then return -1 end
if state[1] == '' and state[2] == 'l:' .. current then
  if not refresh_key_expiry(KEYS[1], now_ms) then return -1 end
  return 2
end
if state[1] ~= 'g:' .. current then return 0 end
if state[2] ~= '' then redis.call('ZREM', KEYS[1], state[2]) end
redis.call('ZADD', KEYS[1], 0, 'l:' .. current)
redis.call('ZREM', KEYS[1], state[1])
if not refresh_key_expiry(KEYS[1], now_ms) then return -1 end
return 1
`

var retainDrainCapacityScript = capacityLifecycleScriptPrelude + `
local current = ARGV[1]
local now_ms = redis_now_ms()
if not current or not valid_token(current) then return -1 end
if not cleanup(KEYS[1], now_ms) then return -1 end
local state = lifecycle_state(KEYS[1])
if not state then return -1 end
if state[1] ~= 'g:' .. current then return 0 end
redis.call('PERSIST', KEYS[1])
return 1
`

var activateCapacityScript = capacityLifecycleScriptPrelude + `
local current = ARGV[1]
local now_ms = redis_now_ms()
if not current or not valid_token(current) then return -1 end
if not cleanup(KEYS[1], now_ms) then return -1 end
local state = lifecycle_state(KEYS[1])
if not state then return -1 end
if state[1] ~= 'g:' .. current then return 0 end
if state[2] ~= '' and state[2] ~= 'l:' .. current then redis.call('ZREM', KEYS[1], state[2]) end
if state[2] == '' or state[2] ~= 'l:' .. current then
  if redis.call('ZADD', KEYS[1], 'NX', 0, 'l:' .. current) ~= 1 then return -1 end
end
local removed = redis.call('ZREM', KEYS[1], state[1])
if not refresh_key_expiry(KEYS[1], now_ms) then return -1 end
return removed
`

var compensateActivationCapacityScript = capacityLifecycleScriptPrelude + `
local current = ARGV[1]
local now_ms = redis_now_ms()
if not current or not valid_token(current) then return -1 end
if not cleanup(KEYS[1], now_ms) then return -1 end
local state = lifecycle_state(KEYS[1])
if not state then return -1 end
if state[1] == 'g:' .. current and state[2] == '' then
  redis.call('PERSIST', KEYS[1])
  return 2
end
if state[1] ~= '' or state[2] ~= 'l:' .. current then return 0 end
if redis.call('ZADD', KEYS[1], 'NX', 0, 'g:' .. current) ~= 1 then return -1 end
redis.call('ZREM', KEYS[1], state[2])
redis.call('PERSIST', KEYS[1])
return 1
`

// DrainHandle is an internal fenced lifecycle capability. Current is the only
// transition authority. Previous is historical and must never be restored.
// Neither field may be projected to APIs, logs, audits, or public errors.
type DrainHandle struct {
	Current     string
	Previous    string
	ActiveCount int
}

type ActivationCompensationResult uint8

const (
	ActivationCompensationStale ActivationCompensationResult = iota
	ActivationCompensationApplied
	ActivationCompensationAlreadyApplied
)

type RedisCapacityLimiter struct {
	client cache.ScriptCmdable
}

func NewRedisCapacityLimiter(client cache.ScriptCmdable) (*RedisCapacityLimiter, error) {
	if client == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &RedisCapacityLimiter{client: client}, nil
}

func (l *RedisCapacityLimiter) Acquire(
	ctx context.Context,
	providerKey string,
	token string,
	fence string,
	capacity int,
	duration time.Duration,
) (int64, error) {
	if l == nil || l.client == nil {
		return 0, domainsandbox.ErrInvalidInput
	}
	if err := validateCapacityOperation(ctx, providerKey, capacity, duration); err != nil {
		return 0, err
	}
	if !validCapacityLeaseToken(token) || !validCapacityLeaseToken(fence) {
		return 0, domainsandbox.ErrInvalidInput
	}
	result, err := l.client.RunScript(
		ctx,
		acquireLifecycleCapacityScript,
		[]string{capacityRedisKey(providerKey)},
		capacity,
		duration.Milliseconds(),
		token,
		fence,
	).Result()
	if err != nil {
		return 0, mapCapacityInfrastructureError(ctx, err)
	}
	status, expiryUnixMilli, ok := parseCapacityLeaseResult(result)
	if !ok {
		return 0, domainsandbox.ErrUnavailable
	}
	switch status {
	case 1:
		return expiryUnixMilli, nil
	case 0:
		return 0, domainsandbox.ErrCapacityExhausted
	case -2, -3:
		return 0, domainsandbox.ErrExecutionForbidden
	default:
		return 0, domainsandbox.ErrUnavailable
	}
}

func (l *RedisCapacityLimiter) Renew(
	ctx context.Context,
	providerKey string,
	token string,
	fence string,
	renewalID string,
	duration time.Duration,
) (int64, error) {
	if l == nil || l.client == nil {
		return 0, domainsandbox.ErrInvalidInput
	}
	if err := validateLeaseOperation(ctx, providerKey, token, fence, duration); err != nil {
		return 0, err
	}
	if !validCapacityLeaseToken(renewalID) {
		return 0, domainsandbox.ErrInvalidInput
	}
	result, err := l.client.RunScript(
		ctx,
		renewLifecycleCapacityScript,
		[]string{capacityRedisKey(providerKey)},
		duration.Milliseconds(),
		token,
		fence,
		renewalID,
	).Result()
	if err != nil {
		return 0, mapCapacityInfrastructureError(ctx, err)
	}
	status, expiryUnixMilli, ok := parseCapacityLeaseResult(result)
	if !ok {
		return 0, domainsandbox.ErrUnavailable
	}
	switch status {
	case 1:
		return expiryUnixMilli, nil
	case 0:
		return 0, domainsandbox.ErrExecutionForbidden
	default:
		return 0, domainsandbox.ErrUnavailable
	}
}

func (l *RedisCapacityLimiter) Release(ctx context.Context, providerKey, token, fence string) error {
	if l == nil || l.client == nil || ctx == nil ||
		domainsandbox.ValidateProviderKey(providerKey) != nil ||
		!validCapacityLeaseToken(token) || !validCapacityLeaseToken(fence) {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := l.client.RunScript(
		ctx,
		releaseLifecycleCapacityScript,
		[]string{capacityRedisKey(providerKey)},
		token,
		fence,
	).Result()
	if err != nil {
		return mapCapacityInfrastructureError(ctx, err)
	}
	removed, ok := result.(int64)
	if !ok || removed < 0 || removed > 1 {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func (l *RedisCapacityLimiter) HasActiveLeases(ctx context.Context, providerKey string) (bool, error) {
	if l == nil || l.client == nil || ctx == nil || domainsandbox.ValidateProviderKey(providerKey) != nil {
		return false, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	result, err := l.client.RunScript(
		ctx,
		hasActiveLifecycleCapacityScript,
		[]string{capacityRedisKey(providerKey)},
	).Result()
	if err != nil {
		return false, mapCapacityInfrastructureError(ctx, err)
	}
	status, ok := result.(int64)
	if !ok {
		return false, domainsandbox.ErrUnavailable
	}
	switch status {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, domainsandbox.ErrUnavailable
	}
}

func (l *RedisCapacityLimiter) BeginDrain(
	ctx context.Context,
	providerKey string,
) (DrainHandle, error) {
	if l == nil || l.client == nil || ctx == nil || domainsandbox.ValidateProviderKey(providerKey) != nil {
		return DrainHandle{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return DrainHandle{}, err
	}
	fence, err := newCapacityDrainFence()
	if err != nil {
		return DrainHandle{}, err
	}
	result, err := l.client.RunScript(
		ctx,
		beginDrainCapacityScript,
		[]string{capacityRedisKey(providerKey)},
		fence,
	).Result()
	if err != nil {
		return DrainHandle{}, mapCapacityInfrastructureError(ctx, err)
	}
	items, ok := result.([]interface{})
	if !ok || len(items) != 3 {
		return DrainHandle{}, domainsandbox.ErrUnavailable
	}
	status, statusOK := items[0].(int64)
	activeCount, countOK := items[1].(int64)
	previous, previousOK := items[2].(string)
	if !statusOK || status != 1 || !countOK || !previousOK || activeCount < 0 ||
		activeCount > domainsandbox.MaxProviderConcurrency ||
		(previous != "" && !validCapacityLeaseToken(previous)) {
		return DrainHandle{}, domainsandbox.ErrUnavailable
	}
	return DrainHandle{Current: fence, Previous: previous, ActiveCount: int(activeCount)}, nil
}

func (l *RedisCapacityLimiter) RestoreActive(ctx context.Context, providerKey, current string) error {
	return l.runDrainTransition(ctx, providerKey, current, restoreActiveCapacityScript)
}

func (l *RedisCapacityLimiter) RetainDrain(ctx context.Context, providerKey, current string) error {
	return l.runDrainTransition(ctx, providerKey, current, retainDrainCapacityScript)
}

func (l *RedisCapacityLimiter) runDrainTransition(
	ctx context.Context,
	providerKey string,
	current string,
	script string,
) error {
	if l == nil || l.client == nil || ctx == nil || domainsandbox.ValidateProviderKey(providerKey) != nil ||
		!validCapacityLeaseToken(current) || script == "" {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := l.client.RunScript(
		ctx,
		script,
		[]string{capacityRedisKey(providerKey)},
		current,
	).Result()
	if err != nil {
		return mapCapacityInfrastructureError(ctx, err)
	}
	status, ok := result.(int64)
	if !ok {
		return domainsandbox.ErrUnavailable
	}
	switch status {
	case 1, 2:
		return nil
	case 0:
		return domainsandbox.ErrExecutionForbidden
	default:
		return domainsandbox.ErrUnavailable
	}
}

func (l *RedisCapacityLimiter) Activate(ctx context.Context, providerKey, current string) error {
	if l == nil || l.client == nil || ctx == nil || domainsandbox.ValidateProviderKey(providerKey) != nil {
		return domainsandbox.ErrInvalidInput
	}
	if !validCapacityLeaseToken(current) {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := l.client.RunScript(
		ctx,
		activateCapacityScript,
		[]string{capacityRedisKey(providerKey)},
		current,
	).Result()
	if err != nil {
		return mapCapacityInfrastructureError(ctx, err)
	}
	removed, ok := result.(int64)
	if !ok || removed < 0 || removed > 1 {
		return domainsandbox.ErrUnavailable
	}
	if removed == 0 {
		return domainsandbox.ErrExecutionForbidden
	}
	return nil
}

func (l *RedisCapacityLimiter) CompensateActivation(
	ctx context.Context,
	providerKey string,
	current string,
) (ActivationCompensationResult, error) {
	if l == nil || l.client == nil || ctx == nil || domainsandbox.ValidateProviderKey(providerKey) != nil ||
		!validCapacityLeaseToken(current) {
		return ActivationCompensationStale, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return ActivationCompensationStale, err
	}
	result, err := l.client.RunScript(
		ctx,
		compensateActivationCapacityScript,
		[]string{capacityRedisKey(providerKey)},
		current,
	).Result()
	if err != nil {
		return ActivationCompensationStale, mapCapacityInfrastructureError(ctx, err)
	}
	status, ok := result.(int64)
	if !ok {
		return ActivationCompensationStale, domainsandbox.ErrUnavailable
	}
	switch status {
	case 0:
		return ActivationCompensationStale, nil
	case 1:
		return ActivationCompensationApplied, nil
	case 2:
		return ActivationCompensationAlreadyApplied, nil
	default:
		return ActivationCompensationStale, domainsandbox.ErrUnavailable
	}
}

func newCapacityDrainFence() (string, error) {
	raw := make([]byte, capacityLeaseTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", domainsandbox.ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validateCapacityOperation(
	ctx context.Context,
	providerKey string,
	capacity int,
	duration time.Duration,
) error {
	if ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if domainsandbox.ValidateProviderKey(providerKey) != nil ||
		capacity < domainsandbox.MinProviderConcurrency ||
		capacity > domainsandbox.MaxProviderConcurrency ||
		!validRedisCapacityLeaseDuration(duration) {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func validateLeaseOperation(
	ctx context.Context,
	providerKey string,
	token string,
	fence string,
	duration time.Duration,
) error {
	if err := validateCapacityOperation(ctx, providerKey, domainsandbox.MinProviderConcurrency, duration); err != nil {
		return err
	}
	if !validCapacityLeaseToken(token) || !validCapacityLeaseToken(fence) {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func validRedisCapacityLeaseDuration(duration time.Duration) bool {
	return duration >= MinCapacityLeaseDuration &&
		duration <= MaxCapacityLeaseDuration &&
		duration%time.Millisecond == 0
}

func validCapacityLeaseToken(token string) bool {
	if len(token) != base64.RawURLEncoding.EncodedLen(capacityLeaseTokenBytes) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == capacityLeaseTokenBytes
}

func capacityRedisKey(providerKey string) string {
	return capacityRedisKeyPrefix + providerKey
}

func parseCapacityLeaseResult(result interface{}) (status int64, expiry int64, ok bool) {
	items, ok := result.([]interface{})
	if !ok || len(items) != 2 {
		return 0, 0, false
	}
	status, statusOK := items[0].(int64)
	expiry, expiryOK := items[1].(int64)
	if !statusOK || !expiryOK || expiry <= 0 {
		return 0, 0, false
	}
	return status, expiry, true
}

func mapCapacityInfrastructureError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return domainsandbox.ErrUnavailable
}
