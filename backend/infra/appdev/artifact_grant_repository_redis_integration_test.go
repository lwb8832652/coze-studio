// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
)

func TestArtifactGrantRedisDisposableIntegration(t *testing.T) {
	address := os.Getenv("APPDEV_ARTIFACT_GRANT_REDIS_DISPOSABLE_ADDR")
	if address == "" {
		t.Skip("APPDEV_ARTIFACT_GRANT_REDIS_DISPOSABLE_ADDR is not configured")
	}
	randomPrefix := make([]byte, 12)
	if _, err := rand.Read(randomPrefix); err != nil {
		t.Fatal(err)
	}
	namespace := "appdev:artifact-grant:test:" + hex.EncodeToString(randomPrefix)
	client := redisimpl.NewWithAddrAndPassword(address, os.Getenv("APPDEV_ARTIFACT_GRANT_REDIS_DISPOSABLE_PASSWORD"))
	repository, err := NewRedisArtifactGrantRepository(client, WithArtifactGrantRedisNamespace(namespace))
	if err != nil {
		t.Fatal(err)
	}
	grantID, token, spec := artifactGrantRepositoryFixture(t, 0x7a)
	key := artifactGrantRedisKey(namespace, grantID)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = client.Del(cleanupCtx, key).Result()
	})
	issueArtifactGrantForRepositoryTest(t, repository, grantID, token, spec, 30*time.Second)
	input := domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Audience: spec.Audience, Direction: spec.Direction,
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(64)
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready.Done()
			<-start
			if _, err := repository.Consume(context.Background(), input); err == nil {
				successes.Add(1)
			}
		}()
	}
	ready.Wait()
	close(start)
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful consumers = %d", successes.Load())
	}
}
