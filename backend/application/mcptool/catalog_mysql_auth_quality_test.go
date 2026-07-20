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

package mcptool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestMCPManagementReturnsOpaqueAuthForNestedSecrets(t *testing.T) {
	server := &toolapi.MCPToolServer{
		Auth: `{
			"headers": {
				"Authorization": "Bearer top-secret",
				"nested": [
					{"aUtHoRiZaTiOn": "Basic nested-secret"},
					{"Proxy-Authorization": "Bearer proxy-secret"}
				]
			}
		}`,
	}

	response := cloneServerForResponse(server)

	require.JSONEq(t, mcpAuthConfiguredSentinel, response.Auth)
	require.NotContains(t, response.Auth, "Authorization")
	require.NotContains(t, response.Auth, "Proxy-Authorization")
	require.NotContains(t, response.Auth, "top-secret")
	require.NotContains(t, response.Auth, "nested-secret")
	require.NotContains(t, response.Auth, "proxy-secret")
	require.Contains(t, server.Auth, "top-secret")
}

func TestMySQLCatalogSchemaMatchesAtlasMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	type columnMetadata struct {
		Name         string `gorm:"column:name"`
		NotNull      int    `gorm:"column:notnull"`
		DefaultValue string `gorm:"column:dflt_value"`
	}
	columns := make([]columnMetadata, 0)
	require.NoError(t, db.Raw("PRAGMA table_info('mcp_tool_servers')").Scan(&columns).Error)
	columnsByName := make(map[string]columnMetadata, len(columns))
	for _, column := range columns {
		columnsByName[column.Name] = column
	}

	creator, ok := columnsByName["creator_id"]
	require.True(t, ok)
	require.Equal(t, 1, creator.NotNull)
	require.Equal(t, "0", strings.Trim(creator.DefaultValue, "'\""))

	require.Equal(t,
		[]string{"space_id", "enabled", "deleted_at"},
		sqliteIndexColumnNames(t, db, "idx_mcp_tool_servers_space_enabled"),
	)
	require.Equal(t,
		[]string{"creator_id", "updated_at"},
		sqliteIndexColumnNames(t, db, "idx_mcp_tool_servers_creator_updated"),
	)
	require.Equal(t,
		[]string{"space_id", "source_type", "updated_at"},
		sqliteIndexColumnNames(t, db, "idx_mcp_tool_servers_space_source_updated"),
	)
}

func TestMySQLCatalogAuthFailsClosedWithoutCodec(t *testing.T) {
	t.Run("empty auth remains usable", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		catalog := NewMySQLCatalog(db)

		require.NoError(t, catalog.Upsert(context.Background(), mcpServerForAuthTest(100, "")))
		got, err := catalog.Get(context.Background(), 100)
		require.NoError(t, err)
		require.JSONEq(t, `{}`, got.Auth)

		var po mcpToolServerPO
		require.NoError(t, db.Where("server_id = ?", 100).First(&po).Error)
		require.JSONEq(t, `{}`, string(po.Auth))
	})

	t.Run("non-empty auth cannot be persisted", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		catalog := NewMySQLCatalog(db)
		server := mcpServerForAuthTest(100, `{"token":"raw-secret-token"}`)

		err = catalog.Upsert(context.Background(), server)

		require.EqualError(t, err, "mcp tool auth codec is required")
		require.NotContains(t, err.Error(), "raw-secret-token")
		var count int64
		require.NoError(t, db.Model(&mcpToolServerPO{}).Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("non-empty stored auth cannot be read", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		writer := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(&recordingMCPAuthCodec{
			encoded: `{"_coze_mcp_auth":{"version":"test","nonce":"test","ciphertext":"encoded-auth"}}`,
		}))
		require.NoError(t, writer.Upsert(
			context.Background(),
			mcpServerForAuthTest(100, `{"token":"raw-secret-token"}`),
		))

		_, err = NewMySQLCatalog(db).Get(context.Background(), 100)

		require.EqualError(t, err, "mcp tool auth codec is required")
		require.NotContains(t, err.Error(), "raw-secret-token")
	})
}

func TestMySQLCatalogAuthRejectsNonObjectBeforeWrite(t *testing.T) {
	for name, auth := range map[string]string{
		"invalid JSON": "not-json",
		"JSON string":  `"not-an-object"`,
		"JSON array":   `[{"token":"secret"}]`,
		"JSON null":    `null`,
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			codec, err := NewAESMCPAuthCodec("0123456789abcdef")
			require.NoError(t, err)
			catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

			err = catalog.Upsert(context.Background(), mcpServerForAuthTest(100, auth))

			require.EqualError(t, err, "mcp tool auth must be a JSON object")
			require.NotContains(t, err.Error(), "secret")
			var count int64
			require.NoError(t, db.Model(&mcpToolServerPO{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestMySQLCatalogAuthRejectsInvalidUpdateWithoutOverwritingStoredValue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	const validAuth = `{"headers":{"Authorization":"Bearer original-secret"}}`
	require.NoError(t, catalog.Upsert(
		context.Background(),
		mcpServerForAuthTest(100, validAuth),
	))
	originalStored := loadStoredAuthForTest(t, db, 100)

	server := mcpServerForAuthTest(100, `null`)
	server.Name = "must-not-be-updated"
	err = catalog.Upsert(context.Background(), server)

	require.EqualError(t, err, "mcp tool auth must be a JSON object")
	require.Equal(t, originalStored, loadStoredAuthForTest(t, db, 100))
	got, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, "auth-test-mcp", got.Name)
	require.JSONEq(t, validAuth, got.Auth)
}

func TestMySQLCatalogAuthJSONObjectRoundTrips(t *testing.T) {
	t.Run("empty object without codec", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		catalog := NewMySQLCatalog(db)

		require.NoError(t, catalog.Upsert(
			context.Background(),
			mcpServerForAuthTest(100, `{}`),
		))
		got, err := catalog.Get(context.Background(), 100)
		require.NoError(t, err)
		require.JSONEq(t, `{}`, got.Auth)
	})

	t.Run("nested object with codec", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		codec, err := NewAESMCPAuthCodec("0123456789abcdef")
		require.NoError(t, err)
		catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
		const auth = `{"headers":{"Authorization":"Bearer nested-secret"},"metadata":{"region":"cn"}}`

		require.NoError(t, catalog.Upsert(
			context.Background(),
			mcpServerForAuthTest(100, auth),
		))
		require.NotContains(t, loadStoredAuthForTest(t, db, 100), "nested-secret")
		got, err := catalog.Get(context.Background(), 100)
		require.NoError(t, err)
		require.JSONEq(t, auth, got.Auth)
	})
}

func TestMCPAESAuthCodecEncryptsAndRoundTrips(t *testing.T) {
	const secret = "0123456789abcdef"
	const rawAuth = `{"type":"bearer","token":"raw-secret-token"}`
	codec, err := NewAESMCPAuthCodec(secret)
	require.NoError(t, err)

	encoded, err := codec.EncodeMCPAuth(context.Background(), rawAuth)
	require.NoError(t, err)
	require.NotContains(t, encoded, "raw-secret-token")
	require.NotContains(t, encoded, secret)
	var outerEnvelope map[string]map[string]string
	require.NoError(t, json.Unmarshal([]byte(encoded), &outerEnvelope))
	require.Len(t, outerEnvelope, 1)
	envelope, ok := outerEnvelope["_coze_mcp_auth"]
	require.True(t, ok)
	require.Equal(t, "aes-gcm-v1", envelope["version"])
	require.NotEmpty(t, envelope["nonce"])
	require.NotEmpty(t, envelope["ciphertext"])

	decoded, err := codec.DecodeMCPAuth(context.Background(), encoded)
	require.NoError(t, err)
	require.JSONEq(t, rawAuth, decoded)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	require.NoError(t, catalog.Upsert(
		context.Background(),
		mcpServerForAuthTest(100, rawAuth),
	))

	var po mcpToolServerPO
	require.NoError(t, db.Where("server_id = ?", 100).First(&po).Error)
	require.NotContains(t, string(po.Auth), "raw-secret-token")
	require.NotContains(t, string(po.Auth), secret)

	got, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.JSONEq(t, rawAuth, got.Auth)
}

func TestMCPAESAuthCodecRejectsTamperingTruncationAndWrongKey(t *testing.T) {
	const rawAuth = `{"token":"raw-secret-token"}`
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	encoded, err := codec.EncodeMCPAuth(context.Background(), rawAuth)
	require.NoError(t, err)

	var outerEnvelope map[string]map[string]string
	require.NoError(t, json.Unmarshal([]byte(encoded), &outerEnvelope))
	original, ok := outerEnvelope["_coze_mcp_auth"]
	require.True(t, ok)
	mutations := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{
			name: "nonce bit",
			mutate: func(envelope map[string]string) {
				envelope["nonce"] = flipFirstBase64Bit(t, envelope["nonce"])
			},
		},
		{
			name: "ciphertext bit",
			mutate: func(envelope map[string]string) {
				envelope["ciphertext"] = flipFirstBase64Bit(t, envelope["ciphertext"])
			},
		},
		{
			name: "truncated nonce",
			mutate: func(envelope map[string]string) {
				envelope["nonce"] = truncateBase64(envelope["nonce"])
			},
		},
		{
			name: "truncated ciphertext",
			mutate: func(envelope map[string]string) {
				envelope["ciphertext"] = truncateBase64(envelope["ciphertext"])
			},
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			tampered := make(map[string]string, len(original))
			for key, value := range original {
				tampered[key] = value
			}
			mutation.mutate(tampered)
			payload, err := json.Marshal(map[string]map[string]string{
				"_coze_mcp_auth": tampered,
			})
			require.NoError(t, err)

			var decodeErr error
			require.NotPanics(t, func() {
				_, decodeErr = codec.DecodeMCPAuth(context.Background(), string(payload))
			})
			require.Error(t, decodeErr)
			require.NotContains(t, decodeErr.Error(), "raw-secret-token")
		})
	}

	wrongKeyCodec, err := NewAESMCPAuthCodec("abcdef0123456789")
	require.NoError(t, err)
	var wrongKeyErr error
	require.NotPanics(t, func() {
		_, wrongKeyErr = wrongKeyCodec.DecodeMCPAuth(context.Background(), encoded)
	})
	require.Error(t, wrongKeyErr)
	require.NotContains(t, wrongKeyErr.Error(), "raw-secret-token")
}

func TestMCPAESAuthCodecRejectsMissingAndInvalidSecrets(t *testing.T) {
	require.Equal(t, "MCP_AES_AUTH_SECRET", MCPAESAuthSecretEnv)
	for name, secret := range map[string]string{
		"missing":   "",
		"too short": "short",
		"15 bytes":  strings.Repeat("x", 15),
		"17 bytes":  strings.Repeat("x", 17),
		"33 bytes":  strings.Repeat("x", 33),
	} {
		t.Run(name, func(t *testing.T) {
			codec, err := NewAESMCPAuthCodec(secret)

			require.Nil(t, codec)
			require.Error(t, err)
			require.Contains(t, err.Error(), MCPAESAuthSecretEnv)
			if secret != "" {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}

	for _, length := range []int{16, 24, 32} {
		codec, err := NewAESMCPAuthCodec(strings.Repeat("k", length))
		require.NoError(t, err)
		require.NotNil(t, codec)
	}
}

func TestMySQLCatalogLegacyAuthMigratesOnGet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	const legacyAuth = `{"headers":{"Authorization":"Bearer legacy-secret"}}`
	insertStoredAuthForTest(t, db, 100, legacyAuth)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

	got, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.JSONEq(t, legacyAuth, got.Auth)
	persisted := loadStoredAuthForTest(t, db, 100)
	assertAESGCMEnvelope(t, persisted)
	require.NotContains(t, persisted, "legacy-secret")

	got, err = catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.JSONEq(t, legacyAuth, got.Auth)
}

func TestMySQLCatalogLegacyAuthMigratesOnList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	const legacyAuth = `{"token":"legacy-list-secret"}`
	insertStoredAuthForTest(t, db, 100, legacyAuth)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

	listed, err := catalog.List(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.JSONEq(t, legacyAuth, listed[0].Auth)
	persisted := loadStoredAuthForTest(t, db, 100)
	assertAESGCMEnvelope(t, persisted)
	require.NotContains(t, persisted, "legacy-list-secret")

	listed, err = catalog.List(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.JSONEq(t, legacyAuth, listed[0].Auth)
}

func TestMySQLCatalogLegacyBusinessEnvelopeFieldsStillMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	const legacyAuth = `{"version":"v1","nonce":"business","ciphertext":"business","token":"secret"}`
	insertStoredAuthForTest(t, db, 100, legacyAuth)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

	got, err := catalog.Get(context.Background(), 100)

	require.NoError(t, err)
	require.JSONEq(t, legacyAuth, got.Auth)
	persisted := loadStoredAuthForTest(t, db, 100)
	assertAESGCMEnvelope(t, persisted)
	require.NotContains(t, persisted, `"token":"secret"`)
}

func TestMySQLCatalogConcurrentLegacyAuthMigrationUsesCurrentValue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	const legacyAuth = `{"token":"stale-legacy-secret"}`
	const concurrentAuth = `{"token":"concurrent-secret"}`
	insertStoredAuthForTest(t, db, 100, legacyAuth)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	concurrentEnvelope, err := codec.EncodeMCPAuth(context.Background(), concurrentAuth)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE concurrent_auth_value (auth TEXT NOT NULL)`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO concurrent_auth_value (auth) VALUES (?)`,
		concurrentEnvelope,
	).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER simulate_concurrent_auth_write
		BEFORE UPDATE OF auth ON mcp_tool_servers
		WHEN NEW.auth <> (SELECT auth FROM concurrent_auth_value LIMIT 1)
		BEGIN
			UPDATE mcp_tool_servers
			SET auth = (SELECT auth FROM concurrent_auth_value LIMIT 1)
			WHERE server_id = OLD.server_id;
			SELECT RAISE(IGNORE);
		END
	`).Error)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

	got, err := catalog.Get(context.Background(), 100)

	require.NoError(t, err)
	require.JSONEq(t, concurrentAuth, got.Auth)
	require.Equal(t, concurrentEnvelope, loadStoredAuthForTest(t, db, 100))
}

func TestMySQLCatalogConcurrentLegacyAuthMigrationFailsClosedAfterLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	insertStoredAuthForTest(t, db, 100, `{"token":"legacy-secret"}`)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER force_legacy_auth_conflict
		BEFORE UPDATE OF auth ON mcp_tool_servers
		WHEN instr(CAST(NEW.auth AS TEXT), '_coze_mcp_auth') > 0
		BEGIN
			UPDATE mcp_tool_servers
			SET auth = '{"token":"continually-changing-legacy"}'
			WHERE server_id = OLD.server_id;
			SELECT RAISE(IGNORE);
		END
	`).Error)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

	_, err = catalog.Get(context.Background(), 100)

	require.EqualError(t, err, "mcp tool legacy auth migration conflict")
	require.NotContains(t, err.Error(), "legacy-secret")
}

func TestMySQLCatalogLegacyAuthMigrationFailsClosed(t *testing.T) {
	t.Run("requires AES-GCM codec", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		insertStoredAuthForTest(t, db, 100, `{"token":"legacy-secret"}`)
		catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(&recordingMCPAuthCodec{}))

		_, err = catalog.Get(context.Background(), 100)

		require.EqualError(t, err, "mcp tool legacy auth migration requires AES-GCM codec")
		require.NotContains(t, err.Error(), "legacy-secret")
	})

	t.Run("persistence failure", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		insertStoredAuthForTest(t, db, 100, `{"token":"legacy-secret"}`)
		require.NoError(t, db.Exec(`
			CREATE TRIGGER reject_mcp_auth_migration
			BEFORE UPDATE OF auth ON mcp_tool_servers
			BEGIN
				SELECT RAISE(FAIL, 'auth update rejected');
			END
		`).Error)
		codec, err := NewAESMCPAuthCodec("0123456789abcdef")
		require.NoError(t, err)
		catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

		_, err = catalog.Get(context.Background(), 100)

		require.EqualError(t, err, "mcp tool legacy auth migration failed")
		require.NotContains(t, err.Error(), "legacy-secret")
	})
}

func TestMySQLCatalogLegacyAuthRejectsInvalidNonEnvelopeValues(t *testing.T) {
	for name, stored := range map[string]string{
		"invalid JSON": "not-json",
		"JSON string":  `"plaintext"`,
		"JSON array":   `[{"token":"plaintext"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			insertStoredAuthForTest(t, db, 100, stored)
			codec, err := NewAESMCPAuthCodec("0123456789abcdef")
			require.NoError(t, err)
			catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

			_, err = catalog.Get(context.Background(), 100)

			require.EqualError(t, err, "mcp tool auth decode failed")
			require.Equal(t, stored, loadStoredAuthForTest(t, db, 100))
		})
	}
}

func TestMySQLCatalogLegacyAuthRejectsDamagedNamespacedEnvelope(t *testing.T) {
	for name, stored := range map[string]string{
		"invalid marker type": `{"_coze_mcp_auth":"broken"}`,
		"missing nonce":       `{"_coze_mcp_auth":{"version":"aes-gcm-v1","ciphertext":"AA"}}`,
		"truncated values":    `{"_coze_mcp_auth":{"version":"aes-gcm-v1","nonce":"AA","ciphertext":"AA"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			insertStoredAuthForTest(t, db, 100, stored)
			codec, err := NewAESMCPAuthCodec("0123456789abcdef")
			require.NoError(t, err)
			catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))

			_, err = catalog.Get(context.Background(), 100)

			require.EqualError(t, err, "mcp tool auth decode failed")
			require.Equal(t, stored, loadStoredAuthForTest(t, db, 100))
		})
	}
}

func TestMySQLCatalogManagementListDegradesUnreadableAuthWithoutWeakeningStrictReads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	writerCodec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	readerCodec, err := NewAESMCPAuthCodec("fedcba9876543210")
	require.NoError(t, err)
	writer := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(writerCodec))
	require.NoError(t, writer.Upsert(
		context.Background(),
		mcpServerForAuthTest(100, `{"token":"top-secret"}`),
	))
	reader := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(readerCodec))

	_, err = reader.List(context.Background(), 1)
	require.EqualError(t, err, "mcp tool auth decode failed")

	servers, err := reader.ListForManagement(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, mcpAuthConfiguredSentinel, servers[0].Auth)
	require.False(t, servers[0].Enabled)
	require.Equal(t, mcpToolHealthStatusUnhealthy, servers[0].HealthStatus)
	require.Equal(t, "credential_unavailable", servers[0].HealthError)
	require.NotContains(t, servers[0].Auth, "top-secret")

	_, err = reader.Get(context.Background(), 100)
	require.EqualError(t, err, "mcp tool auth decode failed")
}

func sqliteIndexColumnNames(t *testing.T, db *gorm.DB, indexName string) []string {
	t.Helper()
	type indexColumn struct {
		Sequence int    `gorm:"column:seqno"`
		Name     string `gorm:"column:name"`
	}
	columns := make([]indexColumn, 0)
	require.NoError(t, db.Raw("PRAGMA index_info('"+indexName+"')").Scan(&columns).Error)
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}

	return names
}

func mcpServerForAuthTest(serverID int64, auth string) *toolapi.MCPToolServer {
	return &toolapi.MCPToolServer{
		ServerID:   serverID,
		SpaceID:    1,
		CreatorID:  42,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "auth-test-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       auth,
		CreatedAt:  10,
		UpdatedAt:  20,
	}
}

func flipFirstBase64Bit(t *testing.T, value string) string {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	require.NoError(t, err)
	if len(decoded) == 0 {
		decoded = []byte{0}
	}
	decoded[0] ^= 1

	return base64.RawURLEncoding.EncodeToString(decoded)
}

func truncateBase64(value string) string {
	if len(value) <= 1 {
		return ""
	}

	return value[:len(value)-1]
}

func insertStoredAuthForTest(t *testing.T, db *gorm.DB, serverID int64, auth string) {
	t.Helper()
	require.NoError(t, db.Create(&mcpToolServerPO{
		ServerID:   serverID,
		SpaceID:    1,
		CreatorID:  42,
		SourceType: string(toolapi.MCPServerSourceTypeCustom),
		Name:       "legacy-auth-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     datatypes.JSON(`{}`),
		Auth:       datatypes.JSON(auth),
		Tools:      datatypes.JSON(`[]`),
		Resources:  datatypes.JSON(`[]`),
		Prompts:    datatypes.JSON(`[]`),
		CreatedAt:  10,
		UpdatedAt:  20,
	}).Error)
}

func loadStoredAuthForTest(t *testing.T, db *gorm.DB, serverID int64) string {
	t.Helper()
	var po mcpToolServerPO
	require.NoError(t, db.Where("server_id = ?", serverID).First(&po).Error)

	return string(po.Auth)
}

func assertAESGCMEnvelope(t *testing.T, stored string) {
	t.Helper()
	var outerEnvelope map[string]map[string]string
	require.NoError(t, json.Unmarshal([]byte(stored), &outerEnvelope))
	require.Len(t, outerEnvelope, 1)
	envelope, ok := outerEnvelope["_coze_mcp_auth"]
	require.True(t, ok)
	require.Equal(t, "aes-gcm-v1", envelope["version"])
	require.NotEmpty(t, envelope["nonce"])
	require.NotEmpty(t, envelope["ciphertext"])
}
