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

package agentthread

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPGuardrailTimeout    = 10 * time.Second
	maxHTTPGuardrailResponseBytes  = int64(1024 * 1024)
	guardrailHTTPScanRequestSchema = "coze.guardrail_scan_request.v1"
)

type HTTPGuardrailProviderOptions struct {
	Endpoint string
	Token    string
	Timeout  time.Duration
	Client   *http.Client
}

type httpGuardrailProvider struct {
	endpoint string
	token    string
	client   *http.Client
}

type httpGuardrailScanRequest struct {
	Schema     string            `json:"schema"`
	SpaceID    int64             `json:"space_id,omitempty"`
	ThreadID   int64             `json:"thread_id,omitempty"`
	RunID      int64             `json:"run_id,omitempty"`
	UserID     int64             `json:"user_id,omitempty"`
	TargetType string            `json:"target_type,omitempty"`
	TargetID   string            `json:"target_id,omitempty"`
	Operation  string            `json:"operation,omitempty"`
	Source     string            `json:"source,omitempty"`
	FailMode   string            `json:"fail_mode,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type httpGuardrailScanResponse struct {
	Action     string            `json:"action"`
	Provider   string            `json:"provider,omitempty"`
	ReasonCode string            `json:"reason_code,omitempty"`
	Message    string            `json:"message,omitempty"`
	RuleIDs    []string          `json:"rule_ids,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func NewHTTPGuardrailProvider(
	opts HTTPGuardrailProviderOptions,
) (GuardrailProvider, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("guardrail scanner endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("guardrail scanner endpoint is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("guardrail scanner endpoint scheme is invalid")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("guardrail scanner endpoint must not include userinfo")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultHTTPGuardrailTimeout
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	return &httpGuardrailProvider{
		endpoint: endpoint,
		token:    strings.TrimSpace(opts.Token),
		client:   client,
	}, nil
}

func (p *httpGuardrailProvider) EvaluateGuardrail(
	ctx context.Context,
	request GuardrailRequest,
) (GuardrailDecision, error) {
	if p == nil || p.client == nil {
		return GuardrailDecision{}, fmt.Errorf("guardrail scanner is not configured")
	}

	payload, err := json.Marshal(httpGuardrailScanRequest{
		Schema:     guardrailHTTPScanRequestSchema,
		SpaceID:    request.SpaceID,
		ThreadID:   request.ThreadID,
		RunID:      request.RunID,
		UserID:     request.UserID,
		TargetType: string(request.TargetType),
		TargetID:   sanitizeGuardrailIdentifier(request.TargetID, 128),
		Operation:  sanitizeGuardrailIdentifier(request.Operation, 64),
		Source:     sanitizeGuardrailIdentifier(request.Source, 64),
		FailMode:   string(request.FailMode),
		Metadata:   sanitizeGuardrailMetadata(request.Metadata),
	})
	if err != nil {
		return GuardrailDecision{}, err
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return GuardrailDecision{}, fmt.Errorf("guardrail scanner request is invalid")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return GuardrailDecision{}, fmt.Errorf("guardrail scanner request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return GuardrailDecision{}, fmt.Errorf("guardrail scanner http status %d", resp.StatusCode)
	}

	var decoded httpGuardrailScanResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxHTTPGuardrailResponseBytes)).Decode(&decoded); err != nil {
		return GuardrailDecision{}, fmt.Errorf("guardrail scanner response is invalid")
	}
	action := GuardrailAction(strings.ToLower(strings.TrimSpace(decoded.Action)))
	if !isExternalGuardrailAction(action) {
		return GuardrailDecision{}, fmt.Errorf("guardrail scanner response action is invalid")
	}

	return normalizeGuardrailDecision(GuardrailDecision{
		Action:     action,
		Provider:   decoded.Provider,
		ReasonCode: decoded.ReasonCode,
		Message:    decoded.Message,
		RuleIDs:    decoded.RuleIDs,
		Metadata:   decoded.Metadata,
	}), nil
}

func isExternalGuardrailAction(action GuardrailAction) bool {
	switch action {
	case GuardrailActionWarn, GuardrailActionConfirm, GuardrailActionDeny:
		return true
	default:
		return false
	}
}
