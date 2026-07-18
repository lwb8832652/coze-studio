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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func providerExecutionBuildRunning(t *testing.T, repository *ProviderExecutionRepository) (*domainappdev.ProviderExecution, domainappdev.ProviderExecutionOwnerHash) {
	t.Helper()
	ctx := context.Background()
	record, err := repository.EnsureStart(ctx, providerExecutionStartInput("build-id", "build-idempotency", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	ownerHash := testOwnerHash("build-owner")
	record, err = repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a", Generation: record.Generation, ExpectedVersion: record.Version,
		OwnerHash: ownerHash, LeaseDuration: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	record, err = repository.StartSubmission(ctx, providerExecutionStartSubmissionInput(t, cas, "build-fixture-launch"))
	if err != nil {
		t.Fatalf("start submission: %v", err)
	}
	record = markProviderExecutionSubmittedForTest(t, repository, record, ownerHash, "build-fixture-launch")
	cas = providerExecutionCAS(record, ownerHash, time.Time{})
	record, err = repository.SaveSubmission(ctx, providerExecutionSaveSubmissionInput(
		t, cas, "build-fixture-launch", "provider-execution-1", time.Hour, "",
	))
	if err != nil {
		t.Fatalf("save submission: %v", err)
	}
	record = completeProviderExecutionLaunchForTest(t, repository, record, ownerHash, "build-fixture-launch")
	return record, ownerHash
}

func providerExecutionReserveBuildInput(t *testing.T, record *domainappdev.ProviderExecution, ownerHash domainappdev.ProviderExecutionOwnerHash, operation string) domainappdev.ReserveProviderBuildInput {
	t.Helper()
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	hash, err := domainappdev.HashProviderExecutionBuildOperationID(operation, cas, record.ProviderExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	return domainappdev.ReserveProviderBuildInput{
		OwnerCAS: cas, ProviderExecutionID: record.ProviderExecutionID, OperationID: operation, OperationHash: hash,
	}
}

func TestProviderExecutionRepositoryBuildReservationIdempotencyAndFences(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, ownerHash := providerExecutionBuildRunning(t, repository)
	input := providerExecutionReserveBuildInput(t, record, ownerHash, "build-operation-1")
	reserved, err := repository.ReserveBuild(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.ArtifactStatus != domainappdev.ProviderExecutionArtifactBeginPending || reserved.BuildOperationID != input.OperationID ||
		reserved.ArtifactVersion != 1 || reserved.Version != record.Version+1 {
		t.Fatalf("reserved = %#v", reserved)
	}
	retry, err := repository.ReserveBuild(context.Background(), input)
	if err != nil || retry.Version != reserved.Version || retry.ArtifactVersion != 1 {
		t.Fatalf("idempotent reserve = %#v, %v", retry, err)
	}

	other := providerExecutionReserveBuildInput(t, reserved, ownerHash, "build-operation-2")
	if _, err := repository.ReserveBuild(context.Background(), other); !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("different active operation error = %v", err)
	}
	running, err := repository.AdvanceBuildObservation(context.Background(), domainappdev.AdvanceProviderBuildObservationInput{
		OwnerCAS: providerExecutionCAS(reserved, ownerHash, time.Time{}), ProviderExecutionID: reserved.ProviderExecutionID,
		OperationHash: input.OperationHash, Status: domainappdev.ProviderExecutionArtifactBuilding,
	})
	if err != nil || running.ArtifactStatus != domainappdev.ProviderExecutionArtifactBuilding || running.Version != reserved.Version+1 {
		t.Fatalf("positive begin observation = %#v, %v", running, err)
	}
	staleOwner := input
	staleOwner.OwnerCAS = providerExecutionCAS(reserved, testOwnerHash("stale-owner"), time.Time{})
	if _, err := repository.ReserveBuild(context.Background(), staleOwner); !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("stale owner error = %v", err)
	}
	staleEpoch := input
	staleEpoch.OwnerCAS = providerExecutionCAS(reserved, ownerHash, time.Time{})
	staleEpoch.OwnerCAS.OwnerEpoch++
	staleEpoch.OperationHash, err = domainappdev.HashProviderExecutionBuildOperationID(staleEpoch.OperationID, staleEpoch.OwnerCAS, staleEpoch.ProviderExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBuild(context.Background(), staleEpoch); !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("stale epoch error = %v", err)
	}
	wrongProvider := input
	wrongProvider.OwnerCAS = providerExecutionCAS(reserved, ownerHash, time.Time{})
	wrongProvider.ProviderExecutionID = "provider-execution-2"
	wrongProvider.OperationHash, err = domainappdev.HashProviderExecutionBuildOperationID(wrongProvider.OperationID, wrongProvider.OwnerCAS, wrongProvider.ProviderExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBuild(context.Background(), wrongProvider); !errors.Is(err, domainappdev.ErrProviderExecutionIDConflict) {
		t.Fatalf("provider execution fence error = %v", err)
	}
	crossTenant := input
	crossTenant.OwnerCAS.SpaceID = "2002"
	crossTenant.OperationHash, err = domainappdev.HashProviderExecutionBuildOperationID(crossTenant.OperationID, crossTenant.OwnerCAS, crossTenant.ProviderExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBuild(context.Background(), crossTenant); !errors.Is(err, domainappdev.ErrProviderExecutionGenerationConflict) {
		t.Fatalf("cross tenant error = %v", err)
	}
}

func TestProviderExecutionRepositoryBuildObservationPublishAndResponseLoss(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, ownerHash := providerExecutionBuildRunning(t, repository)
	reserve := providerExecutionReserveBuildInput(t, record, ownerHash, "build-operation-1")
	record, err := repository.ReserveBuild(context.Background(), reserve)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := domainappdev.ProviderBuildArtifactDescriptor{
		Kind:   domainappdev.ProviderExecutionArtifactKindAppDevBuildArchive,
		Digest: "sha256:" + strings.Repeat("d", 64), Size: 8192,
	}
	observation := domainappdev.AdvanceProviderBuildObservationInput{
		OwnerCAS: providerExecutionCAS(record, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash, Status: domainappdev.ProviderExecutionArtifactDescriptorReady, Descriptor: descriptor,
	}
	descriptorReady, err := repository.AdvanceBuildObservation(context.Background(), observation)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := repository.AdvanceBuildObservation(context.Background(), observation)
	if err != nil || retry.Version != descriptorReady.Version {
		t.Fatalf("descriptor response-loss retry = %#v, %v", retry, err)
	}
	mismatch := observation
	mismatch.OwnerCAS = providerExecutionCAS(descriptorReady, ownerHash, time.Time{})
	mismatch.Descriptor.Digest = "sha256:" + strings.Repeat("e", 64)
	if _, err := repository.AdvanceBuildObservation(context.Background(), mismatch); !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("descriptor mismatch error = %v", err)
	}

	begin := domainappdev.BeginProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(descriptorReady, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash,
	}
	publishing, err := repository.BeginArtifactPublish(context.Background(), begin)
	if err != nil {
		t.Fatal(err)
	}
	beginRetry, err := repository.BeginArtifactPublish(context.Background(), begin)
	if err != nil || beginRetry.Version != publishing.Version {
		t.Fatalf("begin publish response-loss retry = %#v, %v", beginRetry, err)
	}
	complete := domainappdev.CompleteProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(publishing, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash, ObjectKey: "appdev/builds/internal/archive.zip", Digest: descriptor.Digest, Size: descriptor.Size,
	}
	ready, err := repository.CompleteArtifactPublish(context.Background(), complete)
	if err != nil {
		t.Fatal(err)
	}
	responseLossRetry, err := repository.CompleteArtifactPublish(context.Background(), complete)
	if err != nil || responseLossRetry.Version != ready.Version || responseLossRetry.ArtifactObjectKey != complete.ObjectKey {
		t.Fatalf("complete response-loss retry = %#v, %v", responseLossRetry, err)
	}
	if _, err := repository.BeginArtifactPublish(context.Background(), domainappdev.BeginProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(ready, ownerHash, time.Time{}), ProviderExecutionID: ready.ProviderExecutionID, OperationHash: reserve.OperationHash,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("ready regression error = %v", err)
	}
}

func TestProviderExecutionRepositoryBuildFailureAndStoppedRuntimeFailClosed(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, ownerHash := providerExecutionBuildRunning(t, repository)
	reserve := providerExecutionReserveBuildInput(t, record, ownerHash, "build-failure")
	record, err := repository.ReserveBuild(context.Background(), reserve)
	if err != nil {
		t.Fatal(err)
	}
	failure := domainappdev.FailProviderBuildInput{
		OwnerCAS: providerExecutionCAS(record, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash, SafeErrorCode: "provider_build_failed", SafeErrorMessage: "safe build failure",
	}
	failed, err := repository.FailBuild(context.Background(), failure)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := repository.FailBuild(context.Background(), failure)
	if err != nil || retry.Version != failed.Version || retry.ArtifactStatus != domainappdev.ProviderExecutionArtifactFailed {
		t.Fatalf("failed response-loss retry = %#v, %v", retry, err)
	}

	stopCAS := providerExecutionCAS(failed, ownerHash, time.Time{})
	stopped, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: stopCAS})
	if err != nil {
		t.Fatal(err)
	}
	next := providerExecutionReserveBuildInput(t, stopped, ownerHash, "build-after-stop")
	if _, err := repository.ReserveBuild(context.Background(), next); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("build after desired stop error = %v", err)
	}
}

func TestProviderExecutionRepositoryBuildConcurrentSameOperationCommitsOnce(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, ownerHash := providerExecutionBuildRunning(t, repository)
	reserve := providerExecutionReserveBuildInput(t, record, ownerHash, "build-concurrent")
	const workers = 64
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	results := make(chan *domainappdev.ProviderExecution, workers)
	errorsCh := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			result, err := repository.ReserveBuild(context.Background(), reserve)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}()
	}
	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent reserve: %v", err)
	}
	for result := range results {
		if result.Version != record.Version+1 || result.ArtifactVersion != 1 {
			t.Fatalf("concurrent result = %#v", result)
		}
	}
}

func TestProviderExecutionRepositoryBuildConcurrentCompleteCommitsOnce(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, ownerHash := providerExecutionBuildRunning(t, repository)
	reserve := providerExecutionReserveBuildInput(t, record, ownerHash, "build-complete-concurrent")
	record, err := repository.ReserveBuild(context.Background(), reserve)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := domainappdev.ProviderBuildArtifactDescriptor{
		Kind:   domainappdev.ProviderExecutionArtifactKindAppDevBuildArchive,
		Digest: "sha256:" + strings.Repeat("f", 64), Size: 16384,
	}
	record, err = repository.AdvanceBuildObservation(context.Background(), domainappdev.AdvanceProviderBuildObservationInput{
		OwnerCAS: providerExecutionCAS(record, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash, Status: domainappdev.ProviderExecutionArtifactDescriptorReady, Descriptor: descriptor,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err = repository.BeginArtifactPublish(context.Background(), domainappdev.BeginProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(record, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	complete := domainappdev.CompleteProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(record, ownerHash, time.Time{}), ProviderExecutionID: record.ProviderExecutionID,
		OperationHash: reserve.OperationHash, ObjectKey: "appdev/builds/internal/concurrent.zip", Digest: descriptor.Digest, Size: descriptor.Size,
	}
	const workers = 64
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	results := make(chan *domainappdev.ProviderExecution, workers)
	errorsCh := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			result, completeErr := repository.CompleteArtifactPublish(context.Background(), complete)
			if completeErr != nil {
				errorsCh <- completeErr
				return
			}
			results <- result
		}()
	}
	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for completeErr := range errorsCh {
		t.Fatalf("concurrent complete: %v", completeErr)
	}
	for result := range results {
		if result.Version != record.Version+1 || result.ArtifactStatus != domainappdev.ProviderExecutionArtifactReady {
			t.Fatalf("concurrent complete result = %#v", result)
		}
	}
}

func TestProviderExecutionBuildModelMigrationAndHCLStayAligned(t *testing.T) {
	typeOfRecord := reflect.TypeOf(providerExecutionRecord{})
	columns := []string{
		"build_operation_id", "build_operation_hash", "artifact_status", "artifact_kind", "artifact_digest",
		"artifact_size", "artifact_version", "build_started_at", "artifact_updated_at",
		"artifact_safe_error_code", "artifact_safe_error_message",
	}
	for _, column := range columns {
		found := false
		for i := 0; i < typeOfRecord.NumField(); i++ {
			if strings.Contains(typeOfRecord.Field(i).Tag.Get("gorm"), "column:"+column) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("GORM model missing %s", column)
		}
	}
	for _, path := range []string{
		filepath.Join("..", "..", "..", "docker", "atlas", "migrations", "20260716000100_appdev_provider_executions.sql"),
		filepath.Join("..", "..", "..", "docker", "atlas", "opencoze_latest_schema.hcl"),
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, column := range columns {
			if !strings.Contains(string(body), column) {
				t.Fatalf("%s missing %s", path, column)
			}
		}
	}
	record := providerExecutionRecord{BuildOperationID: "build-operation-secret", ArtifactObjectKey: "appdev/internal/secret.zip"}
	formatted := fmt.Sprintf("%v|%+v|%#v", record, record, record)
	for _, secret := range []string{record.BuildOperationID, record.ArtifactObjectKey} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("providerExecutionRecord leaked %q: %s", secret, formatted)
		}
	}
}
