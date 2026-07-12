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
	"net/http"
	"strings"
)

type NewXClient struct {
	http *safeHTTPClient
}

var _ PlatformClient = (*NewXClient)(nil)

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
	return streamPlatformRun(ctx, c.http, "newx", threadID, input, nil)
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
	return streamExistingPlatformRun(ctx, c.http, "newx", threadID, runID, options, "events", nil)
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
	input RunInput,
	answer string,
) (StreamResult, error) {
	input.Input = map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": strings.TrimSpace(answer)}},
	}
	return c.StreamRun(ctx, threadID, input)
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
