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

package kvstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type versionedValue struct {
	Name string `json:"name"`
}

func TestCompareAndSwapPersistsRevisionAndRejectsStaleWriter(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "kvstore.db")
	db := openVersionedKVTestDB(t, databasePath)
	store := New[versionedValue](db)
	ctx := context.Background()

	revision, err := store.CompareAndSwap(ctx, "test", "key", MissingRevision, &versionedValue{Name: "initial"})
	if err != nil {
		t.Fatalf("initial CompareAndSwap() error = %v", err)
	}
	if revision == "" || revision == MissingRevision {
		t.Fatalf("initial revision = %q, want a persisted revision", revision)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatalf("close first connection: %v", err)
	}

	reopened := openVersionedKVTestDB(t, databasePath)
	store = New[versionedValue](reopened)
	_, firstRevision, err := store.GetVersioned(ctx, "test", "key")
	if err != nil {
		t.Fatalf("first GetVersioned() error = %v", err)
	}
	_, secondRevision, err := store.GetVersioned(ctx, "test", "key")
	if err != nil {
		t.Fatalf("second GetVersioned() error = %v", err)
	}
	if firstRevision != revision || secondRevision != revision {
		t.Fatalf("reopened revisions = %q/%q, want %q", firstRevision, secondRevision, revision)
	}

	nextRevision, err := store.CompareAndSwap(ctx, "test", "key", firstRevision, &versionedValue{Name: "first"})
	if err != nil {
		t.Fatalf("first update CompareAndSwap() error = %v", err)
	}
	if nextRevision == firstRevision {
		t.Fatalf("updated revision = %q, want a new revision", nextRevision)
	}
	if _, err = store.CompareAndSwap(ctx, "test", "key", secondRevision, &versionedValue{Name: "stale"}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale update error = %v, want %v", err, ErrVersionConflict)
	}
	value, finalRevision, err := store.GetVersioned(ctx, "test", "key")
	if err != nil {
		t.Fatalf("final GetVersioned() error = %v", err)
	}
	if value.Name != "first" || finalRevision != nextRevision {
		t.Fatalf("final value/revision = %#v/%q, want first/%q", value, finalRevision, nextRevision)
	}
}

func TestGetVersionedUpgradesLegacyRawValueWithoutBreakingReads(t *testing.T) {
	db := openVersionedKVTestDB(t, filepath.Join(t.TempDir(), "legacy.db"))
	if err := db.Exec(
		"INSERT INTO kv_entries (namespace, key_data, value_data) VALUES (?, ?, ?)",
		"test", "legacy", []byte(`{"name":"legacy"}`),
	).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	store := New[versionedValue](db)
	ctx := context.Background()

	value, revision, err := store.GetVersioned(ctx, "test", "legacy")
	if err != nil {
		t.Fatalf("GetVersioned() error = %v", err)
	}
	if value.Name != "legacy" || revision == "" || revision == MissingRevision {
		t.Fatalf("legacy value/revision = %#v/%q", value, revision)
	}
	if _, err = store.CompareAndSwap(ctx, "test", "legacy", revision, &versionedValue{Name: "upgraded"}); err != nil {
		t.Fatalf("upgrade CompareAndSwap() error = %v", err)
	}
	value, err = store.Get(ctx, "test", "legacy")
	if err != nil || value.Name != "upgraded" {
		t.Fatalf("Get() after upgrade = %#v, %v", value, err)
	}
}

func openVersionedKVTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.Exec(`CREATE TABLE IF NOT EXISTS kv_entries (
id INTEGER PRIMARY KEY AUTOINCREMENT,
namespace TEXT NOT NULL,
key_data TEXT NOT NULL,
value_data BLOB NOT NULL,
UNIQUE(namespace, key_data)
)`).Error; err != nil {
		t.Fatalf("create kv_entries: %v", err)
	}
	return db
}
