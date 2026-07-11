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
	"time"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	defaultRunEventStreamKeepAliveInterval = 15 * time.Second
	runStreamDisconnectCancelTimeout       = 5 * time.Second
)

type runEventStreamWriter interface {
	WriteEvent(id, eventType string, data []byte) error
	WriteKeepAlive() error
}

type disconnectTrackingRunEventStreamWriter struct {
	writer       runEventStreamWriter
	disconnected bool
}

func newDisconnectTrackingRunEventStreamWriter(writer runEventStreamWriter) *disconnectTrackingRunEventStreamWriter {
	return &disconnectTrackingRunEventStreamWriter{writer: writer}
}

func (w *disconnectTrackingRunEventStreamWriter) WriteEvent(id, eventType string, data []byte) error {
	err := w.writer.WriteEvent(id, eventType, data)
	if err != nil {
		w.disconnected = true
	}
	return err
}

func (w *disconnectTrackingRunEventStreamWriter) WriteKeepAlive() error {
	err := w.writer.WriteKeepAlive()
	if err != nil {
		w.disconnected = true
	}
	return err
}

func (w *disconnectTrackingRunEventStreamWriter) disconnectedFrom(ctx context.Context) bool {
	return w.disconnected || ctx.Err() != nil
}

func runEventStreamKeepAliveInterval(time.Duration) time.Duration {
	return defaultRunEventStreamKeepAliveInterval
}

func cancelRunAfterStreamDisconnect(ctx context.Context, runID int64) {
	if runID <= 0 {
		return
	}
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runStreamDisconnectCancelTimeout)
	defer cancel()

	if _, err := appagentthread.SVC.CancelRunOnDisconnect(cancelCtx, &appagentthread.CancelRunOnDisconnectRequest{
		RunID: runID,
	}); err != nil {
		logs.CtxWarnf(cancelCtx, "cancel run after stream disconnect failed, run_id=%d, err=%v", runID, err)
	}
}
