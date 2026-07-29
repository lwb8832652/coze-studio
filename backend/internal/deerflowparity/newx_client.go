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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NewXClient struct {
	http *safeHTTPClient

	workspaceMu       sync.RWMutex
	workspaceByThread map[string]string
}

var _ PlatformClient = (*NewXClient)(nil)

var errNewXNoResumableHumanInteraction = errors.New("newx run has no resumable human interaction")
var errNewXEventIDsNotIncreasing = errors.New("newx run event ids are not strictly increasing")

type newXCanonicalThread struct {
	ThreadID string `json:"thread_id"`
}

type newXCanonicalRunResponse struct {
	RunID    string `json:"run_id"`
	ThreadID string `json:"thread_id"`
	Status   string `json:"status"`
}

type newXCanonicalEventPage struct {
	Data             []map[string]any `json:"data"`
	HasMore          bool             `json:"has_more"`
	NextAfterEventID string           `json:"next_after_event_id"`
}

type newXCanonicalMessagePageResponse struct {
	Data          json.RawMessage `json:"data"`
	HasMore       *bool           `json:"has_more"`
	NextBeforeSeq *string         `json:"next_before_seq"`
	NextAfterSeq  *string         `json:"next_after_seq"`
}

type newXCanonicalStreamMetadata struct {
	RunID    string `json:"run_id"`
	ThreadID string `json:"thread_id"`
}

type newXCanonicalStreamEvent struct {
	EventID   string `json:"event_id"`
	ThreadID  string `json:"thread_id"`
	RunID     string `json:"run_id"`
	EventType string `json:"event_type"`
}

type newXCanonicalStreamEnd struct {
	RunID    string `json:"run_id"`
	ThreadID string `json:"thread_id"`
	Status   string `json:"status"`
}

func (c *NewXClient) Product() Product { return ProductNewX }

func NewNewXClient(baseURL string, options ClientOptions) (*NewXClient, error) {
	client, err := newSafeHTTPClient(baseURL, options)
	if err != nil {
		return nil, err
	}
	return &NewXClient{
		http:              client,
		workspaceByThread: make(map[string]string),
	}, nil
}

func newXPositiveResourceID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value[0] < '1' || value[0] > '9' {
		return "", errors.New("resource id is invalid")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
		return "", errors.New("resource id is invalid")
	}
	return value, nil
}

func newXSpaceHeaders(spaceID string) http.Header {
	headers := make(http.Header)
	headers.Set("X-Coze-Space-ID", spaceID)
	return headers
}

func (c *NewXClient) threadScope(threadID string) (string, http.Header, error) {
	threadID, err := newXPositiveResourceID(threadID)
	if err != nil {
		return "", nil, errors.New("newx thread id is invalid")
	}
	c.workspaceMu.RLock()
	spaceID, ok := c.workspaceByThread[threadID]
	c.workspaceMu.RUnlock()
	if !ok {
		return "", nil, errors.New("newx thread workspace is unknown")
	}
	if _, err := newXPositiveResourceID(spaceID); err != nil {
		return "", nil, errors.New("newx thread workspace is invalid")
	}
	return threadID, newXSpaceHeaders(spaceID), nil
}

func (c *NewXClient) bindThreadWorkspace(threadID, spaceID string) error {
	c.workspaceMu.Lock()
	defer c.workspaceMu.Unlock()
	if current, ok := c.workspaceByThread[threadID]; ok {
		if current == spaceID {
			return nil
		}
		return errors.New("newx thread workspace binding conflict")
	}
	c.workspaceByThread[threadID] = spaceID
	return nil
}

func newXContainsCallerIdentity(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if newXCallerIdentityKey(key) {
				return true
			}
			if newXContainsCallerIdentity(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if newXContainsCallerIdentity(child) {
				return true
			}
		}
	}
	return false
}

func newXCallerIdentityKey(key string) bool {
	var normalized strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(key)) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			normalized.WriteRune(character)
		}
	}
	switch normalized.String() {
	case "ownerid", "spaceid", "userid":
		return true
	default:
		return false
	}
}

func newXSerializedInput(input RunInput) (map[string]any, bool, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, false, fmt.Errorf("newx run input is not serializable: %w", err)
	}
	var serialized map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&serialized); err != nil {
		return nil, false, errors.New("newx run input serialization is invalid")
	}
	if serialized == nil {
		return nil, false, errors.New("newx run input serialization is invalid")
	}
	return serialized, newXContainsCallerIdentity(serialized), nil
}

func (c *NewXClient) doCanonicalJSON(
	ctx context.Context,
	method string,
	endpointName string,
	path string,
	body any,
	headers http.Header,
	out any,
) (http.Header, error) {
	var requestBody io.Reader
	requestHeaders := cloneHeaders(headers)
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%s encode request: %w", endpointName, err)
		}
		if int64(len(encoded)) > defaultMaxRequestBytes {
			return nil, fmt.Errorf("%s request exceeds size limit", endpointName)
		}
		requestBody = bytes.NewReader(encoded)
		requestHeaders.Set("Content-Type", "application/json")
	}
	response, err := c.http.send(ctx, method, endpointName, path, requestBody, requestHeaders)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = readBounded(response.Body, c.http.maxResponseBytes)
		return nil, fmt.Errorf("%s returned HTTP %d", endpointName, response.StatusCode)
	}
	payload, err := readBounded(response.Body, c.http.maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("%s %w", endpointName, err)
	}
	if out == nil {
		return response.Header.Clone(), nil
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil, fmt.Errorf("%s returned an empty JSON body", endpointName)
	}
	if bytes.Equal(payload, []byte("null")) {
		return nil, fmt.Errorf("%s returned a null JSON body", endpointName)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "+json") {
		return nil, fmt.Errorf("%s returned unsupported content type", endpointName)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", endpointName)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s returned invalid JSON", endpointName)
	}
	return response.Header.Clone(), nil
}

func newXRunHandle(threadID, expectedRunID string, response newXCanonicalRunResponse) (RunHandle, error) {
	responseThreadID, err := newXPositiveResourceID(response.ThreadID)
	if err != nil || responseThreadID != threadID {
		return RunHandle{}, errors.New("newx run thread id mismatch")
	}
	runID, err := newXPositiveResourceID(response.RunID)
	if err != nil {
		return RunHandle{}, errors.New("newx run id is invalid")
	}
	if expectedRunID != "" && runID != expectedRunID {
		return RunHandle{}, errors.New("newx run id mismatch")
	}
	status := canonicalRunStatus(response.Status)
	if status == "" || status == "unknown" {
		return RunHandle{}, errors.New("newx run status is invalid")
	}
	return RunHandle{ThreadID: threadID, RunID: runID, Status: status}, nil
}

func (c *NewXClient) Login(ctx context.Context, credentials Credentials) error {
	if err := validateCredentials(credentials); err != nil {
		return err
	}
	var response struct {
		Code int64 `json:"code"`
	}
	if err := c.http.doJSON(ctx, http.MethodPost, "newx_login", "/api/passport/web/email/login/", map[string]any{
		"email":    credentials.Email,
		"password": credentials.Password,
	}, nil, &response); err != nil {
		return err
	}
	if response.Code != 0 {
		return errors.New("newx login was rejected")
	}
	if cookie, ok := c.http.cookie("session_key"); !ok || strings.TrimSpace(cookie.Value) == "" {
		return errors.New("newx login did not establish an authenticated session")
	}
	return nil
}

func (c *NewXClient) CreateThread(ctx context.Context, options ThreadOptions) (string, error) {
	spaceID, err := newXPositiveResourceID(options.SpaceID)
	if err != nil {
		return "", errors.New("newx space id is invalid")
	}
	response := newXCanonicalThread{}
	if _, err := c.doCanonicalJSON(
		ctx,
		http.MethodPost,
		"newx_create_thread",
		"/api/workbench/threads",
		map[string]any{"metadata": map[string]any{"source": "api"}},
		newXSpaceHeaders(spaceID),
		&response,
	); err != nil {
		return "", err
	}
	threadID, err := newXPositiveResourceID(response.ThreadID)
	if err != nil {
		return "", errors.New("newx create thread returned an invalid thread id")
	}
	if err := c.bindThreadWorkspace(threadID, spaceID); err != nil {
		return "", err
	}
	return threadID, nil
}

func (c *NewXClient) StreamRun(ctx context.Context, threadID string, input RunInput) (StreamResult, error) {
	handle, err := c.StartRun(ctx, threadID, input)
	if err != nil {
		return StreamResult{}, err
	}
	return c.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{})
}

func (c *NewXClient) StartRun(ctx context.Context, threadID string, input RunInput) (RunHandle, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return RunHandle{}, err
	}
	if strings.TrimSpace(input.AssistantID) == "" || input.Input == nil {
		return RunHandle{}, errors.New("run input is incomplete")
	}
	input.StreamMode = []string{"events"}
	serializedInput, containsCallerIdentity, err := newXSerializedInput(input)
	if err != nil {
		return RunHandle{}, err
	}
	if containsCallerIdentity {
		return RunHandle{}, errors.New("newx run input must not contain caller identity")
	}
	response := newXCanonicalRunResponse{}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) + "/runs"
	if _, err := c.doCanonicalJSON(
		ctx, http.MethodPost, "newx_start_run", path, serializedInput, headers, &response,
	); err != nil {
		return RunHandle{}, err
	}
	return newXRunHandle(threadID, "", response)
}

func (c *NewXClient) StreamExistingRun(
	ctx context.Context,
	threadID string,
	runID string,
	options StreamOptions,
) (StreamResult, error) {
	if options.StopAfterFrames > 0 {
		return c.streamCanonicalRunSegment(ctx, threadID, runID, options)
	}

	threadID, _, err := c.threadScope(threadID)
	if err != nil {
		return StreamResult{}, err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return StreamResult{}, errors.New("newx run id is invalid")
	}
	cursor := strings.TrimSpace(options.AfterEventID)
	if cursor != "" {
		if _, err := newXPositiveResourceID(cursor); err != nil {
			return StreamResult{}, errors.New("stream cursor is invalid")
		}
	}
	result := StreamResult{ThreadID: threadID, RunID: runID, LastEventID: cursor}
	for {
		segment, err := c.streamCanonicalRunSegment(ctx, threadID, runID, StreamOptions{
			AfterEventID: cursor,
		})
		if err != nil {
			return StreamResult{}, err
		}
		result.Frames = append(result.Frames, segment.Frames...)
		result.TerminalFrameObserved = result.TerminalFrameObserved ||
			segment.TerminalFrameObserved
		progressed := segment.LastEventID != "" && segment.LastEventID != cursor
		if segment.LastEventID != "" {
			cursor = segment.LastEventID
			result.LastEventID = segment.LastEventID
		}
		if segment.Terminal != "" {
			result.Terminal = segment.Terminal
			return result, nil
		}

		handle, err := c.GetRun(ctx, threadID, runID)
		if err != nil {
			return StreamResult{}, err
		}
		if isTerminalRunStatus(handle.Status) {
			terminalSegment, streamErr := c.streamCanonicalRunSegment(
				ctx,
				threadID,
				runID,
				StreamOptions{AfterEventID: cursor},
			)
			if streamErr != nil {
				return StreamResult{}, streamErr
			}
			result.Frames = append(result.Frames, terminalSegment.Frames...)
			if terminalSegment.LastEventID != "" {
				result.LastEventID = terminalSegment.LastEventID
			}
			result.TerminalFrameObserved = terminalSegment.TerminalFrameObserved
			if !terminalSegment.TerminalFrameObserved || terminalSegment.Terminal == "" {
				return StreamResult{}, errors.New("newx terminal run stream omitted terminal frame")
			}
			if terminalSegment.Terminal != handle.Status {
				return StreamResult{}, errors.New("newx stream terminal does not match run status")
			}
			result.Terminal = terminalSegment.Terminal
			return result, nil
		}
		if !progressed {
			if err := waitForNewXStreamReconnect(ctx); err != nil {
				return StreamResult{}, err
			}
		}
	}
}

func (c *NewXClient) streamCanonicalRunSegment(
	ctx context.Context,
	threadID string,
	runID string,
	options StreamOptions,
) (StreamResult, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return StreamResult{}, err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return StreamResult{}, errors.New("newx run id is invalid")
	}
	query := url.Values{}
	query.Set("stream_mode", "events")
	if cursor := strings.TrimSpace(options.AfterEventID); cursor != "" {
		cursor, err = newXPositiveResourceID(cursor)
		if err != nil {
			return StreamResult{}, errors.New("stream cursor is invalid")
		}
		query.Set("after_event_id", cursor)
		headers.Set("Last-Event-ID", cursor)
	}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) +
		"/runs/" + url.PathEscape(runID) + "/stream?" + query.Encode()
	response, err := c.http.openSSE(
		ctx, http.MethodGet, "newx_stream_existing_run", path, nil, headers,
	)
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
		return StreamResult{}, fmt.Errorf("newx stream parse failed: %w", err)
	}
	lastEventID, err := validateNewXCanonicalStreamFrames(
		threadID, runID, options.AfterEventID, frames,
	)
	if err != nil {
		return StreamResult{}, err
	}
	result, err := streamResultFromFrames("newx", threadID, runID, frames)
	if err != nil {
		return StreamResult{}, err
	}
	if result.RunID != runID {
		return StreamResult{}, errors.New("newx stream run id mismatch")
	}
	result.LastEventID = lastEventID
	return result, nil
}

func decodeNewXCanonicalStreamFrame(frame SSEFrame, frameKind string, out any) error {
	payload := bytes.TrimSpace(frame.Data)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return fmt.Errorf("newx stream %s frame is invalid", frameKind)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("newx stream %s frame is invalid", frameKind)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("newx stream %s frame is invalid", frameKind)
	}
	return nil
}

func validateNewXCanonicalStreamFrames(
	threadID string,
	runID string,
	afterEventID string,
	frames []SSEFrame,
) (string, error) {
	lastEventID := strings.TrimSpace(afterEventID)
	var lastEventCursor int64
	if lastEventID != "" {
		validated, err := newXPositiveResourceID(lastEventID)
		if err != nil {
			return "", errors.New("newx stream cursor is invalid")
		}
		lastEventID = validated
		lastEventCursor, _ = strconv.ParseInt(validated, 10, 64)
	}

	for _, frame := range frames {
		switch strings.TrimSpace(frame.Event) {
		case "metadata":
			if strings.TrimSpace(frame.ID) != "" {
				return "", errors.New("newx stream metadata frame has an event id")
			}
			metadata := newXCanonicalStreamMetadata{}
			if err := decodeNewXCanonicalStreamFrame(frame, "metadata", &metadata); err != nil {
				return "", err
			}
			metadataThreadID, threadErr := newXPositiveResourceID(metadata.ThreadID)
			metadataRunID, runErr := newXPositiveResourceID(metadata.RunID)
			if threadErr != nil || runErr != nil || metadataThreadID != threadID || metadataRunID != runID {
				return "", errors.New("newx stream metadata identity mismatch")
			}
		case "events":
			frameID, err := newXPositiveResourceID(frame.ID)
			if err != nil {
				return "", errors.New("newx stream event id is invalid")
			}
			frameCursor, _ := strconv.ParseInt(frameID, 10, 64)
			if frameCursor <= lastEventCursor {
				return "", errors.New("newx stream event ids are not strictly increasing")
			}
			event := newXCanonicalStreamEvent{}
			if err := decodeNewXCanonicalStreamFrame(frame, "events", &event); err != nil {
				return "", err
			}
			eventID, eventErr := newXPositiveResourceID(event.EventID)
			eventThreadID, threadErr := newXPositiveResourceID(event.ThreadID)
			eventRunID, runErr := newXPositiveResourceID(event.RunID)
			if eventErr != nil || eventID != frameID {
				return "", errors.New("newx stream payload event id mismatch")
			}
			if threadErr != nil || runErr != nil || eventThreadID != threadID || eventRunID != runID {
				return "", errors.New("newx stream event identity mismatch")
			}
			if !safeEventFamilyPattern.MatchString(strings.TrimSpace(event.EventType)) {
				return "", errors.New("newx stream event type is invalid")
			}
			lastEventID = frameID
			lastEventCursor = frameCursor
		case "end":
			if strings.TrimSpace(frame.ID) != "" {
				return "", errors.New("newx stream end frame has an event id")
			}
			end := newXCanonicalStreamEnd{}
			if err := decodeNewXCanonicalStreamFrame(frame, "end", &end); err != nil {
				return "", err
			}
			endThreadID, threadErr := newXPositiveResourceID(end.ThreadID)
			endRunID, runErr := newXPositiveResourceID(end.RunID)
			if threadErr != nil || runErr != nil || endThreadID != threadID || endRunID != runID {
				return "", errors.New("newx stream end identity mismatch")
			}
			if terminal := canonicalTerminal(end.Status); !isTerminalRunStatus(terminal) {
				return "", errors.New("newx stream terminal status is invalid")
			}
		case "error":
			return "", errors.New("newx stream returned a runtime error")
		default:
			return "", errors.New("newx stream frame type is invalid")
		}
	}
	return lastEventID, nil
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "success", "cancelled", "failed", "interrupted":
		return true
	default:
		return false
	}
}

func waitForNewXStreamReconnect(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *NewXClient) GetRun(ctx context.Context, threadID, runID string) (RunHandle, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return RunHandle{}, err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return RunHandle{}, errors.New("newx run id is invalid")
	}
	response := newXCanonicalRunResponse{}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) + "/runs/" + url.PathEscape(runID)
	if _, err := c.doCanonicalJSON(
		ctx, http.MethodGet, "newx_get_run", path, nil, headers, &response,
	); err != nil {
		return RunHandle{}, err
	}
	return newXRunHandle(threadID, runID, response)
}

func (c *NewXClient) CancelRun(ctx context.Context, threadID, runID string) error {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return errors.New("newx run id is invalid")
	}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) +
		"/runs/" + url.PathEscape(runID) + "/cancel"
	_, err = c.doCanonicalJSON(ctx, http.MethodPost, "newx_cancel_run", path, nil, headers, nil)
	return err
}

func (c *NewXClient) FollowUpRun(
	ctx context.Context,
	threadID string,
	sourceRunID string,
	input RunInput,
	answer string,
) (StreamResult, error) {
	sourceRun, err := c.GetRun(ctx, threadID, sourceRunID)
	if err != nil {
		return StreamResult{}, err
	}
	if sourceRun.Status == "interrupted" {
		pending, pendingErr := c.pendingHumanInteraction(ctx, threadID, sourceRunID)
		switch {
		case pendingErr == nil && pending.InteractionKind == "clarification":
			return c.resumeHumanInteraction(ctx, threadID, sourceRunID, pending, answer)
		case pendingErr != nil && !errors.Is(pendingErr, errNewXNoResumableHumanInteraction):
			return StreamResult{}, pendingErr
		}
	}
	input.Input = map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": strings.TrimSpace(answer)}},
	}
	return c.StreamRun(ctx, threadID, input)
}

type newXPendingHumanInteraction struct {
	InterruptID     string
	InteractionID   string
	InteractionKind string
}

func (c *NewXClient) resumeHumanInteraction(
	ctx context.Context,
	threadID string,
	sourceRunID string,
	pending newXPendingHumanInteraction,
	answer string,
) (StreamResult, error) {
	if pending.InteractionKind != "clarification" {
		return StreamResult{}, errors.New("newx interrupted follow-up is not a clarification")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return StreamResult{}, errors.New("newx clarification answer is required")
	}
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return StreamResult{}, err
	}
	sourceRunID, err = newXPositiveResourceID(sourceRunID)
	if err != nil {
		return StreamResult{}, errors.New("newx run id is invalid")
	}
	interruptID := strings.TrimSpace(pending.InterruptID)
	if err := validateOpaqueID(interruptID); err != nil {
		return StreamResult{}, errors.New("newx interrupt id is invalid")
	}
	interactionID := strings.TrimSpace(pending.InteractionID)
	if err := validateOpaqueID(interactionID); err != nil {
		return StreamResult{}, errors.New("newx interaction id is invalid")
	}

	response := newXCanonicalRunResponse{}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) +
		"/runs/" + url.PathEscape(sourceRunID) + "/resume"
	_, err = c.doCanonicalJSON(ctx, http.MethodPost, "newx_resume_human_interaction", path, map[string]any{
		"interrupt_id": interruptID,
		"response": map[string]any{
			"schema":         "coze.human_interaction_response.v1",
			"interaction_id": interactionID,
			"kind":           pending.InteractionKind,
			"decision":       "answered",
			"answer":         answer,
			"source":         "deerflow_parity_acceptance",
		},
	}, headers, &response)
	if err != nil {
		return StreamResult{}, err
	}
	handle, err := newXRunHandle(threadID, "", response)
	if err != nil {
		return StreamResult{}, fmt.Errorf("newx resume human interaction returned an invalid run: %w", err)
	}
	return c.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{})
}

func (c *NewXClient) pendingHumanInteraction(
	ctx context.Context,
	threadID string,
	runID string,
) (newXPendingHumanInteraction, error) {
	const pageSize = 200
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return newXPendingHumanInteraction{}, err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return newXPendingHumanInteraction{}, errors.New("newx run id is invalid")
	}
	var latest newXPendingHumanInteraction
	var afterEventID string
	for {
		query := url.Values{}
		query.Set("limit", strconv.Itoa(pageSize))
		if afterEventID != "" {
			query.Set("after_event_id", afterEventID)
		}
		path := "/api/workbench/threads/" + url.PathEscape(threadID) +
			"/runs/" + url.PathEscape(runID) + "/events?" + query.Encode()
		response := newXCanonicalEventPage{}
		responseHeaders, err := c.doCanonicalJSON(
			ctx,
			http.MethodGet,
			"newx_list_public_human_interactions",
			path,
			nil,
			headers,
			&response,
		)
		if err != nil {
			return newXPendingHumanInteraction{}, err
		}
		if _, err := newXCanonicalPaginationTotal(responseHeaders); err != nil {
			return newXPendingHumanInteraction{}, err
		}
		lastEventID, err := validateNewXCanonicalEvents(
			threadID, runID, afterEventID, response.Data,
		)
		if err != nil {
			if errors.Is(err, errNewXEventIDsNotIncreasing) {
				return newXPendingHumanInteraction{}, fmt.Errorf(
					"newx run event pagination made no progress: %w", err,
				)
			}
			return newXPendingHumanInteraction{}, err
		}
		for _, event := range response.Data {
			if pending, ok := newXPendingInteractionFromEvent(event); ok {
				latest = pending
			}
		}
		if !response.HasMore {
			break
		}
		if len(response.Data) == 0 {
			return newXPendingHumanInteraction{}, errors.New("newx run event pagination made no progress")
		}
		nextAfterEventID, err := newXPositiveResourceID(response.NextAfterEventID)
		if err != nil || nextAfterEventID != lastEventID {
			return newXPendingHumanInteraction{}, errors.New("newx run event pagination made no progress")
		}
		afterEventID = nextAfterEventID
	}
	if latest.InteractionID == "" {
		return newXPendingHumanInteraction{}, fmt.Errorf(
			"%w: %s",
			errNewXNoResumableHumanInteraction,
			runID,
		)
	}
	return latest, nil
}

func newXPendingInteractionFromEvent(event map[string]any) (newXPendingHumanInteraction, bool) {
	if !strings.EqualFold(strings.TrimSpace(stringValue(event["event_type"])), "run.interrupted") {
		return newXPendingHumanInteraction{}, false
	}
	payload := mapValue(event["payload"])
	items := newXInterruptItems(payload["interrupts"])
	for itemIndex := len(items) - 1; itemIndex >= 0; itemIndex-- {
		raw := mapValue(items[itemIndex])
		info := mapValue(raw["info"])
		pending := newXPendingHumanInteraction{
			InterruptID:     strings.TrimSpace(stringValue(raw["id"])),
			InteractionID:   strings.TrimSpace(stringValue(info["interaction_id"])),
			InteractionKind: strings.ToLower(strings.TrimSpace(stringValue(info["kind"]))),
		}
		if pending.InterruptID == "" || pending.InteractionID == "" ||
			strings.TrimSpace(stringValue(info["schema"])) != "coze.human_interaction.v1" {
			continue
		}
		if err := validateOpaqueID(pending.InterruptID); err != nil {
			continue
		}
		if err := validateOpaqueID(pending.InteractionID); err != nil {
			continue
		}
		return pending, true
	}
	return newXPendingHumanInteraction{}, false
}

func newXInterruptItems(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	if container := mapValue(value); container != nil {
		items, _ := container["items"].([]any)
		return items
	}
	return nil
}

func validateNewXCanonicalCheckpoint(threadID string, value any) error {
	checkpoint, ok := value.(map[string]any)
	if !ok {
		return errors.New("checkpoint is not an object")
	}
	checkpointThreadID, ok := checkpoint["thread_id"].(string)
	if !ok || checkpointThreadID != threadID {
		return errors.New("checkpoint thread id mismatch")
	}
	if _, err := newXPositiveResourceID(checkpointThreadID); err != nil {
		return errors.New("checkpoint thread id is invalid")
	}
	checkpointID, ok := checkpoint["checkpoint_id"].(string)
	if !ok {
		return errors.New("checkpoint id is missing")
	}
	if _, err := newXPositiveResourceID(checkpointID); err != nil {
		return errors.New("checkpoint id is invalid")
	}
	if _, ok := checkpoint["checkpoint_ns"].(string); !ok {
		return errors.New("checkpoint namespace is invalid")
	}
	if _, ok := checkpoint["checkpoint_map"].(map[string]any); !ok {
		return errors.New("checkpoint map is invalid")
	}
	return nil
}

func validateNewXCanonicalThreadState(threadID string, state map[string]any) error {
	invalid := func(reason string) error {
		return fmt.Errorf("newx canonical thread state is invalid: %s", reason)
	}
	if state == nil {
		return invalid("state is not an object")
	}
	if _, ok := state["values"].(map[string]any); !ok {
		return invalid("values is not an object")
	}
	next, ok := state["next"].([]any)
	if !ok {
		return invalid("next is not an array")
	}
	for _, value := range next {
		if _, ok := value.(string); !ok {
			return invalid("next contains a non-string value")
		}
	}
	if err := validateNewXCanonicalCheckpoint(threadID, state["checkpoint"]); err != nil {
		return invalid(err.Error())
	}
	if _, ok := state["metadata"].(map[string]any); !ok {
		return invalid("metadata is not an object")
	}
	createdAt, ok := state["created_at"].(string)
	if !ok || strings.TrimSpace(createdAt) == "" {
		return invalid("created_at is invalid")
	}
	if _, ok := state["tasks"].([]any); !ok {
		return invalid("tasks is not an array")
	}
	if _, ok := state["interrupts"].([]any); !ok {
		return invalid("interrupts is not an array")
	}
	if parent, exists := state["parent_checkpoint"]; exists {
		if err := validateNewXCanonicalCheckpoint(threadID, parent); err != nil {
			return invalid("parent " + err.Error())
		}
	}
	return nil
}

func (c *NewXClient) GetThreadState(ctx context.Context, threadID string) (map[string]any, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return nil, err
	}
	response := map[string]any{}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) + "/state"
	if _, err := c.doCanonicalJSON(
		ctx, http.MethodGet, "newx_thread_state", path, nil, headers, &response,
	); err != nil {
		return nil, err
	}
	if err := validateNewXCanonicalThreadState(threadID, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *NewXClient) GetThreadHistory(ctx context.Context, threadID string, limit int) ([]map[string]any, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	path := "/api/workbench/threads/" + url.PathEscape(threadID) + "/history?" + query.Encode()
	var response []map[string]any
	if _, err := c.doCanonicalJSON(
		ctx, http.MethodGet, "newx_thread_history", path, nil, headers, &response,
	); err != nil {
		return nil, err
	}
	for index, state := range response {
		if err := validateNewXCanonicalThreadState(threadID, state); err != nil {
			return nil, fmt.Errorf("newx canonical thread history item %d is invalid: %w", index, err)
		}
	}
	return response, nil
}

func (c *NewXClient) ListRunMessages(ctx context.Context, threadID, runID string, page PageRequest) (MessagePage, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return MessagePage{}, err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return MessagePage{}, errors.New("newx run id is invalid")
	}
	if page.BeforeSeq > 0 && page.AfterSeq > 0 {
		return MessagePage{}, errors.New("before and after message cursors are mutually exclusive")
	}
	query := url.Values{}
	if page.Limit > 0 {
		query.Set("limit", strconv.Itoa(min(page.Limit, 1000)))
	}
	if page.BeforeSeq > 0 {
		query.Set("before_seq", strconv.FormatInt(page.BeforeSeq, 10))
	}
	if page.AfterSeq > 0 {
		query.Set("after_seq", strconv.FormatInt(page.AfterSeq, 10))
	}
	path := "/api/workbench/threads/" + url.PathEscape(threadID) +
		"/runs/" + url.PathEscape(runID) + "/messages"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	response := newXCanonicalMessagePageResponse{}
	responseHeaders, err := c.doCanonicalJSON(
		ctx, http.MethodGet, "newx_run_messages", path, nil, headers, &response,
	)
	if err != nil {
		return MessagePage{}, err
	}
	return newXMessagePage(threadID, runID, page, response, responseHeaders)
}

func (c *NewXClient) ListRunEvents(ctx context.Context, threadID, runID string, limit int) ([]map[string]any, error) {
	threadID, headers, err := c.threadScope(threadID)
	if err != nil {
		return nil, err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return nil, errors.New("newx run id is invalid")
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	path := "/api/workbench/threads/" + url.PathEscape(threadID) +
		"/runs/" + url.PathEscape(runID) + "/events?" + query.Encode()
	response := newXCanonicalEventPage{}
	responseHeaders, err := c.doCanonicalJSON(
		ctx, http.MethodGet, "newx_run_events", path, nil, headers, &response,
	)
	if err != nil {
		return nil, err
	}
	if _, err := newXCanonicalPaginationTotal(responseHeaders); err != nil {
		return nil, err
	}
	lastEventID, err := validateNewXCanonicalEvents(threadID, runID, "", response.Data)
	if err != nil {
		return nil, err
	}
	if response.HasMore {
		nextAfterEventID, err := newXPositiveResourceID(response.NextAfterEventID)
		if err != nil || len(response.Data) == 0 || nextAfterEventID != lastEventID {
			return nil, errors.New("newx run event page returned an invalid cursor")
		}
	}
	return response.Data, nil
}

func (c *NewXClient) WaitForEvent(ctx context.Context, threadID, runID, eventFamily string) error {
	threadID, _, err := c.threadScope(threadID)
	if err != nil {
		return err
	}
	runID, err = newXPositiveResourceID(runID)
	if err != nil {
		return errors.New("newx run id is invalid")
	}
	return waitForPlatformEvent(ctx, c, threadID, runID, eventFamily)
}

func newXCanonicalPaginationTotal(headers http.Header) (int64, error) {
	raw := strings.TrimSpace(headers.Get("X-Pagination-Total"))
	if raw == "" {
		return 0, errors.New("newx response omitted X-Pagination-Total")
	}
	total, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || total < 0 || strconv.FormatInt(total, 10) != raw {
		return 0, errors.New("newx response returned invalid X-Pagination-Total")
	}
	return total, nil
}

func newXMessagePage(
	threadID string,
	runID string,
	request PageRequest,
	response newXCanonicalMessagePageResponse,
	headers http.Header,
) (MessagePage, error) {
	invalid := func(reason string) (MessagePage, error) {
		return MessagePage{}, fmt.Errorf("newx canonical message page is invalid: %s", reason)
	}
	data := bytes.TrimSpace(response.Data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return invalid("data is missing")
	}
	if response.HasMore == nil {
		return invalid("has_more is missing")
	}
	var messages []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&messages); err != nil || messages == nil {
		return invalid("data is not an array")
	}
	if rawTotal := strings.TrimSpace(headers.Get("X-Pagination-Total")); rawTotal != "" {
		total, err := newXCanonicalPaginationTotal(headers)
		if err != nil {
			return MessagePage{}, err
		}
		if total < int64(len(messages)) {
			return invalid("X-Pagination-Total is below the returned data count")
		}
	}
	if err := validateNewXCanonicalMessages(threadID, runID, messages); err != nil {
		return MessagePage{}, err
	}
	if !*response.HasMore {
		if response.NextBeforeSeq != nil || response.NextAfterSeq != nil {
			return invalid("a terminal page advertises a next cursor")
		}
		return MessagePage{Data: messages, HasMore: false}, nil
	}
	if len(messages) == 0 {
		return invalid("has_more is true with empty data")
	}
	boundaryIndex := len(messages) - 1
	nextCursor := response.NextAfterSeq
	if request.BeforeSeq > 0 {
		boundaryIndex = 0
		nextCursor = response.NextBeforeSeq
	}
	expected, ok := messages[boundaryIndex]["seq"].(string)
	if !ok {
		return invalid("pagination boundary message has no sequence")
	}
	if _, err := newXPositiveResourceID(expected); err != nil {
		return invalid("pagination boundary sequence is invalid")
	}
	if nextCursor == nil || strings.TrimSpace(*nextCursor) != expected {
		return invalid("next cursor does not match the pagination boundary")
	}
	return MessagePage{Data: messages, HasMore: true}, nil
}

func validateNewXCanonicalMessages(threadID, runID string, messages []map[string]any) error {
	invalid := func(reason string) error {
		return fmt.Errorf("newx canonical message is invalid: %s", reason)
	}
	for _, message := range messages {
		messageID, ok := message["message_id"].(string)
		if !ok {
			return invalid("message_id is missing")
		}
		if _, err := newXPositiveResourceID(messageID); err != nil {
			return invalid("message_id is invalid")
		}
		messageThreadID, threadOK := message["thread_id"].(string)
		messageRunID, runOK := message["run_id"].(string)
		if !threadOK || !runOK || messageThreadID != threadID || messageRunID != runID {
			return invalid("thread or run identity mismatch")
		}
		role, ok := message["role"].(string)
		if !ok || strings.TrimSpace(role) == "" {
			return invalid("role is invalid")
		}
		if _, ok := message["content"].(string); !ok {
			return invalid("content is missing")
		}
		if _, ok := message["metadata"].(map[string]any); !ok {
			return invalid("metadata is not an object")
		}
		createdAt, ok := message["created_at"].(string)
		if !ok || strings.TrimSpace(createdAt) == "" {
			return invalid("created_at is invalid")
		}
		if rawSeq, exists := message["seq"]; exists {
			seq, ok := rawSeq.(string)
			if !ok {
				return invalid("sequence is not a string")
			}
			if _, err := newXPositiveResourceID(seq); err != nil {
				return invalid("sequence is invalid")
			}
		}
	}
	return nil
}

func validateNewXCanonicalEvents(
	threadID string,
	runID string,
	afterEventID string,
	events []map[string]any,
) (string, error) {
	lastEventID := strings.TrimSpace(afterEventID)
	var lastEventCursor int64
	if lastEventID != "" {
		validated, err := newXPositiveResourceID(lastEventID)
		if err != nil {
			return "", errors.New("newx run event cursor is invalid")
		}
		lastEventID = validated
		lastEventCursor, _ = strconv.ParseInt(validated, 10, 64)
	}
	for _, event := range events {
		eventID, err := newXPositiveResourceID(stringValue(event["event_id"]))
		if err != nil {
			return "", errors.New("newx run event id is invalid")
		}
		eventCursor, _ := strconv.ParseInt(eventID, 10, 64)
		if eventCursor <= lastEventCursor {
			return "", errNewXEventIDsNotIncreasing
		}
		if stringValue(event["thread_id"]) != threadID || stringValue(event["run_id"]) != runID {
			return "", errors.New("newx run event identity mismatch")
		}
		if !safeEventFamilyPattern.MatchString(strings.TrimSpace(stringValue(event["event_type"]))) {
			return "", errors.New("newx run event type is invalid")
		}
		lastEventID = eventID
		lastEventCursor = eventCursor
	}
	return lastEventID, nil
}
