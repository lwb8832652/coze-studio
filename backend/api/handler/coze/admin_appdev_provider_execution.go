// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type adminProviderQuarantineDispositionRequest struct {
	ExpectedVersion uint64                                                  `json:"expected_version"`
	ExpectedState   domainappdev.ProviderExecutionLaunchState               `json:"expected_state"`
	IdempotencyKey  string                                                  `json:"idempotency_key"`
	Acknowledgement domainappdev.ProviderExecutionQuarantineAcknowledgement `json:"acknowledgement"`
	Reason          string                                                  `json:"reason"`
	EvidenceHash    string                                                  `json:"evidence_hash"`
}

func DisposeAdminAppDevProviderExecutionQuarantine(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	control := appdev.CurrentProviderQuarantineDispositionControl()
	if control == nil {
		adminProviderExecutionError(c, domainappdev.ErrProviderExecutionUnavailable)
		return
	}
	spaceID := c.Param("space_id")
	projectID := c.Param("project_id")
	generation, err := parseAdminProviderExecutionUint(c.Param("generation"))
	if err != nil || !domainappdev.ValidProviderExecutionSpaceID(spaceID) ||
		!domainappdev.ValidProviderExecutionProjectID(projectID) {
		adminProviderExecutionError(c, domainappdev.ErrProviderExecutionInvalid)
		return
	}
	var request adminProviderQuarantineDispositionRequest
	if err = decodeAdminSandboxJSON(c, &request); err != nil {
		adminProviderExecutionError(c, domainappdev.ErrProviderExecutionInvalid)
		return
	}
	if request.ExpectedVersion == 0 ||
		request.ExpectedState != domainappdev.ProviderExecutionLaunchQuarantined ||
		!domainappdev.ValidProviderExecutionQuarantineDisposition(request.Acknowledgement, request.Reason) ||
		!validAdminProviderExecutionOperationID(request.IdempotencyKey) ||
		!validAdminProviderExecutionEvidenceHash(request.EvidenceHash) {
		adminProviderExecutionError(c, domainappdev.ErrProviderExecutionInvalid)
		return
	}
	result, err := control.Dispose(ctx, appdev.ProviderQuarantineDispositionActor{
		UserID: actor.UserID, SystemAdmin: actor.SystemAdmin,
	}, appdev.ProviderQuarantineDispositionRequest{
		SpaceID: spaceID, ProjectID: projectID, Generation: generation,
		ExpectedVersion: request.ExpectedVersion, ExpectedState: request.ExpectedState,
		OperationID: request.IdempotencyKey, Acknowledgement: request.Acknowledgement,
		Reason: request.Reason, EvidenceHash: request.EvidenceHash,
	})
	if err != nil {
		adminProviderExecutionError(c, err)
		return
	}
	c.JSON(http.StatusOK, adminSandboxEnvelope{Code: 0, Msg: "success", Data: result})
}

func validAdminProviderExecutionOperationID(value string) bool {
	_, err := domainappdev.HashProviderExecutionOperationID(value)
	return err == nil
}

func validAdminProviderExecutionEvidenceHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func RejectAdminAppDevProviderExecutionTrailingSlash(_ context.Context, c *app.RequestContext) {
	c.AbortWithStatusJSON(http.StatusNotFound, adminSandboxEnvelope{
		Code: http.StatusNotFound, ErrorCode: "NOT_FOUND", Msg: "not found",
	})
}

func parseAdminProviderExecutionUint(raw string) (uint64, error) {
	if raw == "" || raw[0] == '0' || strings.TrimSpace(raw) != raw {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 || strconv.FormatUint(value, 10) != raw {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	return value, nil
}

func adminProviderExecutionError(c *app.RequestContext, err error) {
	status := http.StatusInternalServerError
	code := "PROVIDER_EXECUTION_INTERNAL"
	message := "internal server error"
	switch {
	case errors.Is(err, appdev.ErrProviderQuarantineDispositionPermissionDenied):
		status, code, message = http.StatusForbidden, "PROVIDER_EXECUTION_FORBIDDEN", "operation is not permitted"
	case errors.Is(err, domainappdev.ErrProviderExecutionInvalid):
		status, code, message = http.StatusBadRequest, "PROVIDER_EXECUTION_INVALID", "invalid request"
	case errors.Is(err, domainappdev.ErrProviderExecutionNotFound):
		status, code, message = http.StatusNotFound, "PROVIDER_EXECUTION_NOT_FOUND", "provider execution was not found"
	case errors.Is(err, domainappdev.ErrProviderExecutionConflict):
		status, code, message = http.StatusConflict, "PROVIDER_EXECUTION_CONFLICT", "provider execution changed"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, domainappdev.ErrProviderExecutionUnavailable):
		status, code, message = http.StatusServiceUnavailable, "PROVIDER_EXECUTION_UNAVAILABLE", "provider execution control is unavailable"
	}
	c.AbortWithStatusJSON(status, adminSandboxEnvelope{
		Code: status, ErrorCode: code, Msg: message,
	})
}
