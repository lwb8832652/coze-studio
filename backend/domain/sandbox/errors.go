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

package sandbox

import "errors"

const (
	ErrCodeInvalidInput          = "SANDBOX_INVALID_INPUT"
	ErrCodeProviderNotFound      = "SANDBOX_PROVIDER_NOT_FOUND"
	ErrCodeProviderDisabled      = "SANDBOX_PROVIDER_DISABLED"
	ErrCodeDefaultMissing        = "SANDBOX_DEFAULT_MISSING"
	ErrCodeProviderUnhealthy     = "SANDBOX_PROVIDER_UNHEALTHY"
	ErrCodeCredentialInvalid     = "SANDBOX_CREDENTIAL_INVALID"
	ErrCodeCapacityExhausted     = "SANDBOX_CAPACITY_EXHAUSTED"
	ErrCodeExecutionForbidden    = "SANDBOX_EXECUTION_FORBIDDEN"
	ErrCodeVersionConflict       = "SANDBOX_VERSION_CONFLICT"
	ErrCodeProviderAlreadyExists = "SANDBOX_PROVIDER_ALREADY_EXISTS"
	ErrCodeProviderInUse         = "SANDBOX_PROVIDER_IN_USE"
	ErrCodeScopeUnsupported      = "SANDBOX_SCOPE_UNSUPPORTED"
	ErrCodeConfigurationInvalid  = "SANDBOX_CONFIGURATION_INVALID"
	ErrCodeLocalDebugUnavailable = "SANDBOX_LOCAL_DEBUG_UNAVAILABLE"
	ErrCodeUnavailable           = "SANDBOX_UNAVAILABLE"
)

var (
	ErrInvalidInput          = newCodedError(ErrCodeInvalidInput, "sandbox input is invalid")
	ErrProviderNotFound      = newCodedError(ErrCodeProviderNotFound, "sandbox provider was not found")
	ErrProviderDisabled      = newCodedError(ErrCodeProviderDisabled, "sandbox provider is disabled")
	ErrDefaultMissing        = newCodedError(ErrCodeDefaultMissing, "sandbox default provider is missing")
	ErrProviderUnhealthy     = newCodedError(ErrCodeProviderUnhealthy, "sandbox provider is unhealthy")
	ErrCredentialInvalid     = newCodedError(ErrCodeCredentialInvalid, "sandbox credential is invalid")
	ErrCapacityExhausted     = newCodedError(ErrCodeCapacityExhausted, "sandbox capacity is exhausted")
	ErrExecutionForbidden    = newCodedError(ErrCodeExecutionForbidden, "sandbox execution is forbidden")
	ErrVersionConflict       = newCodedError(ErrCodeVersionConflict, "sandbox version conflict")
	ErrProviderAlreadyExists = newCodedError(ErrCodeProviderAlreadyExists, "sandbox provider already exists")
	ErrProviderInUse         = newCodedError(ErrCodeProviderInUse, "sandbox provider is in use")
	ErrScopeUnsupported      = newCodedError(ErrCodeScopeUnsupported, "sandbox provider does not support the requested scope")
	ErrConfigurationInvalid  = newCodedError(ErrCodeConfigurationInvalid, "sandbox configuration is invalid")
	ErrLocalDebugUnavailable = newCodedError(ErrCodeLocalDebugUnavailable, "local debug sandbox is unavailable")
	ErrUnavailable           = newCodedError(ErrCodeUnavailable, "sandbox is unavailable")
	ErrHealthMonitorDBClock  = newCodedError(ErrCodeUnavailable, "sandbox health monitor database clock is unavailable")
	ErrHealthMonitorOutbox   = newCodedError(ErrCodeUnavailable, "sandbox health notification projection is unavailable")
)

type codedError struct {
	code    string
	message string
}

func newCodedError(code, message string) *codedError {
	return &codedError{code: code, message: message}
}

func (e *codedError) Error() string {
	return e.message
}

func (e *codedError) Code() string {
	return e.code
}

func ErrorCodeOf(err error) string {
	if err == nil {
		return ""
	}

	var coded *codedError
	if !errors.As(err, &coded) {
		return ""
	}
	switch coded {
	case ErrInvalidInput,
		ErrProviderNotFound,
		ErrProviderDisabled,
		ErrDefaultMissing,
		ErrProviderUnhealthy,
		ErrCredentialInvalid,
		ErrCapacityExhausted,
		ErrExecutionForbidden,
		ErrVersionConflict,
		ErrProviderAlreadyExists,
		ErrProviderInUse,
		ErrScopeUnsupported,
		ErrConfigurationInvalid,
		ErrLocalDebugUnavailable,
		ErrUnavailable,
		ErrHealthMonitorDBClock,
		ErrHealthMonitorOutbox:
		return coded.code
	default:
		return ""
	}
}
