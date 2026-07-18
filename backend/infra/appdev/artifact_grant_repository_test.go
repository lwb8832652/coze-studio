// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
)

func newArtifactGrantRedisRepositoryForTest(t *testing.T) (*RedisArtifactGrantRepository, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	repository, err := NewRedisArtifactGrantRepository(redisimpl.NewWithAddrAndPassword(server.Addr(), ""))
	if err != nil {
		t.Fatal(err)
	}
	return repository, server
}

func artifactGrantRepositoryFixture(t *testing.T, seed byte) (domainappdev.ArtifactGrantID, domainappdev.ArtifactGrantToken, domainappdev.ArtifactGrantSpec) {
	t.Helper()
	grantID, err := domainappdev.NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{seed}, domainappdev.ArtifactGrantIDBytes)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := domainappdev.NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{seed + 1}, domainappdev.ArtifactGrantTokenBytes)))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact-payload")
	digest := sha256.Sum256(payload)
	spec := domainappdev.ArtifactGrantSpec{
		Audience: domainappdev.ArtifactGrantAudience{
			SpaceID: "1001", ProjectID: "project-a", ProviderKey: "provider-a",
			ProviderScope: domainsandbox.ScopeAppDev, Operation: "snapshot.upload",
		},
		Direction: domainappdev.ArtifactGrantDirectionUpload,
		ObjectKey: "appdev/1001/project-a/snapshot.zip", Digest: domainappdev.ArtifactGrantDigest(digest),
		Size: int64(len(payload)), MaxSize: 1024,
	}
	return grantID, token, spec
}

func issueArtifactGrantForRepositoryTest(t *testing.T, repository *RedisArtifactGrantRepository, grantID domainappdev.ArtifactGrantID, token domainappdev.ArtifactGrantToken, spec domainappdev.ArtifactGrantSpec, ttl time.Duration) *domainappdev.ArtifactGrantRecord {
	t.Helper()
	record, err := repository.Issue(context.Background(), domainappdev.IssueArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Spec: spec, TTL: ttl,
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestArtifactGrantRedisIssueUsesAtomicNXTTLAndStoresNoBearer(t *testing.T) {
	repository, server := newArtifactGrantRedisRepositoryForTest(t)
	grantID, token, spec := artifactGrantRepositoryFixture(t, 0x20)
	ttl := 1500 * time.Millisecond
	record := issueArtifactGrantForRepositoryTest(t, repository, grantID, token, spec, ttl)
	if record.State != domainappdev.ArtifactGrantStateIssued || !record.GrantID.Equal(grantID) || record.Spec != spec {
		t.Fatalf("issued record = %#v", record)
	}
	if _, err := repository.Issue(context.Background(), domainappdev.IssueArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Spec: spec, TTL: ttl,
	}); !errors.Is(err, domainappdev.ErrArtifactGrantConflict) {
		t.Fatalf("duplicate issue = %v", err)
	}
	keys := server.Keys()
	if len(keys) != 1 || keys[0] != artifactGrantRedisKey(defaultArtifactGrantRedisNamespace, grantID) {
		t.Fatalf("Redis keys = %#v", keys)
	}
	if strings.Contains(keys[0], token.Bearer()) || strings.Contains(keys[0], spec.ObjectKey) || strings.Contains(keys[0], spec.Audience.ProjectID) {
		t.Fatalf("Redis key leaked grant data: %q", keys[0])
	}
	fields, err := server.HKeys(keys[0])
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		value := server.HGet(keys[0], field)
		values[field] = value
		if strings.Contains(field, token.Bearer()) || strings.Contains(value, token.Bearer()) {
			t.Fatalf("Redis stored bearer token in %q", field)
		}
	}
	if values["token_hash"] != hex.EncodeToString(token.Hash().Bytes()) {
		t.Fatalf("stored token hash = %q", values["token_hash"])
	}
	if got := server.TTL(keys[0]); got <= 0 || got > ttl {
		t.Fatalf("Redis TTL = %v", got)
	}
}

func TestArtifactGrantRedisUsesDomainMinimumTTL(t *testing.T) {
	repository, _ := newArtifactGrantRedisRepositoryForTest(t)
	grantID, token, spec := artifactGrantRepositoryFixture(t, 0x2a)
	record, err := repository.Issue(context.Background(), domainappdev.IssueArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Spec: spec, TTL: time.Microsecond,
	})
	if err != nil || record == nil || record.ExpiresAt.Sub(record.IssuedAt) != time.Microsecond {
		t.Fatalf("minimum TTL issue = %#v, %v", record, err)
	}
	otherID, otherToken, otherSpec := artifactGrantRepositoryFixture(t, 0x2b)
	if record, err := repository.Issue(context.Background(), domainappdev.IssueArtifactGrantRepositoryInput{
		GrantID: otherID, TokenHash: otherToken.Hash(), Spec: otherSpec, TTL: time.Nanosecond,
	}); record != nil || !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) {
		t.Fatalf("sub-microsecond issue = %#v, %v", record, err)
	}
}

func TestArtifactGrantRedisConsumeChecksExactAudienceAndAllowsOnlyOneOf64(t *testing.T) {
	repository, _ := newArtifactGrantRedisRepositoryForTest(t)
	grantID, token, spec := artifactGrantRepositoryFixture(t, 0x30)
	issueArtifactGrantForRepositoryTest(t, repository, grantID, token, spec, time.Minute)

	_, otherToken, _ := artifactGrantRepositoryFixture(t, 0x31)
	if _, err := repository.Consume(context.Background(), domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: otherToken.Hash(), Audience: spec.Audience, Direction: spec.Direction,
	}); !errors.Is(err, domainappdev.ErrArtifactGrantDenied) {
		t.Fatalf("token mismatch = %v", err)
	}
	if _, err := repository.Consume(context.Background(), domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Audience: spec.Audience, Direction: domainappdev.ArtifactGrantDirectionDownload,
	}); !errors.Is(err, domainappdev.ErrArtifactGrantDirectionMismatch) {
		t.Fatalf("direction mismatch = %v", err)
	}

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
			record, err := repository.Consume(context.Background(), input)
			if err == nil {
				if record.State != domainappdev.ArtifactGrantStateConsumed {
					t.Errorf("consume state = %q", record.State)
				}
				successes.Add(1)
				return
			}
			if !errors.Is(err, domainappdev.ErrArtifactGrantConsumed) {
				t.Errorf("concurrent consume = %v", err)
			}
		}()
	}
	ready.Wait()
	close(start)
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful consumers = %d", successes.Load())
	}
	if _, err := repository.Consume(context.Background(), input); !errors.Is(err, domainappdev.ErrArtifactGrantConsumed) {
		t.Fatalf("replay = %v", err)
	}
}

func TestArtifactGrantRedisConsumeAndRevokeCheckEveryAudienceField(t *testing.T) {
	mutations := []struct {
		name  string
		field string
		value string
	}{
		{name: "space id", field: "space_id", value: "2002"},
		{name: "project id", field: "project_id", value: "project-b"},
		{name: "provider key", field: "provider_key", value: "provider-b"},
		{name: "provider scope", field: "provider_scope", value: "agent"},
		{name: "operation", field: "operation", value: "snapshot.download"},
	}
	for actionIndex, action := range []struct {
		name string
		run  func(*RedisArtifactGrantRepository, domainappdev.ArtifactGrantID, domainappdev.ArtifactGrantToken, domainappdev.ArtifactGrantSpec) error
	}{
		{name: "consume", run: func(repository *RedisArtifactGrantRepository, grantID domainappdev.ArtifactGrantID, token domainappdev.ArtifactGrantToken, spec domainappdev.ArtifactGrantSpec) error {
			_, err := repository.Consume(context.Background(), domainappdev.ConsumeArtifactGrantRepositoryInput{
				GrantID: grantID, TokenHash: token.Hash(), Audience: spec.Audience, Direction: spec.Direction,
			})
			return err
		}},
		{name: "revoke", run: func(repository *RedisArtifactGrantRepository, grantID domainappdev.ArtifactGrantID, token domainappdev.ArtifactGrantToken, spec domainappdev.ArtifactGrantSpec) error {
			_, err := repository.Revoke(context.Background(), domainappdev.RevokeArtifactGrantRepositoryInput{
				GrantID: grantID, TokenHash: token.Hash(), Audience: spec.Audience, Direction: spec.Direction,
			})
			return err
		}},
	} {
		for mutationIndex, mutation := range mutations {
			t.Run(action.name+"/"+mutation.name, func(t *testing.T) {
				repository, server := newArtifactGrantRedisRepositoryForTest(t)
				grantID, token, spec := artifactGrantRepositoryFixture(t, byte(0x80+actionIndex*16+mutationIndex))
				issueArtifactGrantForRepositoryTest(t, repository, grantID, token, spec, time.Minute)
				server.HSet(artifactGrantRedisKey(defaultArtifactGrantRedisNamespace, grantID), mutation.field, mutation.value)
				if err := action.run(repository, grantID, token, spec); !errors.Is(err, domainappdev.ErrArtifactGrantAudienceMismatch) {
					t.Fatalf("%s with mutated %s = %v", action.name, mutation.field, err)
				}
			})
		}
	}
}

func TestArtifactGrantRedisTokenHashComparisonScansFixedCanonicalLength(t *testing.T) {
	for _, script := range []string{artifactGrantConsumeScript, artifactGrantRevokeScript} {
		if strings.Contains(script, `redis.call("HGET", KEYS[1], "token_hash") ~= ARGV[1]`) ||
			!strings.Contains(script, "for index = 1, 64 do") ||
			!strings.Contains(script, "difference = difference + math.abs") {
			t.Fatal("token hash script does not use fixed-length difference accumulation")
		}
	}
	mutations := []struct {
		name   string
		mutate func(string) string
	}{
		{name: "first", mutate: func(value string) string { return "0" + value[1:] }},
		{name: "middle", mutate: func(value string) string { return value[:32] + "0" + value[33:] }},
		{name: "last", mutate: func(value string) string { return value[:63] + "0" }},
		{name: "short", mutate: func(value string) string { return value[:63] }},
		{name: "long", mutate: func(value string) string { return value + "0" }},
		{name: "non hex", mutate: func(string) string { return strings.Repeat("g", 64) }},
	}
	for actionIndex, action := range []struct {
		name   string
		script string
	}{
		{name: "consume", script: artifactGrantConsumeScript},
		{name: "revoke", script: artifactGrantRevokeScript},
	} {
		for mutationIndex, mutation := range mutations {
			t.Run(action.name+"/"+mutation.name, func(t *testing.T) {
				repository, _ := newArtifactGrantRedisRepositoryForTest(t)
				grantID, token, spec := artifactGrantRepositoryFixture(t, byte(0xa0+actionIndex*16+mutationIndex))
				issueArtifactGrantForRepositoryTest(t, repository, grantID, token, spec, time.Minute)
				args := artifactGrantRedisCapabilityArgs(token.Hash(), spec.Audience, spec.Direction)
				canonical := args[0].(string)
				mutated := mutation.mutate(canonical)
				if mutated == canonical {
					mutated = "f" + canonical[1:]
					if canonical[0] == 'f' {
						mutated = "0" + canonical[1:]
					}
				}
				args[0] = mutated
				result, err := repository.client.RunScript(context.Background(), action.script,
					[]string{artifactGrantRedisKey(defaultArtifactGrantRedisNamespace, grantID)}, args...).Result()
				if err != nil {
					t.Fatal(err)
				}
				reply, err := parseArtifactGrantRedisReply(result)
				if err != nil || len(reply) != 2 || reply[1] != "token" {
					t.Fatalf("mutated hash reply = %#v, %v", reply, err)
				}
			})
		}
	}
}

func TestArtifactGrantRedisExpiryAndRevokeAreFailClosedAndIdempotent(t *testing.T) {
	repository, server := newArtifactGrantRedisRepositoryForTest(t)
	expiredID, expiredToken, expiredSpec := artifactGrantRepositoryFixture(t, 0x40)
	issueArtifactGrantForRepositoryTest(t, repository, expiredID, expiredToken, expiredSpec, 20*time.Millisecond)
	server.FastForward(time.Second)
	if _, err := repository.Consume(context.Background(), domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: expiredID, TokenHash: expiredToken.Hash(), Audience: expiredSpec.Audience, Direction: expiredSpec.Direction,
	}); !errors.Is(err, domainappdev.ErrArtifactGrantDenied) {
		t.Fatalf("expired consume = %v", err)
	}

	revokedID, revokedToken, revokedSpec := artifactGrantRepositoryFixture(t, 0x50)
	issueArtifactGrantForRepositoryTest(t, repository, revokedID, revokedToken, revokedSpec, time.Minute)
	revokeInput := domainappdev.RevokeArtifactGrantRepositoryInput{
		GrantID: revokedID, TokenHash: revokedToken.Hash(), Audience: revokedSpec.Audience, Direction: revokedSpec.Direction,
	}
	for attempt := 0; attempt < 2; attempt++ {
		record, err := repository.Revoke(context.Background(), revokeInput)
		if err != nil || record.State != domainappdev.ArtifactGrantStateRevoked {
			t.Fatalf("revoke attempt %d = %#v, %v", attempt, record, err)
		}
	}
	_, forgedToken, _ := artifactGrantRepositoryFixture(t, 0x51)
	forged := revokeInput
	forged.TokenHash = forgedToken.Hash()
	if _, err := repository.Revoke(context.Background(), forged); !errors.Is(err, domainappdev.ErrArtifactGrantDenied) {
		t.Fatalf("forged idempotent revoke = %v", err)
	}
	if _, err := repository.Consume(context.Background(), domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: revokedID, TokenHash: revokedToken.Hash(), Audience: revokedSpec.Audience, Direction: revokedSpec.Direction,
	}); !errors.Is(err, domainappdev.ErrArtifactGrantRevoked) {
		t.Fatalf("consume revoked = %v", err)
	}

	consumedID, consumedToken, consumedSpec := artifactGrantRepositoryFixture(t, 0x60)
	issueArtifactGrantForRepositoryTest(t, repository, consumedID, consumedToken, consumedSpec, time.Minute)
	if _, err := repository.Consume(context.Background(), domainappdev.ConsumeArtifactGrantRepositoryInput{
		GrantID: consumedID, TokenHash: consumedToken.Hash(), Audience: consumedSpec.Audience, Direction: consumedSpec.Direction,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Revoke(context.Background(), domainappdev.RevokeArtifactGrantRepositoryInput{
		GrantID: consumedID, TokenHash: consumedToken.Hash(), Audience: consumedSpec.Audience, Direction: consumedSpec.Direction,
	}); !errors.Is(err, domainappdev.ErrArtifactGrantConsumed) {
		t.Fatalf("revoke consumed = %v", err)
	}
}

type artifactGrantScriptCmdStub struct {
	result any
	err    error
}

func (c artifactGrantScriptCmdStub) Err() error           { return c.err }
func (c artifactGrantScriptCmdStub) Result() (any, error) { return c.result, c.err }

type artifactGrantScriptRunnerStub struct {
	result any
	err    error
}

func (r artifactGrantScriptRunnerStub) RunScript(context.Context, string, []string, ...interface{}) cache.ScriptCmd {
	return artifactGrantScriptCmdStub{result: r.result, err: r.err}
}

func TestArtifactGrantRedisMalformedReplyContextAndRawErrorsFailClosed(t *testing.T) {
	grantID, token, spec := artifactGrantRepositoryFixture(t, 0x70)
	for _, test := range []struct {
		name   string
		result any
		err    error
	}{
		{name: "missing fields", result: []interface{}{artifactGrantRedisProtocolVersion, "ok"}},
		{name: "unknown version", result: []interface{}{"artifact-grant/v999", "ok", "1", "2"}},
		{name: "wrong type", result: []interface{}{artifactGrantRedisProtocolVersion, int64(1)}},
		{name: "oversized", result: []interface{}{artifactGrantRedisProtocolVersion, strings.Repeat("x", 9000)}},
		{name: "raw redis", err: errors.New("ERR token/hash/internal-object-key")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, err := NewRedisArtifactGrantRepository(artifactGrantScriptRunnerStub{result: test.result, err: test.err})
			if err != nil {
				t.Fatal(err)
			}
			record, err := repository.Issue(context.Background(), domainappdev.IssueArtifactGrantRepositoryInput{
				GrantID: grantID, TokenHash: token.Hash(), Spec: spec, TTL: time.Minute,
			})
			if record != nil || !errors.Is(err, domainappdev.ErrArtifactGrantUnavailable) || strings.Contains(err.Error(), "token/hash/internal-object-key") {
				t.Fatalf("Issue() = %#v, %v", record, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repository, _ := NewRedisArtifactGrantRepository(artifactGrantScriptRunnerStub{})
	if record, err := repository.Issue(ctx, domainappdev.IssueArtifactGrantRepositoryInput{
		GrantID: grantID, TokenHash: token.Hash(), Spec: spec, TTL: time.Minute,
	}); record != nil || !errors.Is(err, domainappdev.ErrArtifactGrantUnavailable) {
		t.Fatalf("canceled Issue() = %#v, %v", record, err)
	}
}
