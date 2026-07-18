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

package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
)

func TestRedisActivationCompensationAppliesAndIsIdempotent(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handle, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil || limiter.Activate(ctx, testCapacityProviderA, handle.Current) != nil {
		t.Fatalf("activate generation A: handle=%#v err=%v", handle, err)
	}
	result, err := limiter.CompensateActivation(ctx, testCapacityProviderA, handle.Current)
	if err != nil || result != ActivationCompensationApplied {
		t.Fatalf("first compensation = %v, %v", result, err)
	}
	result, err = limiter.CompensateActivation(ctx, testCapacityProviderA, handle.Current)
	if err != nil || result != ActivationCompensationAlreadyApplied {
		t.Fatalf("repeated compensation = %v, %v", result, err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(31), canonicalCapacityToken(131), 1, time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("applied compensation did not restore drain: %v", err)
	}
}

func TestRedisActivationCompensationIsStaleAfterNewerActivation(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handleA, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.Activate(ctx, testCapacityProviderA, handleA.Current); err != nil {
		t.Fatalf("activate A: %v", err)
	}
	handleB, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.Activate(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("activate B: %v", err)
	}
	result, err := limiter.CompensateActivation(ctx, testCapacityProviderA, handleA.Current)
	if err != nil || result != ActivationCompensationStale {
		t.Fatalf("stale A compensation = %v, %v", result, err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(32), canonicalCapacityToken(132), 1, time.Second); err != nil {
		t.Fatalf("stale A drained active B: %v", err)
	}
}

func TestRedisActivationCompensationIsStaleAfterNewerBeginDrain(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handleA, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.Activate(ctx, testCapacityProviderA, handleA.Current); err != nil {
		t.Fatalf("activate A: %v", err)
	}
	handleB, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil {
		t.Fatalf("begin B: %v", err)
	}
	result, err := limiter.CompensateActivation(ctx, testCapacityProviderA, handleA.Current)
	if err != nil || result != ActivationCompensationStale {
		t.Fatalf("A compensation after BeginDrain(B) = %v, %v", result, err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(33), canonicalCapacityToken(133), 1, time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("A compensation cleared B gate %#v: %v", handleB, err)
	}
}

func TestRedisActivationCompensationFailsClosedForLegacyStateAndRedisError(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	leaseToken := canonicalCapacityToken(34)
	leaseFence := canonicalCapacityToken(134)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, leaseToken, leaseFence, 1, time.Second); err != nil {
		t.Fatalf("seed legacy state: %v", err)
	}
	result, err := limiter.CompensateActivation(ctx, testCapacityProviderA, canonicalCapacityToken(234))
	if err != nil || result != ActivationCompensationStale {
		t.Fatalf("legacy state compensation = %v, %v", result, err)
	}
	active, err := limiter.HasActiveLeases(ctx, testCapacityProviderA)
	if err != nil || !active {
		t.Fatalf("legacy state was mutated: active=%v err=%v", active, err)
	}

	failing, err := NewRedisCapacityLimiter(scriptClientFunc(func(context.Context, string, []string, ...interface{}) cache.ScriptCmd {
		return &staticScriptCmd{err: errors.New("redis unavailable")}
	}))
	if err != nil {
		t.Fatalf("create failing limiter: %v", err)
	}
	if _, err := failing.CompensateActivation(ctx, testCapacityProviderA, canonicalCapacityToken(235)); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Redis error did not fail closed: %v", err)
	}
}

func TestRedisActiveLifecycleGenerationHasBoundedTTL(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handle, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil || limiter.Activate(ctx, testCapacityProviderA, handle.Current) != nil {
		t.Fatalf("activate lifecycle generation: %#v %v", handle, err)
	}
	ttl := server.TTL(capacityRedisKey(testCapacityProviderA))
	if ttl <= 0 || ttl > MaxCapacityLifecycleGenerationRetention {
		t.Fatalf("active generation TTL = %v", ttl)
	}
	server.FastForward(MaxCapacityLifecycleGenerationRetention + time.Millisecond)
	if server.Exists(capacityRedisKey(testCapacityProviderA)) {
		t.Fatal("idle lifecycle generation outlived bounded TTL")
	}
}

func TestRedisLifecycleEpochCannotRollBackAfterNewerDrainIsRetained(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handleA, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil || limiter.Activate(ctx, testCapacityProviderA, handleA.Current) != nil {
		t.Fatalf("activate epoch A: handle=%#v err=%v", handleA, err)
	}
	handleB, err := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err != nil {
		t.Fatalf("begin epoch B: %v", err)
	}
	result, err := limiter.CompensateActivation(ctx, testCapacityProviderA, handleA.Current)
	if err != nil || result != ActivationCompensationStale {
		t.Fatalf("A compensation after B = %v, %v", result, err)
	}
	if err := limiter.RetainDrain(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("retain failed epoch B: %v", err)
	}
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleA.Current); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("stale A restored after B: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(35), canonicalCapacityToken(135), 1, time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("retained B gate exposed active A while database is disabled: %v", err)
	}
}

func TestRedisRestoreActivePublishesCurrentEpochAndStaleTokensAreNoOps(t *testing.T) {
	limiter, _, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handleA, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.Activate(ctx, testCapacityProviderA, handleA.Current); err != nil {
		t.Fatalf("activate A: %v", err)
	}
	handleB, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("restore B: %v", err)
	}
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("repeat restore B: %v", err)
	}
	result, err := limiter.CompensateActivation(ctx, testCapacityProviderA, handleB.Current)
	if err != nil || result != ActivationCompensationApplied {
		t.Fatalf("restored generation is not B: %v, %v", result, err)
	}
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleB.Current); err != nil {
		t.Fatalf("restore B after compensation: %v", err)
	}
	handleC, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.RestoreActive(ctx, testCapacityProviderA, handleB.Current); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("expired B restore overwrote C: %v", err)
	}
	if err := limiter.RetainDrain(ctx, testCapacityProviderA, handleB.Current); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("expired B retain overwrote C: %v", err)
	}
	if err := limiter.RetainDrain(ctx, testCapacityProviderA, handleC.Current); err != nil {
		t.Fatalf("retain C: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, canonicalCapacityToken(36), canonicalCapacityToken(136), 1, time.Second); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("stale B operation cleared C: %v", err)
	}
}

func TestRedisLifecycleTTLUsesLongestGenerationLeaseAndTombstoneHorizon(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	handle, _ := limiter.BeginDrain(ctx, testCapacityProviderA)
	if err := limiter.Activate(ctx, testCapacityProviderA, handle.Current); err != nil {
		t.Fatalf("activate generation: %v", err)
	}
	shortToken := canonicalCapacityToken(37)
	shortFence := canonicalCapacityToken(137)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, shortToken, shortFence, 2, time.Second); err != nil {
		t.Fatalf("acquire one-second lease: %v", err)
	}
	assertLifecycleTTLNearHour(t, server.TTL(capacityRedisKey(testCapacityProviderA)))
	longToken := canonicalCapacityToken(38)
	longFence := canonicalCapacityToken(138)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, longToken, longFence, 2, 10*time.Second); err != nil {
		t.Fatalf("acquire second lease: %v", err)
	}
	assertLifecycleTTLNearHour(t, server.TTL(capacityRedisKey(testCapacityProviderA)))
	if err := limiter.Release(ctx, testCapacityProviderA, shortToken, shortFence); err != nil {
		t.Fatalf("release short lease: %v", err)
	}
	if _, err := limiter.HasActiveLeases(ctx, testCapacityProviderA); err != nil {
		t.Fatalf("refresh active state: %v", err)
	}
	assertLifecycleTTLNearHour(t, server.TTL(capacityRedisKey(testCapacityProviderA)))
	if err := limiter.Release(ctx, testCapacityProviderA, longToken, longFence); err != nil {
		t.Fatalf("release long lease: %v", err)
	}
	assertLifecycleTTLNearHour(t, server.TTL(capacityRedisKey(testCapacityProviderA)))
}

func TestRedisLifecycleTTLWithoutGenerationTracksLongestRequiredHorizon(t *testing.T) {
	limiter, server, _ := newRedisCapacityFixture(t)
	ctx := context.Background()
	shortToken := canonicalCapacityToken(39)
	shortFence := canonicalCapacityToken(139)
	longToken := canonicalCapacityToken(40)
	longFence := canonicalCapacityToken(140)
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, shortToken, shortFence, 2, time.Second); err != nil {
		t.Fatalf("acquire short legacy lease: %v", err)
	}
	if _, err := limiter.Acquire(ctx, testCapacityProviderA, longToken, longFence, 2, 10*time.Second); err != nil {
		t.Fatalf("acquire long legacy lease: %v", err)
	}
	if ttl := server.TTL(capacityRedisKey(testCapacityProviderA)); ttl < 9*time.Second || ttl > 10*time.Second {
		t.Fatalf("legacy longest-lease TTL = %v", ttl)
	}
	if err := limiter.Release(ctx, testCapacityProviderA, shortToken, shortFence); err != nil {
		t.Fatalf("release legacy short lease: %v", err)
	}
	assertLifecycleTTLNearHour(t, server.TTL(capacityRedisKey(testCapacityProviderA)))
}

func assertLifecycleTTLNearHour(t *testing.T, ttl time.Duration) {
	t.Helper()
	if ttl < 59*time.Minute || ttl > MaxCapacityLifecycleGenerationRetention+time.Second {
		t.Fatalf("lifecycle TTL = %v", ttl)
	}
}
