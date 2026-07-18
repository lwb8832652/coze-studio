// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"errors"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
)

var (
	ErrCodePluginInvalidRequest = errors.New("code plugin invalid request")
	ErrCodePluginPermission     = errors.New("code plugin operation is forbidden")
	ErrCodePluginValidation     = errors.New("code plugin validation failed")
	ErrCodePluginConflict       = errors.New("code plugin conflict")
	ErrCodePluginUnavailable    = errors.New("code plugin execution service unavailable")
)

func codePluginInvalid(err error) error {
	return fmt.Errorf("%w: %w", ErrCodePluginInvalidRequest, err)
}

func codePluginPermission(err error) error {
	return fmt.Errorf("%w: %w", ErrCodePluginPermission, err)
}

func codePluginValidation(err error) error {
	return fmt.Errorf("%w: %w", ErrCodePluginValidation, err)
}

func codePluginConflict(err error) error {
	return fmt.Errorf("%w: %w", ErrCodePluginConflict, err)
}

func codePluginUnavailable(err error) error {
	return fmt.Errorf("%w: %w", ErrCodePluginUnavailable, err)
}

func classifyCodePluginError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCodePluginInvalidRequest) ||
		errors.Is(err, ErrCodePluginPermission) ||
		errors.Is(err, ErrCodePluginValidation) ||
		errors.Is(err, ErrCodePluginConflict) ||
		errors.Is(err, ErrCodePluginUnavailable) {
		return err
	}
	switch {
	case errors.Is(err, repository.ErrCodeDraftConflict),
		errors.Is(err, repository.ErrCodeDraftNotDebugged),
		errors.Is(err, repository.ErrCodeVersionExists):
		return codePluginConflict(err)
	case errors.Is(err, repository.ErrCodeDraftNotFound),
		errors.Is(err, repository.ErrCodeMainVersionMissing),
		errors.Is(err, repository.ErrCodeSpaceMismatch):
		return codePluginInvalid(err)
	case errors.Is(err, coderunner.ErrCodeRunnerUnavailable):
		return codePluginUnavailable(err)
	}
	var validationErr *codePluginValidationError
	if errors.As(err, &validationErr) {
		return codePluginValidation(err)
	}
	return err
}

func classifyCodePluginRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCodePluginInvalidRequest) ||
		errors.Is(err, ErrCodePluginPermission) ||
		errors.Is(err, ErrCodePluginValidation) ||
		errors.Is(err, ErrCodePluginConflict) ||
		errors.Is(err, ErrCodePluginUnavailable) {
		return err
	}
	switch {
	case errors.Is(err, repository.ErrCodeDraftConflict),
		errors.Is(err, repository.ErrCodeDraftNotDebugged),
		errors.Is(err, repository.ErrCodeVersionExists):
		return codePluginConflict(err)
	case errors.Is(err, repository.ErrCodeDraftNotFound):
		return codePluginInvalid(err)
	case errors.Is(err, repository.ErrCodeMainVersionMissing),
		errors.Is(err, repository.ErrCodeSpaceMismatch):
		return codePluginUnavailable(err)
	default:
		return codePluginUnavailable(err)
	}
}
