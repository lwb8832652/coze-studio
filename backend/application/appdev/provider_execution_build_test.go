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
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type providerExecutionBuildRepositoryFake struct {
	domainappdev.ProviderExecutionRepository
	record         *domainappdev.ProviderExecution
	reserve        domainappdev.ReserveProviderBuildInput
	advance        domainappdev.AdvanceProviderBuildObservationInput
	beginPublish   domainappdev.BeginProviderArtifactPublishInput
	complete       domainappdev.CompleteProviderArtifactPublishInput
	fail           domainappdev.FailProviderBuildInput
	loadRecovery   *domainappdev.ProviderExecution
	buildCallError error
}

func (fake *providerExecutionBuildRepositoryFake) ReserveBuild(_ context.Context, input domainappdev.ReserveProviderBuildInput) (*domainappdev.ProviderExecution, error) {
	fake.reserve = input
	return fake.record, fake.buildCallError
}

func (fake *providerExecutionBuildRepositoryFake) AdvanceBuildObservation(_ context.Context, input domainappdev.AdvanceProviderBuildObservationInput) (*domainappdev.ProviderExecution, error) {
	fake.advance = input
	return fake.record, fake.buildCallError
}

func (fake *providerExecutionBuildRepositoryFake) BeginArtifactPublish(_ context.Context, input domainappdev.BeginProviderArtifactPublishInput) (*domainappdev.ProviderExecution, error) {
	fake.beginPublish = input
	return fake.record, fake.buildCallError
}

func (fake *providerExecutionBuildRepositoryFake) CompleteArtifactPublish(_ context.Context, input domainappdev.CompleteProviderArtifactPublishInput) (*domainappdev.ProviderExecution, error) {
	fake.complete = input
	return fake.record, fake.buildCallError
}

func (fake *providerExecutionBuildRepositoryFake) FailBuild(_ context.Context, input domainappdev.FailProviderBuildInput) (*domainappdev.ProviderExecution, error) {
	fake.fail = input
	return fake.record, fake.buildCallError
}

func (fake *providerExecutionBuildRepositoryFake) LoadOwnedRecovery(context.Context, domainappdev.ProviderExecutionOwnerCAS) (*domainappdev.ProviderExecution, error) {
	return fake.loadRecovery, nil
}

func providerExecutionBuildServiceRecord(owner ProviderExecutionOwner, version uint64) *domainappdev.ProviderExecution {
	now := time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
	return &domainappdev.ProviderExecution{
		ID: "apx_build", SpaceID: owner.spaceID, ProjectID: owner.projectID, Generation: owner.generation,
		IdempotencyKey: "start-operation", DesiredState: domainappdev.ProviderExecutionDesiredRun,
		ObservedState: domainappdev.ProviderExecutionObservedRunning, ProviderKey: owner.providerKey, ProviderScope: owner.providerScope,
		ProviderExecutionID: "provider-execution-1", ArtifactStatus: domainappdev.ProviderExecutionArtifactBeginPending,
		Version: version + 1, CreatedAt: now, UpdatedAt: now,
	}
}

func TestProviderExecutionServiceBuildMethodsBindOwnerProviderAndOperation(t *testing.T) {
	owner := providerExecutionTestOwner(t)
	repository := &providerExecutionBuildRepositoryFake{record: providerExecutionBuildServiceRecord(owner, 4)}
	service, err := NewProviderExecutionService(repository, &providerExecutionCountingCodec{})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := service.ReserveBuild(context.Background(), ReserveProviderExecutionBuildRequest{
		Owner: owner, ExpectedVersion: 4, ProviderExecutionID: "provider-execution-1", OperationID: "build-operation-1",
	})
	if err != nil || metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactBeginPending {
		t.Fatalf("ReserveBuild() = %#v, %v", metadata, err)
	}
	if repository.reserve.OperationHash.IsZero() || repository.reserve.OperationID != "build-operation-1" ||
		repository.reserve.ProviderExecutionID != "provider-execution-1" || repository.reserve.OwnerCAS.OwnerEpoch != owner.epoch {
		t.Fatalf("reserve input = %#v", repository.reserve)
	}

	descriptor := domainappdev.ProviderBuildArtifactDescriptor{
		Kind:   domainappdev.ProviderExecutionArtifactKindAppDevBuildArchive,
		Digest: "sha256:" + strings.Repeat("c", 64), Size: 2048,
	}
	_, err = service.AdvanceBuildObservation(context.Background(), AdvanceProviderExecutionBuildObservationRequest{
		Owner: owner, ExpectedVersion: 5, ProviderExecutionID: "provider-execution-1", OperationID: "build-operation-1",
		Status: domainappdev.ProviderExecutionArtifactDescriptorReady, Descriptor: descriptor,
	})
	if err != nil || !repository.advance.OperationHash.Equal(repository.reserve.OperationHash) || repository.advance.Descriptor != descriptor {
		t.Fatalf("AdvanceBuildObservation() input=%#v err=%v", repository.advance, err)
	}
	_, err = service.BeginArtifactPublish(context.Background(), BeginProviderArtifactPublishRequest{
		Owner: owner, ExpectedVersion: 5, ProviderExecutionID: "provider-execution-1", OperationID: "build-operation-1",
	})
	if err != nil || !repository.beginPublish.OperationHash.Equal(repository.reserve.OperationHash) {
		t.Fatalf("BeginArtifactPublish() input=%#v err=%v", repository.beginPublish, err)
	}
	_, err = service.CompleteArtifactPublish(context.Background(), CompleteProviderArtifactPublishRequest{
		Owner: owner, ExpectedVersion: 5, ProviderExecutionID: "provider-execution-1", OperationID: "build-operation-1",
		ObjectKey: "appdev/internal/build.zip", Digest: descriptor.Digest, Size: descriptor.Size,
	})
	if err != nil || !repository.complete.OperationHash.Equal(repository.reserve.OperationHash) || repository.complete.ObjectKey == "" {
		t.Fatalf("CompleteArtifactPublish() input=%#v err=%v", repository.complete, err)
	}
	_, err = service.FailBuild(context.Background(), FailProviderExecutionBuildRequest{
		Owner: owner, ExpectedVersion: 5, ProviderExecutionID: "provider-execution-1", OperationID: "build-operation-1",
		SafeErrorCode: "provider_build_failed", SafeErrorMessage: "build failed",
	})
	if err != nil || !repository.fail.OperationHash.Equal(repository.reserve.OperationHash) {
		t.Fatalf("FailBuild() input=%#v err=%v", repository.fail, err)
	}
}

func TestProviderExecutionServiceBuildFailsClosedAndRecoveryKeepsOperationInternal(t *testing.T) {
	owner := providerExecutionTestOwner(t)
	record := providerExecutionBuildServiceRecord(owner, 4)
	record.BuildOperationID = "restart-build-operation"
	cas, casErr := providerExecutionOwnerCAS(owner, 4)
	if casErr != nil {
		t.Fatal(casErr)
	}
	record.BuildOperationHash, casErr = domainappdev.HashProviderExecutionBuildOperationID(record.BuildOperationID, cas, record.ProviderExecutionID)
	if casErr != nil {
		t.Fatal(casErr)
	}
	record.ArtifactObjectKey = "appdev/internal/secret-build.zip"
	repository := &providerExecutionBuildRepositoryFake{record: record, loadRecovery: record}
	service, err := NewProviderExecutionService(repository, &providerExecutionCountingCodec{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReserveBuild(context.Background(), ReserveProviderExecutionBuildRequest{
		Owner: owner, ExpectedVersion: 4, ProviderExecutionID: "", OperationID: "build-operation-1",
	})
	if !errors.Is(err, ErrProviderExecutionServiceInvalid) {
		t.Fatalf("invalid provider execution ID error = %v", err)
	}
	recovery, err := service.LoadRecovery(context.Background(), LoadProviderExecutionRecoveryRequest{
		Owner: owner, ExpectedVersion: 4, ProviderKey: owner.providerKey, ProviderScope: owner.providerScope,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, status, ok := recovery.buildOperation()
	if !ok || operation != record.BuildOperationID || status != record.ArtifactStatus {
		t.Fatalf("build recovery = %q/%q/%v", operation, status, ok)
	}
	formatted := fmt.Sprintf("%v|%+v|%#v", recovery, recovery, recovery)
	for _, secret := range []string{record.BuildOperationID, record.ArtifactObjectKey} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("recovery leaked %q: %s", secret, formatted)
		}
	}
}
