// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
	infraannouncement "github.com/coze-dev/coze-studio/backend/infra/announcement"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	DefaultAudienceBatchSize = 500
	DefaultSynchronousSteps  = 8
	DefaultReplayPageSize    = 50
	DefaultFailurePersistenceTimeout = 2 * time.Second

	ProjectionPendingCode = "projection_pending"
	ProjectionRetryCode   = "projection_retry_pending"
)

type Actor struct {
	UserID      int64
	SystemAdmin bool
}

type CreateRequest struct {
	IdempotencyKey string
	Draft          domainannouncement.Draft
}

type UpdateRequest struct {
	AnnouncementID int64
	ExpectedVersion int64
	Draft           domainannouncement.Draft
}

type ScheduleRequest struct {
	AnnouncementID int64
	ExpectedVersion int64
	ScheduledAt     time.Time
}

type PublishRequest struct {
	AnnouncementID int64
	ExpectedVersion int64
	IdempotencyKey  string
}

type CancelRequest struct {
	AnnouncementID int64
	ExpectedVersion int64
}

type PublicationResult struct {
	Announcement *domainannouncement.Announcement
	Replayed     bool
	Deferred     bool
	ErrorCode    string
}

type ReplayResult struct {
	Processed  int            `json:"processed"`
	Completed  int            `json:"completed"`
	Failed     int            `json:"failed"`
	Deferred   int            `json:"deferred"`
	ErrorCodes map[string]int `json:"error_codes,omitempty"`
}

type Service struct {
	repository                domainannouncement.Repository
	now                       func() time.Time
	batchSize                 int
	synchronousSteps          int
	replayPageSize            int
	failurePersistenceTimeout time.Duration
}

var SVC = NewService(nil)

func NewService(repository domainannouncement.Repository) *Service {
	return &Service{
		repository:       repository,
		now:              time.Now,
		batchSize:        DefaultAudienceBatchSize,
		synchronousSteps: DefaultSynchronousSteps,
		replayPageSize:   DefaultReplayPageSize,
		failurePersistenceTimeout: DefaultFailurePersistenceTimeout,
	}
}

func InitService(
	db *gorm.DB,
	idGenerator idgen.IDGenerator,
) *Service {
	SVC = NewService(infraannouncement.NewMySQLRepository(db, idGenerator))
	return SVC
}

func (s *Service) IsConfigured() bool {
	return s != nil && s.repository != nil
}

func (s *Service) Create(
	ctx context.Context,
	actor Actor,
	request CreateRequest,
) (*domainannouncement.MutationResult, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	key, err := domainannouncement.NormalizeIdempotencyKey(
		request.IdempotencyKey,
	)
	if err != nil {
		return nil, err
	}
	draft, err := domainannouncement.NormalizeDraft(request.Draft)
	if err != nil {
		return nil, err
	}
	requestHash, err := stableHash(draft)
	if err != nil {
		return nil, domainannouncement.ErrStorage
	}
	return s.repository.Create(ctx, domainannouncement.CreateCommand{
		ActorID:        actor.UserID,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		Draft:          draft,
		Now:            s.currentTime(),
	})
}

func (s *Service) Update(
	ctx context.Context,
	actor Actor,
	request UpdateRequest,
) (*domainannouncement.Announcement, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	if request.AnnouncementID <= 0 || request.ExpectedVersion <= 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	draft, err := domainannouncement.NormalizeDraft(request.Draft)
	if err != nil {
		return nil, err
	}
	return s.repository.Update(ctx, domainannouncement.UpdateCommand{
		AnnouncementID: request.AnnouncementID,
		ActorID:         actor.UserID,
		ExpectedVersion: request.ExpectedVersion,
		Draft:           draft,
		Now:             s.currentTime(),
	})
}

func (s *Service) Schedule(
	ctx context.Context,
	actor Actor,
	request ScheduleRequest,
) (*domainannouncement.Announcement, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	now := s.currentTime()
	if request.AnnouncementID <= 0 ||
		request.ExpectedVersion <= 0 ||
		domainannouncement.ValidateScheduleTime(now, request.ScheduledAt) != nil {
		return nil, domainannouncement.ErrInvalidInput
	}
	return s.repository.Schedule(ctx, domainannouncement.ScheduleCommand{
		AnnouncementID: request.AnnouncementID,
		ActorID:         actor.UserID,
		ExpectedVersion: request.ExpectedVersion,
		ScheduledAt:     request.ScheduledAt.UTC(),
		Now:             now,
	})
}

func (s *Service) Publish(
	ctx context.Context,
	actor Actor,
	request PublishRequest,
) (*PublicationResult, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	if request.AnnouncementID <= 0 || request.ExpectedVersion <= 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	key, err := domainannouncement.NormalizeIdempotencyKey(
		request.IdempotencyKey,
	)
	if err != nil {
		return nil, err
	}
	requestHash, err := stableHash(struct {
		AnnouncementID int64  `json:"announcement_id"`
		ExpectedVersion int64 `json:"expected_version"`
	}{
		AnnouncementID: request.AnnouncementID,
		ExpectedVersion: request.ExpectedVersion,
	})
	if err != nil {
		return nil, domainannouncement.ErrStorage
	}
	mutation, err := s.repository.RequestPublish(
		ctx,
		domainannouncement.PublishCommand{
			AnnouncementID: request.AnnouncementID,
			ActorID:         actor.UserID,
			ExpectedVersion: request.ExpectedVersion,
			IdempotencyKey:  key,
			RequestHash:     requestHash,
			Now:             s.currentTime(),
		},
	)
	if err != nil {
		return nil, err
	}
	result, err := s.advance(
		ctx,
		request.AnnouncementID,
		actor.UserID,
		mutation.Announcement,
		s.synchronousSteps,
	)
	if err != nil {
		return nil, err
	}
	result.Replayed = mutation.Replayed
	return result, nil
}

func (s *Service) Cancel(
	ctx context.Context,
	actor Actor,
	request CancelRequest,
) (*domainannouncement.Announcement, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	if request.AnnouncementID <= 0 || request.ExpectedVersion <= 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	return s.repository.Cancel(ctx, domainannouncement.CancelCommand{
		AnnouncementID: request.AnnouncementID,
		ActorID:         actor.UserID,
		ExpectedVersion: request.ExpectedVersion,
		Now:             s.currentTime(),
	})
}

func (s *Service) Get(
	ctx context.Context,
	actor Actor,
	announcementID int64,
) (*domainannouncement.Announcement, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	if announcementID <= 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	return s.repository.Get(ctx, announcementID)
}

func (s *Service) List(
	ctx context.Context,
	actor Actor,
	filter domainannouncement.ListFilter,
) ([]*domainannouncement.Announcement, int64, error) {
	if err := authorize(actor); err != nil {
		return nil, 0, err
	}
	if !s.IsConfigured() {
		return nil, 0, domainannouncement.ErrStorage
	}
	if filter.Offset < 0 ||
		filter.Limit <= 0 ||
		filter.Limit > domainannouncement.MaxPageSize ||
		(filter.Status != "" && !filter.Status.Valid()) {
		return nil, 0, domainannouncement.ErrInvalidInput
	}
	return s.repository.List(ctx, filter)
}

func (s *Service) ListAuditEvents(
	ctx context.Context,
	actor Actor,
	announcementID int64,
	offset int,
	limit int,
) ([]*domainannouncement.AuditEvent, int64, error) {
	if err := authorize(actor); err != nil {
		return nil, 0, err
	}
	if !s.IsConfigured() {
		return nil, 0, domainannouncement.ErrStorage
	}
	if announcementID <= 0 ||
		offset < 0 ||
		limit <= 0 ||
		limit > domainannouncement.MaxAuditPageSize {
		return nil, 0, domainannouncement.ErrInvalidInput
	}
	return s.repository.ListAuditEvents(
		ctx,
		announcementID,
		offset,
		limit,
	)
}

func (s *Service) Replay(
	ctx context.Context,
	actor Actor,
	announcementID int64,
) (*ReplayResult, error) {
	if err := authorize(actor); err != nil {
		return nil, err
	}
	if !s.IsConfigured() {
		return nil, domainannouncement.ErrStorage
	}
	if announcementID < 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	if announcementID > 0 {
		current, err := s.repository.Get(ctx, announcementID)
		if err != nil {
			return nil, err
		}
		result, err := s.advance(
			ctx,
			announcementID,
			actor.UserID,
			current,
			s.synchronousSteps,
		)
		if err != nil {
			return nil, err
		}
		replay := &ReplayResult{Processed: 1}
		if result.Deferred {
			replay.Deferred = 1
		} else {
			replay.Completed = 1
		}
		return replay, nil
	}
	return s.replayCandidates(ctx, actor.UserID, s.synchronousSteps)
}

func (s *Service) runReplayCycle(ctx context.Context) error {
	if !s.IsConfigured() {
		return domainannouncement.ErrStorage
	}
	_, err := s.replayCandidates(ctx, 0, 1)
	return err
}

func (s *Service) replayCandidates(
	ctx context.Context,
	actorID int64,
	steps int,
) (*ReplayResult, error) {
	result := &ReplayResult{}
	afterID := int64(0)
	for {
		ids, err := s.repository.ListReplayCandidates(
			ctx,
			s.currentTime(),
			afterID,
			s.replayPageSize,
		)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return result, nil
		}
		for _, announcementID := range ids {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			publication, err := s.advance(
				ctx,
				announcementID,
				actorID,
				nil,
				steps,
			)
			result.Processed++
			afterID = announcementID
			if err != nil {
				errorCode := domainannouncement.StableErrorCode(err)
				result.Failed++
				if result.ErrorCodes == nil {
					result.ErrorCodes = make(map[string]int)
				}
				result.ErrorCodes[errorCode]++
				logs.CtxErrorf(
					ctx,
					"[announcement] replay_candidate_failed announcement_id=%d error_code=%s",
					announcementID,
					errorCode,
				)
				continue
			}
			if publication.Deferred {
				result.Deferred++
			} else {
				result.Completed++
			}
		}
		if len(ids) < s.replayPageSize {
			return result, nil
		}
	}
}

func (s *Service) advance(
	ctx context.Context,
	announcementID int64,
	actorID int64,
	initial *domainannouncement.Announcement,
	steps int,
) (*PublicationResult, error) {
	result := &PublicationResult{Announcement: initial}
	if steps <= 0 {
		steps = 1
	}
	for step := 0; step < steps; step++ {
		advanced, err := s.repository.AdvancePublication(
			ctx,
			announcementID,
			actorID,
			s.currentTime(),
			s.batchSize,
		)
		if err != nil {
			if !isRetryableProjectionError(err) {
				return nil, err
			}
			code := domainannouncement.StableErrorCode(err)
			timeout := s.failurePersistenceTimeout
			if timeout <= 0 {
				timeout = DefaultFailurePersistenceTimeout
			}
			failureCtx, cancel := context.WithTimeout(
				context.WithoutCancel(ctx),
				timeout,
			)
			failed, markErr := s.repository.MarkProjectionFailed(
				failureCtx,
				announcementID,
				actorID,
				code,
				s.currentTime(),
			)
			cancel()
			if markErr == nil && failed != nil {
				result.Announcement = failed
			}
			if markErr != nil {
				logs.CtxErrorf(
					ctx,
					"[announcement] persist_projection_failure_failed announcement_id=%d error_code=%s",
					announcementID,
					domainannouncement.StableErrorCode(markErr),
				)
			}
			result.Deferred = true
			result.ErrorCode = ProjectionRetryCode
			return result, nil
		}
		if advanced != nil {
			result.Announcement = advanced.Announcement
			if advanced.Done {
				return result, nil
			}
			if !advanced.Progressed {
				break
			}
		}
	}
	result.Deferred = true
	result.ErrorCode = ProjectionPendingCode
	return result, nil
}

func isRetryableProjectionError(err error) bool {
	return errors.Is(err, domainannouncement.ErrStorage) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func authorize(actor Actor) error {
	if actor.UserID <= 0 || !actor.SystemAdmin {
		return domainannouncement.ErrPermissionDenied
	}
	return nil
}

func stableHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now()
	}
	return s.now().UTC()
}

type WorkerOptions struct {
	PollInterval time.Duration
	Now          func() time.Time
}

func DefaultWorkerOptions() WorkerOptions {
	return WorkerOptions{
		PollInterval: 5 * time.Second,
		Now:          time.Now,
	}
}

type Worker struct {
	service *Service
	options WorkerOptions

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewWorker(service *Service, options WorkerOptions) *Worker {
	defaults := DefaultWorkerOptions()
	if options.PollInterval <= 0 {
		options.PollInterval = defaults.PollInterval
	}
	if options.Now == nil {
		options.Now = defaults.Now
	}
	return &Worker{
		service: service,
		options: options,
		done:    make(chan struct{}),
	}
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.started = true
	go w.run(workerCtx)
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	w.runOnce(ctx)
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) {
	if w.service == nil {
		return
	}
	if err := w.service.runReplayCycle(ctx); err != nil &&
		!errors.Is(err, context.Canceled) {
		logs.CtxErrorf(
			ctx,
			"[announcement] replay_cycle_failed error_code=%s",
			domainannouncement.StableErrorCode(err),
		)
	}
}

func (w *Worker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return nil
	}
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) ShutdownName() string {
	return "announcement-publication-worker"
}

func (result *PublicationResult) Validate() error {
	if result == nil || result.Announcement == nil {
		return fmt.Errorf("%w: publication result", domainannouncement.ErrStorage)
	}
	return nil
}
