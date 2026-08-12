// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

const redisAcceptExecutionScript = `
local function refresh_ttl(key, ttl_ms)
  -- New counters/lists do not have a TTL yet, so GT alone would leave
  -- them permanent. NX establishes the first expiry; GT then preserves
  -- the longest outstanding execution without shortening it for later work.
  local created = redis.call('PEXPIRE', key, ttl_ms, 'NX')
  if created == 0 then
    redis.call('PEXPIRE', key, ttl_ms, 'GT')
  end
end

local existing = redis.call('GET', KEYS[1])
if existing then
  return {'replay', existing}
end
local global = redis.call('GET', KEYS[3])
local space = redis.call('GET', KEYS[4])
local user = redis.call('GET', KEYS[5])
if (global and tonumber(global) >= tonumber(ARGV[1])) or
   (space and tonumber(space) >= tonumber(ARGV[2])) or
   (user and tonumber(user) >= tonumber(ARGV[3])) then
  return {'capacity'}
end
redis.call('SET', KEYS[2], ARGV[4], 'PX', ARGV[5])
redis.call('SET', KEYS[1], ARGV[6], 'PX', ARGV[5])
redis.call('INCR', KEYS[3])
redis.call('INCR', KEYS[4])
redis.call('INCR', KEYS[5])
redis.call('RPUSH', KEYS[6], ARGV[6])
refresh_ttl(KEYS[3], ARGV[5])
refresh_ttl(KEYS[4], ARGV[5])
refresh_ttl(KEYS[5], ARGV[5])
refresh_ttl(KEYS[6], ARGV[5])
return {'accepted', ARGV[6]}
`

const redisTransitionExecutionScript = `
local current = redis.call('GET', KEYS[1])
if not current then return {'missing'} end
if current ~= ARGV[1] then return {'stale'} end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
if ARGV[4] == 'terminal' then
  for index = 2, 4 do
    local depth = redis.call('GET', KEYS[index])
    if depth and tonumber(depth) > 0 then redis.call('DECR', KEYS[index]) end
  end
  redis.call('LREM', KEYS[5], 0, ARGV[5])
end
return {'updated'}
`
