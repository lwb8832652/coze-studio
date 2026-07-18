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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestProviderExecutionActiveObservationCASRejectsStaleAndTerminalRegression(t *testing.T) {
	const (
		spaceID   = int64(9101)
		projectID = "observation-project"
		provider  = "remote-default"
	)
	repository, _ := newProviderExecutionSQLiteRepository(t, spaceID, projectID)
	ctx := context.Background()
	record, err := repository.EnsureStart(ctx, domainappdev.EnsureProviderExecutionStartInput{
		ID: "apx_" + strings.Repeat("a", 32), SpaceID: "9101", ProjectID: projectID,
		IdempotencyKey: "observation-start", ProviderKey: provider, ProviderScope: domainsandbox.ScopeAppDev,
	})
	require.NoError(t, err)
	token, err := domainappdev.NewProviderExecutionOwnerToken(strings.NewReader(strings.Repeat("o", 32)))
	require.NoError(t, err)
	claimed, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: "9101", ProjectID: projectID, Generation: record.Generation, ExpectedVersion: record.Version,
		OwnerHash: token.OwnerIdentityHash(), LeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	ownerCAS := func(version uint64) domainappdev.ProviderExecutionOwnerCAS {
		return domainappdev.ProviderExecutionOwnerCAS{
			SpaceID: "9101", ProjectID: projectID, Generation: claimed.Generation, ExpectedVersion: version,
			OwnerHash: token.OwnerIdentityHash(), OwnerEpoch: claimed.OwnerEpoch,
			ProviderKey: provider, ProviderScope: domainsandbox.ScopeAppDev,
		}
	}
	submittingCAS := ownerCAS(claimed.Version)
	submitting, err := repository.StartSubmission(ctx, providerExecutionStartSubmissionInput(t, submittingCAS, "observation-fixture-launch"))
	require.NoError(t, err)
	submitting = markProviderExecutionSubmittedForTest(
		t, repository, submitting, token.OwnerIdentityHash(), "observation-fixture-launch",
	)
	running, err := repository.SaveSubmission(ctx, providerExecutionSaveSubmissionInput(
		t, ownerCAS(submitting.Version), "observation-fixture-launch",
		"provider-execution-real", time.Minute, "/preview/initial",
	))
	require.NoError(t, err)
	running = completeProviderExecutionLaunchForTest(t, repository, running, token.OwnerIdentityHash(), "observation-fixture-launch")

	observed, err := repository.ObserveActive(ctx, domainappdev.ObserveProviderExecutionActiveInput{
		OwnerCAS: ownerCAS(running.Version), ProviderExecutionID: "provider-execution-real",
		ProviderLeaseDuration: time.Minute, PreviewRoute: "/preview/current",
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionObservedRunning, observed.ObservedState)
	require.Equal(t, "/preview/current", observed.PreviewRoute)
	require.Equal(t, running.Version+1, observed.Version)

	_, err = repository.ObserveActive(ctx, domainappdev.ObserveProviderExecutionActiveInput{
		OwnerCAS: ownerCAS(running.Version), ProviderExecutionID: "provider-execution-real",
		ProviderLeaseDuration: time.Minute, PreviewRoute: "/preview/stale",
	})
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionVersionConflict)
	current, err := repository.loadByGeneration(ctx, spaceID, projectID, running.Generation)
	require.NoError(t, err)
	require.Equal(t, "/preview/current", current.PreviewRoute)

	operationHash, err := domainappdev.HashProviderExecutionOperationID("observation-terminal")
	require.NoError(t, err)
	terminal, err := repository.AdvanceTerminal(ctx, domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS: ownerCAS(observed.Version), OperationHash: operationHash,
		ObservedState: domainappdev.ProviderExecutionObservedSucceeded, PreviewRoute: "/preview/final",
	})
	require.NoError(t, err)
	_, err = repository.ObserveActive(ctx, domainappdev.ObserveProviderExecutionActiveInput{
		OwnerCAS: ownerCAS(terminal.Version), ProviderExecutionID: "provider-execution-real",
		ProviderLeaseDuration: time.Minute, PreviewRoute: "/preview/regressed",
	})
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionStateConflict)
	after, err := repository.loadByGeneration(ctx, spaceID, projectID, running.Generation)
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionObservedSucceeded, after.ObservedState)
	require.Equal(t, "/preview/final", after.PreviewRoute)
}
