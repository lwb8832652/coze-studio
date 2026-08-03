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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

func TestJournalUserSettingsUseAuthenticatedUserAndCAS(t *testing.T) {
	store := &journalUserSettingsStoreStub{}
	service := &ApplicationService{
		JournalUserSettingsStore: store,
		JournalSettingsNow:       func() int64 { return 1234 },
	}

	settings, err := service.GetJournalUserSettings(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 0.40, settings.SplitRatio)
	require.Equal(t, kvstore.MissingRevision, settings.Revision)
	require.Equal(t, journalUserSettingsNamespace, store.getNamespace)
	require.Equal(t, "42", store.getKey)

	updated, err := service.PatchJournalUserSettings(context.Background(), PatchJournalUserSettingsRequest{
		ViewerID: 42, SplitRatio: 0.63, Revision: kvstore.MissingRevision,
	})
	require.NoError(t, err)
	require.Equal(t, 0.63, updated.SplitRatio)
	require.Equal(t, "rev-next", updated.Revision)
	require.Equal(t, int64(1234), updated.UpdatedAt)
	require.Equal(t, "42", store.casKey)
	require.Equal(t, kvstore.MissingRevision, store.expectedRevision)
	require.Equal(t, 0.63, store.casValue.SplitRatio)
}

func TestJournalUserSettingsRejectRangeAndStaleRevision(t *testing.T) {
	store := &journalUserSettingsStoreStub{casErr: kvstore.ErrVersionConflict}
	service := &ApplicationService{JournalUserSettingsStore: store}

	_, err := service.PatchJournalUserSettings(context.Background(), PatchJournalUserSettingsRequest{
		ViewerID: 42, SplitRatio: 0.39, Revision: "rev-current",
	})
	require.ErrorIs(t, err, ErrJournalSettingsInvalid)

	_, err = service.PatchJournalUserSettings(context.Background(), PatchJournalUserSettingsRequest{
		ViewerID: 42, SplitRatio: 0.50, Revision: "rev-stale",
	})
	require.ErrorIs(t, err, ErrJournalSettingsConflict)
}

type journalUserSettingsStoreStub struct {
	getNamespace     string
	getKey           string
	getValue         *JournalUserSettingsRecord
	getRevision      string
	getErr           error
	casNamespace     string
	casKey           string
	expectedRevision string
	casValue         JournalUserSettingsRecord
	casErr           error
}

func (s *journalUserSettingsStoreStub) GetVersioned(
	_ context.Context,
	namespace, key string,
) (*JournalUserSettingsRecord, string, error) {
	s.getNamespace, s.getKey = namespace, key
	if s.getErr != nil {
		return nil, "", s.getErr
	}
	if s.getValue == nil {
		return nil, "", kvstore.ErrKeyNotFound
	}
	value := *s.getValue
	return &value, s.getRevision, nil
}

func (s *journalUserSettingsStoreStub) CompareAndSwap(
	_ context.Context,
	namespace, key, expectedRevision string,
	value *JournalUserSettingsRecord,
) (string, error) {
	s.casNamespace, s.casKey = namespace, key
	s.expectedRevision = expectedRevision
	if value != nil {
		s.casValue = *value
	}
	if s.casErr != nil {
		return "", s.casErr
	}
	return "rev-next", nil
}
