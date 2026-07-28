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

package storageconfig

import "errors"

var (
	ErrConfigInvalid         = errors.New("object storage config invalid")
	ErrNotFound              = errors.New("object storage config not found")
	ErrVersionConflict       = errors.New("object storage version conflict")
	ErrConnectionFailed      = errors.New("object storage connection failed")
	ErrActiveDeleteForbidden = errors.New("object storage active delete forbidden")
	ErrCredentialUnavailable = errors.New("object storage credential unavailable")
	ErrProviderUnsupported   = errors.New("object storage provider unsupported")
	ErrPrimaryConfigMissing  = errors.New("object storage primary config missing")
	ErrMigrationConfirmation = errors.New("object storage migration confirmation required")
)

func ErrorCodeOf(err error) string {
	switch {
	case errors.Is(err, ErrConfigInvalid):
		return "OBJECT_STORAGE_CONFIG_INVALID"
	case errors.Is(err, ErrNotFound):
		return "OBJECT_STORAGE_NOT_FOUND"
	case errors.Is(err, ErrVersionConflict):
		return "OBJECT_STORAGE_VERSION_CONFLICT"
	case errors.Is(err, ErrConnectionFailed):
		return "OBJECT_STORAGE_CONNECTION_FAILED"
	case errors.Is(err, ErrActiveDeleteForbidden):
		return "OBJECT_STORAGE_ACTIVE_DELETE_FORBIDDEN"
	case errors.Is(err, ErrCredentialUnavailable):
		return "OBJECT_STORAGE_CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, ErrProviderUnsupported):
		return "OBJECT_STORAGE_PROVIDER_UNSUPPORTED"
	case errors.Is(err, ErrPrimaryConfigMissing):
		return "OBJECT_STORAGE_PRIMARY_CONFIG_MISSING"
	case errors.Is(err, ErrMigrationConfirmation):
		return "OBJECT_STORAGE_MIGRATION_CONFIRMATION_REQUIRED"
	default:
		return "OBJECT_STORAGE_INTERNAL"
	}
}
