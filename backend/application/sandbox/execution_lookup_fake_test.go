// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"

	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	testExecutionLookupFoundOperation    = "test_execution_lookup_found"
	testExecutionLookupNotFoundOperation = "test_execution_lookup_not_found"
)

func legacyRuntimeFakeLookup(
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	if _, err := infrasandbox.NormalizeExecutionLookupRequest(input); err != nil {
		return infrasandbox.ExecutionLookupResult{
			Status: infrasandbox.ExecutionLookupUnknown,
		}, err
	}
	switch input.OperationID {
	case testExecutionLookupFoundOperation:
		return infrasandbox.ExecutionLookupResult{
			Status: infrasandbox.ExecutionLookupFound,
			Execution: infrasandbox.ExecuteResult{
				ExecutionID: "test-execution-lookup-found",
				Status:      infrasandbox.ExecutionStatusAccepted,
			},
		}, nil
	case testExecutionLookupNotFoundOperation:
		return infrasandbox.ExecutionLookupResult{
			Status: infrasandbox.ExecutionLookupNotFound,
		}, nil
	default:
		return infrasandbox.ExecutionLookupResult{
			Status: infrasandbox.ExecutionLookupUnknown,
		}, nil
	}
}

func (*task9AsyncRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*qualityLifecycleRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*qualitySyncRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*qualitySyncContractRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*qualityDeadlineRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*latestIgnoringRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*latestCancelRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*latestReconcileRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*latestBuildCloseRuntime) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*latestAsyncWithoutReconcile) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*blockingRuntimeProvider) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}

func (*runtimeProviderStub) LookupExecution(
	_ context.Context,
	input infrasandbox.ExecutionLookupRequest,
) (infrasandbox.ExecutionLookupResult, error) {
	return legacyRuntimeFakeLookup(input)
}
