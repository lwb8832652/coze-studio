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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	journalUserSettingsNamespace = "workbench.journal.user_settings.v1"
	journalDefaultSplitRatio     = 0.40
	journalMinimumSplitRatio     = 0.40
	journalMaximumSplitRatio     = 0.70
)

var (
	ErrJournalSettingsInvalid     = errors.New("journal settings are invalid")
	ErrJournalSettingsConflict    = errors.New("journal settings revision conflict")
	ErrJournalSettingsUnavailable = errors.New("journal settings store is unavailable")
)

type JournalUserSettingsRecord struct {
	SplitRatio float64 `json:"split_ratio"`
	UpdatedAt  int64   `json:"updated_at"`
}

type JournalUserSettingsStore interface {
	GetVersioned(
		context.Context,
		string,
		string,
	) (*JournalUserSettingsRecord, string, error)
	CompareAndSwap(
		context.Context,
		string,
		string,
		string,
		*JournalUserSettingsRecord,
	) (string, error)
}

type JournalUserSettings struct {
	SplitRatio float64
	Revision   string
	UpdatedAt  int64
}

type PatchJournalUserSettingsRequest struct {
	ViewerID   int64
	SplitRatio float64
	Revision   string
}

func (s *ApplicationService) GetJournalUserSettings(
	ctx context.Context,
	viewerID int64,
) (*JournalUserSettings, error) {
	if viewerID <= 0 {
		return nil, ErrJournalSettingsInvalid
	}
	defaults := &JournalUserSettings{
		SplitRatio: journalDefaultSplitRatio,
		Revision:   kvstore.MissingRevision,
	}
	if s == nil || s.JournalUserSettingsStore == nil {
		return defaults, nil
	}
	record, revision, err := s.JournalUserSettingsStore.GetVersioned(
		ctx,
		journalUserSettingsNamespace,
		strconv.FormatInt(viewerID, 10),
	)
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return defaults, nil
	}
	if err != nil {
		logs.CtxWarnf(ctx, "journal user settings read unavailable")
		return defaults, nil
	}
	if record == nil || !validJournalSplitRatio(record.SplitRatio) ||
		strings.TrimSpace(revision) == "" {
		logs.CtxWarnf(ctx, "journal user settings record is invalid")
		return defaults, nil
	}
	return &JournalUserSettings{
		SplitRatio: record.SplitRatio,
		Revision:   revision,
		UpdatedAt:  record.UpdatedAt,
	}, nil
}

func (s *ApplicationService) PatchJournalUserSettings(
	ctx context.Context,
	req PatchJournalUserSettingsRequest,
) (*JournalUserSettings, error) {
	if s == nil || s.JournalUserSettingsStore == nil {
		return nil, ErrJournalSettingsUnavailable
	}
	revision := strings.TrimSpace(req.Revision)
	if req.ViewerID <= 0 || !validJournalSplitRatio(req.SplitRatio) || revision == "" {
		return nil, ErrJournalSettingsInvalid
	}
	now := time.Now().UnixMilli()
	if s.JournalSettingsNow != nil {
		now = s.JournalSettingsNow()
	}
	record := &JournalUserSettingsRecord{SplitRatio: req.SplitRatio, UpdatedAt: now}
	nextRevision, err := s.JournalUserSettingsStore.CompareAndSwap(
		ctx,
		journalUserSettingsNamespace,
		strconv.FormatInt(req.ViewerID, 10),
		revision,
		record,
	)
	if errors.Is(err, kvstore.ErrVersionConflict) {
		return nil, ErrJournalSettingsConflict
	}
	if err != nil {
		return nil, fmt.Errorf("%w: compare and swap failed", ErrJournalSettingsUnavailable)
	}
	return &JournalUserSettings{
		SplitRatio: record.SplitRatio, Revision: nextRevision, UpdatedAt: record.UpdatedAt,
	}, nil
}

func validJournalSplitRatio(value float64) bool {
	return value >= journalMinimumSplitRatio && value <= journalMaximumSplitRatio
}
