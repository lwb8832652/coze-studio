// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

func TestCodePluginRepositoryMySQLConcurrentPublishAndDelete(t *testing.T) {
	if os.Getenv("COZE_CODE_PLUGIN_MYSQL_INTEGRATION") != "1" {
		t.Skip("set COZE_CODE_PLUGIN_MYSQL_INTEGRATION=1 to run against an isolated local MySQL schema")
	}
	dsn := os.Getenv("COZE_CODE_PLUGIN_MYSQL_INTEGRATION_DSN")
	require.NotEmpty(t, dsn)
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(config.DBName, "coze_code_plugin_repo_test_"),
		"integration database must use the coze_code_plugin_repo_test_ prefix")
	if config.Net != "unix" {
		host, _, splitErr := net.SplitHostPort(config.Addr)
		require.NoError(t, splitErr)
		require.True(t, host == "localhost" || net.ParseIP(host).IsLoopback(),
			"integration database must be local")
	}

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{TranslateError: true})
	require.NoError(t, err)
	resetCodePluginMySQLSchema(t, db)
	t.Cleanup(func() {
		for _, table := range []string{
			"plugin_code_version_files",
			"plugin_code_versions",
			"plugin_code_draft_files",
			"plugin_code_drafts",
			"plugin_draft",
		} {
			require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+table+"`").Error)
		}
	})

	const pluginID int64 = 901
	require.NoError(t, db.Exec("INSERT INTO `plugin_draft` (`id`, `space_id`) VALUES (?, ?)", pluginID, 10).Error)
	repo := NewCodePluginRepository(db)
	ctx := context.Background()
	current, err := repo.SaveDraftCAS(ctx, codeDraftFixture(pluginID, 10, "return 0"), 0)
	require.NoError(t, err)

	for iteration := 1; iteration <= 12; iteration++ {
		versionName := "race-" + string(rune('a'+iteration))
		next := codeDraftFixture(pluginID, 10, "return "+versionName)
		var waitGroup sync.WaitGroup
		waitGroup.Add(2)
		require.NoError(t, repo.MarkDebuggedCAS(ctx, current.PluginID, current.Revision))
		saveResult := make(chan error, 1)
		publishResult := make(chan error, 1)
		go func(expectedRevision int64) {
			defer waitGroup.Done()
			_, saveErr := repo.SaveDraftCAS(ctx, next, expectedRevision)
			saveResult <- saveErr
		}(current.Revision)
		go func() {
			defer waitGroup.Done()
			publishResult <- repo.PublishDebuggedVersion(ctx, pluginID, versionName, 99)
		}()
		waitGroup.Wait()
		require.NoError(t, <-saveResult)
		publishErr := <-publishResult
		require.True(t, publishErr == nil || errors.Is(publishErr, ErrCodeDraftNotDebugged))

		version, exists, getErr := repo.GetVersion(ctx, pluginID, versionName)
		require.NoError(t, getErr)
		if publishErr == nil {
			require.True(t, exists)
			prepared, prepareErr := entity.PrepareCodeDraft(&entity.CodeDraft{
				PluginID:  version.PluginID,
				SpaceID:   version.SpaceID,
				Runtime:   version.Runtime,
				EntryFile: version.EntryFile,
				Files:     version.Files,
			})
			require.NoError(t, prepareErr)
			require.Equal(t, prepared.SourceBundleRef, version.SourceBundleRef)
		} else {
			require.False(t, exists)
		}
		current, exists, getErr = repo.GetDraft(ctx, pluginID)
		require.NoError(t, getErr)
		require.True(t, exists)
	}

	require.NoError(t, repo.MarkDebuggedCAS(ctx, current.PluginID, current.Revision))
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	errorsCh := make(chan error, 2)
	go func() {
		defer waitGroup.Done()
		errorsCh <- repo.PublishDebuggedVersion(ctx, pluginID, "delete-race", 99)
	}()
	go func() {
		defer waitGroup.Done()
		errorsCh <- repo.DeletePluginData(ctx, pluginID)
	}()
	waitGroup.Wait()
	close(errorsCh)
	for operationErr := range errorsCh {
		require.True(t, operationErr == nil || errors.Is(operationErr, ErrCodeDraftNotFound))
	}
	for _, table := range []string{
		"plugin_code_version_files",
		"plugin_code_versions",
		"plugin_code_draft_files",
		"plugin_code_drafts",
	} {
		var count int64
		require.NoError(t, db.Table(table).Where("plugin_id = ?", pluginID).Count(&count).Error)
		require.Zero(t, count)
	}
}

func resetCodePluginMySQLSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{
		"plugin_code_version_files",
		"plugin_code_versions",
		"plugin_code_draft_files",
		"plugin_code_drafts",
		"plugin_draft",
	} {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+table+"`").Error)
	}
	require.NoError(t, db.Exec(
		"CREATE TABLE `plugin_draft` (`id` bigint unsigned NOT NULL, `space_id` bigint unsigned NOT NULL, PRIMARY KEY (`id`)) ENGINE=InnoDB",
	).Error)

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	migrationPath := filepath.Join(filepath.Dir(currentFile), "../../../../docker/atlas/migrations/20260718000100_plugin_code_parity.sql")
	migration, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	for _, statement := range strings.Split(string(migration), ";") {
		statement = strings.TrimSpace(statement)
		if statement != "" {
			require.NoError(t, db.Exec(statement).Error)
		}
	}
}
