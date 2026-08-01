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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	journalcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/journal_contract"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	canonicalJournalStreamPageSize     = 200
	canonicalJournalStreamQueueSize    = 64
	canonicalJournalStreamCloseTimeout = time.Second
)

var (
	errCanonicalJournalSlowConsumer  = errors.New("journal stream consumer is too slow")
	errCanonicalJournalStreamAborted = errors.New("journal stream aborted")
)

func reconnectCanonicalJournalStream(
	ctx context.Context,
	c *app.RequestContext,
	requestLog *canonicalRequestLog,
	threadID int64,
	runID int64,
	protocolVersion string,
) {
	afterEventID, public := canonicalReconnectRunEventCursor(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	afterSequence, public := canonicalJournalSequenceCursor(c, "after_sequence")
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	_, afterSequenceSet := c.GetQuery("after_sequence")
	attemptID := strings.TrimSpace(canonicalQueryString(c, "attempt_id"))
	if afterSequenceSet && attemptID == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *canonicalInvalidRequest(
			"attempt_id is required with after_sequence",
			"journal_attempt_cursor_missing",
		))
		return
	}
	requestLog.AfterEventID = afterEventID
	requestLog.StreamModes = string(canonicalJournalProtocolV11)

	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
		return
	}

	lease, ok := acquireCanonicalJournalAdmission(ctx, c, appagentthread.JournalAdmissionRequest{
		SpaceID: canonicalSpaceIDFromContext(ctx), ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID, RunID: runID, Kind: appagentthread.JournalAdmissionKindStream,
	})
	if !ok {
		return
	}
	defer releaseCanonicalJournalAdmission(ctx, lease)

	bootstrap, err := appagentthread.SVC.GetJournalBootstrap(
		ctx,
		appagentthread.GetJournalBootstrapRequest{
			ViewerID: workbenchViewerIDFromCtx(ctx), SpaceID: canonicalSpaceIDFromContext(ctx),
			ThreadID: threadID, RunID: runID, AttemptID: attemptID,
			AfterSequence: afterSequence, AfterSequenceSet: afterSequenceSet,
			AfterEventID: afterEventID,
			Limit:        canonicalJournalStreamPageSize,
		},
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if bootstrap == nil || bootstrap.SelectedAttempt == nil {
		writeCanonicalJournalApplicationError(ctx, c, fmt.Errorf("journal stream bootstrap is empty"))
		return
	}
	attemptID = bootstrap.SelectedAttempt.AttemptID

	c.Header("Content-Location", canonicalRunPath(threadID, runID))
	c.Header("Location", canonicalRunStreamPath(threadID, runID))
	streamWriter := canonicalRunStreamWriterFactory(c)
	if streamWriter.writer == nil {
		writeCanonicalJournalApplicationError(ctx, c, fmt.Errorf("canonical run stream writer is unavailable"))
		return
	}
	setCanonicalRunStreamHeaders(c)
	c.Response.Header.Set("Cache-Control", "private, no-store")
	defer func() {
		if streamWriter.close != nil && streamWriter.close() != nil {
			logs.CtxWarnf(
				ctx,
				"event_name=workbench.journal.stream.close_failed client_contract=%s thread_id=%d run_id=%d",
				canonicalContractVersion,
				threadID,
				runID,
			)
		}
	}()
	boundedWriter := newBoundedCanonicalJournalStreamWriter(
		streamWriter.writer,
		canonicalJournalStreamQueueSize,
	)
	defer func() {
		if err := boundedWriter.Close(); err != nil {
			logCanonicalJournalSlowConsumer(ctx, run, err)
		}
	}()
	streamCanonicalJournalEvents(ctx, boundedWriter, run, bootstrap, canonicalJournalStreamConfig{
		ViewerID: workbenchViewerIDFromCtx(ctx), SpaceID: canonicalSpaceIDFromContext(ctx),
		AttemptID: attemptID, AfterSequence: afterSequence, AfterEventID: afterEventID,
		ProtocolVersion: protocolVersion, AdmissionLease: lease,
	})
}

type canonicalJournalStreamConfig struct {
	ViewerID          int64
	SpaceID           int64
	AttemptID         string
	AfterSequence     uint64
	AfterEventID      int64
	ProtocolVersion   string
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	Timeout           time.Duration
	AdmissionLease    appagentthread.JournalAdmissionLease
}

func (c canonicalJournalStreamConfig) normalized() canonicalJournalStreamConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Duration(defaultRunEventStreamIntervalMs) * time.Millisecond
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = defaultRunEventStreamKeepAliveInterval
	}
	if c.Timeout <= 0 {
		c.Timeout = time.Duration(defaultRunEventStreamTimeoutMs) * time.Millisecond
	}
	if c.ProtocolVersion == "" {
		c.ProtocolVersion = "1.1"
	}
	return c
}

type canonicalJournalQueuedWrite struct {
	id        string
	eventType string
	data      []byte
	keepAlive bool
}

type boundedCanonicalJournalStreamWriter struct {
	writer    runEventStreamWriter
	queue     chan canonicalJournalQueuedWrite
	done      chan struct{}
	abort     chan struct{}
	once      sync.Once
	abortOnce sync.Once

	mu       sync.Mutex
	writeErr error
	aborted  bool
}

func newBoundedCanonicalJournalStreamWriter(
	writer runEventStreamWriter,
	capacity int,
) *boundedCanonicalJournalStreamWriter {
	if capacity < 1 {
		capacity = 1
	}
	bounded := &boundedCanonicalJournalStreamWriter{
		writer: writer, queue: make(chan canonicalJournalQueuedWrite, capacity),
		done: make(chan struct{}), abort: make(chan struct{}),
	}
	go bounded.writeLoop()
	return bounded
}

func (w *boundedCanonicalJournalStreamWriter) WriteEvent(
	id string,
	eventType string,
	data []byte,
) error {
	return w.enqueue(canonicalJournalQueuedWrite{
		id: id, eventType: eventType, data: append([]byte(nil), data...),
	})
}

func (w *boundedCanonicalJournalStreamWriter) WriteKeepAlive() error {
	return w.enqueue(canonicalJournalQueuedWrite{keepAlive: true})
}

func (w *boundedCanonicalJournalStreamWriter) enqueue(
	write canonicalJournalQueuedWrite,
) error {
	if w == nil || w.writer == nil {
		return fmt.Errorf("journal stream writer is unavailable")
	}
	w.mu.Lock()
	err := w.writeErr
	aborted := w.aborted
	w.mu.Unlock()
	if err != nil {
		return err
	}
	if aborted {
		return errCanonicalJournalStreamAborted
	}
	select {
	case w.queue <- write:
		return nil
	default:
		return errCanonicalJournalSlowConsumer
	}
}

func (w *boundedCanonicalJournalStreamWriter) writeLoop() {
	defer close(w.done)
	for {
		select {
		case <-w.abort:
			return
		default:
		}
		var write canonicalJournalQueuedWrite
		var ok bool
		select {
		case <-w.abort:
			return
		case write, ok = <-w.queue:
			if !ok {
				return
			}
		}
		select {
		case <-w.abort:
			return
		default:
		}
		var err error
		if write.keepAlive {
			err = w.writer.WriteKeepAlive()
		} else {
			err = w.writer.WriteEvent(write.id, write.eventType, write.data)
		}
		if err == nil {
			continue
		}
		w.mu.Lock()
		w.writeErr = err
		w.mu.Unlock()
		return
	}
}

func (w *boundedCanonicalJournalStreamWriter) Abort() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.aborted = true
	w.mu.Unlock()
	w.abortOnce.Do(func() { close(w.abort) })
}

func (w *boundedCanonicalJournalStreamWriter) Close() error {
	if w == nil {
		return nil
	}
	w.once.Do(func() { close(w.queue) })
	select {
	case <-w.done:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.writeErr
	case <-time.After(canonicalJournalStreamCloseTimeout):
		return errCanonicalJournalSlowConsumer
	}
}

func streamCanonicalJournalEvents(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	run *appagentthread.RunSummary,
	initial *appagentthread.JournalBootstrapResult,
	config canonicalJournalStreamConfig,
) {
	if writer == nil || run == nil || run.ThreadID <= 0 || run.RunID <= 0 ||
		initial == nil || initial.SelectedAttempt == nil {
		return
	}
	config = config.normalized()
	if config.AttemptID == "" {
		config.AttemptID = initial.SelectedAttempt.AttemptID
	}
	state := &canonicalJournalStreamState{
		run: run, config: config, writer: writer,
		afterSequence: config.AfterSequence, afterEventID: config.AfterEventID,
	}
	if config.AfterEventID > 0 {
		state.afterSequence = initial.ResolvedAfterSequence
	}
	if !writeCanonicalJournalStreamMetadata(ctx, writer, run, initial, config.ProtocolVersion) {
		return
	}
	continueStreaming, terminal := state.consumeBootstrap(ctx, initial)
	if !continueStreaming {
		return
	}
	if terminal {
		writeCanonicalJournalStreamEnd(ctx, writer, initial.SelectedAttempt, initial.LatestSequence)
		return
	}

	pollTicker := time.NewTicker(config.PollInterval)
	defer pollTicker.Stop()
	heartbeatTicker := time.NewTicker(config.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	timer := time.NewTimer(config.Timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case <-pollTicker.C:
			continueStreaming, terminal = state.refresh(ctx)
			if !continueStreaming {
				return
			}
			if terminal {
				writeCanonicalJournalStreamEnd(ctx, writer, state.attempt, state.latestSequence)
				return
			}
		case <-heartbeatTicker.C:
			if config.AdmissionLease != nil {
				if err := config.AdmissionLease.Renew(ctx); err != nil {
					writeCanonicalJournalStreamControl(
						ctx, writer, journalcontract.JournalControlFrameTypeCapabilityUnavailable,
						state.attempt, state.latestSequence, "", true, config.ProtocolVersion,
					)
					return
				}
			}
			continueStreaming, terminal = state.refresh(ctx)
			if !continueStreaming {
				return
			}
			if terminal {
				writeCanonicalJournalStreamEnd(ctx, writer, state.attempt, state.latestSequence)
				return
			}
			if !writeCanonicalJournalStreamHeartbeat(
				ctx, writer, state.attempt, state.latestSequence,
			) {
				return
			}
		}
	}
}

type canonicalJournalStreamState struct {
	run    *appagentthread.RunSummary
	config canonicalJournalStreamConfig
	writer canonicalRunStreamProtocolWriter

	afterSequence  uint64
	afterEventID   int64
	latestSequence uint64
	attempt        *domainentity.RunAttempt
}

func (s *canonicalJournalStreamState) refresh(
	ctx context.Context,
) (continueStreaming bool, terminal bool) {
	bootstrap, err := appagentthread.SVC.GetJournalBootstrap(
		ctx,
		appagentthread.GetJournalBootstrapRequest{
			ViewerID: s.config.ViewerID, SpaceID: s.config.SpaceID,
			ThreadID: s.run.ThreadID, RunID: s.run.RunID, AttemptID: s.config.AttemptID,
			AfterSequence: s.afterSequence, AfterSequenceSet: true,
			AfterEventID: s.afterEventID,
			Limit:        canonicalJournalStreamPageSize,
		},
	)
	if err != nil {
		if canonicalJournalStreamAccessRevoked(err) {
			abortCanonicalJournalStreamWriter(s.writer)
			return false, false
		}
		controlType := journalcontract.JournalControlFrameTypeCapabilityUnavailable
		errorCode := ""
		retryable := true
		switch {
		case errors.Is(err, domainrepo.ErrJournalCursorExpired):
			errorCode = "JOURNAL_CURSOR_EXPIRED"
		case errors.Is(err, domainrepo.ErrJournalEventGap):
			errorCode = "JOURNAL_EVENT_GAP"
		}
		writeCanonicalJournalStreamControl(
			ctx, s.writer, controlType, s.attempt, s.latestSequence,
			errorCode, retryable, s.config.ProtocolVersion,
		)
		logCanonicalRunStreamFailure(ctx, "journal_refresh", s.run, err)
		return false, false
	}
	return s.consumeBootstrap(ctx, bootstrap)
}

func (s *canonicalJournalStreamState) consumeBootstrap(
	ctx context.Context,
	bootstrap *appagentthread.JournalBootstrapResult,
) (continueStreaming bool, terminal bool) {
	for {
		if bootstrap == nil || bootstrap.SelectedAttempt == nil ||
			bootstrap.SelectedAttempt.AttemptID != s.config.AttemptID ||
			bootstrap.SelectedAttempt.JournalRunID != s.run.RunID ||
			bootstrap.SelectedAttempt.ThreadID != s.run.ThreadID {
			writeCanonicalJournalStreamControl(
				ctx, s.writer, journalcontract.JournalControlFrameTypeCapabilityUnavailable,
				s.attempt, s.latestSequence, "JOURNAL_EVENT_GAP", true,
				s.config.ProtocolVersion,
			)
			return false, false
		}
		s.attempt = bootstrap.SelectedAttempt
		s.latestSequence = bootstrap.LatestSequence
		switch s.attempt.ProjectionState {
		case domainentity.JournalProjectionStateDisabled:
			writeCanonicalJournalStreamControl(
				ctx, s.writer, journalcontract.JournalControlFrameTypeJournalDisabled,
				s.attempt, s.latestSequence, "", false, s.config.ProtocolVersion,
			)
			return false, false
		case domainentity.JournalProjectionStateDegraded:
			writeCanonicalJournalStreamControl(
				ctx, s.writer, journalcontract.JournalControlFrameTypeJournalDegraded,
				s.attempt, s.latestSequence, "", false, s.config.ProtocolVersion,
			)
			return false, false
		}
		if bootstrap.LatestSequence < s.afterSequence {
			writeCanonicalJournalStreamControl(
				ctx, s.writer, journalcontract.JournalControlFrameTypeCapabilityUnavailable,
				s.attempt, s.latestSequence, "JOURNAL_EVENT_GAP", true,
				s.config.ProtocolVersion,
			)
			return false, false
		}
		progressed := false
		for _, event := range bootstrap.Events {
			if event == nil || event.Sequence <= s.afterSequence {
				continue
			}
			if event.AttemptID != s.config.AttemptID ||
				event.ThreadID != s.run.ThreadID || event.JournalRunID != s.run.RunID ||
				event.Visibility != domainentity.JournalVisibilityUser ||
				event.Sequence != s.afterSequence+1 {
				writeCanonicalJournalStreamControl(
					ctx, s.writer, journalcontract.JournalControlFrameTypeCapabilityUnavailable,
					s.attempt, s.latestSequence, "JOURNAL_EVENT_GAP", true,
					s.config.ProtocolVersion,
				)
				return false, false
			}
			if !writeCanonicalJournalStreamEvent(ctx, s.writer, event) {
				return false, false
			}
			s.afterSequence = event.Sequence
			s.afterEventID = event.ID
			progressed = true
		}
		if !bootstrap.HasMore {
			return true, s.attempt.Status.IsTerminal() && s.afterSequence >= s.latestSequence
		}
		if !progressed {
			writeCanonicalJournalStreamControl(
				ctx, s.writer, journalcontract.JournalControlFrameTypeCapabilityUnavailable,
				s.attempt, s.latestSequence, "JOURNAL_EVENT_GAP", true,
				s.config.ProtocolVersion,
			)
			return false, false
		}
		var err error
		bootstrap, err = appagentthread.SVC.GetJournalBootstrap(
			ctx,
			appagentthread.GetJournalBootstrapRequest{
				ViewerID: s.config.ViewerID, SpaceID: s.config.SpaceID,
				ThreadID: s.run.ThreadID, RunID: s.run.RunID, AttemptID: s.config.AttemptID,
				AfterSequence: s.afterSequence, AfterSequenceSet: true,
				AfterEventID: s.afterEventID,
				Limit:        canonicalJournalStreamPageSize,
			},
		)
		if err != nil {
			if canonicalJournalStreamAccessRevoked(err) {
				abortCanonicalJournalStreamWriter(s.writer)
			} else {
				writeCanonicalJournalStreamControl(
					ctx, s.writer, journalcontract.JournalControlFrameTypeCapabilityUnavailable,
					s.attempt, s.latestSequence, "", true, s.config.ProtocolVersion,
				)
				logCanonicalRunStreamFailure(ctx, "journal_backfill", s.run, err)
			}
			return false, false
		}
	}
}

func canonicalJournalStreamAccessRevoked(err error) bool {
	return errors.Is(err, appagentthread.ErrThreadAccessDenied) ||
		errors.Is(err, appagentthread.ErrJournalSnapshotNoPermission)
}

func abortCanonicalJournalStreamWriter(writer canonicalRunStreamProtocolWriter) {
	if aborter, ok := writer.(interface{ Abort() }); ok {
		aborter.Abort()
	}
}

func logCanonicalJournalSlowConsumer(
	ctx context.Context,
	run *appagentthread.RunSummary,
	err error,
) {
	if !errors.Is(err, errCanonicalJournalSlowConsumer) || run == nil {
		return
	}
	logs.CtxWarnf(
		ctx,
		"event_name=workbench.journal.stream.slow_consumer client_contract=%s thread_id=%d run_id=%d",
		canonicalContractVersion,
		run.ThreadID,
		run.RunID,
	)
}
