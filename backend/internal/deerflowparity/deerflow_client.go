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

package deerflowparity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type DeerFlowClient struct {
	http      *safeHTTPClient
	csrfToken string
}

var _ PlatformClient = (*DeerFlowClient)(nil)

func (c *DeerFlowClient) Product() Product { return ProductDeerFlow }

func NewDeerFlowClient(baseURL string, options ClientOptions) (*DeerFlowClient, error) {
	client, err := newSafeHTTPClient(baseURL, options)
	if err != nil {
		return nil, err
	}
	return &DeerFlowClient{http: client}, nil
}

func (c *DeerFlowClient) Login(ctx context.Context, credentials Credentials) error {
	if err := validateCredentials(credentials); err != nil {
		return err
	}
	if err := c.http.doForm(ctx, "deerflow_login", "/api/v1/auth/login/local", url.Values{
		"username": {credentials.Email},
		"password": {credentials.Password},
	}, nil, nil); err != nil {
		return err
	}
	if _, ok := c.http.cookie("access_token"); !ok {
		return errors.New("deerflow login did not establish an authenticated session")
	}
	csrf, ok := c.http.cookie("csrf_token")
	if !ok || strings.TrimSpace(csrf.Value) == "" {
		return errors.New("deerflow login did not establish CSRF state")
	}
	c.csrfToken = csrf.Value
	return nil
}

func (c *DeerFlowClient) CreateThread(ctx context.Context, _ ThreadOptions) (string, error) {
	requestedID := uuid.NewString()
	var response struct {
		ThreadID string `json:"thread_id"`
	}
	if err := c.http.doJSON(ctx, http.MethodPost, "deerflow_create_thread", "/api/threads", map[string]any{
		"thread_id": requestedID,
		"metadata":  map[string]any{},
	}, c.csrfHeaders(), &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ThreadID) == "" {
		return "", errors.New("deerflow create thread returned no thread id")
	}
	return response.ThreadID, nil
}

func (c *DeerFlowClient) StreamRun(ctx context.Context, threadID string, input RunInput) (StreamResult, error) {
	handle, err := c.StartRun(ctx, threadID, input)
	if err != nil {
		return StreamResult{}, err
	}
	return c.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{})
}

func (c *DeerFlowClient) StartRun(ctx context.Context, threadID string, input RunInput) (RunHandle, error) {
	return startPlatformRun(ctx, c.http, "deerflow", threadID, deerFlowRunInput(input), c.csrfHeaders())
}

func (c *DeerFlowClient) StreamExistingRun(
	ctx context.Context,
	threadID string,
	runID string,
	options StreamOptions,
) (StreamResult, error) {
	result, err := streamExistingPlatformRun(ctx, c.http, "deerflow", threadID, runID, options, "", c.csrfHeaders())
	if err != nil {
		return StreamResult{}, err
	}
	if result.Terminal == "" {
		handle, getErr := c.GetRun(ctx, threadID, runID)
		if getErr != nil {
			return StreamResult{}, getErr
		}
		result.Terminal = handle.Status
	}
	return result, nil
}

func (c *DeerFlowClient) GetRun(ctx context.Context, threadID, runID string) (RunHandle, error) {
	return getPlatformRun(ctx, c.http, "deerflow", threadID, runID)
}

func (c *DeerFlowClient) CancelRun(ctx context.Context, threadID, runID string) error {
	if err := validateOpaqueID(threadID); err != nil {
		return err
	}
	if err := validateOpaqueID(runID); err != nil {
		return err
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID) +
		"/cancel?action=interrupt&wait=true"
	return c.http.doJSON(ctx, http.MethodPost, "deerflow_cancel_run", path, map[string]any{}, c.csrfHeaders(), nil)
}

func (c *DeerFlowClient) FollowUpRun(
	ctx context.Context,
	threadID string,
	_ string,
	input RunInput,
	answer string,
) (StreamResult, error) {
	input.Input = map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": strings.TrimSpace(answer)}},
	}
	return c.StreamRun(ctx, threadID, input)
}

func (c *DeerFlowClient) GetThreadState(ctx context.Context, threadID string) (map[string]any, error) {
	return getPlatformThreadState(ctx, c.http, "deerflow", threadID)
}

func (c *DeerFlowClient) GetThreadHistory(ctx context.Context, threadID string, limit int) ([]map[string]any, error) {
	return getPlatformThreadHistory(ctx, c.http, "deerflow", threadID, limit, c.csrfHeaders())
}

func (c *DeerFlowClient) ListRunMessages(ctx context.Context, threadID, runID string, page PageRequest) (MessagePage, error) {
	return getPlatformRunMessages(ctx, c.http, "deerflow", threadID, runID, page)
}

func (c *DeerFlowClient) ListRunEvents(ctx context.Context, threadID, runID string, limit int) ([]map[string]any, error) {
	return getPlatformRunEvents(ctx, c.http, "deerflow", threadID, runID, limit)
}

func (c *DeerFlowClient) WaitForEvent(ctx context.Context, threadID, runID, eventFamily string) error {
	return waitForPlatformEvent(ctx, c, threadID, runID, eventFamily)
}

func (c *DeerFlowClient) csrfHeaders() http.Header {
	headers := make(http.Header)
	if c.csrfToken != "" {
		headers.Set("X-CSRF-Token", c.csrfToken)
	}
	return headers
}

func deerFlowRunInput(input RunInput) RunInput {
	copyInput := input
	copyInput.StreamMode = []string{"values"}
	if strings.TrimSpace(copyInput.OnDisconnect) == "" {
		copyInput.OnDisconnect = "cancel"
	}
	return copyInput
}

func streamPlatformRun(
	ctx context.Context,
	client *safeHTTPClient,
	product string,
	threadID string,
	input RunInput,
	headers http.Header,
) (StreamResult, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return StreamResult{}, err
	}
	if strings.TrimSpace(input.AssistantID) == "" || input.Input == nil {
		return StreamResult{}, errors.New("run input is incomplete")
	}
	input = runInputWithThreadID(input, threadID)
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/stream"
	response, err := client.openStream(ctx, http.MethodPost, product+"_stream_run", path, input, headers)
	if err != nil {
		return StreamResult{}, err
	}
	defer response.Body.Close()
	frames, err := ParseSSE(response.Body, SSELimits{})
	if err != nil {
		return StreamResult{}, fmt.Errorf("%s stream parse failed: %w", product, err)
	}
	return streamResultFromFrames(product, threadID, "", frames)
}

func startPlatformRun(
	ctx context.Context,
	client *safeHTTPClient,
	product string,
	threadID string,
	input RunInput,
	headers http.Header,
) (RunHandle, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return RunHandle{}, err
	}
	input = runInputWithThreadID(input, threadID)
	var response struct {
		ThreadID string `json:"thread_id"`
		RunID    string `json:"run_id"`
		Status   string `json:"status"`
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs"
	if err := client.doJSON(ctx, http.MethodPost, product+"_start_run", path, input, headers, &response); err != nil {
		return RunHandle{}, err
	}
	if response.RunID == "" {
		return RunHandle{}, fmt.Errorf("%s start run returned no run id", product)
	}
	if response.ThreadID != "" && response.ThreadID != threadID {
		return RunHandle{}, fmt.Errorf("%s start run thread id mismatch", product)
	}
	return RunHandle{ThreadID: threadID, RunID: response.RunID, Status: canonicalRunStatus(response.Status)}, nil
}

func streamExistingPlatformRun(
	ctx context.Context,
	client *safeHTTPClient,
	product string,
	threadID string,
	runID string,
	options StreamOptions,
	streamMode string,
	headers http.Header,
) (StreamResult, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return StreamResult{}, err
	}
	if err := validateOpaqueID(runID); err != nil {
		return StreamResult{}, err
	}
	query := url.Values{}
	if streamMode != "" {
		query.Set("stream_mode", streamMode)
	}
	if options.AfterEventID != "" {
		if err := validateOpaqueID(options.AfterEventID); err != nil {
			return StreamResult{}, errors.New("stream cursor is invalid")
		}
		query.Set("after_event_id", options.AfterEventID)
		headers = cloneHeaders(headers)
		headers.Set("Last-Event-ID", options.AfterEventID)
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID) + "/stream?" + query.Encode()
	response, err := client.openSSE(ctx, http.MethodGet, product+"_stream_existing_run", path, nil, headers)
	if err != nil {
		return StreamResult{}, err
	}
	defer response.Body.Close()
	var frames []SSEFrame
	if options.StopAfterFrames > 0 {
		frames, err = ParseSSEUntilEvents(response.Body, SSELimits{}, options.StopAfterFrames)
	} else {
		frames, err = ParseSSE(response.Body, SSELimits{})
	}
	if err != nil {
		return StreamResult{}, fmt.Errorf("%s stream parse failed: %w", product, err)
	}
	return streamResultFromFrames(product, threadID, runID, frames)
}

func cancelPlatformRun(
	ctx context.Context,
	client *safeHTTPClient,
	product string,
	threadID string,
	runID string,
	headers http.Header,
) error {
	if err := validateOpaqueID(threadID); err != nil {
		return err
	}
	if err := validateOpaqueID(runID); err != nil {
		return err
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID) + "/cancel"
	return client.doJSON(ctx, http.MethodPost, product+"_cancel_run", path, map[string]any{}, headers, nil)
}

func getPlatformRun(
	ctx context.Context,
	client *safeHTTPClient,
	product string,
	threadID string,
	runID string,
) (RunHandle, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return RunHandle{}, err
	}
	if err := validateOpaqueID(runID); err != nil {
		return RunHandle{}, err
	}
	var response struct {
		ThreadID string `json:"thread_id"`
		RunID    string `json:"run_id"`
		Status   string `json:"status"`
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID)
	if err := client.doJSON(ctx, http.MethodGet, product+"_get_run", path, nil, nil, &response); err != nil {
		return RunHandle{}, err
	}
	if response.RunID != runID || response.ThreadID != threadID {
		return RunHandle{}, fmt.Errorf("%s get run identity mismatch", product)
	}
	return RunHandle{ThreadID: threadID, RunID: runID, Status: canonicalRunStatus(response.Status)}, nil
}

func streamResultFromFrames(product, threadID, fallbackRunID string, frames []SSEFrame) (StreamResult, error) {
	result := StreamResult{ThreadID: threadID, RunID: fallbackRunID, Frames: frames}
	for _, frame := range frames {
		if frame.ID != "" {
			result.LastEventID = frame.ID
		}
		switch frame.Event {
		case "metadata":
			var metadata struct {
				RunID    string `json:"run_id"`
				ThreadID string `json:"thread_id"`
			}
			if err := json.Unmarshal(frame.Data, &metadata); err != nil {
				return StreamResult{}, fmt.Errorf("%s stream metadata is invalid", product)
			}
			if metadata.ThreadID != "" && metadata.ThreadID != threadID {
				return StreamResult{}, fmt.Errorf("%s stream thread id mismatch", product)
			}
			if metadata.RunID != "" {
				result.RunID = metadata.RunID
			}
		case "end":
			var terminal struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(frame.Data, &terminal); err != nil {
				return StreamResult{}, fmt.Errorf("%s stream terminal frame is invalid", product)
			}
			result.Terminal = canonicalTerminal(terminal.Status)
			result.TerminalFrameObserved = true
		case "error":
			return StreamResult{}, fmt.Errorf("%s stream returned a runtime error", product)
		}
	}
	if strings.TrimSpace(result.RunID) == "" {
		return StreamResult{}, fmt.Errorf("%s stream returned no run id", product)
	}
	return result, nil
}

func runInputWithThreadID(input RunInput, threadID string) RunInput {
	copyInput := input
	copyInput.Context = make(map[string]any, len(input.Context)+1)
	for key, value := range input.Context {
		copyInput.Context[key] = value
	}
	copyInput.Context["thread_id"] = threadID
	return copyInput
}

func waitForPlatformEvent(
	ctx context.Context,
	client interface {
		GetRun(context.Context, string, string) (RunHandle, error)
		ListRunEvents(context.Context, string, string, int) ([]map[string]any, error)
	},
	threadID string,
	runID string,
	eventFamily string,
) error {
	if !safeEventFamilyPattern.MatchString(eventFamily) {
		return errors.New("event boundary is invalid")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if eventFamily == "run.started" {
			handle, err := client.GetRun(ctx, threadID, runID)
			if err != nil {
				return err
			}
			switch handle.Status {
			case "running":
				return nil
			case "success", "cancelled", "failed", "interrupted":
				return errors.New("run reached a terminal state before the event boundary")
			}
			select {
			case <-ctx.Done():
				return errors.New("event boundary wait timed out")
			case <-ticker.C:
			}
			continue
		}
		events, err := client.ListRunEvents(ctx, threadID, runID, 1000)
		if err != nil {
			return err
		}
		for _, event := range rawEventsFromMaps(events) {
			family, mapErr := canonicalEventFamily(event)
			if mapErr != nil {
				return mapErr
			}
			if family == eventFamily {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("event boundary wait timed out")
		case <-ticker.C:
		}
	}
}

func getPlatformThreadState(ctx context.Context, client *safeHTTPClient, product, threadID string) (map[string]any, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return nil, err
	}
	var response map[string]any
	path := "/api/threads/" + url.PathEscape(threadID) + "/state"
	if err := client.doJSON(ctx, http.MethodGet, product+"_thread_state", path, nil, nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func getPlatformThreadHistory(
	ctx context.Context,
	client *safeHTTPClient,
	product string,
	threadID string,
	limit int,
	headers http.Header,
) ([]map[string]any, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var response []map[string]any
	path := "/api/threads/" + url.PathEscape(threadID) + "/history"
	if err := client.doJSON(
		ctx,
		http.MethodPost,
		product+"_thread_history",
		path,
		map[string]any{"limit": limit},
		headers,
		&response,
	); err != nil {
		return nil, err
	}
	return response, nil
}

func getPlatformRunMessages(ctx context.Context, client *safeHTTPClient, product, threadID, runID string, page PageRequest) (MessagePage, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return MessagePage{}, err
	}
	if err := validateOpaqueID(runID); err != nil {
		return MessagePage{}, err
	}
	values := url.Values{}
	if page.Limit > 0 {
		values.Set("limit", strconv.Itoa(min(page.Limit, 1000)))
	}
	if page.BeforeSeq > 0 {
		values.Set("before_seq", strconv.FormatInt(page.BeforeSeq, 10))
	}
	if page.AfterSeq > 0 {
		values.Set("after_seq", strconv.FormatInt(page.AfterSeq, 10))
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID) + "/messages"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var response MessagePage
	if err := client.doJSON(ctx, http.MethodGet, product+"_run_messages", path, nil, nil, &response); err != nil {
		return MessagePage{}, err
	}
	return response, nil
}

func getPlatformRunEvents(ctx context.Context, client *safeHTTPClient, product, threadID, runID string, limit int) ([]map[string]any, error) {
	if err := validateOpaqueID(threadID); err != nil {
		return nil, err
	}
	if err := validateOpaqueID(runID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	path := "/api/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID) + "/events?limit=" + strconv.Itoa(limit)
	var response []map[string]any
	if err := client.doJSON(ctx, http.MethodGet, product+"_run_events", path, nil, nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func validateCredentials(credentials Credentials) error {
	if strings.TrimSpace(credentials.Email) == "" || strings.TrimSpace(credentials.Password) == "" {
		return errors.New("credentials are incomplete")
	}
	if len(credentials.Email) > 320 || len(credentials.Password) > 1024 {
		return errors.New("credentials exceed bounds")
	}
	return nil
}

func validateOpaqueID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "/\\\r\n\t") {
		return errors.New("resource id is invalid")
	}
	return nil
}
