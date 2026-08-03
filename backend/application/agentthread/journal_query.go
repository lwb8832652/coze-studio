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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

type JournalQueryRepository interface {
	GetJournalBootstrap(
		context.Context,
		domainrepo.GetJournalBootstrapRequest,
	) (*domainrepo.GetJournalBootstrapResult, error)
}

type JournalContentType = domainentity.JournalSnapshotContentType

const (
	JournalContentTypeDocument = domainentity.JournalSnapshotContentTypeDocument
	JournalContentTypeTerminal = domainentity.JournalSnapshotContentTypeTerminal
	JournalContentTypeCode     = domainentity.JournalSnapshotContentTypeCode
	JournalContentTypeSkill    = domainentity.JournalSnapshotContentTypeSkill
	JournalContentTypeBrowser  = domainentity.JournalSnapshotContentTypeBrowser
)

var journalFixedContentTypes = []JournalContentType{
	JournalContentTypeDocument,
	JournalContentTypeTerminal,
	JournalContentTypeCode,
	JournalContentTypeSkill,
	JournalContentTypeBrowser,
}

type GetJournalBootstrapRequest struct {
	ViewerID         int64
	SpaceID          int64
	ThreadID         int64
	RunID            int64
	AttemptID        string
	AfterSequence    uint64
	AfterSequenceSet bool
	AfterEventID     int64
	Limit            int
}

type JournalBootstrapResult struct {
	Attempts              []*domainentity.RunAttempt
	SelectedAttempt       *domainentity.RunAttempt
	Events                []*domainentity.JournalEvent
	LatestSequence        uint64
	ResolvedAfterSequence uint64
	HasMore               bool
	JournalEnabled        bool
	SnapshotsEnabled      bool
	ContentTypes          []JournalContentType
}

func (s *ApplicationService) GetJournalBootstrap(
	ctx context.Context,
	req GetJournalBootstrapRequest,
) (*JournalBootstrapResult, error) {
	if s == nil || s.JournalQueryRepository == nil || s.ThreadAuthorizer == nil ||
		s.WorkspaceAuthorizer == nil {
		return nil, fmt.Errorf("journal query service is unavailable")
	}
	afterSequenceSet := req.AfterSequenceSet || req.AfterSequence > 0
	if req.ViewerID <= 0 || req.SpaceID <= 0 || req.ThreadID <= 0 ||
		req.RunID <= 0 || afterSequenceSet && strings.TrimSpace(req.AttemptID) == "" {
		return nil, fmt.Errorf("journal query identity is invalid")
	}
	if s.JournalFeatureGate != nil {
		enabled, err := s.JournalFeatureGate.Enabled(ctx, JournalFeatureUI, req.SpaceID)
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, domainrepo.ErrJournalNotEnrolled
		}
	}
	// Journal streams are long-lived. Shadow the request-local Thread/Run cache so
	// each pull observes deletion, ownership, and workspace membership changes.
	authorizationCtx := context.WithValue(
		ctx,
		threadAccessContextKey{},
		(*threadAccessContextState)(nil),
	)
	if err := s.ThreadAuthorizer.AuthorizeThreadAccess(authorizationCtx, ThreadAccessRequest{
		ViewerID: req.ViewerID, SpaceID: req.SpaceID,
		ThreadID: req.ThreadID, RunID: req.RunID,
	}); err != nil {
		return nil, err
	}
	if err := s.WorkspaceAuthorizer.AuthorizeWorkspaceAccess(authorizationCtx, WorkspaceAccessRequest{
		ViewerID: req.ViewerID, SpaceID: req.SpaceID,
	}); err != nil {
		return nil, err
	}

	bootstrap, err := s.JournalQueryRepository.GetJournalBootstrap(
		ctx,
		domainrepo.GetJournalBootstrapRequest{
			RunID: req.RunID, AttemptID: strings.TrimSpace(req.AttemptID),
			AfterSequence: req.AfterSequence, AfterSequenceSet: afterSequenceSet,
			AfterEventID: req.AfterEventID,
			Limit:        req.Limit,
		},
	)
	if err != nil {
		return nil, err
	}
	if bootstrap == nil || bootstrap.SelectedAttempt == nil ||
		bootstrap.SelectedAttempt.ThreadID != req.ThreadID ||
		bootstrap.SelectedAttempt.JournalRunID != req.RunID {
		return nil, fmt.Errorf("journal repository returned an invalid bootstrap")
	}
	if err := s.authorizeJournalBootstrapEvents(ctx, req, bootstrap.Events); err != nil {
		return nil, err
	}
	selected := bootstrap.SelectedAttempt
	if s.JournalFeatureGate != nil && selected.Status.IsActive() {
		projectionEnabled, gateErr := s.JournalFeatureGate.MasterEnabled(ctx, JournalFeatureProjection)
		if gateErr != nil {
			return nil, gateErr
		}
		if !projectionEnabled {
			if s.JournalProjectionController == nil {
				return nil, fmt.Errorf("journal projection controller is unavailable")
			}
			disabled, _, disableErr := s.JournalProjectionController.DisableActiveJournalProjection(
				ctx,
				req.RunID,
				time.Now().UnixMilli(),
			)
			if disableErr != nil && !errors.Is(disableErr, domainrepo.ErrJournalAttemptTerminal) {
				return nil, disableErr
			}
			selected = cloneJournalRunAttempt(selected)
			selected.ProjectionState = domainentity.JournalProjectionStateDisabled
			if disabled != nil {
				selected = disabled
			}
		}
	}
	journalEnabled := selected.ProjectionState != domainentity.JournalProjectionStateDisabled
	snapshotsEnabled := journalEnabled &&
		selected.ProjectionState == domainentity.JournalProjectionStateHealthy &&
		selected.SnapshotsEnabled
	attempts := make([]*domainentity.RunAttempt, 0, len(bootstrap.Attempts))
	for _, attempt := range bootstrap.Attempts {
		cloned := cloneJournalRunAttempt(attempt)
		if cloned != nil && cloned.AttemptID == selected.AttemptID {
			cloned = cloneJournalRunAttempt(selected)
		}
		attempts = append(attempts, cloned)
	}
	return &JournalBootstrapResult{
		Attempts: attempts, SelectedAttempt: selected,
		Events: bootstrap.Events, LatestSequence: bootstrap.LatestSequence,
		ResolvedAfterSequence: bootstrap.ResolvedAfterSequence,
		HasMore:               bootstrap.HasMore, JournalEnabled: journalEnabled,
		SnapshotsEnabled: snapshotsEnabled,
		ContentTypes:     append([]JournalContentType(nil), journalFixedContentTypes...),
	}, nil
}

func cloneJournalRunAttempt(attempt *domainentity.RunAttempt) *domainentity.RunAttempt {
	if attempt == nil {
		return nil
	}
	cloned := *attempt
	return &cloned
}

func (s *ApplicationService) authorizeJournalBootstrapEvents(
	ctx context.Context,
	req GetJournalBootstrapRequest,
	events []*domainentity.JournalEvent,
) error {
	for _, event := range events {
		if event == nil || event.ThreadID != req.ThreadID ||
			event.JournalRunID != req.RunID ||
			event.Visibility != domainentity.JournalVisibilityUser {
			return fmt.Errorf("journal repository returned an invalid event page")
		}
		if event.SnapshotID != "" {
			if err := s.authorizeJournalEventSnapshot(ctx, req, event); err != nil {
				return err
			}
		}
		if event.EventType == "artifact.created" {
			artifactID, err := journalArtifactEventID(event.Payload)
			if err != nil {
				return ErrJournalSnapshotUnavailable
			}
			if err := s.authorizeJournalEventArtifact(ctx, req, event, artifactID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ApplicationService) authorizeJournalEventSnapshot(
	ctx context.Context,
	req GetJournalBootstrapRequest,
	event *domainentity.JournalEvent,
) error {
	if s.JournalSnapshotRepository == nil {
		return ErrJournalSnapshotUnavailable
	}
	snapshot, err := s.JournalSnapshotRepository.GetJournalSnapshot(
		ctx,
		domainrepo.GetJournalSnapshotRequest{
			SpaceID: req.SpaceID, ThreadID: req.ThreadID,
			RunID: req.RunID, SnapshotID: event.SnapshotID,
		},
	)
	if err != nil || snapshot == nil {
		return ErrJournalSnapshotUnavailable
	}
	if snapshot.EventID != event.ID || snapshot.ThreadID != event.ThreadID ||
		snapshot.RunID != event.RunID || snapshot.JournalRunID != event.JournalRunID ||
		snapshot.AttemptID != event.AttemptID ||
		snapshot.Visibility != domainentity.JournalVisibilityUser {
		return ErrJournalSnapshotUnavailable
	}
	return s.authorizeJournalSnapshot(ctx, snapshot, req.ViewerID)
}

func (s *ApplicationService) authorizeJournalEventArtifact(
	ctx context.Context,
	req GetJournalBootstrapRequest,
	event *domainentity.JournalEvent,
	artifactID int64,
) error {
	if s.JournalSnapshotArtifactReader == nil || s.ArtifactAuthorizer == nil {
		return ErrJournalSnapshotNoPermission
	}
	artifact, err := s.JournalSnapshotArtifactReader.GetArtifact(
		ctx,
		event.ThreadID,
		artifactID,
	)
	if err != nil || artifact == nil || artifact.ID != artifactID ||
		artifact.SpaceID != req.SpaceID || artifact.ThreadID != event.ThreadID ||
		artifact.RunID != event.RunID || artifact.DeletedAt != 0 {
		return ErrJournalSnapshotNoPermission
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID: event.ThreadID, ArtifactID: artifactID,
		SpaceID: req.SpaceID, ViewerID: req.ViewerID,
		Operation: ArtifactAccessOperationRead,
	}); err != nil {
		return ErrJournalSnapshotNoPermission
	}
	return nil
}

func journalArtifactEventID(payload string) (int64, error) {
	var envelope struct {
		Type string `json:"type"`
		Data struct {
			ArtifactID json.RawMessage `json:"artifact_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil ||
		envelope.Type != "artifact" || len(envelope.Data.ArtifactID) == 0 {
		return 0, fmt.Errorf("journal artifact payload is invalid")
	}
	rawID := strings.TrimSpace(string(envelope.Data.ArtifactID))
	if strings.HasPrefix(rawID, `"`) {
		var stringID string
		if err := json.Unmarshal(envelope.Data.ArtifactID, &stringID); err != nil {
			return 0, fmt.Errorf("journal artifact id is invalid")
		}
		rawID = strings.TrimSpace(stringID)
	}
	artifactID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || artifactID <= 0 {
		return 0, fmt.Errorf("journal artifact id is invalid")
	}
	return artifactID, nil
}
