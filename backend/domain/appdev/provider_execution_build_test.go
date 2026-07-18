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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func providerExecutionBuildOwnerHash(value string) ProviderExecutionOwnerHash {
	return ProviderExecutionOwnerHash(sha256.Sum256([]byte(value)))
}

func providerExecutionBuildTestRecord(t *testing.T) (*ProviderExecution, ProviderExecutionOwnerCAS) {
	t.Helper()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	owner := providerExecutionBuildOwnerHash("build-owner")
	expires := now.Add(time.Hour)
	submitted := now.Add(-time.Minute)
	record := &ProviderExecution{
		ID: "apx_build", SpaceID: "1001", ProjectID: "project-a", Generation: 7,
		IdempotencyKey: "start-operation", DesiredState: ProviderExecutionDesiredRun,
		ObservedState: ProviderExecutionObservedRunning, ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
		ProviderExecutionID: "provider-execution-1", SubmissionStartedAt: &submitted,
		OwnerIdentityHash: owner, OwnerEpoch: 3, OwnerExpiresAt: &expires,
		Version: 9, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	cas := ProviderExecutionOwnerCAS{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation, ExpectedVersion: record.Version,
		OwnerHash: owner, OwnerEpoch: record.OwnerEpoch, ProviderKey: record.ProviderKey, ProviderScope: record.ProviderScope,
	}
	return record, cas
}

func TestProviderExecutionBuildOperationHashBindsExecutionIdentity(t *testing.T) {
	_, cas := providerExecutionBuildTestRecord(t)
	base, err := HashProviderExecutionBuildOperationID("build-operation", cas, "provider-execution-1")
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		cas        ProviderExecutionOwnerCAS
		providerID string
		operation  string
	}{
		{cas: func() ProviderExecutionOwnerCAS { value := cas; value.SpaceID = "1002"; return value }(), providerID: "provider-execution-1", operation: "build-operation"},
		{cas: func() ProviderExecutionOwnerCAS { value := cas; value.ProjectID = "project-b"; return value }(), providerID: "provider-execution-1", operation: "build-operation"},
		{cas: func() ProviderExecutionOwnerCAS { value := cas; value.Generation++; return value }(), providerID: "provider-execution-1", operation: "build-operation"},
		{cas: func() ProviderExecutionOwnerCAS { value := cas; value.ProviderKey = "provider-b"; return value }(), providerID: "provider-execution-1", operation: "build-operation"},
		{cas: cas, providerID: "provider-execution-2", operation: "build-operation"},
		{cas: cas, providerID: "provider-execution-1", operation: "other-operation"},
	}
	for index, mutation := range mutations {
		hash, hashErr := HashProviderExecutionBuildOperationID(mutation.operation, mutation.cas, mutation.providerID)
		if hashErr != nil {
			t.Fatalf("mutation %d: %v", index, hashErr)
		}
		if base.Equal(hash) {
			t.Fatalf("identity mutation %d did not change hash", index)
		}
	}
	newOwner := cas
	newOwner.OwnerHash = providerExecutionBuildOwnerHash("replacement-owner")
	newOwner.OwnerEpoch++
	afterTakeover, err := HashProviderExecutionBuildOperationID("build-operation", newOwner, "provider-execution-1")
	if err != nil || !base.Equal(afterTakeover) {
		t.Fatalf("owner takeover changed stable build hash: %v", err)
	}
}

func TestProviderExecutionBuildStateCrossFieldValidationAndRedaction(t *testing.T) {
	record, cas := providerExecutionBuildTestRecord(t)
	hash, err := HashProviderExecutionBuildOperationID("build-operation", cas, record.ProviderExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	now := record.UpdatedAt
	record.BuildOperationID = "build-operation"
	record.BuildOperationHash = hash
	record.ArtifactStatus = ProviderExecutionArtifactBeginPending
	record.ArtifactVersion = 1
	record.BuildStartedAt = &now
	record.ArtifactUpdatedAt = &now
	if _, err := HydrateProviderExecution(record); err != nil {
		t.Fatalf("begin-pending record rejected: %v", err)
	}

	record.ArtifactStatus = ProviderExecutionArtifactBuilding
	if _, err := HydrateProviderExecution(record); err != nil {
		t.Fatalf("building record rejected: %v", err)
	}

	record.ArtifactStatus = ProviderExecutionArtifactDescriptorReady
	record.ArtifactKind = ProviderExecutionArtifactKindAppDevBuildArchive
	record.ArtifactDigest = "sha256:" + strings.Repeat("b", 64)
	record.ArtifactSize = 4096
	if _, err := HydrateProviderExecution(record); err != nil {
		t.Fatalf("descriptor record rejected: %v", err)
	}
	record.ArtifactStatus = ProviderExecutionArtifactPublishing
	if _, err := HydrateProviderExecution(record); err != nil {
		t.Fatalf("publishing record rejected: %v", err)
	}
	record.ArtifactStatus = ProviderExecutionArtifactReady
	record.ArtifactObjectKey = "appdev/builds/internal/archive.zip"
	if _, err := HydrateProviderExecution(record); err != nil {
		t.Fatalf("ready record rejected: %v", err)
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	formatted := string(encoded) + fmt.Sprintf("%v|%+v|%#v", record, record, record)
	for _, secret := range []string{record.BuildOperationID, record.ArtifactObjectKey} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("record formatting leaked %q: %s", secret, formatted)
		}
	}

	invalid := *record
	invalid.ArtifactObjectKey = ""
	if _, err := HydrateProviderExecution(&invalid); err == nil {
		t.Fatal("ready build without internal object key accepted")
	}
	invalid = *record
	invalid.ArtifactStatus = ProviderExecutionArtifactBuilding
	if _, err := HydrateProviderExecution(&invalid); err == nil {
		t.Fatal("building state retained ready descriptor/object")
	}
	invalid = *record
	invalid.ArtifactStatus = ProviderExecutionArtifactDescriptorReady
	invalid.ArtifactDigest = "sha256:bad"
	invalid.ArtifactObjectKey = ""
	if _, err := HydrateProviderExecution(&invalid); err == nil {
		t.Fatal("invalid descriptor accepted")
	}
}
