// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
)

const (
	testCapacityProviderA = "provider-capacity-a"
	testCapacityProviderB = "provider-capacity-b"
)

type staticScriptCmd struct {
	result interface{}
	err    error
}

func (c *staticScriptCmd) Err() error {
	return c.err
}

func (c *staticScriptCmd) Result() (interface{}, error) {
	return c.result, c.err
}

type scriptClientFunc func(context.Context, string, []string, ...interface{}) cache.ScriptCmd

func (f scriptClientFunc) RunScript(
	ctx context.Context,
	script string,
	keys []string,
	args ...interface{},
) cache.ScriptCmd {
	return f(ctx, script, keys, args...)
}

func canonicalCapacityToken(fill byte) string {
	raw := make([]byte, capacityLeaseTokenBytes)
	for index := range raw {
		raw[index] = fill
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func indexedCapacityToken(index uint64) string {
	raw := make([]byte, capacityLeaseTokenBytes)
	binary.BigEndian.PutUint64(raw[len(raw)-8:], index+1)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func newRedisCapacityFixture(
	t *testing.T,
) (*RedisCapacityLimiter, *miniredis.Miniredis, cache.Cmdable) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	client := redisimpl.NewWithAddrAndPassword(server.Addr(), "")
	limiter, err := NewRedisCapacityLimiter(client)
	if err != nil {
		t.Fatalf("create limiter: %v", err)
	}
	return limiter, server, client
}

func TestRedisCapacityLimiterAcquireUnderAndAtCapacity(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	token := canonicalCapacityToken(1)
	fence := canonicalCapacityToken(101)

	expiry, err := limiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, 2*time.Second)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if expiry <= 0 {
		t.Fatalf("first acquire returned invalid expiry %d", expiry)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(rawToken) < 16 {
		t.Fatalf("lease token is not at least 128-bit URL-safe entropy: %q, %v", token, err)
	}
	if token == testCapacityProviderA {
		t.Fatal("lease token encoded provider identity")
	}

	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(2), canonicalCapacityToken(102), 1, 2*time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("expected capacity exhausted, got %v", err)
	}
}

func TestRedisCapacityLimiterAcquireRetriesCommittedTokenIdempotently(t *testing.T) {
	_, server, client := newRedisCapacityFixture(t)
	ctx := context.Background()
	serverNow := time.Unix(2_000_000_050, 0).UTC()
	server.SetTime(serverNow)
	token := canonicalCapacityToken(3)
	fence := canonicalCapacityToken(103)
	calls := 0
	var committedExpiry int64
	lossyClient := scriptClientFunc(func(
		ctx context.Context,
		script string,
		keys []string,
		args ...interface{},
	) cache.ScriptCmd {
		result, err := client.RunScript(ctx, script, keys, args...).Result()
		calls++
		if calls == 1 && err == nil {
			items := result.([]interface{})
			committedExpiry = items[1].(int64)
			return &staticScriptCmd{err: errors.New("response lost after commit")}
		}
		return &staticScriptCmd{result: result, err: err}
	})
	limiter, err := NewRedisCapacityLimiter(lossyClient)
	if err != nil {
		t.Fatalf("create lossy limiter: %v", err)
	}

	if _, err := limiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, 2*time.Second); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("lost first response must fail closed, got %v", err)
	}
	retryExpiry, err := limiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, 2*time.Second)
	if err != nil {
		t.Fatalf("retry committed token: %v", err)
	}
	if retryExpiry != committedExpiry {
		t.Fatalf("retry extended or replaced lease: got expiry %d want %d", retryExpiry, committedExpiry)
	}
	card, err := client.RunScript(
		ctx,
		`return redis.call('ZCARD', KEYS[1])`,
		[]string{capacityRedisKey(testCapacityProviderA)},
	).Result()
	if err != nil || card != int64(1) {
		t.Fatalf("ambiguous retry duplicated lease: card=%#v err=%v", card, err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(4), canonicalCapacityToken(104), 1, 2*time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("different token must be rejected at capacity, got %v", err)
	}
}

func TestRedisCapacityLimiterUsesRedisTimeAndCleansExpiredLeases(t *testing.T) {
	limiter, server, client := newRedisCapacityFixture(t)
	ctx := context.Background()
	serverNow := time.Unix(2_000_000_000, 250_000_000).UTC()
	server.SetTime(serverNow)
	leaseDuration := 2 * time.Second

	token := canonicalCapacityToken(5)
	fence := canonicalCapacityToken(105)
	expiry, err := limiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, leaseDuration)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	scoreValue, err := client.RunScript(
		ctx,
		`return redis.call('ZSCORE', KEYS[1], ARGV[1])`,
		[]string{capacityRedisKey(testCapacityProviderA)},
		"a:"+token+":"+fence+":"+token,
	).Result()
	if err != nil {
		t.Fatalf("read lease score: %v", err)
	}
	score, err := strconv.ParseInt(scoreValue.(string), 10, 64)
	if err != nil {
		t.Fatalf("parse lease score %#v: %v", scoreValue, err)
	}
	wantExpiry := serverNow.UnixMilli() + leaseDuration.Milliseconds()
	if score != wantExpiry || expiry != wantExpiry {
		t.Fatalf("expiry must derive from Redis TIME: score=%d result=%d want=%d", score, expiry, wantExpiry)
	}

	server.SetTime(serverNow.Add(leaseDuration))
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(6), canonicalCapacityToken(106), 1, leaseDuration); err != nil {
		t.Fatalf("expired lease was not cleaned before acquire: %v", err)
	}
}

func TestRedisCapacityLimiterRenewRequiresLiveOwnedToken(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	serverNow := time.Unix(2_000_000_100, 0).UTC()
	server.SetTime(serverNow)

	token := canonicalCapacityToken(7)
	fence := canonicalCapacityToken(107)
	_, err := limiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, 2*time.Second)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	server.SetTime(serverNow.Add(time.Second))
	if _, err := limiter.Renew(ctx, testCapacityProviderA, token, fence, canonicalCapacityToken(207), 2*time.Second); err != nil {
		t.Fatalf("renew live token: %v", err)
	}
	if _, err := limiter.Renew(ctx, testCapacityProviderA, token, fence, canonicalCapacityToken(208), 2*time.Second); err != nil {
		t.Fatalf("repeated renew must be safe: %v", err)
	}
	if _, err := limiter.Renew(ctx, testCapacityProviderA, canonicalCapacityToken(8), canonicalCapacityToken(108), canonicalCapacityToken(209), 2*time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("unknown token must fail closed, got %v", err)
	}

	server.SetTime(serverNow.Add(3 * time.Second))
	if _, err := limiter.Renew(ctx, testCapacityProviderA, token, fence, canonicalCapacityToken(210), 2*time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("expired token must fail closed, got %v", err)
	}
}

func TestRedisCapacityLimiterReleaseIsOwnedAndIdempotent(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()

	first := canonicalCapacityToken(9)
	firstFence := canonicalCapacityToken(109)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, first, firstFence, 2, 5*time.Second); err != nil {
		t.Fatalf("acquire first token: %v", err)
	}
	second := canonicalCapacityToken(10)
	secondFence := canonicalCapacityToken(110)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, second, secondFence, 2, 5*time.Second); err != nil {
		t.Fatalf("acquire second token: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, canonicalCapacityToken(11), canonicalCapacityToken(111)); err != nil {
		t.Fatalf("release unknown token must be idempotent: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(12), canonicalCapacityToken(112), 2, 5*time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("unknown token released another lease: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, first, firstFence); err != nil {
		t.Fatalf("release owned token: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, first, firstFence); err != nil {
		t.Fatalf("release twice: %v", err)
	}
	if _, err := limiter.Renew(ctx, testCapacityProviderA, second, secondFence, canonicalCapacityToken(211), 5*time.Second); err != nil {
		t.Fatalf("release removed another token: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(13), canonicalCapacityToken(113), 2, 5*time.Second); err != nil {
		t.Fatalf("released slot was not reusable: %v", err)
	}
}

func TestRedisCapacityLimiterConcurrentAcquireDoesNotOversell(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	const (
		capacity = 7
		attempts = 64
	)
	start := make(chan struct{})
	tokens := make(chan string, attempts)
	errorsSeen := make(chan error, attempts)
	var wait sync.WaitGroup

	for index := 0; index < attempts; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			token := canonicalCapacityToken(byte(index + 1))
			fence := canonicalCapacityToken(byte(index + 101))
			_, err := limiter.Acquire(context.Background(), testCapacityProviderA, token, fence, capacity, 10*time.Second)
			if err != nil {
				errorsSeen <- err
				return
			}
			tokens <- token
		}(index)
	}
	close(start)
	wait.Wait()
	close(tokens)
	close(errorsSeen)

	if got := len(tokens); got != capacity {
		t.Fatalf("capacity oversold or undersold: got %d leases want %d", got, capacity)
	}
	for err := range errorsSeen {
		if !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
			t.Fatalf("unexpected concurrent acquire error: %v", err)
		}
	}
}

func TestRedisCapacityLimiterIsolatesProviders(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()

	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(14), canonicalCapacityToken(114), 1, 5*time.Second); err != nil {
		t.Fatalf("acquire provider A: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(15), canonicalCapacityToken(115), 1, 5*time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("provider A must be full, got %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderB, canonicalCapacityToken(14), canonicalCapacityToken(114), 1, 5*time.Second); err != nil {
		t.Fatalf("provider B must have an isolated capacity key: %v", err)
	}
	if !server.Exists(capacityRedisKey(testCapacityProviderA)) || !server.Exists(capacityRedisKey(testCapacityProviderB)) {
		t.Fatal("expected namespaced key per provider")
	}
}

func TestRedisCapacityLimiterFailsClosedOnRedisAndResultFailures(t *testing.T) {
	tests := []struct {
		name   string
		client cache.ScriptCmdable
		ctx    context.Context
		want   error
	}{
		{
			name: "redis outage",
			client: scriptClientFunc(func(context.Context, string, []string, ...interface{}) cache.ScriptCmd {
				return &staticScriptCmd{err: errors.New("redis unavailable")}
			}),
			ctx:  context.Background(),
			want: domainsandbox.ErrUnavailable,
		},
		{
			name: "malformed result",
			client: scriptClientFunc(func(context.Context, string, []string, ...interface{}) cache.ScriptCmd {
				return &staticScriptCmd{result: []interface{}{int64(1)}}
			}),
			ctx:  context.Background(),
			want: domainsandbox.ErrUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limiter, err := NewRedisCapacityLimiter(test.client)
			if err != nil {
				t.Fatalf("create limiter: %v", err)
			}
			_, err = limiter.Acquire(test.ctx, testCapacityProviderA, canonicalCapacityToken(16), canonicalCapacityToken(116), 1, time.Second)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v want %v", err, test.want)
			}
		})
	}
}

func TestRedisCapacityLimiterPropagatesCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	client := scriptClientFunc(func(ctx context.Context, _ string, _ []string, _ ...interface{}) cache.ScriptCmd {
		called = true
		return &staticScriptCmd{err: ctx.Err()}
	})
	limiter, err := NewRedisCapacityLimiter(client)
	if err != nil {
		t.Fatalf("create limiter: %v", err)
	}
	if _, err := limiter.Acquire(canceled, testCapacityProviderA, canonicalCapacityToken(17), canonicalCapacityToken(117), 1, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if called {
		t.Fatal("canceled acquire reached Redis")
	}
}

func TestRedisCapacityLimiterRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewRedisCapacityLimiter(nil); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("nil Redis client: %v", err)
	}
	client := scriptClientFunc(func(context.Context, string, []string, ...interface{}) cache.ScriptCmd {
		return &staticScriptCmd{result: []interface{}{int64(1), int64(1)}}
	})
	limiter, err := NewRedisCapacityLimiter(client)
	if err != nil {
		t.Fatalf("create limiter: %v", err)
	}
	invalidCases := []struct {
		provider string
		token    string
		fence    string
		capacity int
		duration time.Duration
	}{
		{"", canonicalCapacityToken(18), canonicalCapacityToken(118), 1, time.Second},
		{testCapacityProviderA, "not-canonical", canonicalCapacityToken(118), 1, time.Second},
		{testCapacityProviderA, canonicalCapacityToken(18), "not-canonical", 1, time.Second},
		{testCapacityProviderA, canonicalCapacityToken(18), canonicalCapacityToken(118), 0, time.Second},
		{testCapacityProviderA, canonicalCapacityToken(18), canonicalCapacityToken(118), domainsandbox.MaxProviderConcurrency + 1, time.Second},
		{testCapacityProviderA, canonicalCapacityToken(18), canonicalCapacityToken(118), 1, 0},
		{testCapacityProviderA, canonicalCapacityToken(18), canonicalCapacityToken(118), 1, MaxCapacityLeaseDuration + time.Millisecond},
	}
	for _, test := range invalidCases {
		if _, err := limiter.Acquire(context.Background(), test.provider, test.token, test.fence, test.capacity, test.duration); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("invalid acquire %#v returned %v", test, err)
		}
	}
}

func TestRedisCapacityLimiterDrainFenceClosesAcquireTOCTOU(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	activeToken := canonicalCapacityToken(19)
	activeFence := canonicalCapacityToken(119)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, activeToken, activeFence, 1, 5*time.Second); err != nil {
		t.Fatalf("acquire active lease: %v", err)
	}

	handleA, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil || handleA.ActiveCount != 1 || !validCapacityLeaseToken(handleA.Current) || handleA.Previous != "" {
		t.Fatalf("BeginDrain(A) = %#v, %v", handleA, err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(20), canonicalCapacityToken(120), 1, 5*time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("acquire crossed installed drain gate: %v", err)
	}
	handleB, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil || handleB.ActiveCount != 1 || !validCapacityLeaseToken(handleB.Current) ||
		handleB.Current == handleA.Current || handleB.Previous != handleA.Current {
		t.Fatalf("BeginDrain(B) = %#v, %v", handleB, err)
	}
	if err := limiter.Activate(ctx, testCapacityProviderA, handleA.Current); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Activate(A) error = %v", err)
	}
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleA.Current); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("RestoreActive(A) error = %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(20), canonicalCapacityToken(120), 1, 5*time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("stale A operation cleared B: %v", err)
	}
	if err := limiter.RetainDrain(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("RetainDrain(B): %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(20), canonicalCapacityToken(120), 1, 5*time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("RetainDrain(B) exposed the previous epoch: %v", err)
	}
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("RestoreActive(B): %v", err)
	}
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("RestoreActive(B retry): %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(20), canonicalCapacityToken(120), 1, 5*time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("cleared gate affected active lease accounting: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, activeToken, activeFence); err != nil {
		t.Fatalf("release active lease: %v", err)
	}

	handleC, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil || handleC.ActiveCount != 0 || !validCapacityLeaseToken(handleC.Current) || handleC.Previous != "" {
		t.Fatalf("BeginDrain(C) = %#v, %v", handleC, err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(20), canonicalCapacityToken(120), 1, 5*time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("retained drain did not block acquire: %v", err)
	}
	if err := limiter.Activate(ctx, testCapacityProviderA, handleC.Current); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(20), canonicalCapacityToken(120), 1, 5*time.Second); err != nil {
		t.Fatalf("activate did not clear drain gate: %v", err)
	}
}

func TestRedisCapacityLimiterTombstoneWorksetIsHardBounded(t *testing.T) {
	limiter, server, client := newRedisCapacityFixture(t)
	ctx := context.Background()
	server.SetTime(time.Unix(2_000_000_250, 0).UTC())
	const extraTombstones = 512
	for index := uint64(0); index < MaxCapacityTombstones+extraTombstones; index++ {
		token := indexedCapacityToken(index)
		fence := indexedCapacityToken(index + MaxCapacityTombstones + extraTombstones)
		if _, err := limiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, time.Second); err != nil {
			t.Fatalf("acquire %d: %v", index, err)
		}
		if err := limiter.Release(ctx, testCapacityProviderA, token, fence); err != nil {
			t.Fatalf("release %d: %v", index, err)
		}
		server.FastForward(time.Millisecond)
	}
	card, err := client.RunScript(
		ctx,
		`return redis.call('ZCARD', KEYS[1])`,
		[]string{capacityRedisKey(testCapacityProviderA)},
	).Result()
	if err != nil || card.(int64) > MaxCapacityTombstones {
		t.Fatalf("bounded tombstone ZCARD = %#v, %v", card, err)
	}
	active, err := limiter.HasActiveLeases(ctx, testCapacityProviderA)
	if err != nil || active {
		t.Fatalf("HasActiveLeases(tombstones only) = %t, %v", active, err)
	}

	oldToken := indexedCapacityToken(0)
	oldFence := indexedCapacityToken(MaxCapacityTombstones + extraTombstones)
	newFence := indexedCapacityToken(2 * (MaxCapacityTombstones + extraTombstones))
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, oldToken, newFence, 1, time.Second); err != nil {
		t.Fatalf("evicted token could not start a new generation: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, oldToken, oldFence); err != nil {
		t.Fatalf("stale generation release: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, indexedCapacityToken(3*(MaxCapacityTombstones+extraTombstones)), indexedCapacityToken(4*(MaxCapacityTombstones+extraTombstones)), 1, time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("stale generation affected new lease: %v", err)
	}
}

func TestRedisCapacityLimiterExpiredIdentityCannotReplayOrFenceNewLease(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	serverNow := time.Unix(2_000_000_300, 0).UTC()
	server.SetTime(serverNow)
	oldToken := canonicalCapacityToken(21)
	oldFence := canonicalCapacityToken(121)

	if _, err := limiter.Acquire(ctx, testCapacityProviderA, oldToken, oldFence, 1, time.Second); err != nil {
		t.Fatalf("acquire old lease: %v", err)
	}
	server.SetTime(serverNow.Add(time.Second))
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, oldToken, oldFence, 1, time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("expired identity replay regained execution: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, oldToken, canonicalCapacityToken(122), 1, time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("expired token with new fence bypassed replay protection: %v", err)
	}

	newToken := canonicalCapacityToken(22)
	newFence := canonicalCapacityToken(123)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, newToken, newFence, 1, time.Second); err != nil {
		t.Fatalf("acquire later lease: %v", err)
	}
	if _, err := limiter.Renew(ctx, testCapacityProviderA, oldToken, oldFence, canonicalCapacityToken(221), time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("old holder renewed later lease: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, oldToken, oldFence); err != nil {
		t.Fatalf("old release must remain idempotent: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, newToken, canonicalCapacityToken(124)); err != nil {
		t.Fatalf("wrong fence release must remain idempotent: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(23), canonicalCapacityToken(125), 1, time.Second); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("stale or wrong fence released later lease: %v", err)
	}
	if _, err := limiter.Renew(ctx, testCapacityProviderA, newToken, newFence, canonicalCapacityToken(222), time.Second); err != nil {
		t.Fatalf("exact later generation could not renew: %v", err)
	}
}

func TestRedisCapacityLimiterRenewRetryDoesNotDoubleExtend(t *testing.T) {
	realLimiter, server, client := newRedisCapacityFixture(t)
	ctx := context.Background()
	serverNow := time.Unix(2_000_000_400, 0).UTC()
	server.SetTime(serverNow)
	token := canonicalCapacityToken(24)
	fence := canonicalCapacityToken(124)
	if _, err := realLimiter.Acquire(ctx, testCapacityProviderA, token, fence, 1, 2*time.Second); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	calls := 0
	var committedExpiry int64
	lossyClient := scriptClientFunc(func(
		ctx context.Context,
		script string,
		keys []string,
		args ...interface{},
	) cache.ScriptCmd {
		result, err := client.RunScript(ctx, script, keys, args...).Result()
		calls++
		if calls == 1 && err == nil {
			items := result.([]interface{})
			committedExpiry = items[1].(int64)
			return &staticScriptCmd{err: errors.New("renew response lost after commit")}
		}
		return &staticScriptCmd{result: result, err: err}
	})
	lossyLimiter, err := NewRedisCapacityLimiter(lossyClient)
	if err != nil {
		t.Fatalf("create lossy limiter: %v", err)
	}
	renewalID := canonicalCapacityToken(224)
	if _, err := lossyLimiter.Renew(ctx, testCapacityProviderA, token, fence, renewalID, 2*time.Second); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("lost renew response must fail closed: %v", err)
	}
	retryExpiry, err := lossyLimiter.Renew(ctx, testCapacityProviderA, token, fence, renewalID, 2*time.Second)
	if err != nil {
		t.Fatalf("retry renewal operation: %v", err)
	}
	if retryExpiry != committedExpiry {
		t.Fatalf("renew retry double-extended lease: got %d want %d", retryExpiry, committedExpiry)
	}
	server.SetTime(serverNow.Add(500 * time.Millisecond))
	nextExpiry, err := lossyLimiter.Renew(ctx, testCapacityProviderA, token, fence, canonicalCapacityToken(225), 2*time.Second)
	if err != nil || nextExpiry <= retryExpiry {
		t.Fatalf("new renewal operation did not advance expiry: next=%d prior=%d err=%v", nextExpiry, retryExpiry, err)
	}
}

func TestRedisCapacityLimiterTombstonesDoNotConsumeCapacityAndExpire(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	server.SetTime(time.Unix(2_000_000_500, 0).UTC())
	firstToken := canonicalCapacityToken(25)
	firstFence := canonicalCapacityToken(125)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, firstToken, firstFence, 1, time.Second); err != nil {
		t.Fatalf("acquire first: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, firstToken, firstFence); err != nil {
		t.Fatalf("release first: %v", err)
	}
	secondToken := canonicalCapacityToken(26)
	secondFence := canonicalCapacityToken(126)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, secondToken, secondFence, 1, time.Second); err != nil {
		t.Fatalf("tombstone consumed capacity: %v", err)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, secondToken, secondFence); err != nil {
		t.Fatalf("release second: %v", err)
	}
	server.FastForward(MaxCapacityReplayRetention + time.Millisecond)
	if server.Exists(capacityRedisKey(testCapacityProviderA)) {
		t.Fatal("bounded replay tombstones outlived key TTL")
	}
}

func TestRedisCapacityLimiterRenewNilReceiverFailsClosed(t *testing.T) {
	ctx := context.Background()
	token := canonicalCapacityToken(27)
	fence := canonicalCapacityToken(127)
	renewalID := canonicalCapacityToken(227)
	var nilLimiter *RedisCapacityLimiter
	if _, err := nilLimiter.Renew(ctx, testCapacityProviderA, token, fence, renewalID, time.Second); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("nil receiver renew returned %v", err)
	}
	zeroLimiter := &RedisCapacityLimiter{}
	if _, err := zeroLimiter.Renew(ctx, testCapacityProviderA, token, fence, renewalID, time.Second); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("zero receiver renew returned %v", err)
	}
}
