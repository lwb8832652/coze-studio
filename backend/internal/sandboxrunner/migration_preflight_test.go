// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

const migrationPreflightTestDSN = "runner:do-not-log-this@tcp(dev-mysql.invalid:3306)/coze_dev?parseTime=true"

func TestCheckRequiredMigrationAcceptsOneFullyAppliedRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationQuery)).
		WithArgs(requiredMigrationVersion).
		WillReturnRows(fullyAppliedRequiredMigrationRows(nil))
	mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationSchemaQuery)).
		WithArgs("coze_dev").
		WillReturnRows(sqlmock.NewRows([]string{"required_columns"}).AddRow(requiredMigrationSchemaColumnCount))
	mock.ExpectRollback()
	mock.ExpectClose()

	err = checkRequiredMigration(context.Background(), migrationPreflightGetenv(migrationPreflightTestDSN), func(config *mysqldriver.Config) (*sql.DB, error) {
		require.Equal(t, "coze_dev", config.DBName)
		require.False(t, config.MultiStatements)
		return db, nil
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckRequiredMigrationUsesTenSecondReadOnlyTransaction(t *testing.T) {
	ctx, cancel := migrationPreflightContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(10*time.Second), deadline, 100*time.Millisecond)
	require.True(t, migrationPreflightTxOptions().ReadOnly)
}

func TestCheckRequiredMigrationRejectsUnsafeDSNBeforeOpeningDatabase(t *testing.T) {
	tests := map[string]string{
		"missing":             "",
		"missing database":    "runner:password@tcp(dev-mysql.invalid:3306)/?parseTime=true",
		"multiple statements": "runner:password@tcp(dev-mysql.invalid:3306)/coze_dev?multiStatements=true",
	}
	for name, dsn := range tests {
		t.Run(name, func(t *testing.T) {
			opened := false
			err := checkRequiredMigration(context.Background(), migrationPreflightGetenv(dsn), func(*mysqldriver.Config) (*sql.DB, error) {
				opened = true
				return nil, errors.New("must not open")
			})
			require.ErrorIs(t, err, ErrMigrationPreflight)
			require.False(t, opened)
		})
	}
}

func TestCheckRequiredMigrationFailsClosedForNonAppliedRevisionStates(t *testing.T) {
	tests := map[string]*sqlmock.Rows{
		"missing":           sqlmock.NewRows([]string{"description", "applied", "total", "error"}),
		"wrong description": sqlmock.NewRows([]string{"description", "applied", "total", "error"}).AddRow("agent_run_attempts_interrupted", 4, 4, nil),
		"zero statements":   sqlmock.NewRows([]string{"description", "applied", "total", "error"}).AddRow(requiredMigrationDescription, 0, 0, nil),
		"partial":           sqlmock.NewRows([]string{"description", "applied", "total", "error"}).AddRow(requiredMigrationDescription, 3, 4, nil),
		"failed":            sqlmock.NewRows([]string{"description", "applied", "total", "error"}).AddRow(requiredMigrationDescription, 4, 4, "migration failed"),
		"duplicate": sqlmock.NewRows([]string{"description", "applied", "total", "error"}).
			AddRow(requiredMigrationDescription, 4, 4, nil).
			AddRow(requiredMigrationDescription, 4, 4, nil),
	}
	for name, rows := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationQuery)).
				WithArgs(requiredMigrationVersion).
				WillReturnRows(rows)
			mock.ExpectRollback()
			mock.ExpectClose()

			err = checkRequiredMigration(context.Background(), migrationPreflightGetenv(migrationPreflightTestDSN), func(*mysqldriver.Config) (*sql.DB, error) {
				return db, nil
			})
			require.ErrorIs(t, err, ErrMigrationPreflight)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCheckRequiredMigrationAcceptsEmptyAtlasError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationQuery)).
		WithArgs(requiredMigrationVersion).
		WillReturnRows(fullyAppliedRequiredMigrationRows(""))
	mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationSchemaQuery)).
		WithArgs("coze_dev").
		WillReturnRows(sqlmock.NewRows([]string{"required_columns"}).AddRow(requiredMigrationSchemaColumnCount))
	mock.ExpectRollback()
	mock.ExpectClose()

	err = checkRequiredMigration(context.Background(), migrationPreflightGetenv(migrationPreflightTestDSN), func(*mysqldriver.Config) (*sql.DB, error) {
		return db, nil
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCheckRequiredMigrationFailsClosedWhenSharedAIOSchemaIsMissingOrUnknown(t *testing.T) {
	tests := map[string]func(sqlmock.Sqlmock){
		"missing required column": func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationSchemaQuery)).
				WithArgs("coze_dev").
				WillReturnRows(sqlmock.NewRows([]string{"required_columns"}).AddRow(requiredMigrationSchemaColumnCount - 1))
		},
		"schema query error": func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationSchemaQuery)).
				WithArgs("coze_dev").
				WillReturnError(errors.New("schema leaked detail"))
		},
	}
	for name, expectSchema := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationQuery)).
				WithArgs(requiredMigrationVersion).
				WillReturnRows(fullyAppliedRequiredMigrationRows(nil))
			expectSchema(mock)
			mock.ExpectRollback()
			mock.ExpectClose()

			err = checkRequiredMigration(context.Background(), migrationPreflightGetenv(migrationPreflightTestDSN), func(*mysqldriver.Config) (*sql.DB, error) {
				return db, nil
			})
			require.ErrorIs(t, err, ErrMigrationPreflight)
			require.Equal(t, ErrMigrationPreflight.Error(), err.Error())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCheckRequiredMigrationHidesInfrastructureFailuresBehindStableError(t *testing.T) {
	tests := map[string]func(t *testing.T) migrationDatabaseOpener{
		"open": func(t *testing.T) migrationDatabaseOpener {
			return func(*mysqldriver.Config) (*sql.DB, error) {
				return nil, errors.New("open leaked detail")
			}
		},
		"begin": func(t *testing.T) migrationDatabaseOpener {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			mock.ExpectBegin().WillReturnError(errors.New("begin leaked detail"))
			mock.ExpectClose()
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			return func(*mysqldriver.Config) (*sql.DB, error) { return db, nil }
		},
		"query": func(t *testing.T) migrationDatabaseOpener {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationQuery)).
				WithArgs(requiredMigrationVersion).
				WillReturnError(errors.New("query leaked detail"))
			mock.ExpectRollback()
			mock.ExpectClose()
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			return func(*mysqldriver.Config) (*sql.DB, error) { return db, nil }
		},
		"rollback": func(t *testing.T) migrationDatabaseOpener {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationQuery)).
				WithArgs(requiredMigrationVersion).
				WillReturnRows(fullyAppliedRequiredMigrationRows(nil))
			mock.ExpectQuery(regexp.QuoteMeta(requiredMigrationSchemaQuery)).
				WithArgs("coze_dev").
				WillReturnRows(sqlmock.NewRows([]string{"required_columns"}).AddRow(requiredMigrationSchemaColumnCount))
			mock.ExpectRollback().WillReturnError(errors.New("rollback leaked detail"))
			mock.ExpectClose()
			t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()) })
			return func(*mysqldriver.Config) (*sql.DB, error) { return db, nil }
		},
	}
	for name, opener := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkRequiredMigration(context.Background(), migrationPreflightGetenv(migrationPreflightTestDSN), opener(t))
			require.ErrorIs(t, err, ErrMigrationPreflight)
			require.Equal(t, ErrMigrationPreflight.Error(), err.Error())
		})
	}
}

func fullyAppliedRequiredMigrationRows(migrationError any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"description", "applied", "total", "error"}).
		AddRow(requiredMigrationDescription, 4, 4, migrationError)
}

func migrationPreflightGetenv(dsn string) func(string) string {
	return func(key string) string {
		if key == "MYSQL_DSN" {
			return dsn
		}
		return ""
	}
}
