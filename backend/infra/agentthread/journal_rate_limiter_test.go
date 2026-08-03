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
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
)

type journalLimiterConfigurationProvider struct {
	configuration *adminconfig.BasicConfiguration
}

func (p *journalLimiterConfigurationProvider) GetBaseConfig(context.Context) (*adminconfig.BasicConfiguration, error) {
	return p.configuration, nil
}

type journalLimiterScriptClientFunc func(
	context.Context,
	string,
	[]string,
	...interface{},
) cache.ScriptCmd

func (f journalLimiterScriptClientFunc) RunScript(
	ctx context.Context,
	script string,
	keys []string,
	args ...interface{},
) cache.ScriptCmd {
	return f(ctx, script, keys, args...)
}

type journalLimiterStaticScriptCmd struct {
	result interface{}
	err    error
}

func (c journalLimiterStaticScriptCmd) Err() error { return c.err }

func (c journalLimiterStaticScriptCmd) Result() (interface{}, error) {
	return c.result, c.err
}

func journalLimiterConfiguration() *adminconfig.BasicConfiguration {
	return &adminconfig.BasicConfiguration{
		JournalRuntimeConfiguration: &adminconfig.JournalRuntimeConfiguration{
			SseTenantConnectionCap:         1,
			SseClusterConnectionCap:        2,
			SseSendQueueHighWatermark:      8,
			SseSendQueueMax:                16,
			ShortRequestQPS:                1,
			ShortRequestBurst:              2,
			LeaseTTLSeconds:                30,
			SnapshotFragmentThresholdBytes: 4 << 20,
		},
	}
}

func newJournalRateLimiterFixture(
	t *testing.T,
) (*RedisJournalRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)
	client := redisimpl.NewWithAddrAndPassword(server.Addr(), "")
	limiter, err := NewRedisJournalRateLimiter(
		client,
		&journalLimiterConfigurationProvider{configuration: journalLimiterConfiguration()},
		RedisJournalRateLimiterOptions{Environment: "test"},
	)
	require.NoError(t, err)
	return limiter, server
}

func TestJournalRateLimiterTokenBucketIsAtomicAndTenantScoped(t *testing.T) {
	limiter, server := newJournalRateLimiterFixture(t)
	ctx := context.Background()
	req := appagentthread.JournalAdmissionRequest{
		SpaceID: 11, ViewerID: 21, ThreadID: 31, RunID: 41,
		Kind: appagentthread.JournalAdmissionKindBootstrap,
	}

	for index := 0; index < 2; index++ {
		lease, err := limiter.Acquire(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, lease)
		require.NoError(t, lease.Release(ctx))
	}
	lease, err := limiter.Acquire(ctx, req)
	require.Nil(t, lease)
	var limited *appagentthread.JournalRateLimitError
	require.ErrorAs(t, err, &limited)
	require.Positive(t, limited.RetryAfter)

	keys := server.Keys()
	sort.Strings(keys)
	require.Condition(t, func() bool {
		for _, key := range keys {
			if strings.Contains(key, "journal:{test}:space:11:short:bootstrap") {
				return true
			}
		}
		return false
	}, "Redis key must include environment, tenant, and request class: %v", keys)
}

func TestJournalRateLimiterStreamLeaseEnforcesTenantAndClusterCaps(t *testing.T) {
	limiter, server := newJournalRateLimiterFixture(t)
	ctx := context.Background()
	stream := func(spaceID int64) appagentthread.JournalAdmissionRequest {
		return appagentthread.JournalAdmissionRequest{
			SpaceID: spaceID, ViewerID: spaceID + 100, ThreadID: spaceID + 200,
			RunID: spaceID + 300, Kind: appagentthread.JournalAdmissionKindStream,
		}
	}

	first, err := limiter.Acquire(ctx, stream(1))
	require.NoError(t, err)
	secondSameTenant, err := limiter.Acquire(ctx, stream(1))
	require.Nil(t, secondSameTenant)
	require.ErrorIs(t, err, appagentthread.ErrJournalRateLimited)
	second, err := limiter.Acquire(ctx, stream(2))
	require.NoError(t, err)
	third, err := limiter.Acquire(ctx, stream(3))
	require.Nil(t, third)
	require.ErrorIs(t, err, appagentthread.ErrJournalRateLimited)
	keys := server.Keys()
	require.Condition(t, func() bool {
		for _, key := range keys {
			if strings.Contains(key, "journal:{test}:space:1:sse") {
				return true
			}
		}
		return false
	}, "tenant SSE key is missing: %v", keys)

	require.NoError(t, first.Renew(ctx))
	require.NoError(t, first.Release(ctx))
	replacement, err := limiter.Acquire(ctx, stream(1))
	require.NoError(t, err)
	require.NotNil(t, replacement)
	require.NoError(t, replacement.Release(ctx))
	require.NoError(t, second.Release(ctx))

}

func TestJournalRateLimiterConcurrentStreamAcquireDoesNotOversell(t *testing.T) {
	limiter, _ := newJournalRateLimiterFixture(t)
	ctx := context.Background()
	const workers = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	leases := make([]appagentthread.JournalAdmissionLease, 0, 1)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			lease, err := limiter.Acquire(ctx, appagentthread.JournalAdmissionRequest{
				SpaceID: 1, ViewerID: int64(worker + 1), ThreadID: int64(worker + 100),
				RunID: int64(worker + 200), Kind: appagentthread.JournalAdmissionKindStream,
			})
			if err != nil {
				require.ErrorIs(t, err, appagentthread.ErrJournalRateLimited)
				return
			}
			mu.Lock()
			accepted++
			leases = append(leases, lease)
			mu.Unlock()
		}(index)
	}
	wg.Wait()
	require.Equal(t, 1, accepted)
	for _, lease := range leases {
		require.NoError(t, lease.Release(ctx))
	}
}

func TestJournalRateLimiterFailsClosedOnRedisOrInvalidConfiguration(t *testing.T) {
	redisErr := errors.New("redis unavailable")
	client := journalLimiterScriptClientFunc(func(
		context.Context,
		string,
		[]string,
		...interface{},
	) cache.ScriptCmd {
		return journalLimiterStaticScriptCmd{err: redisErr}
	})
	provider := &journalLimiterConfigurationProvider{configuration: journalLimiterConfiguration()}
	limiter, err := NewRedisJournalRateLimiter(
		client,
		provider,
		RedisJournalRateLimiterOptions{Environment: "production"},
	)
	require.NoError(t, err)
	lease, err := limiter.Acquire(context.Background(), appagentthread.JournalAdmissionRequest{
		SpaceID: 1, ViewerID: 2, Kind: appagentthread.JournalAdmissionKindSnapshot,
	})
	require.Nil(t, lease)
	require.ErrorIs(t, err, appagentthread.ErrJournalAdmissionUnavailable)

	provider.configuration.JournalRuntimeConfiguration.LeaseTTLSeconds = 1
	lease, err = limiter.Acquire(context.Background(), appagentthread.JournalAdmissionRequest{
		SpaceID: 1, ViewerID: 2, Kind: appagentthread.JournalAdmissionKindStream,
	})
	require.Nil(t, lease)
	require.ErrorIs(t, err, appagentthread.ErrJournalAdmissionUnavailable)
}

func TestJournalRateLimiterLeaseExpiresWithoutRelease(t *testing.T) {
	limiter, server := newJournalRateLimiterFixture(t)
	ctx := context.Background()
	req := appagentthread.JournalAdmissionRequest{
		SpaceID: 5, ViewerID: 6, ThreadID: 7, RunID: 8,
		Kind: appagentthread.JournalAdmissionKindStream,
	}
	first, err := limiter.Acquire(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, first)

	server.FastForward(31 * time.Second)
	replacement, err := limiter.Acquire(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, replacement)
	require.NoError(t, replacement.Release(ctx))
}
