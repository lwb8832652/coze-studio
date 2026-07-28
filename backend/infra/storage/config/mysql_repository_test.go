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
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMySQLRepositoryCreateListAndActiveUniqueness(t *testing.T) {
	repository, db := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	first := validRepositoryConfig("minio-a")
	created, err := repository.Create(ctx, first)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second := validRepositoryConfig("minio-b")
	second.Active = true
	if _, err = repository.Create(ctx, second); err != nil {
		t.Fatalf("Create(second active) error = %v", err)
	}
	list, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d", len(list))
	}
	var activeRows int64
	if err = db.Model(&objectStorageConfigPO{}).Where("active_slot = 1").Count(&activeRows).Error; err != nil {
		t.Fatalf("count active error = %v", err)
	}
	if activeRows != 1 || created.ID == 0 {
		t.Fatalf("active rows = %d created id = %d", activeRows, created.ID)
	}
	third := validRepositoryConfig("minio-c")
	third.Active = true
	if _, err = repository.Create(ctx, third); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("Create(second active row) error = %v", err)
	}
}

func TestMySQLRepositoryOptimisticLockAndRuntimeRevision(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	created, err := repository.Create(ctx, validRepositoryConfig("minio-a"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	updated, err := repository.Update(ctx, UpdateConfigInput{
		ID:               created.ID,
		ExpectedVersion:  created.Version,
		Name:             "renamed",
		PublicConfig:     created.PublicConfig,
		CredentialSecret: created.CredentialSecret,
		RuntimeChanged:   false,
	})
	if err != nil {
		t.Fatalf("Update(name) error = %v", err)
	}
	if updated.Version != created.Version+1 || updated.RuntimeRevision != created.RuntimeRevision {
		t.Fatalf("version/runtime = %d/%d", updated.Version, updated.RuntimeRevision)
	}
	_, err = repository.Update(ctx, UpdateConfigInput{
		ID:               created.ID,
		ExpectedVersion:  created.Version,
		Name:             "stale",
		PublicConfig:     created.PublicConfig,
		CredentialSecret: created.CredentialSecret,
		RuntimeChanged:   false,
	})
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestMySQLRepositoryActivateIsAtomicAndDeleteGuardsActive(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	oldConfig := validRepositoryConfig("old")
	oldConfig.Active = true
	old, err := repository.Create(ctx, oldConfig)
	if err != nil {
		t.Fatalf("Create(old) error = %v", err)
	}
	next, err := repository.Create(ctx, validRepositoryConfig("next"))
	if err != nil {
		t.Fatalf("Create(next) error = %v", err)
	}
	activated, err := repository.Activate(ctx, next.ID, next.Version)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if !activated.Active {
		t.Fatal("target is not active")
	}
	if err = repository.Delete(ctx, activated.ID, activated.Version, domain.RuntimeDescriptor{}); !errors.Is(err, domain.ErrActiveDeleteForbidden) {
		t.Fatalf("Delete(active) error = %v", err)
	}
	oldAfter, err := repository.Get(ctx, old.ID)
	if err != nil {
		t.Fatalf("Get(old) error = %v", err)
	}
	if oldAfter.Active {
		t.Fatal("old config is still active")
	}
}

func TestMySQLRepositoryDuplicateNameMapsVersionConflict(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	if _, err := repository.Create(ctx, validRepositoryConfig("minio-a")); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if _, err := repository.Create(ctx, validRepositoryConfig("minio-a")); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("Create(duplicate name) error = %v", err)
	}
}

func TestMySQLRepositoryCreateWithCredentialEncryptsWithAssignedID(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	codec := &recordingObjectStorageCredentialCodec{}
	input := validRepositoryConfig("minio-import")
	input.CredentialSecret = ""
	input.Active = true

	created, err := repository.CreateWithCredential(ctx, input, domain.CredentialInput{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	}, codec)
	if err != nil {
		t.Fatalf("CreateWithCredential() error = %v", err)
	}
	if created.ID == 0 || codec.id != created.ID {
		t.Fatalf("created ID = %d codec ID = %d", created.ID, codec.id)
	}
	if codec.version != CredentialAADVersion {
		t.Fatalf("credential AAD version = %d, want %d", codec.version, CredentialAADVersion)
	}
	if created.CredentialSecret != "encrypted-with-assigned-id" {
		t.Fatalf("CredentialSecret = %q", created.CredentialSecret)
	}
	if !created.Active {
		t.Fatal("created config is not active")
	}
}

func TestMySQLRepositoryCreateWithCredentialRollsBackOnEncryptError(t *testing.T) {
	repository, db := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	codec := &recordingObjectStorageCredentialCodec{err: domain.ErrCredentialUnavailable}
	input := validRepositoryConfig("minio-import")
	input.CredentialSecret = ""

	_, err := repository.CreateWithCredential(ctx, input, domain.CredentialInput{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	}, codec)
	if !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("CreateWithCredential() error = %v", err)
	}
	var rows int64
	if countErr := db.Model(&objectStorageConfigPO{}).Count(&rows).Error; countErr != nil {
		t.Fatalf("count rows error = %v", countErr)
	}
	if rows != 0 {
		t.Fatalf("rows = %d, want rollback", rows)
	}
}

func TestMySQLRepositoryGetActiveEmptyPrimary(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	if _, err := repository.Create(ctx, validRepositoryConfig("minio-a")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := repository.GetActive(ctx); !errors.Is(err, domain.ErrPrimaryConfigMissing) {
		t.Fatalf("GetActive() error = %v", err)
	}
}

func TestMySQLRepositoryUpdateHealthDoesNotBumpVersion(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	input := validRepositoryConfig("minio-a")
	initialUpdatedAt := time.Date(2026, 7, 28, 9, 0, 0, 123000000, time.UTC)
	input.CreatedAt = initialUpdatedAt
	input.UpdatedAt = initialUpdatedAt
	created, err := repository.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	err = repository.UpdateHealth(ctx, created.ID, domain.Health{
		Status:    domain.HealthHealthy,
		Code:      "OK",
		Message:   "connected",
		LatencyMS: 37,
		CheckedAt: &checkedAt,
	})
	if err != nil {
		t.Fatalf("UpdateHealth() error = %v", err)
	}
	after, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if after.Version != created.Version {
		t.Fatalf("version bumped from %d to %d", created.Version, after.Version)
	}
	if !after.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("updated_at changed from %s to %s", created.UpdatedAt.Format(time.RFC3339Nano), after.UpdatedAt.Format(time.RFC3339Nano))
	}
	if after.Health.Status != domain.HealthHealthy || after.Health.Code != "OK" ||
		after.Health.Message != "connected" || after.Health.LatencyMS != 37 ||
		after.Health.CheckedAt == nil || !after.Health.CheckedAt.Equal(checkedAt) {
		t.Fatalf("health = %#v", after.Health)
	}
}

func TestMySQLRepositoryDeleteRuntimeActiveConfigForbidden(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	created, err := repository.Create(ctx, validRepositoryConfig("minio-a"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	err = repository.Delete(ctx, created.ID, created.Version, domain.RuntimeDescriptor{
		Source:          domain.RuntimeSourceDatabase,
		ConfigID:        created.ID,
		RuntimeRevision: created.RuntimeRevision,
		ProviderType:    created.ProviderType,
	})
	if !errors.Is(err, domain.ErrActiveDeleteForbidden) {
		t.Fatalf("Delete(runtime active) error = %v", err)
	}
}

func TestMySQLRepositoryDeleteStaleVersionConflict(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	created, err := repository.Create(ctx, validRepositoryConfig("minio-a"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	updated, err := repository.Update(ctx, UpdateConfigInput{
		ID:               created.ID,
		ExpectedVersion:  created.Version,
		Name:             "renamed",
		PublicConfig:     created.PublicConfig,
		CredentialSecret: created.CredentialSecret,
		RuntimeChanged:   true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.RuntimeRevision != created.RuntimeRevision+1 {
		t.Fatalf("runtime revision = %d", updated.RuntimeRevision)
	}
	if err = repository.Delete(ctx, created.ID, created.Version, domain.RuntimeDescriptor{}); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("Delete(stale version) error = %v", err)
	}
}

func newObjectStorageSQLiteRepository(t *testing.T) (*MySQLRepository, *gorm.DB) {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite error = %v", err)
	}
	if err = db.AutoMigrate(&objectStorageConfigPO{}); err != nil {
		t.Fatalf("migrate sqlite error = %v", err)
	}
	return NewMySQLRepository(db), db
}

func validRepositoryConfig(name string) domain.Config {
	now := time.Now().UTC()
	return domain.Config{
		Name:             name,
		ProviderType:     domain.ProviderMinIO,
		PublicConfig:     domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000", UseSSL: false},
		CredentialSecret: "v1-test-envelope",
		Health:           domain.Health{Status: domain.HealthUnknown},
		Version:          1,
		RuntimeRevision:  1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

type recordingObjectStorageCredentialCodec struct {
	id         uint64
	provider   domain.ProviderType
	version    uint64
	credential domain.CredentialInput
	err        error
}

func (c *recordingObjectStorageCredentialCodec) Encrypt(id uint64, provider domain.ProviderType, version uint64, input domain.CredentialInput) (string, error) {
	c.id = id
	c.provider = provider
	c.version = version
	c.credential = input
	if c.err != nil {
		return "", c.err
	}
	return "encrypted-with-assigned-id", nil
}
