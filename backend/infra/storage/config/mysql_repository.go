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

package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

type MySQLRepository struct {
	db *gorm.DB
}

type UpdateConfigInput struct {
	ID               uint64
	ExpectedVersion  uint64
	Name             string
	PublicConfig     domain.PublicConfig
	CredentialSecret string
	RuntimeChanged   bool
}

type UpdateHealthInput struct {
	ID                      uint64
	ExpectedVersion         uint64
	ExpectedRuntimeRevision uint64
	Health                  domain.Health
}

type CredentialEncryptor interface {
	Encrypt(uint64, domain.ProviderType, uint64, domain.CredentialInput) (string, error)
}

func NewMySQLRepository(db *gorm.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) Count(ctx context.Context) (int64, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	if err = db.Model(&objectStorageConfigPO{}).Count(&count).Error; err != nil {
		return 0, mapRepositoryError(err)
	}
	return count, nil
}

func (r *MySQLRepository) List(ctx context.Context) ([]domain.Config, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []objectStorageConfigPO
	if err = db.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, mapRepositoryError(err)
	}
	configs := make([]domain.Config, 0, len(rows))
	for index := range rows {
		config, err := rows[index].toDomain()
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func (r *MySQLRepository) Get(ctx context.Context, id uint64) (*domain.Config, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po, err := findObjectStorageConfig(db, id, false)
	if err != nil {
		return nil, err
	}
	config, err := po.toDomain()
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *MySQLRepository) GetActive(ctx context.Context) (*domain.Config, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var po objectStorageConfigPO
	err = db.Where("active_slot = 1").First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrPrimaryConfigMissing
	}
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	config, err := po.toDomain()
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *MySQLRepository) Create(ctx context.Context, config domain.Config) (*domain.Config, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po, err := newObjectStorageConfigPO(config, persistenceNow())
	if err != nil {
		return nil, err
	}
	if err = assignSQLiteObjectStorageID(db, po); err != nil {
		return nil, err
	}
	if err = secretWriteDB(db).Create(po).Error; err != nil {
		return nil, mapRepositoryError(err)
	}
	created, err := po.toDomain()
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *MySQLRepository) CreateWithCredential(ctx context.Context, config domain.Config, credential domain.CredentialInput, codec CredentialEncryptor) (*domain.Config, error) {
	if codec == nil {
		return nil, domain.ErrCredentialUnavailable
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var created *domain.Config
	err = db.Transaction(func(tx *gorm.DB) error {
		input := config
		input.CredentialSecret = "pending-object-storage-credential"
		po, err := newObjectStorageConfigPO(input, persistenceNow())
		if err != nil {
			return err
		}
		if err = assignSQLiteObjectStorageID(tx, po); err != nil {
			return err
		}
		if err = secretWriteDB(tx).Create(po).Error; err != nil {
			return mapRepositoryError(err)
		}

		secret, err := codec.Encrypt(po.ID, domain.ProviderType(po.ProviderType), CredentialAADVersion, credential)
		if err != nil {
			return err
		}
		result := secretWriteDB(tx).Model(&objectStorageConfigPO{}).
			Where("id = ?", po.ID).
			UpdateColumn("credential_secret", secret)
		if result.Error != nil {
			return mapRepositoryError(result.Error)
		}
		if result.RowsAffected == 0 {
			return domain.ErrNotFound
		}

		po, err = findObjectStorageConfig(tx, po.ID, false)
		if err != nil {
			return err
		}
		output, err := po.toDomain()
		if err != nil {
			return err
		}
		created = &output
		return nil
	}, unitOfWorkTransactionOptions())
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *MySQLRepository) Update(ctx context.Context, input UpdateConfigInput) (*domain.Config, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	configJSON, err := marshalPublicConfig(input.PublicConfig)
	if err != nil {
		return nil, err
	}
	var updated *domain.Config
	err = db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"name":                   strings.TrimSpace(input.Name),
			"config_json":            configJSON,
			"credential_secret":      input.CredentialSecret,
			"health_status":          string(domain.HealthUnknown),
			"last_health_code":       "",
			"last_health_message":    "",
			"last_health_latency_ms": uint32(0),
			"last_health_at":         nil,
			"version":                gorm.Expr("version + 1"),
			"updated_at":             persistenceNow(),
		}
		if input.RuntimeChanged {
			updates["runtime_revision"] = gorm.Expr("runtime_revision + 1")
		}
		result := secretWriteDB(tx).Model(&objectStorageConfigPO{}).
			Where("id = ? AND version = ?", input.ID, input.ExpectedVersion).
			Updates(updates)
		if result.Error != nil {
			return mapRepositoryError(result.Error)
		}
		if result.RowsAffected == 0 {
			return objectStorageCASFailure(tx, input.ID)
		}
		po, err := findObjectStorageConfig(tx, input.ID, false)
		if err != nil {
			return err
		}
		config, err := po.toDomain()
		if err != nil {
			return err
		}
		updated = &config
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *MySQLRepository) UpdateHealth(ctx context.Context, input UpdateHealthInput) error {
	db, err := r.dbFor(ctx)
	if err != nil {
		return err
	}
	result := db.Model(&objectStorageConfigPO{}).
		Where("id = ? AND version = ? AND runtime_revision = ?", input.ID, input.ExpectedVersion, input.ExpectedRuntimeRevision).
		UpdateColumns(map[string]any{
			"health_status":          string(input.Health.Status),
			"last_health_code":       input.Health.Code,
			"last_health_message":    input.Health.Message,
			"last_health_latency_ms": input.Health.LatencyMS,
			"last_health_at":         healthTime(input.Health.CheckedAt),
		})
	if result.Error != nil {
		return mapRepositoryError(result.Error)
	}
	if result.RowsAffected == 0 {
		return objectStorageCASFailure(db, input.ID)
	}
	return nil
}

func (r *MySQLRepository) Activate(ctx context.Context, id uint64, expectedVersion uint64) (*domain.Config, error) {
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var activated *domain.Config
	err = db.Transaction(func(tx *gorm.DB) error {
		target, err := findObjectStorageConfig(tx, id, true)
		if err != nil {
			return err
		}
		if target.Version != expectedVersion {
			return domain.ErrVersionConflict
		}
		if activeSlotEnabled(target.ActiveSlot) {
			config, err := target.toDomain()
			if err != nil {
				return err
			}
			activated = &config
			return nil
		}

		var current []objectStorageConfigPO
		err = withUpdateLock(tx).Where("active_slot = 1").Find(&current).Error
		if err != nil {
			return mapRepositoryError(err)
		}

		now := persistenceNow()
		if len(current) != 0 {
			result := tx.Model(&objectStorageConfigPO{}).
				Where("active_slot = 1").
				Updates(map[string]any{
					"active_slot": nil,
					"version":     gorm.Expr("version + 1"),
					"updated_at":  now,
				})
			if result.Error != nil {
				return mapRepositoryError(result.Error)
			}
			if result.RowsAffected == 0 {
				return domain.ErrVersionConflict
			}
		}

		slot := uint8(1)
		result := tx.Model(&objectStorageConfigPO{}).
			Where("id = ? AND version = ?", target.ID, target.Version).
			Updates(map[string]any{
				"active_slot": &slot,
				"version":     gorm.Expr("version + 1"),
				"updated_at":  now,
			})
		if result.Error != nil {
			return mapRepositoryError(result.Error)
		}
		if result.RowsAffected == 0 {
			return domain.ErrVersionConflict
		}

		po, err := findObjectStorageConfig(tx, target.ID, false)
		if err != nil {
			return err
		}
		config, err := po.toDomain()
		if err != nil {
			return err
		}
		activated = &config
		return nil
	}, unitOfWorkTransactionOptions())
	if err != nil {
		return nil, err
	}
	return activated, nil
}

func (r *MySQLRepository) Delete(ctx context.Context, id uint64, expectedVersion uint64, runtime domain.RuntimeDescriptor) error {
	db, err := r.dbFor(ctx)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		po, err := findObjectStorageConfig(tx, id, true)
		if err != nil {
			return err
		}
		if po.Version != expectedVersion {
			return domain.ErrVersionConflict
		}
		if activeSlotEnabled(po.ActiveSlot) ||
			(runtime.Source == domain.RuntimeSourceDatabase && runtime.ConfigID == id) {
			return domain.ErrActiveDeleteForbidden
		}
		result := tx.Where("id = ? AND version = ?", id, expectedVersion).Delete(&objectStorageConfigPO{})
		if result.Error != nil {
			return mapRepositoryError(result.Error)
		}
		if result.RowsAffected == 0 {
			return objectStorageCASFailure(tx, id)
		}
		return nil
	}, unitOfWorkTransactionOptions())
}

func (r *MySQLRepository) dbFor(ctx context.Context) (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, domain.ErrConfigInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return r.db.WithContext(ctx), nil
}

func newObjectStorageConfigPO(config domain.Config, now time.Time) (*objectStorageConfigPO, error) {
	configJSON, err := marshalPublicConfig(config.PublicConfig)
	if err != nil {
		return nil, err
	}
	createdAt := config.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := config.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	version := config.Version
	if version == 0 {
		version = 1
	}
	runtimeRevision := config.RuntimeRevision
	if runtimeRevision == 0 {
		runtimeRevision = 1
	}
	health := config.Health
	if health.Status == "" {
		health.Status = domain.HealthUnknown
	}
	var activeSlot *uint8
	if config.Active {
		slot := uint8(1)
		activeSlot = &slot
	}
	return &objectStorageConfigPO{
		ID:                  config.ID,
		Name:                strings.TrimSpace(config.Name),
		ProviderType:        string(config.ProviderType),
		ConfigJSON:          configJSON,
		CredentialSecret:    config.CredentialSecret,
		ActiveSlot:          activeSlot,
		HealthStatus:        string(health.Status),
		LastHealthCode:      health.Code,
		LastHealthMessage:   health.Message,
		LastHealthLatencyMS: health.LatencyMS,
		LastHealthAt:        healthTime(health.CheckedAt),
		Version:             version,
		RuntimeRevision:     runtimeRevision,
		CreatedAt:           createdAt.UTC().Truncate(time.Millisecond),
		UpdatedAt:           updatedAt.UTC().Truncate(time.Millisecond),
	}, nil
}

func (po *objectStorageConfigPO) toDomain() (domain.Config, error) {
	if po == nil {
		return domain.Config{}, domain.ErrNotFound
	}
	var publicConfig domain.PublicConfig
	if err := json.Unmarshal([]byte(po.ConfigJSON), &publicConfig); err != nil {
		return domain.Config{}, domain.ErrConfigInvalid
	}
	return domain.Config{
		ID:               po.ID,
		Name:             po.Name,
		ProviderType:     domain.ProviderType(po.ProviderType),
		PublicConfig:     publicConfig,
		CredentialSecret: po.CredentialSecret,
		Active:           activeSlotEnabled(po.ActiveSlot),
		Health: domain.Health{
			Status:    domain.HealthStatus(po.HealthStatus),
			Code:      po.LastHealthCode,
			Message:   po.LastHealthMessage,
			LatencyMS: po.LastHealthLatencyMS,
			CheckedAt: healthTime(po.LastHealthAt),
		},
		Version:         po.Version,
		RuntimeRevision: po.RuntimeRevision,
		CreatedAt:       po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}, nil
}

func marshalPublicConfig(config domain.PublicConfig) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", domain.ErrConfigInvalid
	}
	return string(data), nil
}

func findObjectStorageConfig(db *gorm.DB, id uint64, lock bool) (*objectStorageConfigPO, error) {
	var po objectStorageConfigPO
	query := db.Where("id = ?", id)
	if lock {
		query = withUpdateLock(query)
	}
	err := query.First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return &po, nil
}

func objectStorageCASFailure(db *gorm.DB, id uint64) error {
	var count int64
	if err := db.Model(&objectStorageConfigPO{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return mapRepositoryError(err)
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	return domain.ErrVersionConflict
}

func assignSQLiteObjectStorageID(db *gorm.DB, po *objectStorageConfigPO) error {
	if db.Dialector.Name() != "sqlite" || po == nil || po.ID != 0 {
		return nil
	}
	var maxID uint64
	if err := db.Model(&objectStorageConfigPO{}).Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
		return mapRepositoryError(err)
	}
	po.ID = maxID + 1
	return nil
}

func mapRepositoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ErrNotFound
	case isConflictDatabaseError(err):
		return domain.ErrVersionConflict
	default:
		return err
	}
}

func isConflictDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1062, 1205, 1213:
			return true
		default:
			return false
		}
	}
	var sqliteError sqlite3.Error
	if errors.As(err, &sqliteError) {
		return sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique ||
			sqliteError.ExtendedCode == sqlite3.ErrConstraintPrimaryKey ||
			sqliteError.Code == sqlite3.ErrBusy ||
			sqliteError.Code == sqlite3.ErrLocked
	}
	return false
}

func withUpdateLock(db *gorm.DB) *gorm.DB {
	return db.Clauses(clause.Locking{Strength: "UPDATE"})
}

func secretWriteDB(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{Logger: logger.Discard})
}

func unitOfWorkTransactionOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false}
}

func persistenceNow() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func activeSlotEnabled(slot *uint8) bool {
	return slot != nil && *slot == 1
}

func healthTime(input *time.Time) *time.Time {
	if input == nil {
		return nil
	}
	value := input.UTC().Truncate(time.Millisecond)
	return &value
}
