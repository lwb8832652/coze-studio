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
	"errors"
	"testing"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type providerBuildRecoverNoCallRouter struct{}

func (providerBuildRecoverNoCallRouter) Resume(context.Context, applicationsandbox.ExecutionCheckpoint, string) (applicationappdev.ProviderRuntimeStatusSelection, error) {
	return nil, errors.New("unexpected provider resume")
}

type providerBuildRecoverNoCallPublisher struct{}

func (providerBuildRecoverNoCallPublisher) PublishWithPublisher(context.Context, infrasandbox.ArtifactPublisher, applicationappdev.PublishBuildArtifactCommand) (*applicationappdev.BuildArtifactReceipt, error) {
	return nil, errors.New("unexpected artifact publish")
}

type providerBuildRecoverOwnerGenerator struct {
	token domainappdev.ProviderExecutionOwnerToken
}

func (generator providerBuildRecoverOwnerGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	return generator.token, nil
}

func TestProviderBuildOrchestratorRecoverNoRecordUsesRealLedgerContract(t *testing.T) {
	orchestrator, repository := newProviderBuildRecoverOrchestrator(t)
	ctx := context.Background()

	projection, err := orchestrator.RecoverBuild(ctx, applicationappdev.ProviderBuildRecoverInput{SpaceID: "1001", ProjectID: "project-a"})
	if err != nil || projection == nil || projection.State != applicationappdev.ProviderBuildStateIdle {
		t.Fatalf("RecoverBuild(no record) = %#v, %v", projection, err)
	}

	if projection, err := orchestrator.BeginBuild(ctx, applicationappdev.ProviderBuildBeginInput{
		SpaceID: "1001", ProjectID: "project-a", OperationID: "build-no-record-1",
	}); projection != nil || !errors.Is(err, applicationappdev.ErrProviderBuildUnavailable) {
		t.Fatalf("BeginBuild(no record) = %#v, %v", projection, err)
	}
	if projection, err := orchestrator.PollBuild(ctx, applicationappdev.ProviderBuildPollInput{
		SpaceID: "1001", ProjectID: "project-a", OperationID: "build-no-record-1",
	}); projection != nil || !errors.Is(err, applicationappdev.ErrProviderBuildUnavailable) {
		t.Fatalf("PollBuild(no record) = %#v, %v", projection, err)
	}
	if projection, err := orchestrator.RecoverBuild(ctx, applicationappdev.ProviderBuildRecoverInput{
		SpaceID: "1001suffix", ProjectID: "project-a",
	}); projection != nil || !errors.Is(err, applicationappdev.ErrProviderBuildUnavailable) {
		t.Fatalf("RecoverBuild(invalid scope) = %#v, %v", projection, err)
	}

	sqlDB, err := repository.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if projection, err := orchestrator.RecoverBuild(ctx, applicationappdev.ProviderBuildRecoverInput{
		SpaceID: "1001", ProjectID: "project-a",
	}); projection != nil || !errors.Is(err, applicationappdev.ErrProviderBuildUnavailable) {
		t.Fatalf("RecoverBuild(DB failure) = %#v, %v", projection, err)
	}
}

func newProviderBuildRecoverOrchestrator(t *testing.T) (*applicationappdev.ProviderBuildOrchestrator, *ProviderExecutionRepository) {
	t.Helper()
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	service, err := applicationappdev.NewProviderExecutionService(repository, &providerExecutionLostResponseCodec{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := domainappdev.NewProviderExecutionOwnerToken(bytes.NewReader(bytes.Repeat([]byte{0x6a}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	orchestrator, err := applicationappdev.NewProviderBuildOrchestrator(
		service,
		providerBuildRecoverNoCallRouter{},
		providerBuildRecoverNoCallPublisher{},
		providerBuildRecoverOwnerGenerator{token: owner},
		applicationappdev.ProviderBuildOrchestratorConfig{ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev},
	)
	if err != nil {
		t.Fatal(err)
	}
	return orchestrator, repository
}
