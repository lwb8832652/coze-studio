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

package coze

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

type langGraphPublicRunEvent struct {
	EventID   int64  `json:"event_id,string"`
	ThreadID  int64  `json:"thread_id,string"`
	RunID     int64  `json:"run_id,string"`
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	CreatedAt int64  `json:"created_at"`
}

func projectLangGraphPublicRunEvent(event *appagentthread.RunEventSummary) *langGraphPublicRunEvent {
	projected := appagentthread.ProjectPublicRunEvent(event)
	if projected == nil {
		return nil
	}

	return &langGraphPublicRunEvent{
		EventID:   projected.EventID,
		ThreadID:  projected.ThreadID,
		RunID:     projected.RunID,
		EventType: projected.EventType,
		Payload:   projected.Payload,
		CreatedAt: projected.CreatedAt,
	}
}

func langGraphErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	workbenchThreadErrorResponse(ctx, c, err)
}
