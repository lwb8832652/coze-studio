// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type artifactGrantGeneratorStub struct {
	token      domainappdev.ArtifactGrantToken
	grantID    domainappdev.ArtifactGrantID
	tokenErr   error
	grantIDErr error
	tokenCalls int
	idCalls    int
}

func (g *artifactGrantGeneratorStub) GenerateToken(context.Context) (domainappdev.ArtifactGrantToken, error) {
	g.tokenCalls++
	return g.token, g.tokenErr
}

func (g *artifactGrantGeneratorStub) GenerateGrantID(context.Context) (domainappdev.ArtifactGrantID, error) {
	g.idCalls++
	return g.grantID, g.grantIDErr
}

type artifactGrantLifecycleRepositoryStub struct {
	issueInput    *domainappdev.IssueArtifactGrantRepositoryInput
	consumeInput  *domainappdev.ConsumeArtifactGrantRepositoryInput
	revokeInput   *domainappdev.RevokeArtifactGrantRepositoryInput
	issueRecord   *domainappdev.ArtifactGrantRecord
	consumeRecord *domainappdev.ArtifactGrantRecord
	revokeRecord  *domainappdev.ArtifactGrantRecord
	issueErr      error
	consumeErr    error
	revokeErr     error
}

func (r *artifactGrantLifecycleRepositoryStub) Issue(_ context.Context, input domainappdev.IssueArtifactGrantRepositoryInput) (*domainappdev.ArtifactGrantRecord, error) {
	r.issueInput = &input
	return r.issueRecord, r.issueErr
}

func (r *artifactGrantLifecycleRepositoryStub) Consume(_ context.Context, input domainappdev.ConsumeArtifactGrantRepositoryInput) (*domainappdev.ArtifactGrantRecord, error) {
	r.consumeInput = &input
	return r.consumeRecord, r.consumeErr
}

func (r *artifactGrantLifecycleRepositoryStub) Revoke(_ context.Context, input domainappdev.RevokeArtifactGrantRepositoryInput) (*domainappdev.ArtifactGrantRecord, error) {
	r.revokeInput = &input
	return r.revokeRecord, r.revokeErr
}

func artifactGrantLifecycleFixture(t *testing.T) (domainappdev.ArtifactGrantToken, domainappdev.ArtifactGrantID, domainappdev.ArtifactGrantSpec) {
	t.Helper()
	token, err := domainappdev.NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{0x41}, domainappdev.ArtifactGrantTokenBytes)))
	if err != nil {
		t.Fatal(err)
	}
	grantID, err := domainappdev.NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{0x42}, domainappdev.ArtifactGrantIDBytes)))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("payload")
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
	return token, grantID, spec
}

func artifactGrantIssuedRecord(grantID domainappdev.ArtifactGrantID, spec domainappdev.ArtifactGrantSpec, state domainappdev.ArtifactGrantState) *domainappdev.ArtifactGrantRecord {
	issuedAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	return &domainappdev.ArtifactGrantRecord{
		GrantID: grantID, Spec: spec, State: state, IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(time.Minute),
	}
}

func TestArtifactGrantIssueStoresOnlyHashAndReturnsRedactedCapability(t *testing.T) {
	token, grantID, spec := artifactGrantLifecycleFixture(t)
	repository := &artifactGrantLifecycleRepositoryStub{issueRecord: artifactGrantIssuedRecord(grantID, spec, domainappdev.ArtifactGrantStateIssued)}
	generator := &artifactGrantGeneratorStub{token: token, grantID: grantID}
	service, err := NewArtifactGrantLifecycleService(repository, WithArtifactGrantGenerator(generator))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := service.Issue(context.Background(), IssueArtifactGrantRequest{Spec: spec, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if repository.issueInput == nil || !repository.issueInput.TokenHash.Equal(token.Hash()) || !repository.issueInput.GrantID.Equal(grantID) {
		t.Fatalf("repository issue input = %#v", repository.issueInput)
	}
	inputType := reflect.TypeOf(domainappdev.IssueArtifactGrantRepositoryInput{})
	for _, forbidden := range []string{"Token", "Bearer", "ObjectKey"} {
		if _, exists := inputType.FieldByName(forbidden); exists {
			t.Fatalf("repository issue input exposes %s", forbidden)
		}
	}
	if generator.tokenCalls != 1 || generator.idCalls != 1 || capability.BearerToken() != token.Bearer() || !capability.GrantID().Equal(grantID) {
		t.Fatalf("generator calls=%d/%d capability=%#v", generator.tokenCalls, generator.idCalls, capability)
	}
	for _, formatted := range []string{fmt.Sprintf("%v", capability), fmt.Sprintf("%+v", capability), fmt.Sprintf("%#v", capability)} {
		if strings.Contains(formatted, token.Bearer()) || strings.Contains(formatted, grantID.Encoded()) || strings.Contains(formatted, spec.ObjectKey) || !strings.Contains(formatted, "REDACTED") {
			t.Fatalf("capability leaked: %q", formatted)
		}
	}
	if encoded, err := json.Marshal(capability); err == nil || len(encoded) != 0 {
		t.Fatalf("capability JSON = %q, %v", encoded, err)
	}
}

func TestArtifactGrantIssueFailsClosedBeforeReturningCapability(t *testing.T) {
	token, grantID, spec := artifactGrantLifecycleFixture(t)
	for _, test := range []struct {
		name       string
		ctx        context.Context
		request    IssueArtifactGrantRequest
		repository *artifactGrantLifecycleRepositoryStub
	}{
		{name: "invalid ttl", ctx: context.Background(), request: IssueArtifactGrantRequest{Spec: spec, TTL: domainappdev.MaxArtifactGrantTTL + time.Nanosecond}, repository: &artifactGrantLifecycleRepositoryStub{}},
		{name: "repository failure", ctx: context.Background(), request: IssueArtifactGrantRequest{Spec: spec, TTL: time.Minute}, repository: &artifactGrantLifecycleRepositoryStub{issueErr: errors.New("redis token/hash/internal-key")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			generator := &artifactGrantGeneratorStub{token: token, grantID: grantID}
			service, err := NewArtifactGrantLifecycleService(test.repository, WithArtifactGrantGenerator(generator))
			if err != nil {
				t.Fatal(err)
			}
			capability, err := service.Issue(test.ctx, test.request)
			if capability != nil || err == nil || strings.Contains(err.Error(), "token/hash/internal-key") {
				t.Fatalf("Issue() = %#v, %v", capability, err)
			}
		})
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	repository := &artifactGrantLifecycleRepositoryStub{}
	generator := &artifactGrantGeneratorStub{token: token, grantID: grantID}
	service, _ := NewArtifactGrantLifecycleService(repository, WithArtifactGrantGenerator(generator))
	if capability, err := service.Issue(canceled, IssueArtifactGrantRequest{Spec: spec, TTL: time.Minute}); capability != nil || !errors.Is(err, context.Canceled) || generator.tokenCalls != 0 || repository.issueInput != nil {
		t.Fatalf("canceled Issue() = %#v, %v", capability, err)
	}
}

func TestArtifactGrantIssueUsesDomainMinimumTTL(t *testing.T) {
	token, grantID, spec := artifactGrantLifecycleFixture(t)
	record := artifactGrantIssuedRecord(grantID, spec, domainappdev.ArtifactGrantStateIssued)
	record.ExpiresAt = record.IssuedAt.Add(time.Microsecond)
	repository := &artifactGrantLifecycleRepositoryStub{issueRecord: record}
	service, err := NewArtifactGrantLifecycleService(repository, WithArtifactGrantGenerator(&artifactGrantGeneratorStub{token: token, grantID: grantID}))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := service.Issue(context.Background(), IssueArtifactGrantRequest{Spec: spec, TTL: time.Microsecond})
	if err != nil || capability == nil || repository.issueInput == nil || repository.issueInput.TTL != time.Microsecond {
		t.Fatalf("minimum TTL issue = %#v, %v, input=%#v", capability, err, repository.issueInput)
	}
	repository.issueInput = nil
	if capability, err := service.Issue(context.Background(), IssueArtifactGrantRequest{Spec: spec, TTL: time.Nanosecond}); capability != nil || !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) || repository.issueInput != nil {
		t.Fatalf("sub-microsecond issue = %#v, %v, input=%#v", capability, err, repository.issueInput)
	}
}

func TestArtifactGrantConsumePassesCompleteAudienceToAtomicRepository(t *testing.T) {
	token, grantID, spec := artifactGrantLifecycleFixture(t)
	repository := &artifactGrantLifecycleRepositoryStub{consumeRecord: artifactGrantIssuedRecord(grantID, spec, domainappdev.ArtifactGrantStateConsumed)}
	service, err := NewArtifactGrantLifecycleService(repository)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Consume(context.Background(), ConsumeArtifactGrantRequest{
		GrantID: grantID, Token: token, Audience: spec.Audience, Direction: spec.Direction,
	})
	if err != nil || record == nil {
		t.Fatalf("Consume() = %#v, %v", record, err)
	}
	if repository.consumeInput == nil || !repository.consumeInput.GrantID.Equal(grantID) ||
		!repository.consumeInput.TokenHash.Equal(token.Hash()) || repository.consumeInput.Audience != spec.Audience ||
		repository.consumeInput.Direction != spec.Direction {
		t.Fatalf("repository consume input = %#v", repository.consumeInput)
	}
}

func TestArtifactGrantRevokeRequiresIdentityCapabilityAndAudience(t *testing.T) {
	token, grantID, spec := artifactGrantLifecycleFixture(t)
	repository := &artifactGrantLifecycleRepositoryStub{revokeRecord: artifactGrantIssuedRecord(grantID, spec, domainappdev.ArtifactGrantStateRevoked)}
	service, err := NewArtifactGrantLifecycleService(repository)
	if err != nil {
		t.Fatal(err)
	}
	request := RevokeArtifactGrantRequest{GrantID: grantID, Token: token, Audience: spec.Audience, Direction: spec.Direction}
	if err := service.Revoke(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(context.Background(), request); err != nil {
		t.Fatalf("idempotent revoke = %v", err)
	}
	if repository.revokeInput == nil || !repository.revokeInput.GrantID.Equal(grantID) ||
		!repository.revokeInput.TokenHash.Equal(token.Hash()) || repository.revokeInput.Audience != spec.Audience ||
		repository.revokeInput.Direction != spec.Direction {
		t.Fatalf("repository revoke input = %#v", repository.revokeInput)
	}
	if err := service.Revoke(context.Background(), RevokeArtifactGrantRequest{GrantID: grantID}); !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) {
		t.Fatalf("enumerable-ID-only revoke = %v", err)
	}
}

func TestArtifactGrantLifecycleNormalizesRepositoryErrorsWithoutSecrets(t *testing.T) {
	token, grantID, spec := artifactGrantLifecycleFixture(t)
	for _, test := range []struct {
		name string
		run  func(*ArtifactGrantLifecycleService) error
		repo *artifactGrantLifecycleRepositoryStub
	}{
		{name: "consume", repo: &artifactGrantLifecycleRepositoryStub{consumeErr: errors.New("token/hash/object-key")}, run: func(service *ArtifactGrantLifecycleService) error {
			_, err := service.Consume(context.Background(), ConsumeArtifactGrantRequest{GrantID: grantID, Token: token, Audience: spec.Audience, Direction: spec.Direction})
			return err
		}},
		{name: "revoke", repo: &artifactGrantLifecycleRepositoryStub{revokeErr: errors.New("token/hash/object-key")}, run: func(service *ArtifactGrantLifecycleService) error {
			return service.Revoke(context.Background(), RevokeArtifactGrantRequest{GrantID: grantID, Token: token, Audience: spec.Audience, Direction: spec.Direction})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewArtifactGrantLifecycleService(test.repo)
			if err != nil {
				t.Fatal(err)
			}
			err = test.run(service)
			if !errors.Is(err, domainappdev.ErrArtifactGrantUnavailable) || strings.Contains(err.Error(), "token/hash/object-key") {
				t.Fatalf("normalized error = %v", err)
			}
		})
	}
}
