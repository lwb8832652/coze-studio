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
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

var errTransactionRepositoriesClosed = errors.New("sandbox transaction repositories are closed")

type MySQLRepository struct {
	db      *gorm.DB
	txState *transactionRepositoryState
}

type transactionRepositoryState struct {
	mu       sync.Mutex
	cond     *sync.Cond
	active   bool
	closing  bool
	inFlight int
}

var (
	_ domainsandbox.ProviderRepository           = (*MySQLRepository)(nil)
	_ domainsandbox.ProviderDefaultRepository    = (*MySQLRepository)(nil)
	_ domainsandbox.ProviderManagementRepository = (*MySQLRepository)(nil)
	_ domainsandbox.ProviderAuditRepository      = (*MySQLRepository)(nil)
	_ domainsandbox.UnitOfWork                   = (*MySQLRepository)(nil)
	_ domainsandbox.ProviderCreateUnitOfWork     = (*MySQLRepository)(nil)
)

const (
	providerCreateLockTimeoutSeconds  = 5
	providerCreateLockCleanupTimeout  = time.Second
	mysqlGetProviderCreateLockSQL     = "SELECT GET_LOCK(CONCAT(DATABASE(), ':coze:sandbox:provider-create'), ?)"
	mysqlReleaseProviderCreateLockSQL = "SELECT RELEASE_LOCK(CONCAT(DATABASE(), ':coze:sandbox:provider-create'))"
)

var errProviderCreateConnectionEvictionFailed = errors.New("sandbox provider-create connection eviction failed")

// NewMySQLRepository accepts a root database handle. Externally supplied
// transactions are unsupported; transaction-bound repositories are created
// exclusively by WithinTransaction so isolation and lifetime remain enforced.
func NewMySQLRepository(db *gorm.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	release, err := r.acquireOperation()
	if err != nil {
		return err
	}
	defer release()
	if callback == nil {
		return domainsandbox.ErrInvalidInput
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return err
	}
	return withUnitOfWorkTransaction(db, func(tx *gorm.DB) error {
		state := newTransactionRepositoryState()
		bound := &MySQLRepository{db: tx, txState: state}
		defer state.close()
		return callback(ctx, domainsandbox.TransactionRepositories{
			Providers: bound,
			Defaults:  bound,
			Audits:    bound,
		})
	})
}

func (r *MySQLRepository) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) (resultErr error) {
	releaseOperation, err := r.acquireOperation()
	if err != nil {
		return err
	}
	defer releaseOperation()
	if callback == nil {
		return domainsandbox.ErrInvalidInput
	}
	if r.txState != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return unavailableDatabaseError(err)
	}
	sqlConnection, err := sqlDB.Conn(ctx)
	if err != nil {
		return unavailableDatabaseError(err)
	}
	returnConnectionToPool := true
	defer func() {
		if !returnConnectionToPool {
			return
		}
		closeErr := sqlConnection.Close()
		if closeErr != nil && !errors.Is(closeErr, sql.ErrConnDone) {
			resultErr = errors.Join(resultErr, unavailableDatabaseError(closeErr))
		}
	}()

	connection := db.Session(&gorm.Session{NewDB: true})
	connection.Statement.ConnPool = sqlConnection
	if acquireErr := acquireProviderCreateLock(connection); acquireErr != nil {
		return acquireErr
	}
	defer func() {
		releaseErr, evictionErr := cleanupProviderCreateLock(ctx, connection, sqlConnection)
		if evictionErr != nil {
			// An unexpected database/sql eviction failure must not return a
			// potentially lock-holding session to the shared pool.
			returnConnectionToPool = false
		}
		resultErr = errors.Join(resultErr, releaseErr, evictionErr)
	}()
	return runProviderCreateTransaction(ctx, connection, callback)
}

func acquireProviderCreateLock(db *gorm.DB) error {
	var acquired sql.NullInt64
	if err := db.Raw(mysqlGetProviderCreateLockSQL, providerCreateLockTimeoutSeconds).Row().Scan(&acquired); err != nil {
		return unavailableDatabaseError(err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func releaseProviderCreateLock(db *gorm.DB) error {
	var released sql.NullInt64
	if err := db.Raw(mysqlReleaseProviderCreateLockSQL).Row().Scan(&released); err != nil {
		return unavailableDatabaseError(err)
	}
	if !released.Valid || released.Int64 != 1 {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func cleanupProviderCreateLock(
	requestCtx context.Context,
	db *gorm.DB,
	connection *sql.Conn,
) (releaseErr, evictionErr error) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), providerCreateLockCleanupTimeout)
	defer cancel()
	if releaseErr = releaseProviderCreateLock(db.WithContext(cleanupCtx)); releaseErr == nil {
		return nil, nil
	}
	return releaseErr, discardProviderCreateConnection(connection)
}

func discardProviderCreateConnection(connection *sql.Conn) error {
	if connection == nil {
		return errors.Join(domainsandbox.ErrUnavailable, errProviderCreateConnectionEvictionFailed)
	}
	err := connection.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	if err == nil {
		err = errProviderCreateConnectionEvictionFailed
	}
	return errors.Join(domainsandbox.ErrUnavailable, errProviderCreateConnectionEvictionFailed, err)
}

func runProviderCreateTransaction(
	ctx context.Context,
	connection *gorm.DB,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) (resultErr error) {
	tx := connection.Begin(unitOfWorkTransactionOptions())
	if tx.Error != nil {
		return unavailableDatabaseError(tx.Error)
	}
	state := newTransactionRepositoryState()
	bound := &MySQLRepository{db: tx, txState: state}
	var closeOnce sync.Once
	closeRepositories := func() { closeOnce.Do(state.close) }
	defer func() {
		closeRepositories()
		if recovered := recover(); recovered != nil {
			rollbackErr := tx.Rollback().Error
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				panic(errors.Join(domainsandbox.ErrUnavailable, rollbackErr))
			}
			panic(recovered)
		}
	}()

	callbackErr := callback(ctx, domainsandbox.TransactionRepositories{
		Providers: bound,
		Defaults:  bound,
		Audits:    bound,
	})
	closeRepositories()
	if callbackErr != nil {
		rollbackErr := tx.Rollback().Error
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(callbackErr, unavailableDatabaseError(rollbackErr))
		}
		return callbackErr
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		rollbackErr := tx.Rollback().Error
		if errors.Is(rollbackErr, sql.ErrTxDone) {
			rollbackErr = nil
		}
		return errors.Join(unavailableDatabaseError(commitErr), unavailableDatabaseError(rollbackErr))
	}
	return nil
}

func unavailableDatabaseError(cause error) error {
	if cause == nil {
		return nil
	}
	return errors.Join(domainsandbox.ErrUnavailable, cause)
}

func (r *MySQLRepository) dbFor(ctx context.Context) (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return r.db.WithContext(ctx), nil
}

func (r *MySQLRepository) acquireOperation() (func(), error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if r.txState == nil {
		return func() {}, nil
	}
	r.txState.mu.Lock()
	if !r.txState.active || r.txState.closing {
		r.txState.mu.Unlock()
		return nil, errTransactionRepositoriesClosed
	}
	r.txState.inFlight++
	r.txState.mu.Unlock()
	return r.txState.endOperation, nil
}

func newTransactionRepositoryState() *transactionRepositoryState {
	state := &transactionRepositoryState{active: true}
	state.cond = sync.NewCond(&state.mu)
	return state
}

func (s *transactionRepositoryState) close() {
	s.mu.Lock()
	s.closing = true
	s.active = false
	s.cond.Broadcast()
	for s.inFlight > 0 {
		s.cond.Wait()
	}
	s.mu.Unlock()
}

func (s *transactionRepositoryState) endOperation() {
	s.mu.Lock()
	s.inFlight--
	if s.inFlight == 0 {
		s.cond.Broadcast()
	}
	s.mu.Unlock()
}
