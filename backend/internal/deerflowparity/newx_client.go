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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type NewXClient struct {
	http *safeHTTPClient
}

var _ PlatformClient = (*NewXClient)(nil)

var errNewXNoResumableHumanInteraction = errors.New("newx run has no resumable human interaction")

func (c *NewXClient) Product() Product { return ProductNewX }

func NewNewXClient(baseURL string, options ClientOptions) (*NewXClient, error) {
	client, err := newSafeHTTPClient(baseURL, options)
	if err != nil {
		return nil, err
	}
	return &NewXClient{http: client}, nil
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
	spaceID := strings.TrimSpace(options.SpaceID)
	if err := validateOpaqueID(spaceID); err != nil {
		return "", errors.New("newx space id is invalid")
	}
	var response struct {
		ThreadID string `json:"thread_id"`
	}
	if err := c.http.doJSON(ctx, http.MethodPost, "newx_create_thread", "/api/threads", map[string]any{
		"metadata": map[string]any{
			"space_id": spaceID,
			"source":   "api",
		},
	}, nil, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ThreadID) == "" {
		return "", errors.New("newx create thread returned no thread id")
	}
	return response.ThreadID, nil
}

func (c *NewXClient) StreamRun(ctx context.Context, threadID string, input RunInput) (StreamResult, error) {
	handle, err := c.StartRun(ctx, threadID, input)
	if err != nil {
		return StreamResult{}, err
	}
	return c.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{})
}

func (c *NewXClient) StartRun(ctx context.Context, threadID string, input RunInput) (RunHandle, error) {
	return startPlatformRun(ctx, c.http, "newx", threadID, input, nil)
}

func (c *NewXClient) StreamExistingRun(
	ctx context.Context,
	threadID string,
	runID string,
	options StreamOptions,
) (StreamResult, error) {
	if options.StopAfterFrames > 0 {
		return streamExistingPlatformRun(ctx, c.http, "newx", threadID, runID, options, "events", nil)
	}

	result := StreamResult{ThreadID: threadID, RunID: runID, LastEventID: options.AfterEventID}
	cursor := options.AfterEventID
	for {
		segment, err := streamExistingPlatformRun(ctx, c.http, "newx", threadID, runID, StreamOptions{
			AfterEventID: cursor,
		}, "events", nil)
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
			terminalSegment, streamErr := streamExistingPlatformRun(
				ctx,
				c.http,
				"newx",
				threadID,
				runID,
				StreamOptions{AfterEventID: cursor},
				"events",
				nil,
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
	return getPlatformRun(ctx, c.http, "newx", threadID, runID)
}

func (c *NewXClient) CancelRun(ctx context.Context, threadID, runID string) error {
	return cancelPlatformRun(ctx, c.http, "newx", threadID, runID, nil)
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

	var response struct {
		Code int64 `json:"code"`
		Data struct {
			ThreadID string `json:"thread_id"`
			RunID    string `json:"run_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	path := "/api/workbench/task_threads/" + url.PathEscape(threadID) +
		"/runs/" + url.PathEscape(sourceRunID) + "/resume"
	err := c.http.doJSON(ctx, http.MethodPost, "newx_resume_human_interaction", path, map[string]any{
		"interrupt_id": pending.InterruptID,
		"response": map[string]any{
			"schema":         "coze.human_interaction_response.v1",
			"interaction_id": pending.InteractionID,
			"kind":           pending.InteractionKind,
			"decision":       "answered",
			"answer":         answer,
			"source":         "deerflow_parity_acceptance",
		},
	}, nil, &response)
	if err != nil {
		return StreamResult{}, err
	}
	if response.Code != 0 || response.Data.ThreadID != threadID || strings.TrimSpace(response.Data.RunID) == "" {
		return StreamResult{}, errors.New("newx resume human interaction returned an invalid run")
	}
	return c.StreamExistingRun(ctx, threadID, response.Data.RunID, StreamOptions{})
}

func (c *NewXClient) pendingHumanInteraction(
	ctx context.Context,
	threadID string,
	runID string,
) (newXPendingHumanInteraction, error) {
	const pageSize = 200
	var latest newXPendingHumanInteraction
	for page := 1; ; page++ {
		query := url.Values{}
		query.Set("run_id", runID)
		query.Set("page", fmt.Sprintf("%d", page))
		query.Set("page_size", fmt.Sprintf("%d", pageSize))
		path := "/api/workbench/task_threads/" + url.PathEscape(threadID) +
			"/run_events?" + query.Encode()
		var response struct {
			Code int64 `json:"code"`
			Data *struct {
				Events []map[string]any `json:"events"`
				Total  int              `json:"total"`
			} `json:"data"`
		}
		if err := c.http.doJSON(
			ctx,
			http.MethodGet,
			"newx_list_public_human_interactions",
			path,
			nil,
			nil,
			&response,
		); err != nil {
			return newXPendingHumanInteraction{}, err
		}
		if response.Code != 0 {
			return newXPendingHumanInteraction{}, errors.New("newx run event listing was rejected")
		}
		if response.Data == nil {
			return newXPendingHumanInteraction{}, errors.New("newx run event listing returned no data")
		}
		if response.Data.Total < 0 {
			return newXPendingHumanInteraction{}, errors.New("newx run event history total is invalid")
		}
		for _, event := range response.Data.Events {
			if pending, ok := newXPendingInteractionFromEvent(event); ok {
				latest = pending
			}
		}
		if page*pageSize >= response.Data.Total {
			break
		}
		if len(response.Data.Events) == 0 {
			return newXPendingHumanInteraction{}, errors.New("newx run event pagination made no progress")
		}
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

func (c *NewXClient) GetThreadState(ctx context.Context, threadID string) (map[string]any, error) {
	return getPlatformThreadState(ctx, c.http, "newx", threadID)
}

func (c *NewXClient) GetThreadHistory(ctx context.Context, threadID string, limit int) ([]map[string]any, error) {
	return getPlatformThreadHistory(ctx, c.http, "newx", threadID, limit, nil)
}

func (c *NewXClient) ListRunMessages(ctx context.Context, threadID, runID string, page PageRequest) (MessagePage, error) {
	return getPlatformRunMessages(ctx, c.http, "newx", threadID, runID, page)
}

func (c *NewXClient) ListRunEvents(ctx context.Context, threadID, runID string, limit int) ([]map[string]any, error) {
	return getPlatformRunEvents(ctx, c.http, "newx", threadID, runID, limit)
}

func (c *NewXClient) WaitForEvent(ctx context.Context, threadID, runID, eventFamily string) error {
	return waitForPlatformEvent(ctx, c, threadID, runID, eventFamily)
}
