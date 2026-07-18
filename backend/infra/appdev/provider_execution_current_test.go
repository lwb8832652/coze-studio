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
	"sync"
	"testing"
	"time"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"gorm.io/gorm"
)

func TestProviderExecutionRepositoryLoadCurrentIgnoresOwnerEligibilityAndScopesTenant(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	running, ownerHash := providerExecutionBuildRunning(t, repository)
	queryCount := 0
	const callbackName = "test:load_current_requires_parsed_space_id"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) { queryCount++ }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })
	for _, malformed := range []string{"", "0", "-1", " 1001", "1001 ", "1001suffix", "1e3", "9223372036854775808"} {
		before := queryCount
		if _, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
			SpaceID: malformed, ProjectID: "project-a",
		}); !errors.Is(err, domainappdev.ErrProviderExecutionInvalid) {
			t.Fatalf("LoadCurrent(%q) error = %v", malformed, err)
		}
		if queryCount != before {
			t.Fatalf("LoadCurrent(%q) reached DB: before=%d after=%d", malformed, before, queryCount)
		}
	}

	current, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a",
	})
	if err != nil || current.Generation != running.Generation || current.OwnerEpoch != running.OwnerEpoch {
		t.Fatalf("LoadCurrent(live owner) = %#v, %v", current, err)
	}
	if _, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-b",
	}); !errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		t.Fatalf("cross-project LoadCurrent error = %v", err)
	}
	if _, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1002", ProjectID: "project-a",
	}); !errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		t.Fatalf("cross-space LoadCurrent error = %v", err)
	}

	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(running, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(stopping, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := repository.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(cleanup, ownerHash, time.Time{}), OperationID: "cleanup-load-current-1",
	})
	if err != nil || completed.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete {
		t.Fatalf("CompleteCleanup() = %#v, %v", completed, err)
	}
	next, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("build-id-next", "build-idempotency-next", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	current, err = repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a",
	})
	if err != nil || current.Generation != next.Generation || current.Generation <= completed.Generation {
		t.Fatalf("LoadCurrent(highest unfinished) = %#v, %v", current, err)
	}
}

func TestProviderExecutionRepositoryPublishingStopCompletesArtifactBeforeCleanup(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	publishing, ownerHash, operationHash, descriptor := providerExecutionPublishingRecord(t, repository)

	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(publishing, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	objectKey := "appdev/builds/internal/stop-race.zip"
	ready, err := repository.CompleteArtifactPublish(context.Background(), domainappdev.CompleteProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(stopping, ownerHash, time.Time{}), ProviderExecutionID: stopping.ProviderExecutionID,
		OperationHash: operationHash, ObjectKey: objectKey, Digest: descriptor.Digest, Size: descriptor.Size,
	})
	if err != nil || ready.DesiredState != domainappdev.ProviderExecutionDesiredStop ||
		ready.ArtifactStatus != domainappdev.ProviderExecutionArtifactReady || ready.ArtifactObjectKey != objectKey {
		t.Fatalf("CompleteArtifactPublish(stopping) = %#v, %v", ready, err)
	}
	cleanup, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(ready, ownerHash, time.Time{}),
	})
	if err != nil || cleanup.ObservedState != domainappdev.ProviderExecutionObservedCleanupPending {
		t.Fatalf("BeginCleanup(after ready) = %#v, %v", cleanup, err)
	}
}

func TestProviderExecutionRepositoryLoadsLatestReadyArtifactAcrossCleanup(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	publishing, ownerHash, operationHash, descriptor := providerExecutionPublishingRecord(t, repository)
	objectKey := "appdev/builds/internal/historical-ready.zip"
	ready, err := repository.CompleteArtifactPublish(context.Background(), domainappdev.CompleteProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(publishing, ownerHash, time.Time{}), ProviderExecutionID: publishing.ProviderExecutionID,
		OperationHash: operationHash, ObjectKey: objectKey, Digest: descriptor.Digest, Size: descriptor.Size,
	})
	if err != nil {
		t.Fatal(err)
	}
	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(ready, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(stopping, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(cleanup, ownerHash, time.Time{}), OperationID: "cleanup-ready-artifact-1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("next-runtime", "next-runtime-idem", time.Time{})); err != nil {
		t.Fatal(err)
	}
	record, err := repository.LoadLatestReadyArtifact(context.Background(), domainappdev.LoadReadyProviderArtifactInput{SpaceID: "1001", ProjectID: "project-a"})
	if err != nil || record.ArtifactObjectKey != objectKey || record.Generation != ready.Generation {
		t.Fatalf("LoadLatestReadyArtifact() = %#v, %v", record, err)
	}
	if _, err := repository.LoadLatestReadyArtifact(context.Background(), domainappdev.LoadReadyProviderArtifactInput{SpaceID: "1002", ProjectID: "project-a"}); !errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		t.Fatalf("cross-tenant ready artifact error = %v", err)
	}
	service, err := applicationappdev.NewProviderExecutionService(repository, &providerExecutionLostResponseCodec{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := service.LoadReadyArtifact(context.Background(), applicationappdev.LoadReadyProviderArtifactRequest{SpaceID: "1001", ProjectID: "project-a"})
	if err != nil || artifact == nil || artifact.Generation != ready.Generation || artifact.Size != descriptor.Size {
		t.Fatalf("service LoadReadyArtifact() = %#v, %v", artifact, err)
	}
	formatted := fmt.Sprintf("%v|%+v|%#v", artifact, artifact, artifact)
	if strings.Contains(formatted, objectKey) || strings.Contains(formatted, descriptor.Digest) {
		t.Fatalf("ready artifact formatting leaked internals: %s", formatted)
	}
}

func TestProviderExecutionRepositoryCleanupRejectsPublishing(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	publishing, ownerHash, _, _ := providerExecutionPublishingRecord(t, repository)
	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(publishing, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(stopping, ownerHash, time.Time{})
	if _, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{OwnerCAS: cas}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("BeginCleanup(publishing) error = %v", err)
	}
	if err := db.Exec("UPDATE appdev_provider_executions SET observed_state = ? WHERE space_id = ? AND project_id = ? AND generation = ?",
		domainappdev.ProviderExecutionObservedCleanupPending, "1001", "project-a", stopping.Generation).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS: cas, OperationID: "cleanup-must-not-cross-publishing",
	}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("CompleteCleanup(publishing) error = %v", err)
	}
	var observedState, artifactStatus string
	if err := db.Raw("SELECT observed_state, artifact_status FROM appdev_provider_executions WHERE space_id = ? AND project_id = ? AND generation = ?",
		"1001", "project-a", stopping.Generation).Row().Scan(&observedState, &artifactStatus); err != nil {
		t.Fatal(err)
	}
	if observedState != string(domainappdev.ProviderExecutionObservedCleanupPending) || artifactStatus != string(domainappdev.ProviderExecutionArtifactPublishing) {
		t.Fatalf("cleanup guard mutated row: observed=%q artifact=%q", observedState, artifactStatus)
	}
}

func TestProviderExecutionRepositoryConcurrentStopCleanupCannotOrphanPublishingArtifact(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	publishing, ownerHash, operationHash, descriptor := providerExecutionPublishingRecord(t, repository)
	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(publishing, ownerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(stopping, ownerHash, time.Time{})
	objectKey := "appdev/builds/internal/concurrent-stop.zip"
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	completeErr := make(chan error, 1)
	cleanupErr := make(chan error, 1)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		ready <- struct{}{}
		<-start
		_, callErr := repository.CompleteArtifactPublish(context.Background(), domainappdev.CompleteProviderArtifactPublishInput{
			OwnerCAS: cas, ProviderExecutionID: stopping.ProviderExecutionID, OperationHash: operationHash,
			ObjectKey: objectKey, Digest: descriptor.Digest, Size: descriptor.Size,
		})
		completeErr <- callErr
	}()
	go func() {
		defer wait.Done()
		ready <- struct{}{}
		<-start
		_, callErr := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{OwnerCAS: cas})
		cleanupErr <- callErr
	}()
	<-ready
	<-ready
	close(start)
	wait.Wait()
	if err := <-completeErr; err != nil {
		t.Fatalf("CompleteArtifactPublish(concurrent) error = %v", err)
	}
	if err := <-cleanupErr; err == nil {
		t.Fatal("BeginCleanup(concurrent) unexpectedly crossed publishing/complete CAS")
	}
	current, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a",
	})
	if err != nil || current.ArtifactStatus != domainappdev.ProviderExecutionArtifactReady || current.ArtifactObjectKey != objectKey {
		t.Fatalf("current artifact after race = %#v, %v", current, err)
	}
	cleanup, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(current, ownerHash, time.Time{}),
	})
	if err != nil || cleanup.ObservedState != domainappdev.ProviderExecutionObservedCleanupPending {
		t.Fatalf("cleanup retry after artifact commit = %#v, %v", cleanup, err)
	}
}

func providerExecutionPublishingRecord(
	t *testing.T,
	repository *ProviderExecutionRepository,
) (*domainappdev.ProviderExecution, domainappdev.ProviderExecutionOwnerHash, domainappdev.ProviderExecutionOperationHash, domainappdev.ProviderBuildArtifactDescriptor) {
	t.Helper()
	record, ownerHash := providerExecutionBuildRunning(t, repository)
	reserve := providerExecutionReserveBuildInput(t, record, ownerHash, "build-publishing-stop-1")
	record, err := repository.ReserveBuild(context.Background(), reserve)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := domainappdev.ProviderBuildArtifactDescriptor{
		Kind:   domainappdev.ProviderExecutionArtifactKindAppDevBuildArchive,
		Digest: "sha256:" + strings.Repeat("a", 64), Size: 4096,
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
	return record, ownerHash, reserve.OperationHash, descriptor
}
