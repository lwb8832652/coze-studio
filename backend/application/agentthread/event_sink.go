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
	"context"
	"encoding/json"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type RunEvent struct {
	ThreadID  int64
	RunID     int64
	EventType string
	Payload   string
}

type RunEventSink interface {
	EmitRunEvent(ctx context.Context, event RunEvent) error
}

type RunEventSinkFunc func(ctx context.Context, event RunEvent) error

func (f RunEventSinkFunc) EmitRunEvent(ctx context.Context, event RunEvent) error {
	if f == nil {
		return fmt.Errorf("run event sink function is required")
	}

	return f(ctx, event)
}

type applicationRunEventSink struct {
	app *ApplicationService
}

func NewApplicationRunEventSink(app *ApplicationService) RunEventSink {
	if app == nil {
		return nil
	}

	return applicationRunEventSink{app: app}
}

func (s applicationRunEventSink) EmitRunEvent(ctx context.Context, event RunEvent) error {
	if s.app == nil {
		return fmt.Errorf("agent thread application service is required")
	}

	_, err := s.app.AppendRunEvent(ctx, &AppendRunEventRequest{
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		EventType: event.EventType,
		Payload:   event.Payload,
	})

	return err
}

func emitRunEvent(ctx context.Context, sink RunEventSink, event RunEvent) {
	if sink == nil {
		return
	}

	if err := sink.EmitRunEvent(ctx, event); err != nil {
		logs.CtxWarnf(ctx, "[agent-run-event] emit event failed, run_id=%d event_type=%s err=%v", event.RunID, event.EventType, err)
	}
}

func encodeRunEventPayload(ctx context.Context, payload map[string]any) string {
	bytes, err := json.Marshal(payload)
	if err != nil {
		logs.CtxWarnf(ctx, "[agent-run-event] marshal payload failed, err=%v", err)

		return `{}`
	}

	return string(bytes)
}
