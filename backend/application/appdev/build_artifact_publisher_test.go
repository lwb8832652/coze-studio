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

package appdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type buildArtifactGrantFake struct {
	issues  []IssueArtifactGrantRequest
	tokens  []domainappdev.ArtifactGrantToken
	specs   map[string]domainappdev.ArtifactGrantSpec
	revoked map[string]bool
}

func (fake *buildArtifactGrantFake) Issue(_ context.Context, request IssueArtifactGrantRequest) (*ArtifactGrantCapability, error) {
	if fake.specs == nil {
		fake.specs = map[string]domainappdev.ArtifactGrantSpec{}
		fake.revoked = map[string]bool{}
	}
	index := len(fake.issues) + 1
	raw := bytes.Repeat([]byte{byte(index)}, domainappdev.ArtifactGrantTokenBytes)
	token, _ := domainappdev.ParseArtifactGrantToken(base64.RawURLEncoding.EncodeToString(raw))
	id, _ := domainappdev.ParseArtifactGrantID(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{byte(index + 32)}, domainappdev.ArtifactGrantIDBytes)))
	now := time.Now()
	fake.issues = append(fake.issues, request)
	fake.tokens = append(fake.tokens, token)
	fake.specs[token.Bearer()] = request.Spec
	return &ArtifactGrantCapability{
		grantID: id, token: token, audience: request.Spec.Audience, direction: request.Spec.Direction,
		digest: request.Spec.Digest, size: request.Spec.Size, maxSize: request.Spec.MaxSize,
		issuedAt: now, expiresAt: now.Add(request.TTL),
	}, nil
}

func (fake *buildArtifactGrantFake) Revoke(_ context.Context, request RevokeArtifactGrantRequest) error {
	fake.revoked[request.Token.Bearer()] = true
	return nil
}

type buildArtifactEndpointFake struct{}

func (buildArtifactEndpointFake) ArtifactUploadEndpoint(_ context.Context, id domainappdev.ArtifactGrantID) (string, error) {
	return "https://gateway.example/internal/artifacts/" + id.Encoded(), nil
}

type buildArtifactStoreFake struct{ objects map[string][]byte }

func (fake *buildArtifactStoreFake) VerifyBuildArtifact(_ context.Context, key string, digest domainappdev.ArtifactGrantDigest, size int64) (bool, error) {
	content, ok := fake.objects[key]
	if !ok {
		return false, nil
	}
	actual := sha256.Sum256(content)
	if int64(len(content)) != size || !bytes.Equal(actual[:], digest[:]) {
		return false, domainappdev.ErrArtifactGrantStorage
	}
	return true, nil
}

type buildArtifactRuntime struct {
	publish func(context.Context, string, infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error)
}

func (*buildArtifactRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}
func (*buildArtifactRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}
func (*buildArtifactRuntime) Cancel(context.Context, string) error { return nil }
func (*buildArtifactRuntime) CloseContext(context.Context) error   { return nil }
func (runtime *buildArtifactRuntime) PublishArtifact(ctx context.Context, executionID string, request infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
	return runtime.publish(ctx, executionID, request)
}

type buildArtifactRuntimeWithoutPublisher struct{}

func (*buildArtifactRuntimeWithoutPublisher) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}
func (*buildArtifactRuntimeWithoutPublisher) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}
func (*buildArtifactRuntimeWithoutPublisher) Cancel(context.Context, string) error { return nil }
func (*buildArtifactRuntimeWithoutPublisher) CloseContext(context.Context) error   { return nil }

func buildArtifactFixture(content []byte) PublishBuildArtifactCommand {
	digest := sha256.Sum256(content)
	return PublishBuildArtifactCommand{
		Audience: domainappdev.ArtifactGrantAudience{
			SpaceID: "1001", ProjectID: "project-1", ProviderKey: "provider-1",
			ProviderScope: domainsandbox.ScopeAppDev, Operation: BuildArtifactPublishOperation,
		},
		Generation: 1, ProviderExecutionID: "execution-1",
		Descriptor: infrasandbox.ArtifactDescriptor{
			Kind:   infrasandbox.ArtifactKindAppDevBuildArchive,
			Digest: "sha256:" + fmt.Sprintf("%x", digest[:]), Size: int64(len(content)),
		},
	}
}

func newBuildArtifactPublisherTestService(t *testing.T, grants *buildArtifactGrantFake, store *buildArtifactStoreFake) *BuildArtifactPublisherService {
	t.Helper()
	service, err := NewBuildArtifactPublisherService(grants, buildArtifactEndpointFake{}, store, DefaultBuildArtifactGrantTTL)
	require.NoError(t, err)
	return service
}

func TestBuildArtifactPublisherIssuesExactGrantPublishesAndVerifies(t *testing.T) {
	content := []byte("verified build archive")
	command := buildArtifactFixture(content)
	grants := &buildArtifactGrantFake{}
	store := &buildArtifactStoreFake{objects: map[string][]byte{}}
	service := newBuildArtifactPublisherTestService(t, grants, store)
	var providerRequest infrasandbox.ArtifactPublishRequest
	runtime := &buildArtifactRuntime{publish: func(_ context.Context, executionID string, request infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
		require.Equal(t, command.ProviderExecutionID, executionID)
		providerRequest = request
		spec := grants.specs[request.Token]
		store.objects[spec.ObjectKey] = append([]byte(nil), content...)
		return infrasandbox.ArtifactPublishResult{Accepted: true}, nil
	}}

	receipt, err := service.Publish(context.Background(), runtime, command)
	require.NoError(t, err)
	require.Len(t, grants.issues, 1)
	spec := grants.issues[0].Spec
	require.Equal(t, command.Audience, spec.Audience)
	require.Equal(t, domainappdev.ArtifactGrantDirectionUpload, spec.Direction)
	require.Equal(t, command.Descriptor.Size, spec.Size)
	require.Equal(t, spec.Size, spec.MaxSize)
	require.LessOrEqual(t, grants.issues[0].TTL, domainappdev.MaxArtifactGrantTTL)
	require.Equal(t, spec.ObjectKey, receipt.InternalObjectKey())
	require.NotContains(t, fmt.Sprintf("%#v", providerRequest), providerRequest.Token)
	require.NotContains(t, fmt.Sprintf("%#v", receipt), spec.ObjectKey)
	encoded, jsonErr := json.Marshal(receipt)
	require.Error(t, jsonErr)
	require.NotContains(t, string(encoded), spec.ObjectKey)
}

func TestBuildArtifactPublisherFailsClosedWithoutCapabilityOrWithInvalidDescriptor(t *testing.T) {
	content := []byte("archive")
	command := buildArtifactFixture(content)
	service := newBuildArtifactPublisherTestService(t, &buildArtifactGrantFake{}, &buildArtifactStoreFake{objects: map[string][]byte{}})
	_, err := service.Publish(context.Background(), &buildArtifactRuntimeWithoutPublisher{}, command)
	require.ErrorIs(t, err, ErrBuildArtifactPublisherUnsupported)

	command.Descriptor.Kind = "provider_object_key"
	_, err = service.Publish(context.Background(), &buildArtifactRuntime{publish: func(context.Context, string, infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
		t.Fatal("invalid descriptor reached provider")
		return infrasandbox.ArtifactPublishResult{}, nil
	}}, command)
	require.ErrorIs(t, err, domainappdev.ErrArtifactGrantInvalid)
}

func TestBuildArtifactPublisherTreatsLostResponseWithVerifiedObjectAsSuccess(t *testing.T) {
	content := []byte("response was lost after upload")
	command := buildArtifactFixture(content)
	grants := &buildArtifactGrantFake{}
	store := &buildArtifactStoreFake{objects: map[string][]byte{}}
	service := newBuildArtifactPublisherTestService(t, grants, store)
	runtime := &buildArtifactRuntime{publish: func(_ context.Context, _ string, request infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
		spec := grants.specs[request.Token]
		store.objects[spec.ObjectKey] = append([]byte(nil), content...)
		return infrasandbox.ArtifactPublishResult{}, errors.New("transport response lost token=must-not-leak")
	}}
	receipt, err := service.Publish(context.Background(), runtime, command)
	require.NoError(t, err)
	require.NotEmpty(t, receipt.InternalObjectKey())
	require.Empty(t, grants.revoked)
}

func TestBuildArtifactPublisherRevokesFailedGrantAndRetriesWithNewTokenSameDestination(t *testing.T) {
	content := []byte("retry archive")
	command := buildArtifactFixture(content)
	grants := &buildArtifactGrantFake{}
	store := &buildArtifactStoreFake{objects: map[string][]byte{}}
	service := newBuildArtifactPublisherTestService(t, grants, store)
	attempt := 0
	runtime := &buildArtifactRuntime{publish: func(_ context.Context, _ string, request infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
		attempt++
		if attempt == 1 {
			return infrasandbox.ArtifactPublishResult{}, errors.New("provider publish failed token=" + request.Token)
		}
		spec := grants.specs[request.Token]
		store.objects[spec.ObjectKey] = append([]byte(nil), content...)
		return infrasandbox.ArtifactPublishResult{Accepted: true}, nil
	}}

	_, firstErr := service.Publish(context.Background(), runtime, command)
	require.ErrorIs(t, firstErr, ErrBuildArtifactPublishUnavailable)
	require.NotContains(t, firstErr.Error(), grants.tokens[0].Bearer())
	require.True(t, grants.revoked[grants.tokens[0].Bearer()])
	receipt, err := service.Publish(context.Background(), runtime, command)
	require.NoError(t, err)
	require.Len(t, grants.issues, 2)
	require.NotEqual(t, grants.tokens[0].Bearer(), grants.tokens[1].Bearer())
	require.Equal(t, grants.issues[0].Spec.ObjectKey, grants.issues[1].Spec.ObjectKey)
	require.Equal(t, grants.issues[1].Spec.ObjectKey, receipt.InternalObjectKey())
}

func TestBuildArtifactPublisherRejectsMissingOrMismatchedFinalObject(t *testing.T) {
	content := []byte("expected archive")
	command := buildArtifactFixture(content)
	for _, testCase := range []struct {
		name  string
		write []byte
		err   error
	}{
		{name: "missing", err: errors.New("publish failed")},
		{name: "digest mismatch", write: []byte("wrong artifact")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			grants := &buildArtifactGrantFake{}
			store := &buildArtifactStoreFake{objects: map[string][]byte{}}
			service := newBuildArtifactPublisherTestService(t, grants, store)
			runtime := &buildArtifactRuntime{publish: func(_ context.Context, _ string, request infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
				if testCase.write != nil {
					store.objects[grants.specs[request.Token].ObjectKey] = append([]byte(nil), testCase.write...)
				}
				return infrasandbox.ArtifactPublishResult{Accepted: testCase.err == nil}, testCase.err
			}}
			_, err := service.Publish(context.Background(), runtime, command)
			require.ErrorIs(t, err, ErrBuildArtifactPublishUnavailable)
		})
	}
}

func TestBuildArtifactPublisherPreservesCanceledContextBeforeGrant(t *testing.T) {
	grants := &buildArtifactGrantFake{}
	service := newBuildArtifactPublisherTestService(t, grants, &buildArtifactStoreFake{objects: map[string][]byte{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Publish(ctx, &buildArtifactRuntime{}, buildArtifactFixture([]byte("archive")))
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, grants.issues)
}

func TestBuildArtifactReceiptDoesNotExposeDescriptorOrExecutionIdentity(t *testing.T) {
	command := buildArtifactFixture([]byte("archive"))
	digest, err := domainappdev.ParseArtifactGrantDigest(command.Descriptor.Digest)
	require.NoError(t, err)
	receipt := newBuildArtifactReceipt("internal/object/key.zip", digest, command.Descriptor.Size)
	formatted := fmt.Sprintf("%v %+v %#v", receipt, receipt, receipt)
	require.NotContains(t, formatted, command.ProviderExecutionID)
	require.NotContains(t, formatted, command.Descriptor.Digest)
	require.NotContains(t, formatted, "internal/object/key.zip")
	_, err = json.Marshal(receipt)
	require.Error(t, err)
	require.NotContains(t, strings.ToLower(err.Error()), "execution-1")
}
