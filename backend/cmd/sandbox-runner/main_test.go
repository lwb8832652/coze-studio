// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunMigrationStatusDispatchesOnlyExactCommand(t *testing.T) {
	output := captureMainLog(t)
	called := 0
	getenv := func(key string) string {
		if key == "MYSQL_DSN" {
			return "runner:do-not-log-this@tcp(dev-mysql.invalid:3306)/coze_dev"
		}
		return ""
	}
	check := func(_ context.Context, gotGetenv func(string) string) error {
		called++
		require.Equal(t, getenv("MYSQL_DSN"), gotGetenv("MYSQL_DSN"))
		return nil
	}

	require.Equal(t, 0, runWithMigrationCheck([]string{"migration-status"}, getenv, check))
	require.Equal(t, 1, called)
	require.Contains(t, output.String(), "sandbox runner migration status is applied")
	require.NotContains(t, output.String(), "do-not-log-this")
	require.NotContains(t, output.String(), "20260814000100")
}

func TestRunMigrationStatusReturnsStableFailureWithoutLeakingCause(t *testing.T) {
	output := captureMainLog(t)
	getenv := func(string) string { return "runner:dsn-secret@tcp(db.invalid:3306)/coze_dev" }
	check := func(context.Context, func(string) string) error {
		return errors.New("database-secret-cause")
	}

	require.Equal(t, 1, runWithMigrationCheck([]string{"migration-status"}, getenv, check))
	require.Contains(t, output.String(), "sandbox runner migration status is unavailable")
	require.NotContains(t, output.String(), "database-secret-cause")
	require.NotContains(t, output.String(), "dsn-secret")
	require.NotContains(t, output.String(), "20260814000100")
}

func TestRunRejectsEveryOtherArgumentShapeWithoutCallingMigrationCheck(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"migration-status", "extra"}, {""}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			output := captureMainLog(t)
			called := false
			check := func(context.Context, func(string) string) error {
				called = true
				return nil
			}
			require.Equal(t, 1, runWithMigrationCheck(args, func(string) string { return "" }, check))
			require.False(t, called)
			require.Contains(t, output.String(), "sandbox runner command is invalid")
		})
	}
}

func TestRunWithoutArgumentsKeepsNormalServerPath(t *testing.T) {
	output := captureMainLog(t)
	called := false
	getenvCalls := 0
	check := func(context.Context, func(string) string) error {
		called = true
		return nil
	}
	getenv := func(string) string {
		getenvCalls++
		return ""
	}

	require.Equal(t, 1, runWithMigrationCheck(nil, getenv, check))
	require.False(t, called)
	require.Positive(t, getenvCalls)
	require.Contains(t, output.String(), "sandbox runner configuration is invalid")
}

func captureMainLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	writer := log.Writer()
	flags := log.Flags()
	prefix := log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(writer)
		log.SetFlags(flags)
		log.SetPrefix(prefix)
	})
	return &output
}
