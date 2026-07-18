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
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrKeyNotFound     = errors.New("key not found")
	ErrVersionConflict = errors.New("kv revision conflict")
)

const (
	MissingRevision      = "missing"
	versionedValueFormat = "coze-kv-envelope-v1"
)

/*
CREATE TABLE IF NOT EXISTS `kv_entries` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `namespace` VARCHAR(255) NOT NULL,
  `key_data` VARCHAR(255) NOT NULL,
  `value_data` LONGBLOB NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_namespace_key` (`namespace`, `key_data`)
) ENGINE=InnoDB CHARSET utf8mb4 COLLATE utf8mb4_general_ci COMMENT 'kv data';
*/

type KVStore[T any] struct {
	repo *gorm.DB
}

type persistedVersionedValue struct {
	Format   string          `json:"_coze_kv_format"`
	Revision string          `json:"_coze_kv_revision"`
	Value    json.RawMessage `json:"_coze_kv_value"`
}

type kvEntry struct {
	Namespace string `gorm:"column:namespace"`
	Key       string `gorm:"column:key_data"`
	Value     []byte `gorm:"column:value_data"`
}

func (kvEntry) TableName() string { return "kv_entries" }

var defaultDB *gorm.DB

func SetDefault(db *gorm.DB) {
	defaultDB = db
}

func New[T any](db *gorm.DB) *KVStore[T] {
	return &KVStore[T]{
		repo: db,
	}
}

func (g *KVStore[T]) db(ctx context.Context) *gorm.DB {
	if g.repo == nil {
		return defaultDB.WithContext(ctx)
	}

	return g.repo.WithContext(ctx)
}

func (g *KVStore[T]) Save(ctx context.Context, namespace, k string, v *T) error {
	if v == nil {
		return fmt.Errorf("cannot save nil value for key: %s", k)
	}

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal failed for key %s for type %T: %w", k, *v, err)
	}

	res := g.db(ctx).Exec(
		"INSERT INTO `kv_entries` (`namespace`, `key_data`, `value_data`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `value_data` = ?",
		namespace, k, data, data,
	)

	if res.Error != nil {
		return fmt.Errorf("failed to save key %s: %w", k, res.Error)
	}

	return nil
}

func (g *KVStore[T]) Get(ctx context.Context, namespace, k string) (*T, error) {
	value, _, err := g.GetVersioned(ctx, namespace, k)
	return value, err
}

func (g *KVStore[T]) GetVersioned(ctx context.Context, namespace, k string) (*T, string, error) {
	data, err := g.readRaw(ctx, namespace, k)
	if err != nil {
		return nil, "", err
	}
	payload, revision, err := decodePersistedValue(data)
	if err != nil {
		return nil, "", fmt.Errorf("decode versioned value for key %s: %w", k, err)
	}
	var obj T
	if err = json.Unmarshal(payload, &obj); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal json for key %s: %w", k, err)
	}
	return &obj, revision, nil
}

// CompareAndSwap atomically replaces a value only when the persisted revision
// matches expectedRevision. The envelope keeps the revision durable across
// process restarts, while the raw-byte predicate provides cross-process CAS.
func (g *KVStore[T]) CompareAndSwap(ctx context.Context, namespace, k, expectedRevision string, value *T) (string, error) {
	if value == nil {
		return "", fmt.Errorf("cannot save nil value for key: %s", k)
	}
	if expectedRevision == "" {
		return "", errors.New("expected revision is required")
	}
	encoded, revision, err := encodePersistedValue(value)
	if err != nil {
		return "", fmt.Errorf("encode versioned value for key %s: %w", k, err)
	}

	if expectedRevision == MissingRevision {
		entry := &kvEntry{Namespace: namespace, Key: k, Value: encoded}
		result := g.db(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "namespace"}, {Name: "key_data"}},
			DoNothing: true,
		}).Create(entry)
		if result.Error != nil {
			return "", fmt.Errorf("create versioned key %s: %w", k, result.Error)
		}
		if result.RowsAffected != 1 {
			return "", ErrVersionConflict
		}
		return revision, nil
	}

	currentRaw, err := g.readRaw(ctx, namespace, k)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return "", ErrVersionConflict
		}
		return "", err
	}
	_, currentRevision, err := decodePersistedValue(currentRaw)
	if err != nil {
		return "", fmt.Errorf("decode current revision for key %s: %w", k, err)
	}
	if currentRevision != expectedRevision {
		return "", ErrVersionConflict
	}

	result := g.db(ctx).Exec(
		"UPDATE `kv_entries` SET `value_data` = ? WHERE `namespace` = ? AND `key_data` = ? AND `value_data` = ?",
		encoded, namespace, k, currentRaw,
	)
	if result.Error != nil {
		return "", fmt.Errorf("compare-and-swap key %s: %w", k, result.Error)
	}
	if result.RowsAffected != 1 {
		return "", ErrVersionConflict
	}
	return revision, nil
}

func (g *KVStore[T]) readRaw(ctx context.Context, namespace, k string) ([]byte, error) {
	row := g.db(ctx).Raw(
		"SELECT `value_data` FROM `kv_entries` WHERE `namespace` = ? AND `key_data` = ? LIMIT 1",
		namespace, k,
	).Row()
	var value []byte
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to get key %s: %w", k, err)
	}
	return value, nil
}

func encodePersistedValue[T any](value *T) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	revision, err := newRevision()
	if err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(persistedVersionedValue{
		Format:   versionedValueFormat,
		Revision: revision,
		Value:    payload,
	})
	return encoded, revision, err
}

func decodePersistedValue(data []byte) ([]byte, string, error) {
	var envelope persistedVersionedValue
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Format == versionedValueFormat {
		if envelope.Revision == "" || len(envelope.Value) == 0 {
			return nil, "", errors.New("versioned value envelope is incomplete")
		}
		return envelope.Value, envelope.Revision, nil
	}
	digest := sha256.Sum256(data)
	return data, "legacy-" + hex.EncodeToString(digest[:]), nil
}

func newRevision() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "rev-" + hex.EncodeToString(random[:]), nil
}

func (g *KVStore[T]) Delete(ctx context.Context, namespace, k string) error {
	res := g.db(ctx).Exec(
		"DELETE FROM `kv_entries` WHERE `namespace` = ? AND `key_data` = ?",
		namespace, k,
	)

	return res.Error
}
