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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

const maxRemoteRunnerResponseBytes = 2 * 1024 * 1024

type disabledRuntimeManager struct {
	reason string
}

type remoteRuntimeManager struct {
	endpoint       string
	previewBaseURL string
	token          string
	client         *http.Client
}

type remoteRuntimeRequest struct {
	SpaceID   string `json:"spaceId"`
	ProjectID string `json:"projectId"`
	SourceURL string `json:"sourceUrl,omitempty"`
}

type remoteRuntimeInfo struct {
	Status          string `json:"status"`
	PreviewURL      string `json:"previewUrl"`
	Message         string `json:"message"`
	LastKeepAliveAt string `json:"lastKeepAliveAt"`
}

type remoteRuntimeLog struct {
	ID        string `json:"id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type remoteRuntimeLogs struct {
	Items []*remoteRuntimeLog `json:"items"`
}

type remoteBuildRequest struct {
	SpaceID        string `json:"spaceId"`
	ProjectID      string `json:"projectId"`
	PublishType    string `json:"publishType"`
	SourceURL      string `json:"sourceUrl"`
	ArtifactPrefix string `json:"artifactPrefix"`
}

type remoteBuildResponse struct {
	Status            string `json:"status"`
	ArtifactObjectKey string `json:"artifactObjectKey"`
	Message           string `json:"message"`
	StartedAt         string `json:"startedAt"`
	FinishedAt        string `json:"finishedAt"`
}

func NewConfiguredRuntimeManager() appdevapp.RuntimeManager {
	endpoint := strings.TrimSpace(os.Getenv("APP_DEV_RUNNER_ENDPOINT"))
	if endpoint != "" {
		manager, err := newRemoteRuntimeManager(
			endpoint,
			strings.TrimSpace(os.Getenv("APP_DEV_PREVIEW_GATEWAY_BASE_URL")),
			strings.TrimSpace(os.Getenv("APP_DEV_RUNNER_TOKEN")),
		)
		if err == nil {
			return manager
		}
		return &disabledRuntimeManager{reason: appdevapp.SanitizeAppDevOutput(err.Error())}
	}

	if appdevapp.IsAppDevHostExecutionEnabled() {
		return NewRuntimeManager()
	}

	return &disabledRuntimeManager{
		reason: "isolated appdev runner is not configured",
	}
}

func newRemoteRuntimeManager(endpoint string, previewBaseURL string, token string) (*remoteRuntimeManager, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.Host == "" || endpointURL.User != nil || (endpointURL.Scheme != "https" && endpointURL.Scheme != "http") {
		return nil, fmt.Errorf("invalid appdev runner endpoint")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "debug") {
		if endpointURL.Scheme != "https" {
			return nil, fmt.Errorf("appdev runner endpoint must use HTTPS")
		}
		if strings.TrimSpace(token) == "" {
			return nil, fmt.Errorf("appdev runner token is required")
		}
	}
	if err := validateRemotePreviewURL(previewBaseURL, previewBaseURL); err != nil {
		return nil, fmt.Errorf("invalid appdev preview gateway: %w", err)
	}

	return &remoteRuntimeManager{
		endpoint:       strings.TrimRight(endpoint, "/"),
		previewBaseURL: strings.TrimRight(previewBaseURL, "/"),
		token:          token,
		client: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}, nil
}

func validateRemotePreviewURL(baseURL string, previewURL string) error {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil {
		return fmt.Errorf("preview gateway must be an HTTPS origin")
	}
	preview, err := url.Parse(strings.TrimSpace(previewURL))
	if err != nil || preview.Scheme != "https" || preview.Host == "" || preview.User != nil {
		return fmt.Errorf("preview URL must use HTTPS")
	}
	if !strings.EqualFold(base.Host, preview.Host) {
		return fmt.Errorf("preview URL is outside the configured gateway")
	}
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	previewPath := preview.EscapedPath()
	if basePath != "" && basePath != "/" && previewPath != basePath && !strings.HasPrefix(previewPath, basePath+"/") {
		return fmt.Errorf("preview URL is outside the configured gateway path")
	}
	return nil
}

func (m *disabledRuntimeManager) runtimeError() error {
	reason := strings.TrimSpace(m.reason)
	if reason == "" {
		reason = "isolated appdev runner is not configured"
	}
	return fmt.Errorf("%s", reason)
}

func (m *disabledRuntimeManager) Start(context.Context, *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, m.runtimeError()
}

func (m *disabledRuntimeManager) Status(context.Context, *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return &domainappdev.RuntimeInfo{
		Status:  domainappdev.RuntimeStatusStopped,
		Message: m.runtimeError().Error(),
	}, nil
}

func (m *disabledRuntimeManager) KeepAlive(context.Context, *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, m.runtimeError()
}

func (m *disabledRuntimeManager) Restart(context.Context, *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, m.runtimeError()
}

func (m *disabledRuntimeManager) Stop(context.Context, *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return &domainappdev.RuntimeInfo{
		Status:  domainappdev.RuntimeStatusStopped,
		Message: "开发环境未启动",
	}, nil
}

func (m *disabledRuntimeManager) Logs(context.Context, *appdevapp.RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error) {
	return []*domainappdev.RuntimeLog{}, nil
}

func (m *remoteRuntimeManager) Start(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return m.runtimeAction(ctx, "start", req)
}

func (m *remoteRuntimeManager) Status(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return m.runtimeAction(ctx, "status", req)
}

func (m *remoteRuntimeManager) KeepAlive(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return m.runtimeAction(ctx, "keep-alive", req)
}

func (m *remoteRuntimeManager) Restart(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return m.runtimeAction(ctx, "restart", req)
}

func (m *remoteRuntimeManager) Stop(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return m.runtimeAction(ctx, "stop", req)
}

func (m *remoteRuntimeManager) Logs(ctx context.Context, req *appdevapp.RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error) {
	var response remoteRuntimeLogs
	if err := m.post(ctx, "logs", req, &response); err != nil {
		return nil, err
	}

	logs := make([]*domainappdev.RuntimeLog, 0, len(response.Items))
	for _, item := range response.Items {
		if item == nil {
			continue
		}
		timestamp, _ := time.Parse(time.RFC3339, item.Timestamp)
		logs = append(logs, &domainappdev.RuntimeLog{
			ID:        item.ID,
			Level:     item.Level,
			Message:   appdevapp.SanitizeAppDevOutput(item.Message),
			Timestamp: timestamp,
		})
	}
	return logs, nil
}

func (m *remoteRuntimeManager) Build(ctx context.Context, req *appdevapp.IsolatedBuildRequest) (*appdevapp.IsolatedBuildResult, error) {
	if req == nil || strings.TrimSpace(req.SpaceID) == "" || strings.TrimSpace(req.ProjectID) == "" {
		return nil, fmt.Errorf("invalid isolated appdev build request")
	}
	sourceURL, err := url.Parse(strings.TrimSpace(req.SourceURL))
	if err != nil || sourceURL.Scheme != "https" || sourceURL.Host == "" || sourceURL.User != nil {
		return nil, fmt.Errorf("appdev source URL must use HTTPS")
	}
	artifactPrefix := fmt.Sprintf(
		"appdev/spaces/%s/projects/%s/builds/",
		safeObjectSegment(req.SpaceID),
		safeObjectSegment(req.ProjectID),
	)
	payload, err := json.Marshal(remoteBuildRequest{
		SpaceID:        strings.TrimSpace(req.SpaceID),
		ProjectID:      strings.TrimSpace(req.ProjectID),
		PublishType:    strings.TrimSpace(req.PublishType),
		SourceURL:      sourceURL.String(),
		ArtifactPrefix: artifactPrefix,
	})
	if err != nil {
		return nil, err
	}
	var response remoteBuildResponse
	if err := m.postPayload(ctx, "/v1/appdev/builds", payload, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" || !strings.HasPrefix(response.ArtifactObjectKey, artifactPrefix) {
		return nil, fmt.Errorf("invalid isolated appdev build artifact")
	}
	startedAt, _ := time.Parse(time.RFC3339, response.StartedAt)
	finishedAt, _ := time.Parse(time.RFC3339, response.FinishedAt)
	return &appdevapp.IsolatedBuildResult{
		Status:            response.Status,
		ArtifactObjectKey: response.ArtifactObjectKey,
		Message:           appdevapp.SanitizeAppDevOutput(response.Message),
		StartedAt:         startedAt,
		FinishedAt:        finishedAt,
	}, nil
}

func (m *remoteRuntimeManager) runtimeAction(ctx context.Context, action string, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	var response remoteRuntimeInfo
	if err := m.post(ctx, action, req, &response); err != nil {
		return nil, err
	}
	if response.PreviewURL != "" {
		if err := validateRemotePreviewURL(m.previewBaseURL, response.PreviewURL); err != nil {
			return nil, err
		}
	}
	lastKeepAliveAt, _ := time.Parse(time.RFC3339, response.LastKeepAliveAt)
	return &domainappdev.RuntimeInfo{
		Status:          domainappdev.RuntimeStatus(response.Status),
		PreviewURL:      response.PreviewURL,
		Message:         appdevapp.SanitizeAppDevOutput(response.Message),
		LastKeepAliveAt: lastKeepAliveAt,
	}, nil
}

func (m *remoteRuntimeManager) post(ctx context.Context, action string, req *appdevapp.RuntimeManagerRequest, response any) error {
	if req == nil || strings.TrimSpace(req.SpaceID) == "" || strings.TrimSpace(req.ProjectID) == "" {
		return fmt.Errorf("invalid appdev runner request")
	}
	payload, err := json.Marshal(remoteRuntimeRequest{
		SpaceID:   strings.TrimSpace(req.SpaceID),
		ProjectID: strings.TrimSpace(req.ProjectID),
		SourceURL: strings.TrimSpace(req.SourceURL),
	})
	if err != nil {
		return err
	}
	return m.postPayload(ctx, "/v1/appdev/runtimes/"+action, payload, response)
}

func (m *remoteRuntimeManager) postPayload(ctx context.Context, path string, payload []byte, response any) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if m.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.token)
	}

	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("appdev runner request failed")
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, maxRemoteRunnerResponseBytes+1))
	if err != nil {
		return fmt.Errorf("appdev runner response could not be read")
	}
	if len(body) > maxRemoteRunnerResponseBytes {
		return fmt.Errorf("appdev runner response is too large")
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("appdev runner request failed with status %d", httpResp.StatusCode)
	}
	if err := json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("invalid appdev runner response")
	}
	return nil
}
