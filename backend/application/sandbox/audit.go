// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"sort"
	"strconv"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	auditActionCreate      = "provider.create"
	auditActionUpdate      = "provider.update"
	auditActionEnable      = "provider.enable"
	auditActionDisable     = "provider.disable"
	auditActionSetDefault  = "provider.set_default"
	auditActionDelete      = "provider.delete"
	auditActionHealthCheck = "provider.health_check"

	auditResultSuccess = "success"
	auditResultFailure = "failure"

	auditChangedFieldName       = "name"
	auditChangedFieldScope      = "scopes"
	auditChangedFieldPolicy     = "policy"
	auditChangedFieldEndpoint   = "endpoint"
	auditChangedFieldCredential = "credential"
)

var controlPlaneAuditMetadataAllowlist = map[string]struct{}{
	domainsandbox.AuditMetadataKeyScope:          {},
	domainsandbox.AuditMetadataKeyChangedFields:  {},
	domainsandbox.AuditMetadataKeyPreviousStatus: {},
	domainsandbox.AuditMetadataKeyNewStatus:      {},
	domainsandbox.AuditMetadataKeyHealthCode:     {},
	domainsandbox.AuditMetadataKeyVersion:        {},
}

func appendControlPlaneAudit(ctx context.Context, repository domainsandbox.ProviderAuditRepository, actor Actor,
	providerID int64, action, result string, metadata map[string]string) error {
	if repository == nil {
		return domainsandbox.ErrUnavailable
	}
	normalizedMetadata, err := domainsandbox.NormalizeAuditMetadata(metadata)
	if err != nil {
		return stableControlPlaneError(err)
	}
	input, err := domainsandbox.NormalizeAppendProviderAuditEventInput(domainsandbox.AppendProviderAuditEventInput{
		ProviderID: providerID, ActorUserID: actor.UserID, Action: action, Result: result,
		RequestID: strings.TrimSpace(actor.RequestID), Metadata: normalizedMetadata,
	})
	if err != nil {
		return stableControlPlaneError(err)
	}
	if _, err = repository.AppendProviderAuditEvent(ctx, input); err != nil {
		return stableControlPlaneError(err)
	}
	return nil
}

func changedFieldsAuditMetadata(fields []string, version uint64) (map[string]string, error) {
	ordered := append([]string(nil), fields...)
	sort.Strings(ordered)
	metadata := map[string]string{
		domainsandbox.AuditMetadataKeyChangedFields: strings.Join(ordered, ","),
		domainsandbox.AuditMetadataKeyVersion:       strconv.FormatUint(version, 10),
	}
	return domainsandbox.NormalizeAuditMetadata(metadata)
}

func (s *Service) ListAuditEvents(ctx context.Context, actor Actor, request ListAuditEventsRequest) (*ListAuditEventsResult, error) {
	if err := validateControlPlaneActor(actor, false); err != nil {
		return nil, err
	}
	if request.ProviderID <= 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeProviderAuditListRequest(domainsandbox.ProviderAuditListRequest{
		ProviderID: request.ProviderID, Action: request.Action, Result: request.Result,
		Offset: request.Offset, Limit: request.Limit,
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	var events []*domainsandbox.ProviderAuditEvent
	var total int64
	err = s.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		provider, getErr := repositories.Providers.GetProvider(txCtx, normalized.ProviderID)
		if getErr != nil {
			return stableControlPlaneError(getErr)
		}
		if authErr := authorizeControlPlaneScopes(actor, provider.Scopes); authErr != nil {
			return authErr
		}
		var listErr error
		events, total, listErr = repositories.Audits.ListProviderAuditEvents(txCtx, normalized)
		return stableControlPlaneError(listErr)
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	items := make([]AuditEventDTO, 0, len(events))
	for _, event := range events {
		if event == nil {
			return nil, domainsandbox.ErrUnavailable
		}
		items = append(items, projectControlPlaneAudit(event))
	}
	return &ListAuditEventsResult{Items: items, Total: total}, nil
}

func projectControlPlaneAudit(event *domainsandbox.ProviderAuditEvent) AuditEventDTO {
	metadata := make(map[string]string)
	for key, value := range event.Metadata {
		if _, allowed := controlPlaneAuditMetadataAllowlist[key]; !allowed {
			continue
		}
		candidate, err := domainsandbox.NormalizeAuditMetadata(map[string]string{key: value})
		if err == nil {
			metadata[key] = candidate[key]
		}
	}
	return AuditEventDTO{
		ID: event.ID, ProviderID: event.ProviderID, ActorUserID: event.ActorUserID,
		Action:    sanitizeControlPlaneText(event.Action, domainsandbox.MaxAuditActionLength),
		Result:    sanitizeControlPlaneText(event.Result, domainsandbox.MaxAuditResultLength),
		RequestID: sanitizeControlPlaneText(event.RequestID, maxControlPlaneRequestIDBytes),
		Metadata:  metadata, CreatedAt: event.CreatedAt,
	}
}
