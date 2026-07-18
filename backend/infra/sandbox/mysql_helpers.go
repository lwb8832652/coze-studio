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

package sandbox

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func consistentReadTransactionOptions(transactionBound bool) (*sql.TxOptions, bool) {
	if transactionBound {
		return nil, false
	}
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}, true
}

func unitOfWorkTransactionOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false}
}

func (r *MySQLRepository) withConsistentRead(db *gorm.DB, fn func(*gorm.DB) error) error {
	options, startRoot := consistentReadTransactionOptions(r.txState != nil)
	if !startRoot {
		return fn(db)
	}
	return db.Transaction(fn, options)
}

func withUnitOfWorkTransaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(fn, unitOfWorkTransactionOptions())
}

func findLiveProvider(db *gorm.DB, providerID int64, lock bool) (*providerPO, error) {
	providerPOID, err := positiveDomainInt64ToUint64(providerID)
	if err != nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	var po providerPO
	query := db.Where("id = ? AND deleted_at IS NULL", providerPOID)
	if lock {
		query = withUpdateLock(query)
	}
	err = query.First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainsandbox.ErrProviderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &po, nil
}

func findProviderDefault(db *gorm.DB, scope domainsandbox.Scope, lock bool) (*providerDefaultPO, error) {
	var po providerDefaultPO
	query := db.Where("scope = ?", string(scope))
	if lock {
		query = withUpdateLock(query)
	}
	err := query.First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &po, nil
}

func providerCASFailure(db *gorm.DB, providerID int64) error {
	providerPOID, err := positiveDomainInt64ToUint64(providerID)
	if err != nil {
		return domainsandbox.ErrProviderNotFound
	}
	var count int64
	if err := db.Model(&providerPO{}).Where("id = ? AND deleted_at IS NULL", providerPOID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domainsandbox.ErrProviderNotFound
	}
	return domainsandbox.ErrVersionConflict
}

func domainDefault(po *providerDefaultPO) (*domainsandbox.ProviderDefault, error) {
	if po == nil {
		return nil, nil
	}
	return po.toDomain()
}

func validateDefaultScope(scope domainsandbox.Scope) error {
	_, _, err := domainsandbox.NormalizeSetProviderDefaultInput(domainsandbox.SetProviderDefaultInput{
		Scope: scope, ProviderID: 1, ExpectedVersion: 0, ActorUserID: 1,
	}, nil)
	return err
}

func validateProviderPersistenceFields(
	providerType domainsandbox.ProviderType,
	endpointSecret string,
	endpointHint string,
	credentialSecret string,
	credentialFingerprint string,
) error {
	return domainsandbox.ValidateProviderSecretPersistence(
		providerType,
		endpointSecret,
		endpointHint,
		credentialSecret,
		credentialFingerprint,
	)
}

func withUpdateLock(db *gorm.DB) *gorm.DB {
	// Dialectors that do not support row locks (notably the SQLite test
	// dialect) suppress this clause while retaining the lock intent on the
	// statement. MySQL emits SELECT ... FOR UPDATE.
	return db.Clauses(clause.Locking{Strength: "UPDATE"})
}

func secretWriteDB(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{Logger: logger.Discard})
}

func persistenceNow() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func escapeLikeKeyword(keyword string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword)
}

func isDuplicateDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return true
	}
	var sqliteError sqlite3.Error
	if errors.As(err, &sqliteError) {
		return sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique ||
			sqliteError.ExtendedCode == sqlite3.ErrConstraintPrimaryKey
	}
	return false
}
