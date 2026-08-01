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

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	defaultJournalSnapshotFragmentLimit = 100
	maxJournalSnapshotFragmentLimit     = 200
	maxJournalSnapshotSummaryBytes      = 128 * 1024
)

func (r *threadRepository) ReserveJournalSnapshot(
	ctx context.Context,
	req ReserveJournalSnapshotRequest,
) (*ReserveJournalSnapshotResult, error) {
	reservation, now, err := normalizeJournalSnapshotReservation(req)
	if err != nil {
		return nil, err
	}
	result := &ReserveJournalSnapshotResult{}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		executionRoot, executionPath, err := resolveJournalRunPath(tx, reservation.RunID)
		if err != nil {
			return err
		}
		if executionRoot.ThreadID != reservation.ThreadID || executionRoot.SpaceID != reservation.SpaceID {
			return ErrJournalSnapshotConflict
		}
		attempt, err := lockJournalAttemptForEvent(
			tx,
			executionRoot.ID,
			executionPath,
			reservation.AttemptID,
		)
		if err != nil {
			return err
		}
		reservation.JournalRunID = attempt.JournalRunID
		reservation.AttemptID = attempt.AttemptID
		var existing journalSnapshotPO
		lookupErr := tx.Where(
			"journal_run_id = ? AND attempt_id = ? AND action_id = ? AND revision = ?",
			reservation.JournalRunID,
			reservation.AttemptID,
			reservation.ActionID,
			reservation.Revision,
		).First(&existing).Error
		if lookupErr == nil {
			if existing.SnapshotID != reservation.SnapshotID ||
				existing.SpaceID != reservation.SpaceID || existing.ThreadID != reservation.ThreadID ||
				existing.RunID != reservation.RunID || existing.ContentHash != reservation.ContentHash ||
				existing.ACLDomain != reservation.ACLDomain {
				return ErrJournalSnapshotConflict
			}
			result.Snapshot = existing.toEntity()
			result.Event, err = loadJournalSnapshotEvent(tx, existing.EventID)
			if err != nil {
				return err
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		if !attempt.SnapshotsEnabled {
			return fmt.Errorf("journal content snapshots are disabled for this attempt")
		}
		if !entity.RunAttemptStatus(attempt.Status).IsActive() {
			return ErrJournalAttemptTerminal
		}
		if entity.JournalProjectionState(attempt.ProjectionState) !=
			entity.JournalProjectionStateHealthy {
			return ErrJournalProjectionInactive
		}

		query := tx.Where("snapshot_id = ?", reservation.SnapshotID)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var current journalSnapshotReservationPO
		lookupErr = query.First(&current).Error
		switch {
		case errors.Is(lookupErr, gorm.ErrRecordNotFound):
			po := journalSnapshotReservationToPO(reservation)
			if err := tx.Create(po).Error; err != nil {
				return err
			}
			result.Reservation = po.toEntity()
			return nil
		case lookupErr != nil:
			return lookupErr
		case !journalSnapshotReservationIdentityMatches(current.toEntity(), reservation):
			return ErrJournalSnapshotConflict
		case current.ExpiresAt > now:
			result.Reservation = current.toEntity()
			return nil
		default:
			po := journalSnapshotReservationToPO(reservation)
			if err := tx.Model(&journalSnapshotReservationPO{}).
				Where("snapshot_id = ? AND expires_at <= ?", reservation.SnapshotID, now).
				Updates(map[string]any{
					"reservation_token": po.ReservationToken,
					"event_id":          po.EventID,
					"idempotency_key":   po.IdempotencyKey,
					"content_hash":      po.ContentHash,
					"acl_domain":        po.ACLDomain,
					"staging_prefix":    po.StagingPrefix,
					"expires_at":        po.ExpiresAt,
					"created_at":        po.CreatedAt,
				}).Error; err != nil {
				return err
			}
			result.Reservation = cloneJournalSnapshotReservation(reservation)
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *threadRepository) DeleteExpiredJournalSnapshotReservations(
	ctx context.Context,
	spaceID int64,
	now int64,
	limit int,
) (int64, error) {
	if spaceID <= 0 || now <= 0 || limit <= 0 || limit > 1_000 {
		return 0, fmt.Errorf("invalid journal snapshot reservation cleanup request")
	}
	var snapshotIDs []string
	err := r.db.WithContext(ctx).Model(&journalSnapshotReservationPO{}).
		Where("space_id = ? AND expires_at <= ?", spaceID, now).
		Order("expires_at ASC").
		Limit(limit).
		Pluck("snapshot_id", &snapshotIDs).Error
	if err != nil || len(snapshotIDs) == 0 {
		return 0, err
	}
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND expires_at <= ? AND snapshot_id IN ?", spaceID, now, snapshotIDs).
		Delete(&journalSnapshotReservationPO{})
	return result.RowsAffected, result.Error
}

func (r *threadRepository) CreateJournalSnapshot(
	ctx context.Context,
	req CreateJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, *entity.JournalEvent, bool, error) {
	snapshot, fragments, event, err := normalizeJournalSnapshotCreateRequest(req)
	if err != nil {
		return nil, nil, false, err
	}

	var created *entity.JournalContentSnapshot
	var appended *entity.JournalEvent
	var replayed bool
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		executionRoot, executionPath, err := resolveJournalRunPath(tx, snapshot.RunID)
		if err != nil {
			return err
		}
		if executionRoot.ThreadID != snapshot.ThreadID || executionRoot.SpaceID != snapshot.SpaceID {
			return ErrJournalSnapshotConflict
		}
		attempt, err := lockJournalAttemptForEvent(
			tx,
			executionRoot.ID,
			executionPath,
			snapshot.AttemptID,
		)
		if err != nil {
			return err
		}
		if !attempt.SnapshotsEnabled {
			return fmt.Errorf("journal content snapshots are disabled for this attempt")
		}
		if !entity.RunAttemptStatus(attempt.Status).IsActive() {
			return ErrJournalAttemptTerminal
		}
		if entity.JournalProjectionState(attempt.ProjectionState) !=
			entity.JournalProjectionStateHealthy {
			return ErrJournalProjectionInactive
		}

		snapshot.JournalRunID = attempt.JournalRunID
		snapshot.AttemptID = attempt.AttemptID
		event.JournalRunID = attempt.JournalRunID
		event.AttemptID = attempt.AttemptID
		event.SnapshotID = snapshot.SnapshotID

		var existing journalSnapshotPO
		lookupErr := tx.Where("snapshot_id = ?", snapshot.SnapshotID).First(&existing).Error
		if lookupErr == nil {
			if !journalSnapshotReplayMatches(existing.toEntity(), snapshot) {
				return ErrJournalSnapshotConflict
			}
			created = existing.toEntity()
			appended, err = loadJournalSnapshotEvent(tx, existing.EventID)
			if err != nil {
				return err
			}
			replayed = true
			return nil
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}

		reservationToken := strings.TrimSpace(req.ReservationToken)
		if journalSnapshotHasStagedObjects(fragments, snapshot) || reservationToken != "" {
			reservation, reservationErr := lockJournalSnapshotReservation(
				tx,
				snapshot.SnapshotID,
				reservationToken,
			)
			if reservationErr != nil {
				return reservationErr
			}
			if reservation.ExpiresAt <= time.Now().UnixMilli() {
				return ErrJournalSnapshotReservationExpired
			}
			if reservation.SpaceID != snapshot.SpaceID ||
				reservation.ThreadID != snapshot.ThreadID ||
				reservation.RunID != snapshot.RunID ||
				reservation.JournalRunID != snapshot.JournalRunID ||
				reservation.AttemptID != snapshot.AttemptID ||
				reservation.ActionID != snapshot.ActionID ||
				reservation.Revision != snapshot.Revision ||
				reservation.EventID != snapshot.EventID ||
				reservation.IdempotencyKey != event.IdempotencyKey ||
				reservation.ContentHash != snapshot.ContentHash ||
				reservation.ACLDomain != snapshot.ACLDomain ||
				!journalSnapshotObjectsMatchReservation(
					reservation.StagingPrefix,
					fragments,
					snapshot,
				) {
				return ErrJournalSnapshotConflict
			}
		}
		po := journalSnapshotToPO(snapshot)
		if err := tx.Create(po).Error; err != nil {
			return err
		}
		for _, fragment := range fragments {
			fragment.SnapshotID = snapshot.SnapshotID
			if err := tx.Create(journalSnapshotFragmentToPO(fragment)).Error; err != nil {
				return err
			}
		}
		appended, err = appendJournalEventLocked(tx, attempt, event)
		if err != nil {
			return err
		}
		if appended.ID != snapshot.EventID || appended.SnapshotID != snapshot.SnapshotID {
			return ErrJournalSnapshotConflict
		}
		if reservationToken != "" {
			if err := tx.Where(
				"snapshot_id = ? AND reservation_token = ?",
				snapshot.SnapshotID,
				reservationToken,
			).Delete(&journalSnapshotReservationPO{}).Error; err != nil {
				return err
			}
		}
		created = cloneJournalSnapshot(snapshot)
		created.Fragments = cloneJournalSnapshotFragments(fragments)
		return nil
	})
	if err == nil {
		return created, appended, replayed, nil
	}

	if existing, lookupErr := r.GetJournalSnapshot(ctx, GetJournalSnapshotRequest{
		SpaceID: snapshot.SpaceID, ThreadID: snapshot.ThreadID,
		RunID: snapshot.JournalRunID, SnapshotID: snapshot.SnapshotID,
	}); lookupErr == nil && journalSnapshotReplayMatches(existing, snapshot) {
		existingEvent, eventErr := r.GetJournalEvent(ctx, existing.EventID)
		if eventErr == nil {
			return existing, existingEvent, true, nil
		}
	}
	return nil, nil, false, err
}

func journalSnapshotReplayMatches(
	existing *entity.JournalContentSnapshot,
	candidate *entity.JournalContentSnapshot,
) bool {
	return existing != nil && candidate != nil &&
		existing.SnapshotID == candidate.SnapshotID &&
		existing.SpaceID == candidate.SpaceID &&
		existing.ThreadID == candidate.ThreadID &&
		existing.RunID == candidate.RunID &&
		existing.JournalRunID == candidate.JournalRunID &&
		existing.AttemptID == candidate.AttemptID &&
		existing.ActionID == candidate.ActionID &&
		existing.Revision == candidate.Revision &&
		existing.ContentType == candidate.ContentType &&
		existing.Status == candidate.Status &&
		existing.Visibility == candidate.Visibility &&
		existing.ErrorCode == candidate.ErrorCode &&
		existing.MIMEType == candidate.MIMEType &&
		existing.Encoding == candidate.Encoding &&
		existing.Compression == candidate.Compression &&
		existing.SummaryHash == candidate.SummaryHash &&
		existing.ContentLength == candidate.ContentLength &&
		existing.ContentHash == candidate.ContentHash &&
		existing.ACLDomain == candidate.ACLDomain &&
		existing.SourceResourceType == candidate.SourceResourceType &&
		existing.SourceResourceID == candidate.SourceResourceID &&
		existing.SourceRevision == candidate.SourceRevision &&
		existing.ExpiresAt == candidate.ExpiresAt
}

func (r *threadRepository) GetJournalSnapshot(
	ctx context.Context,
	req GetJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, error) {
	if req.SpaceID <= 0 || req.ThreadID <= 0 || req.RunID <= 0 ||
		strings.TrimSpace(req.SnapshotID) == "" {
		return nil, ErrJournalSnapshotNotFound
	}
	var po journalSnapshotPO
	err := r.db.WithContext(ctx).Where(
		"space_id = ? AND thread_id = ? AND journal_run_id = ? AND snapshot_id = ? AND cleanup_state = ? AND deleted_at IS NULL",
		req.SpaceID, req.ThreadID, req.RunID, strings.TrimSpace(req.SnapshotID),
		entity.JournalSnapshotCleanupStateActive,
	).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrJournalSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *threadRepository) ListJournalSnapshotFragments(
	ctx context.Context,
	req ListJournalSnapshotFragmentsRequest,
) (*ListJournalSnapshotFragmentsResult, error) {
	if req.SpaceID <= 0 || strings.TrimSpace(req.SnapshotID) == "" {
		return nil, ErrJournalSnapshotNotFound
	}
	limit := req.Limit
	if limit == 0 {
		limit = defaultJournalSnapshotFragmentLimit
	}
	if limit < 1 || limit > maxJournalSnapshotFragmentLimit || req.AfterIndex < -1 {
		return nil, fmt.Errorf("journal snapshot fragment pagination is invalid")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&journalSnapshotPO{}).Where(
		"space_id = ? AND snapshot_id = ? AND cleanup_state = ? AND deleted_at IS NULL",
		req.SpaceID, strings.TrimSpace(req.SnapshotID),
		entity.JournalSnapshotCleanupStateActive,
	).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrJournalSnapshotNotFound
	}

	var rows []journalSnapshotFragmentPO
	err := r.db.WithContext(ctx).Where(
		"snapshot_id = ? AND fragment_index > ?",
		strings.TrimSpace(req.SnapshotID), req.AfterIndex,
	).Order("fragment_index ASC").Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := &ListJournalSnapshotFragmentsResult{NextIndex: req.AfterIndex}
	if len(rows) > limit {
		result.HasMore = true
		rows = rows[:limit]
	}
	result.Fragments = make([]*entity.JournalSnapshotFragment, 0, len(rows))
	for i := range rows {
		fragment := rows[i].toEntity()
		result.Fragments = append(result.Fragments, fragment)
		result.NextIndex = fragment.FragmentIndex
	}
	return result, nil
}

func (r *threadRepository) RecordJournalSnapshotAccess(
	ctx context.Context,
	audit *entity.JournalSnapshotAccessAudit,
) (*entity.JournalSnapshotAccessAudit, bool, error) {
	normalized, err := normalizeJournalSnapshotAccessAudit(audit)
	if err != nil {
		return nil, false, err
	}
	po := journalSnapshotAccessAuditToPO(normalized)
	err = r.db.WithContext(ctx).Create(po).Error
	if err == nil {
		return po.toEntity(), false, nil
	}

	var existing journalSnapshotAccessAuditPO
	lookupErr := r.db.WithContext(ctx).Where(
		"space_id = ? AND snapshot_id = ? AND action = ? AND actor_id = ? AND idempotency_key = ?",
		normalized.SpaceID, normalized.SnapshotID, normalized.Action,
		normalized.ActorID, normalized.IdempotencyKey,
	).First(&existing).Error
	if lookupErr == nil {
		return existing.toEntity(), true, nil
	}
	return nil, false, err
}

func (r *threadRepository) IsJournalSnapshotObjectProtected(
	ctx context.Context,
	spaceID int64,
	objectKey string,
	now int64,
) (bool, error) {
	objectKey = strings.TrimSpace(objectKey)
	if spaceID <= 0 || objectKey == "" || now <= 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&journalSnapshotPO{}).
		Where("space_id = ? AND (object_key = ? OR original_object_key = ?)", spaceID, objectKey, objectKey).
		Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	err = r.db.WithContext(ctx).Table("agent_journal_snapshot_fragments AS f").
		Joins("JOIN agent_journal_snapshots AS s ON s.snapshot_id = f.snapshot_id").
		Where("s.space_id = ? AND f.object_key = ?", spaceID, objectKey).
		Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	stagingPrefix := path.Dir(objectKey) + "/"
	err = r.db.WithContext(ctx).Model(&journalSnapshotReservationPO{}).
		Where(
			"space_id = ? AND staging_prefix = ? AND expires_at > ?",
			spaceID,
			stagingPrefix,
			now,
		).
		Count(&count).Error
	return count > 0, err
}

func normalizeJournalSnapshotCreateRequest(
	req CreateJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, []*entity.JournalSnapshotFragment, *entity.JournalEvent, error) {
	if req.Snapshot == nil || req.Event == nil {
		return nil, nil, nil, fmt.Errorf("journal snapshot and event are required")
	}
	snapshot := cloneJournalSnapshot(req.Snapshot)
	snapshot.SnapshotID = strings.TrimSpace(snapshot.SnapshotID)
	snapshot.AttemptID = strings.TrimSpace(snapshot.AttemptID)
	snapshot.ActionID = strings.TrimSpace(snapshot.ActionID)
	snapshot.MIMEType = strings.TrimSpace(snapshot.MIMEType)
	snapshot.Encoding = strings.TrimSpace(snapshot.Encoding)
	snapshot.ContentHash = strings.TrimSpace(snapshot.ContentHash)
	snapshot.SummaryHash = strings.TrimSpace(snapshot.SummaryHash)
	snapshot.ACLDomain = strings.TrimSpace(snapshot.ACLDomain)
	snapshot.SourceRevision = strings.TrimSpace(snapshot.SourceRevision)
	if snapshot.Compression == "" {
		snapshot.Compression = entity.JournalSnapshotCompressionIdentity
	}
	if snapshot.CleanupState == "" {
		snapshot.CleanupState = entity.JournalSnapshotCleanupStateActive
	}
	if snapshot.SnapshotID == "" || snapshot.SpaceID <= 0 || snapshot.ThreadID <= 0 ||
		snapshot.RunID <= 0 || snapshot.EventID <= 0 || snapshot.ActionID == "" ||
		!snapshot.ContentType.Valid() || !snapshot.Status.Valid() ||
		snapshot.Visibility != entity.JournalVisibilityUser || snapshot.MIMEType == "" ||
		snapshot.Encoding == "" || !snapshot.Compression.Valid() ||
		snapshot.ContentHash == "" || snapshot.ACLDomain == "" ||
		snapshot.CleanupState != entity.JournalSnapshotCleanupStateActive || snapshot.DeletedAt != 0 {
		return nil, nil, nil, fmt.Errorf("journal snapshot contract is invalid")
	}
	if len(snapshot.SnapshotID) > 64 || len(snapshot.AttemptID) > 64 ||
		len(snapshot.ActionID) > 191 ||
		len(snapshot.MIMEType) > 191 || len(snapshot.Encoding) > 32 ||
		len(snapshot.Compression) > 32 || len(snapshot.CleanupState) > 32 ||
		len(snapshot.ContentHash) > 64 || len(snapshot.ACLDomain) > 191 ||
		len(snapshot.SourceResourceType) > 64 || len(snapshot.SourceResourceID) > 191 ||
		len(snapshot.SourceRevision) > 64 ||
		len(snapshot.SummaryJSON) > maxJournalSnapshotSummaryBytes ||
		len(snapshot.SummaryHash) > 64 ||
		len(snapshot.ObjectKey) > 1024 || len(snapshot.OriginalObjectKey) > 1024 {
		return nil, nil, nil, fmt.Errorf("journal snapshot field exceeds storage limits")
	}
	if snapshot.Revision == 0 {
		snapshot.Revision = 1
	}
	hasSourceType := snapshot.SourceResourceType != ""
	if hasSourceType != (snapshot.SourceResourceID != "") ||
		hasSourceType != (snapshot.SourceRevision != "") {
		return nil, nil, nil, fmt.Errorf("journal snapshot source identity is incomplete")
	}
	if snapshot.CreatedAt <= 0 {
		snapshot.CreatedAt = time.Now().UnixMilli()
	}
	if snapshot.ExpiresAt != snapshot.CreatedAt+entity.JournalSnapshotRetentionMillis {
		return nil, nil, nil, fmt.Errorf("journal snapshot retention is invalid")
	}
	event, err := normalizeJournalEvent(req.Event)
	if err != nil {
		return nil, nil, nil, err
	}
	if event.ID != snapshot.EventID || event.ThreadID != snapshot.ThreadID ||
		event.RunID != snapshot.RunID ||
		(event.SnapshotID != "" && event.SnapshotID != snapshot.SnapshotID) {
		return nil, nil, nil, ErrJournalSnapshotConflict
	}
	if len(event.IdempotencyKey) > 191 || len(event.TraceID) > 128 {
		return nil, nil, nil, fmt.Errorf("journal snapshot event field exceeds storage limits")
	}
	fragments := cloneJournalSnapshotFragments(req.Fragments)
	for _, fragment := range fragments {
		if fragment == nil || strings.TrimSpace(fragment.FragmentID) == "" ||
			fragment.FragmentIndex < 0 || !fragment.Kind.Valid() ||
			(fragment.SnapshotID != "" && fragment.SnapshotID != snapshot.SnapshotID) ||
			(fragment.InlineContent != "" && fragment.ObjectKey != "") {
			return nil, nil, nil, fmt.Errorf("journal snapshot fragment contract is invalid")
		}
		fragment.FragmentID = strings.TrimSpace(fragment.FragmentID)
		fragment.ObjectKey = strings.TrimSpace(fragment.ObjectKey)
		fragment.MetadataJSON = strings.TrimSpace(fragment.MetadataJSON)
		fragment.MIMEType = strings.TrimSpace(fragment.MIMEType)
		if len(fragment.FragmentID) > 64 || len(fragment.ObjectKey) > 1024 ||
			len(fragment.ContentHash) > 64 || len(fragment.MetadataJSON) > 64*1024 ||
			len(fragment.MIMEType) > 191 ||
			(fragment.MetadataJSON != "" && !json.Valid([]byte(fragment.MetadataJSON))) {
			return nil, nil, nil, fmt.Errorf("journal snapshot fragment field exceeds storage limits")
		}
		if fragment.CreatedAt <= 0 {
			fragment.CreatedAt = snapshot.CreatedAt
		}
	}
	if snapshot.IsFragmented != (len(fragments) > 0) {
		return nil, nil, nil, fmt.Errorf("journal snapshot fragmented state does not match fragments")
	}
	if snapshot.IsFragmented {
		if snapshot.ContentJSON != "" || snapshot.ObjectKey == "" ||
			snapshot.SummaryJSON == "" || len(snapshot.SummaryHash) != 64 ||
			!json.Valid([]byte(snapshot.SummaryJSON)) {
			return nil, nil, nil, fmt.Errorf(
				"fragmented journal snapshot must carry an immutable manifest and summary",
			)
		}
		summaryDigest := sha256.Sum256([]byte(snapshot.SummaryJSON))
		if snapshot.SummaryHash != hex.EncodeToString(summaryDigest[:]) {
			return nil, nil, nil, fmt.Errorf("journal snapshot summary hash is invalid")
		}
	} else {
		if (snapshot.ContentJSON == "") == (snapshot.ObjectKey == "") {
			return nil, nil, nil, fmt.Errorf("journal snapshot must use exactly one content storage mode")
		}
		if snapshot.SummaryJSON != "" || snapshot.SummaryHash != "" {
			return nil, nil, nil, fmt.Errorf("inline journal snapshot must not carry a fragment summary")
		}
	}
	if snapshot.FragmentCount != 0 && snapshot.FragmentCount != uint32(len(fragments)) {
		return nil, nil, nil, fmt.Errorf("journal snapshot fragment count does not match fragments")
	}
	snapshot.FragmentCount = uint32(len(fragments))
	return snapshot, fragments, event, nil
}

func normalizeJournalSnapshotReservation(
	req ReserveJournalSnapshotRequest,
) (*entity.JournalSnapshotReservation, int64, error) {
	if req.Reservation == nil {
		return nil, 0, fmt.Errorf("journal snapshot reservation is required")
	}
	reservation := cloneJournalSnapshotReservation(req.Reservation)
	reservation.SnapshotID = strings.TrimSpace(reservation.SnapshotID)
	reservation.ReservationToken = strings.TrimSpace(reservation.ReservationToken)
	reservation.AttemptID = strings.TrimSpace(reservation.AttemptID)
	reservation.ActionID = strings.TrimSpace(reservation.ActionID)
	reservation.IdempotencyKey = strings.TrimSpace(reservation.IdempotencyKey)
	reservation.ContentHash = strings.TrimSpace(reservation.ContentHash)
	reservation.ACLDomain = strings.TrimSpace(reservation.ACLDomain)
	reservation.StagingPrefix = strings.TrimSpace(reservation.StagingPrefix)
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	cleanPrefix := path.Clean(strings.TrimSuffix(reservation.StagingPrefix, "/")) + "/"
	if reservation.SnapshotID == "" || reservation.ReservationToken == "" ||
		reservation.SpaceID <= 0 || reservation.ThreadID <= 0 || reservation.RunID <= 0 ||
		reservation.ActionID == "" || reservation.Revision == 0 || reservation.EventID <= 0 ||
		reservation.IdempotencyKey == "" || reservation.ContentHash == "" ||
		reservation.ACLDomain == "" || reservation.StagingPrefix == "" ||
		reservation.ExpiresAt <= now || reservation.CreatedAt <= 0 ||
		cleanPrefix != reservation.StagingPrefix || strings.HasPrefix(cleanPrefix, "/") ||
		strings.HasPrefix(cleanPrefix, "../") {
		return nil, 0, fmt.Errorf("journal snapshot reservation contract is invalid")
	}
	if len(reservation.SnapshotID) > 64 || len(reservation.ReservationToken) > 64 ||
		len(reservation.AttemptID) > 64 || len(reservation.ActionID) > 191 ||
		len(reservation.IdempotencyKey) > 191 || len(reservation.ContentHash) > 64 ||
		len(reservation.ACLDomain) > 191 || len(reservation.StagingPrefix) > 1024 {
		return nil, 0, fmt.Errorf("journal snapshot reservation field exceeds storage limits")
	}
	return reservation, now, nil
}

func lockJournalSnapshotReservation(
	tx *gorm.DB,
	snapshotID string,
	reservationToken string,
) (*journalSnapshotReservationPO, error) {
	if strings.TrimSpace(reservationToken) == "" {
		return nil, ErrJournalSnapshotReservationExpired
	}
	query := tx.Where(
		"snapshot_id = ? AND reservation_token = ?",
		strings.TrimSpace(snapshotID),
		strings.TrimSpace(reservationToken),
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var reservation journalSnapshotReservationPO
	err := query.First(&reservation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrJournalSnapshotReservationExpired
	}
	if err != nil {
		return nil, err
	}
	return &reservation, nil
}

func journalSnapshotHasStagedObjects(
	fragments []*entity.JournalSnapshotFragment,
	snapshot *entity.JournalContentSnapshot,
) bool {
	if snapshot != nil && snapshot.ObjectKey != "" {
		return true
	}
	for _, fragment := range fragments {
		if fragment != nil && fragment.ObjectKey != "" {
			return true
		}
	}
	return false
}

func journalSnapshotObjectsMatchReservation(
	stagingPrefix string,
	fragments []*entity.JournalSnapshotFragment,
	snapshot *entity.JournalContentSnapshot,
) bool {
	stagingPrefix = strings.TrimSpace(stagingPrefix)
	if stagingPrefix == "" {
		return false
	}
	if snapshot != nil && snapshot.ObjectKey != "" &&
		!strings.HasPrefix(snapshot.ObjectKey, stagingPrefix) {
		return false
	}
	for _, fragment := range fragments {
		if fragment != nil && fragment.ObjectKey != "" &&
			!strings.HasPrefix(fragment.ObjectKey, stagingPrefix) {
			return false
		}
	}
	return true
}

func journalSnapshotReservationIdentityMatches(
	current *entity.JournalSnapshotReservation,
	candidate *entity.JournalSnapshotReservation,
) bool {
	return current != nil && candidate != nil &&
		current.SnapshotID == candidate.SnapshotID &&
		current.SpaceID == candidate.SpaceID &&
		current.ThreadID == candidate.ThreadID &&
		current.RunID == candidate.RunID &&
		current.JournalRunID == candidate.JournalRunID &&
		current.AttemptID == candidate.AttemptID &&
		current.ActionID == candidate.ActionID &&
		current.Revision == candidate.Revision &&
		current.IdempotencyKey == candidate.IdempotencyKey &&
		current.ContentHash == candidate.ContentHash &&
		current.ACLDomain == candidate.ACLDomain
}

func normalizeJournalSnapshotAccessAudit(
	audit *entity.JournalSnapshotAccessAudit,
) (*entity.JournalSnapshotAccessAudit, error) {
	if audit == nil {
		return nil, fmt.Errorf("journal snapshot access audit is required")
	}
	normalized := *audit
	normalized.AttemptID = strings.TrimSpace(normalized.AttemptID)
	normalized.SnapshotID = strings.TrimSpace(normalized.SnapshotID)
	normalized.IdempotencyKey = strings.TrimSpace(normalized.IdempotencyKey)
	normalized.TargetHash = strings.TrimSpace(normalized.TargetHash)
	normalized.TraceID = strings.TrimSpace(normalized.TraceID)
	if normalized.SpaceID <= 0 || normalized.ThreadID <= 0 || normalized.RunID <= 0 ||
		normalized.SnapshotID == "" || !normalized.Action.Valid() ||
		normalized.ActorID <= 0 || normalized.IdempotencyKey == "" {
		return nil, fmt.Errorf("journal snapshot access audit identity is invalid")
	}
	if len(normalized.AttemptID) > 64 || len(normalized.SnapshotID) > 64 ||
		len(normalized.Action) > 64 || len(normalized.IdempotencyKey) > 191 ||
		len(normalized.TraceID) > 128 || len(normalized.TargetHash) != 64 {
		return nil, fmt.Errorf("journal snapshot access audit field exceeds storage limits")
	}
	switch normalized.PermissionResult {
	case entity.JournalSnapshotPermissionAllowed,
		entity.JournalSnapshotPermissionDenied,
		entity.JournalSnapshotPermissionExpired:
	default:
		return nil, fmt.Errorf("journal snapshot permission result is invalid")
	}
	if normalized.CreatedAt <= 0 {
		normalized.CreatedAt = time.Now().UnixMilli()
	}
	return &normalized, nil
}

func loadJournalSnapshotEvent(tx *gorm.DB, eventID int64) (*entity.JournalEvent, error) {
	var po runEventPO
	if err := tx.Where("id = ?", eventID).First(&po).Error; err != nil {
		return nil, err
	}
	return journalEventFromPO(&po), nil
}

func journalSnapshotToPO(snapshot *entity.JournalContentSnapshot) *journalSnapshotPO {
	var contentJSON []byte
	if snapshot.ContentJSON != "" {
		contentJSON = []byte(snapshot.ContentJSON)
	}
	var summaryJSON []byte
	if snapshot.SummaryJSON != "" {
		summaryJSON = []byte(snapshot.SummaryJSON)
	}
	return &journalSnapshotPO{
		SnapshotID: snapshot.SnapshotID, SpaceID: snapshot.SpaceID,
		ThreadID: snapshot.ThreadID, RunID: snapshot.RunID,
		JournalRunID: snapshot.JournalRunID, AttemptID: snapshot.AttemptID,
		EventID: snapshot.EventID, ActionID: snapshot.ActionID, Revision: snapshot.Revision,
		ContentType: string(snapshot.ContentType), Status: string(snapshot.Status),
		IsFragmented: snapshot.IsFragmented, FragmentCount: snapshot.FragmentCount,
		Visibility: string(snapshot.Visibility),
		ErrorCode:  stringPtrOrNil(snapshot.ErrorCode), MIMEType: snapshot.MIMEType,
		Encoding: snapshot.Encoding, Compression: string(snapshot.Compression),
		ContentJSON: contentJSON,
		ObjectKey:   stringPtrOrNil(snapshot.ObjectKey),
		SummaryJSON: summaryJSON, SummaryHash: stringPtrOrNil(snapshot.SummaryHash),
		ContentLength: snapshot.ContentLength,
		ContentHash:   snapshot.ContentHash, ACLDomain: snapshot.ACLDomain,
		SourceResourceType: stringPtrOrNil(snapshot.SourceResourceType),
		SourceResourceID:   stringPtrOrNil(snapshot.SourceResourceID),
		SourceRevision:     stringPtrOrNil(snapshot.SourceRevision),
		OriginalObjectKey:  stringPtrOrNil(snapshot.OriginalObjectKey),
		ExpiresAt:          snapshot.ExpiresAt,
		CleanupState:       string(snapshot.CleanupState),
		DeletedAt:          int64PtrOrNil(snapshot.DeletedAt), CreatedAt: snapshot.CreatedAt,
	}
}

func (po *journalSnapshotPO) toEntity() *entity.JournalContentSnapshot {
	if po == nil {
		return nil
	}
	return &entity.JournalContentSnapshot{
		SnapshotID: po.SnapshotID, SpaceID: po.SpaceID, ThreadID: po.ThreadID,
		RunID: po.RunID, JournalRunID: po.JournalRunID, AttemptID: po.AttemptID,
		EventID: po.EventID, ActionID: po.ActionID, Revision: po.Revision,
		ContentType: entity.JournalSnapshotContentType(po.ContentType),
		Status:      entity.JournalContentStatus(po.Status), IsFragmented: po.IsFragmented,
		FragmentCount: po.FragmentCount,
		Visibility:    entity.JournalVisibility(po.Visibility), ErrorCode: stringFromPtr(po.ErrorCode),
		MIMEType: po.MIMEType, Encoding: po.Encoding,
		Compression: entity.JournalSnapshotCompression(po.Compression),
		ContentJSON: string(po.ContentJSON),
		ObjectKey:   stringFromPtr(po.ObjectKey),
		SummaryJSON: string(po.SummaryJSON), SummaryHash: stringFromPtr(po.SummaryHash),
		ContentLength: po.ContentLength,
		ContentHash:   po.ContentHash, ACLDomain: po.ACLDomain,
		SourceResourceType: stringFromPtr(po.SourceResourceType),
		SourceResourceID:   stringFromPtr(po.SourceResourceID),
		SourceRevision:     stringFromPtr(po.SourceRevision),
		OriginalObjectKey:  stringFromPtr(po.OriginalObjectKey),
		ExpiresAt:          po.ExpiresAt,
		CleanupState:       entity.JournalSnapshotCleanupState(po.CleanupState),
		DeletedAt:          int64FromPtr(po.DeletedAt), CreatedAt: po.CreatedAt,
	}
}

func journalSnapshotReservationToPO(
	reservation *entity.JournalSnapshotReservation,
) *journalSnapshotReservationPO {
	return &journalSnapshotReservationPO{
		SnapshotID:       reservation.SnapshotID,
		ReservationToken: reservation.ReservationToken,
		SpaceID:          reservation.SpaceID, ThreadID: reservation.ThreadID,
		RunID: reservation.RunID, JournalRunID: reservation.JournalRunID,
		AttemptID: reservation.AttemptID, ActionID: reservation.ActionID,
		Revision: reservation.Revision, EventID: reservation.EventID,
		IdempotencyKey: reservation.IdempotencyKey,
		ContentHash:    reservation.ContentHash, ACLDomain: reservation.ACLDomain,
		StagingPrefix: reservation.StagingPrefix,
		ExpiresAt:     reservation.ExpiresAt, CreatedAt: reservation.CreatedAt,
	}
}

func (po *journalSnapshotReservationPO) toEntity() *entity.JournalSnapshotReservation {
	if po == nil {
		return nil
	}
	return &entity.JournalSnapshotReservation{
		SnapshotID: po.SnapshotID, ReservationToken: po.ReservationToken,
		SpaceID: po.SpaceID, ThreadID: po.ThreadID, RunID: po.RunID,
		JournalRunID: po.JournalRunID, AttemptID: po.AttemptID,
		ActionID: po.ActionID, Revision: po.Revision, EventID: po.EventID,
		IdempotencyKey: po.IdempotencyKey, ContentHash: po.ContentHash,
		ACLDomain: po.ACLDomain, StagingPrefix: po.StagingPrefix,
		ExpiresAt: po.ExpiresAt, CreatedAt: po.CreatedAt,
	}
}

func journalSnapshotFragmentToPO(fragment *entity.JournalSnapshotFragment) *journalSnapshotFragmentPO {
	var inlineContent []byte
	if fragment.ObjectKey == "" {
		inlineContent = []byte(fragment.InlineContent)
	}
	return &journalSnapshotFragmentPO{
		FragmentID: fragment.FragmentID, SnapshotID: fragment.SnapshotID,
		FragmentIndex: fragment.FragmentIndex, Kind: string(fragment.Kind),
		MetadataJSON: []byte(fragment.MetadataJSON),
		MIMEType:     stringPtrOrNil(fragment.MIMEType), InlineContent: inlineContent,
		ObjectKey: stringPtrOrNil(fragment.ObjectKey), ByteStart: fragment.ByteStart,
		ByteEnd: fragment.ByteEnd, SizeBytes: fragment.SizeBytes,
		ContentHash: fragment.ContentHash, CreatedAt: fragment.CreatedAt,
	}
}

func (po *journalSnapshotFragmentPO) toEntity() *entity.JournalSnapshotFragment {
	if po == nil {
		return nil
	}
	return &entity.JournalSnapshotFragment{
		FragmentID: po.FragmentID, SnapshotID: po.SnapshotID,
		FragmentIndex: po.FragmentIndex,
		Kind:          entity.JournalSnapshotFragmentKind(po.Kind),
		MetadataJSON:  string(po.MetadataJSON), MIMEType: stringFromPtr(po.MIMEType),
		InlineContent: string(po.InlineContent),
		ObjectKey:     stringFromPtr(po.ObjectKey), ByteStart: po.ByteStart,
		ByteEnd: po.ByteEnd, SizeBytes: po.SizeBytes,
		ContentHash: po.ContentHash, CreatedAt: po.CreatedAt,
	}
}

func journalSnapshotAccessAuditToPO(
	audit *entity.JournalSnapshotAccessAudit,
) *journalSnapshotAccessAuditPO {
	return &journalSnapshotAccessAuditPO{
		ID: audit.ID, SpaceID: audit.SpaceID, ThreadID: audit.ThreadID,
		RunID: audit.RunID, AttemptID: stringPtrOrNil(audit.AttemptID),
		SnapshotID: audit.SnapshotID, ContentType: stringPtrOrNil(string(audit.ContentType)),
		Action: string(audit.Action), ActorID: audit.ActorID,
		PermissionResult: string(audit.PermissionResult),
		IdempotencyKey:   audit.IdempotencyKey, TargetHash: audit.TargetHash,
		TraceID:   stringPtrOrNil(audit.TraceID),
		CreatedAt: audit.CreatedAt,
	}
}

func (po *journalSnapshotAccessAuditPO) toEntity() *entity.JournalSnapshotAccessAudit {
	if po == nil {
		return nil
	}
	return &entity.JournalSnapshotAccessAudit{
		ID: po.ID, SpaceID: po.SpaceID, ThreadID: po.ThreadID, RunID: po.RunID,
		AttemptID: stringFromPtr(po.AttemptID), SnapshotID: po.SnapshotID,
		ContentType: entity.JournalSnapshotContentType(stringFromPtr(po.ContentType)),
		Action:      entity.JournalSnapshotAction(po.Action), ActorID: po.ActorID,
		PermissionResult: entity.JournalSnapshotPermissionResult(po.PermissionResult),
		IdempotencyKey:   po.IdempotencyKey, TargetHash: po.TargetHash,
		TraceID:   stringFromPtr(po.TraceID),
		CreatedAt: po.CreatedAt,
	}
}

func cloneJournalSnapshot(snapshot *entity.JournalContentSnapshot) *entity.JournalContentSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.Fragments = cloneJournalSnapshotFragments(snapshot.Fragments)
	return &clone
}

func cloneJournalSnapshotReservation(
	reservation *entity.JournalSnapshotReservation,
) *entity.JournalSnapshotReservation {
	if reservation == nil {
		return nil
	}
	clone := *reservation
	return &clone
}

func cloneJournalSnapshotFragments(
	fragments []*entity.JournalSnapshotFragment,
) []*entity.JournalSnapshotFragment {
	result := make([]*entity.JournalSnapshotFragment, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment == nil {
			result = append(result, nil)
			continue
		}
		clone := *fragment
		result = append(result, &clone)
	}
	return result
}
