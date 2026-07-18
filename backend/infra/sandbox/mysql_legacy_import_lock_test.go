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
	"io"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type providerCreateReleaseMode int

const (
	providerCreateReleaseSuccess providerCreateReleaseMode = iota
	providerCreateReleaseFailure
	providerCreateReleaseTimeout
)

type providerCreateReleaseObservation struct {
	contextErr  error
	deadline    time.Time
	hasDeadline bool
}

type providerCreateLockDriver struct {
	mu           sync.Mutex
	releaseMode  providerCreateReleaseMode
	releaseErr   error
	connections  []*providerCreateLockConn
	releaseCalls []providerCreateReleaseObservation
}

type providerCreateLockConnector struct {
	driver *providerCreateLockDriver
}

type providerCreateLockConn struct {
	driver *providerCreateLockDriver
	mu     sync.Mutex
	closed bool
}

type providerCreateLockTx struct{}

type providerCreateLockRows struct {
	value int64
	done  bool
}

func (c *providerCreateLockConnector) Connect(context.Context) (driver.Conn, error) {
	connection := &providerCreateLockConn{driver: c.driver}
	c.driver.mu.Lock()
	c.driver.connections = append(c.driver.connections, connection)
	c.driver.mu.Unlock()
	return connection, nil
}

func (c *providerCreateLockConnector) Driver() driver.Driver { return c.driver }

func (d *providerCreateLockDriver) Open(string) (driver.Conn, error) {
	return (&providerCreateLockConnector{driver: d}).Connect(context.Background())
}

func (c *providerCreateLockConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (c *providerCreateLockConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *providerCreateLockConn) Begin() (driver.Tx, error) {
	return &providerCreateLockTx{}, nil
}

func (c *providerCreateLockConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &providerCreateLockTx{}, nil
}

func (c *providerCreateLockConn) QueryContext(
	ctx context.Context,
	query string,
	_ []driver.NamedValue,
) (driver.Rows, error) {
	switch query {
	case getProviderCreateLockSQL:
		return &providerCreateLockRows{value: 1}, nil
	case releaseProviderCreateLockSQL:
		deadline, hasDeadline := ctx.Deadline()
		c.driver.mu.Lock()
		c.driver.releaseCalls = append(c.driver.releaseCalls, providerCreateReleaseObservation{
			contextErr:  ctx.Err(),
			deadline:    deadline,
			hasDeadline: hasDeadline,
		})
		mode := c.driver.releaseMode
		releaseErr := c.driver.releaseErr
		c.driver.mu.Unlock()
		switch mode {
		case providerCreateReleaseFailure:
			return nil, releaseErr
		case providerCreateReleaseTimeout:
			<-ctx.Done()
			return nil, ctx.Err()
		default:
			return &providerCreateLockRows{value: 1}, nil
		}
	default:
		return nil, errors.New("unexpected provider-create lock query")
	}
}

func (providerCreateLockTx) Commit() error   { return nil }
func (providerCreateLockTx) Rollback() error { return nil }

func (r *providerCreateLockRows) Columns() []string { return []string{"value"} }
func (r *providerCreateLockRows) Close() error      { return nil }
func (r *providerCreateLockRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = r.value
	return nil
}

func (d *providerCreateLockDriver) releaseObservations() []providerCreateReleaseObservation {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]providerCreateReleaseObservation(nil), d.releaseCalls...)
}

func (d *providerCreateLockDriver) firstConnectionClosed() bool {
	d.mu.Lock()
	connection := d.connections[0]
	d.mu.Unlock()
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.closed
}

const (
	getProviderCreateLockSQL     = "SELECT GET_LOCK(CONCAT(DATABASE(), ':coze:sandbox:provider-create'), ?)"
	releaseProviderCreateLockSQL = "SELECT RELEASE_LOCK(CONCAT(DATABASE(), ':coze:sandbox:provider-create'))"
)

type providerCreateTransactionRunner interface {
	WithinProviderCreateTransaction(
		context.Context,
		func(context.Context, domainsandbox.TransactionRepositories) error,
	) error
}

func newProviderCreateTransactionRepository(t *testing.T) (*MySQLRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return NewMySQLRepository(db), mock
}

func newProviderCreateLockCleanupRepository(
	t *testing.T,
	mode providerCreateReleaseMode,
) (*MySQLRepository, *providerCreateLockDriver) {
	t.Helper()
	lockDriver := &providerCreateLockDriver{
		releaseMode: mode,
		releaseErr:  errors.New("release failed"),
	}
	sqlDB := sql.OpenDB(&providerCreateLockConnector{driver: lockDriver})
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open cleanup-test gorm: %v", err)
	}
	return NewMySQLRepository(db), lockDriver
}

func requireProviderCreateTransactionRunner(t *testing.T, repository *MySQLRepository) providerCreateTransactionRunner {
	t.Helper()
	runner, ok := any(repository).(providerCreateTransactionRunner)
	if !ok {
		t.Fatal("MySQLRepository does not implement the shared provider-create transaction protocol")
	}
	return runner
}

func expectProviderCreateLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(getProviderCreateLockSQL)).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(1))
}

func expectProviderCreateUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(releaseProviderCreateLockSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
}

func TestProviderCreateTransactionReleasesOnlyAfterCommit(t *testing.T) {
	repository, mock := newProviderCreateTransactionRepository(t)
	expectProviderCreateLock(mock)
	mock.ExpectBegin()
	mock.ExpectCommit()
	expectProviderCreateUnlock(mock)
	called := false

	err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
		context.Background(),
		func(context.Context, domainsandbox.TransactionRepositories) error {
			called = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("WithinProviderCreateTransaction() error = %v", err)
	}
	if !called {
		t.Fatal("callback was not called")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ordered SQL expectations: %v", err)
	}
}

func TestProviderCreateTransactionRollsBackBeforeReleaseAndPreservesFailures(t *testing.T) {
	repository, mock := newProviderCreateTransactionRepository(t)
	callbackErr := errors.New("callback failed")
	rollbackErr := errors.New("rollback failed")
	expectProviderCreateLock(mock)
	mock.ExpectBegin()
	mock.ExpectRollback().WillReturnError(rollbackErr)
	expectProviderCreateUnlock(mock)

	err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
		context.Background(),
		func(context.Context, domainsandbox.TransactionRepositories) error { return callbackErr },
	)
	if !errors.Is(err, callbackErr) || !errors.Is(err, rollbackErr) || !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("error = %v, want callback, rollback, and unavailable causes", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ordered SQL expectations: %v", err)
	}
}

func TestProviderCreateTransactionFailsClosedOnAcquireCommitAndReleaseFailure(t *testing.T) {
	t.Run("acquire timeout", func(t *testing.T) {
		repository, mock := newProviderCreateTransactionRepository(t)
		mock.ExpectQuery(regexp.QuoteMeta(getProviderCreateLockSQL)).WithArgs(5).
			WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(0))
		called := false
		err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
			context.Background(),
			func(context.Context, domainsandbox.TransactionRepositories) error { called = true; return nil },
		)
		if !errors.Is(err, domainsandbox.ErrUnavailable) || called {
			t.Fatalf("error/called = %v/%t, want unavailable/false", err, called)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("SQL expectations: %v", err)
		}
	})

	t.Run("commit failure still releases", func(t *testing.T) {
		repository, mock := newProviderCreateTransactionRepository(t)
		commitErr := errors.New("commit failed")
		expectProviderCreateLock(mock)
		mock.ExpectBegin()
		mock.ExpectCommit().WillReturnError(commitErr)
		expectProviderCreateUnlock(mock)
		err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
			context.Background(),
			func(context.Context, domainsandbox.TransactionRepositories) error { return nil },
		)
		if !errors.Is(err, commitErr) || !errors.Is(err, domainsandbox.ErrUnavailable) {
			t.Fatalf("error = %v, want commit and unavailable causes", err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("ordered SQL expectations: %v", err)
		}
	})

	t.Run("release failure after commit", func(t *testing.T) {
		repository, mock := newProviderCreateTransactionRepository(t)
		releaseErr := errors.New("release failed")
		expectProviderCreateLock(mock)
		mock.ExpectBegin()
		mock.ExpectCommit()
		mock.ExpectQuery(regexp.QuoteMeta(releaseProviderCreateLockSQL)).WillReturnError(releaseErr)
		err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
			context.Background(),
			func(context.Context, domainsandbox.TransactionRepositories) error { return nil },
		)
		if !errors.Is(err, releaseErr) || !errors.Is(err, domainsandbox.ErrUnavailable) {
			t.Fatalf("error = %v, want release and unavailable causes", err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("ordered SQL expectations: %v", err)
		}
	})
}

func TestProviderCreateTransactionReleasesWithBoundedCleanupContextAfterRequestCancellation(t *testing.T) {
	repository, lockDriver := newProviderCreateLockCleanupRepository(t, providerCreateReleaseSuccess)
	requestCtx, cancel := context.WithCancel(context.Background())
	callbackErr := errors.New("callback canceled request")

	err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
		requestCtx,
		func(context.Context, domainsandbox.TransactionRepositories) error {
			cancel()
			return callbackErr
		},
	)
	if !errors.Is(err, callbackErr) {
		t.Fatalf("error = %v, want callback error", err)
	}
	observations := lockDriver.releaseObservations()
	if len(observations) != 1 {
		t.Fatalf("release calls = %d, want 1", len(observations))
	}
	if observations[0].contextErr != nil {
		t.Fatalf("release context error = %v, want active cleanup context", observations[0].contextErr)
	}
	if !observations[0].hasDeadline {
		t.Fatal("release context has no cleanup deadline")
	}
	remaining := time.Until(observations[0].deadline)
	if remaining <= 0 || remaining > 2*time.Second {
		t.Fatalf("release cleanup deadline remaining = %v, want (0, 2s]", remaining)
	}
	if lockDriver.firstConnectionClosed() {
		t.Fatal("successful release discarded a healthy physical connection")
	}
}

func TestProviderCreateTransactionDiscardsPhysicalConnectionWhenReleaseFails(t *testing.T) {
	repository, lockDriver := newProviderCreateLockCleanupRepository(t, providerCreateReleaseFailure)

	err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
		context.Background(),
		func(context.Context, domainsandbox.TransactionRepositories) error { return nil },
	)
	if !errors.Is(err, lockDriver.releaseErr) || !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("error = %v, want release and unavailable causes", err)
	}
	if !lockDriver.firstConnectionClosed() {
		t.Fatal("release failure returned the physical connection to the pool")
	}
}

func TestProviderCreateTransactionDiscardsPhysicalConnectionWhenReleaseTimesOut(t *testing.T) {
	repository, lockDriver := newProviderCreateLockCleanupRepository(t, providerCreateReleaseTimeout)

	err := requireProviderCreateTransactionRunner(t, repository).WithinProviderCreateTransaction(
		context.Background(),
		func(context.Context, domainsandbox.TransactionRepositories) error { return nil },
	)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("error = %v, want deadline and unavailable causes", err)
	}
	if !lockDriver.firstConnectionClosed() {
		t.Fatal("release timeout returned the physical connection to the pool")
	}
}
