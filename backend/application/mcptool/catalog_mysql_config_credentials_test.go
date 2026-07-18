// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
	"github.com/coze-dev/coze-studio/backend/pkg/secureaead"
)

const preSecureAEADExtractionMCPPayload = `{"version":"aes-gcm-v1","nonce":"AAECAwQFBgcICQoL","ciphertext":"hh2wcnURVRTSBGt_p_rTVC2kLpRz2y8_HgrHgIrI2QkqNeI28PpFGoHADvYVmw7ogM7v"}`

const preSecureAEADExtractionMCPFixture = `{"_coze_mcp_auth":{"version":"aes-gcm-v1","nonce":"AAECAwQFBgcICQoL","ciphertext":"hh2wcnURVRTSBGt_p_rTVC2kLpRz2y8_HgrHgIrI2QkqNeI28PpFGoHADvYVmw7ogM7v"}}`

func TestAESMCPAuthCodecDecodesPreExtractionFixture(t *testing.T) {
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)

	decoded, err := codec.DecodeMCPAuth(context.Background(), preSecureAEADExtractionMCPFixture)
	require.NoError(t, err)
	require.Equal(t, `{"token":"legacy-synthetic-secret"}`, decoded)
}

func TestAESMCPAuthCodecDeterministicNonceRemainsByteCompatible(t *testing.T) {
	codec := &AESMCPAuthCodec{
		secret:      "0123456789abcdef",
		nonceSource: bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}),
	}

	encoded, err := codec.EncodeMCPAuth(context.Background(), `{"token":"legacy-synthetic-secret"}`)
	require.NoError(t, err)
	require.Equal(t, preSecureAEADExtractionMCPFixture, encoded)
}

func TestAESMCPAuthCodecEnforcesSymmetricPlaintextAndEnvelopeBounds(t *testing.T) {
	codec := &AESMCPAuthCodec{
		secret:      "0123456789abcdef",
		nonceSource: bytes.NewReader(bytes.Repeat([]byte{0x41}, 12)),
	}
	boundary := mcpAuthObjectAtSize(t, mcpAESAuthMaxPlaintextBytes)
	encoded, err := codec.EncodeMCPAuth(context.Background(), boundary)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), mcpAESAuthMaxEnvelopeBytes)
	decoded, err := codec.DecodeMCPAuth(context.Background(), encoded)
	require.NoError(t, err)
	require.Equal(t, boundary, decoded)

	overLimit := mcpAuthObjectAtSize(t, mcpAESAuthMaxPlaintextBytes+1)
	_, err = codec.EncodeMCPAuth(context.Background(), overLimit)
	require.ErrorIs(t, err, errMCPAuthEncryptionFailed)
	assertSafeMCPAuthCodecError(t, err, overLimit)

	for _, size := range []int{mcpAESAuthMaxEnvelopeBytes, mcpAESAuthMaxEnvelopeBytes + 1} {
		_, err = codec.DecodeMCPAuth(context.Background(), strings.Repeat(":", size))
		require.ErrorIs(t, err, errMCPAuthEnvelopeInvalid)
	}
	oversizedCiphertextPayload := mcpAuthPayloadJSON(
		t,
		"AAECAwQFBgcICQoL",
		strings.Repeat("A", mcpAESAuthMaxEncodedCiphertextBytes+1),
	)
	_, err = codec.DecodeMCPAuth(context.Background(), mcpEnvelopeForPayload(oversizedCiphertextPayload))
	require.ErrorIs(t, err, errMCPAuthEnvelopeInvalid)
}

func TestAESMCPAuthCodecRejectsStrictEnvelopeViolations(t *testing.T) {
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	fixedNonce := "AAECAwQFBgcICQoL"
	fixedCiphertext := "hh2wcnURVRTSBGt_p_rTVC2kLpRz2y8_HgrHgIrI2QkqNeI28PpFGoHADvYVmw7ogM7v"
	canonicalAlternateCiphertext := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 19))
	alternateCiphertext := alternateMCPRawURLBase64(canonicalAlternateCiphertext)
	alternateDecoded, decodeErr := base64.RawURLEncoding.DecodeString(alternateCiphertext)
	canonicalDecoded, canonicalErr := base64.RawURLEncoding.DecodeString(canonicalAlternateCiphertext)
	require.NoError(t, decodeErr)
	require.NoError(t, canonicalErr)
	require.Equal(t, canonicalDecoded, alternateDecoded)
	require.NotEqual(t, canonicalAlternateCiphertext, alternateCiphertext)

	standardNonce := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 12))
	tests := []struct {
		name   string
		stored string
	}{
		{name: "duplicate outer field", stored: `{"_coze_mcp_auth":` + preSecureAEADExtractionMCPPayload + `,"_coze_mcp_auth":` + preSecureAEADExtractionMCPPayload + `}`},
		{name: "unknown outer field", stored: `{"_coze_mcp_auth":` + preSecureAEADExtractionMCPPayload + `,"unknown":true}`},
		{name: "missing outer field", stored: `{"unknown":true}`},
		{name: "outer array", stored: `[]`},
		{name: "outer null", stored: `null`},
		{name: "payload scalar", stored: `{"_coze_mcp_auth":"value"}`},
		{name: "duplicate payload version", stored: mcpEnvelopeForPayload(`{"version":"aes-gcm-v1","version":"aes-gcm-v1","nonce":"` + fixedNonce + `","ciphertext":"` + fixedCiphertext + `"}`)},
		{name: "duplicate payload nonce", stored: mcpEnvelopeForPayload(`{"version":"aes-gcm-v1","nonce":"` + fixedNonce + `","nonce":"` + fixedNonce + `","ciphertext":"` + fixedCiphertext + `"}`)},
		{name: "unknown payload field", stored: mcpEnvelopeForPayload(`{"version":"aes-gcm-v1","nonce":"` + fixedNonce + `","ciphertext":"` + fixedCiphertext + `","unknown":true}`)},
		{name: "missing payload field", stored: mcpEnvelopeForPayload(`{"version":"aes-gcm-v1","nonce":"` + fixedNonce + `"}`)},
		{name: "trailing data", stored: preSecureAEADExtractionMCPFixture + `{}`},
		{name: "padded nonce", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce+"=", fixedCiphertext))},
		{name: "nonce carriage return", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce+"\r", fixedCiphertext))},
		{name: "nonce newline", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce+"\n", fixedCiphertext))},
		{name: "nonce space", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce+" ", fixedCiphertext))},
		{name: "nonce tab", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce+"\t", fixedCiphertext))},
		{name: "standard alphabet confusion", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, standardNonce, fixedCiphertext))},
		{name: "padded ciphertext", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce, fixedCiphertext+"="))},
		{name: "alternate ciphertext text", stored: mcpEnvelopeForPayload(mcpAuthPayloadJSON(t, fixedNonce, alternateCiphertext))},
		{name: "invalid UTF-8", stored: string(append(append([]byte(nil), []byte(preSecureAEADExtractionMCPFixture)...), 0xff))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codec.DecodeMCPAuth(context.Background(), test.stored)
			require.ErrorIs(t, err, errMCPAuthEnvelopeInvalid)
			assertSafeMCPAuthCodecError(t, err, test.stored)
		})
	}
}

func TestAESMCPAuthCodecRequiresUniqueJSONObjectPlaintext(t *testing.T) {
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	invalidUTF8 := string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	invalid := []string{
		"",
		"null",
		`"scalar"`,
		"123",
		"[]",
		`{"token":"first","token":"second"}`,
		`{"outer":{"token":"first","token":"second"}}`,
		`{"token":"value"} {}`,
		invalidUTF8,
	}
	for index, plaintext := range invalid {
		t.Run(strings.ReplaceAll(plaintextName(index), " ", "_"), func(t *testing.T) {
			stored := rawMCPAuthEnvelope(t, plaintext, byte(index+1))
			_, err := codec.DecodeMCPAuth(context.Background(), stored)
			require.ErrorIs(t, err, errMCPAuthDecryptionFailed)
			assertSafeMCPAuthCodecError(t, err, plaintext, stored)

			_, err = codec.EncodeMCPAuth(context.Background(), plaintext)
			require.ErrorIs(t, err, errMCPAuthEncryptionFailed)
			assertSafeMCPAuthCodecError(t, err, plaintext)
		})
	}

	valid := `{"Authorization":"Bearer synthetic","X-Custom":{"nested":true},"env":{"TOKEN":"value"}}`
	stored := rawMCPAuthEnvelope(t, valid, 0x7f)
	decoded, err := codec.DecodeMCPAuth(context.Background(), stored)
	require.NoError(t, err)
	require.Equal(t, valid, decoded)
}

func mcpAuthObjectAtSize(t *testing.T, size int) string {
	t.Helper()
	const prefix = `{"credential":"`
	const suffix = `"}`
	if size < len(prefix)+len(suffix) {
		t.Fatal("requested MCP auth fixture size is too small")
	}
	value := prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
	require.Len(t, value, size)
	return value
}

func mcpAuthPayloadJSON(t *testing.T, nonce, ciphertext string) string {
	t.Helper()
	encoded, err := json.Marshal(mcpAESAuthEnvelopePayload{
		Version: mcpAESAuthEnvelopeVersion, Nonce: nonce, Ciphertext: ciphertext,
	})
	require.NoError(t, err)
	return string(encoded)
}

func mcpEnvelopeForPayload(payload string) string {
	return `{"_coze_mcp_auth":` + payload + `}`
}

func rawMCPAuthEnvelope(t *testing.T, plaintext string, nonceFill byte) string {
	t.Helper()
	nonce := bytes.Repeat([]byte{nonceFill}, 12)
	gotNonce, ciphertext, err := secureaead.Seal(
		[]byte("0123456789abcdef"),
		[]byte(mcpAESAuthEnvelopeVersion),
		[]byte(plaintext),
		bytes.NewReader(nonce),
	)
	require.NoError(t, err)
	encoded, err := json.Marshal(mcpAESAuthEnvelope{
		Payload: mcpAESAuthEnvelopePayload{
			Version:    mcpAESAuthEnvelopeVersion,
			Nonce:      base64.RawURLEncoding.EncodeToString(gotNonce),
			Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		},
	})
	require.NoError(t, err)
	return string(encoded)
}

func alternateMCPRawURLBase64(canonical string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	remainder := len(canonical) % 4
	if len(canonical) == 0 || remainder != 2 && remainder != 3 {
		return canonical
	}
	last := strings.IndexByte(alphabet, canonical[len(canonical)-1])
	if last < 0 {
		return canonical
	}
	alternate := []byte(canonical)
	alternate[len(alternate)-1] = alphabet[last^1]
	return string(alternate)
}

func plaintextName(index int) string {
	return "invalid_plaintext_" + string(rune('a'+index))
}

func assertSafeMCPAuthCodecError(t *testing.T, err error, markers ...string) {
	t.Helper()
	require.Error(t, err)
	require.LessOrEqual(t, len(err.Error()), 96)
	for _, marker := range markers {
		if marker != "" && len(marker) <= 1024 {
			require.NotContains(t, err.Error(), marker)
		}
	}
}

func TestMySQLServiceStoresConfigCredentialsOnlyAsEncryptedAuth(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})

	remoteReq := validManagementUpsertRequest(0, 10)
	remoteReq.Name = "remote-credentials"
	remoteReq.Config = `{"url":"https://mcp.example.com/events","headers":{"Authorization":"Bearer database-secret","X-Custom":"custom-database-secret"}}`
	remote, err := svc.UpsertServer(managementContext(7), remoteReq)
	require.NoError(t, err)
	stdioReq := validManagementUpsertRequest(0, 10)
	stdioReq.Name = "stdio-credentials"
	stdioReq.ServerType = "stdio"
	stdioReq.Config = `{"command":"npx","args":["server"],"env":{"MCP_TOKEN":"stdio-database-secret"}}`
	stdio, err := svc.UpsertServer(managementContext(7), stdioReq)
	require.NoError(t, err)

	for _, serverID := range []int64{remote.Data.ServerID, stdio.Data.ServerID} {
		var po mcpToolServerPO
		require.NoError(t, db.Where("server_id = ?", serverID).First(&po).Error)
		require.NotContains(t, string(po.Config), "database-secret")
		require.NotContains(t, string(po.Config), `"headers"`)
		require.NotContains(t, string(po.Config), `"env"`)
		require.NotContains(t, string(po.Auth), "database-secret")
		assertAESGCMEnvelope(t, string(po.Auth))
	}

	resolvedRemote, err := svc.ResolveADKMCPRuntimeServer(context.Background(), remote.Data.ServerID)
	require.NoError(t, err)
	policy := mcpruntime.Policy{
		RemoteEnabled:        true,
		RemoteAllowedHosts:   []string{"mcp.example.com"},
		RemoteMaxConfigBytes: 64 * 1024,
		RemoteMaxHeaders:     16,
		RemoteMaxHeaderBytes: 16 * 1024,
		HTTPTimeout:          2 * time.Second,
	}
	remoteConnection, err := policy.ParseRemote(mcpruntime.Connection{
		ServerType: resolvedRemote.ServerType,
		Config:     resolvedRemote.Config,
		Auth:       resolvedRemote.Auth,
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer database-secret", remoteConnection.Headers["Authorization"])
	require.Equal(t, "custom-database-secret", remoteConnection.Headers["X-Custom"])

	resolvedStdio, err := svc.ResolveADKMCPRuntimeServer(context.Background(), stdio.Data.ServerID)
	require.NoError(t, err)
	stdioConnection, err := mcpruntime.ParseStdioConnection(mcpruntime.Connection{
		ServerType: resolvedStdio.ServerType,
		Config:     resolvedStdio.Config,
		Auth:       resolvedStdio.Auth,
	}, 64*1024)
	require.NoError(t, err)
	require.Equal(t, "stdio-database-secret", stdioConnection.Env["MCP_TOKEN"])
}

func TestMySQLCatalogRejectsPlaintextConfigCredentialsOnDirectWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	server := mcpServerForAuthTest(100, `{}`)
	server.Config = `{"command":"npx","env":{"TOKEN":"direct-write-secret"}}`

	err = catalog.Upsert(context.Background(), server)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "direct-write-secret")
	var count int64
	require.NoError(t, db.Model(&mcpToolServerPO{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestMySQLLegacyConfigCredentialsLazyMigrateWithCASAndEncryption(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	require.NoError(t, db.Create(&mcpToolServerPO{
		ServerID:   100,
		SpaceID:    10,
		CreatorID:  7,
		SourceType: string(toolapi.MCPServerSourceTypeCustom),
		Name:       "legacy-config-credentials",
		ServerType: "stdio",
		Enabled:    true,
		Config:     datatypes.JSON(`{"command":"npx","args":["server"],"env":{"MCP_TOKEN":"legacy-config-secret"}}`),
		Auth:       datatypes.JSON(`{}`),
		Tools:      datatypes.JSON(`[]`),
		Resources:  datatypes.JSON(`[]`),
		Prompts:    datatypes.JSON(`[]`),
		CreatedAt:  10,
		UpdatedAt:  20,
	}).Error)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	svc := NewApplicationService(&Components{Catalog: catalog})

	resolved, err := svc.ResolveADKMCPRuntimeServer(context.Background(), 100)
	require.NoError(t, err)
	stdioConnection, err := mcpruntime.ParseStdioConnection(mcpruntime.Connection{
		ServerType: resolved.ServerType,
		Config:     resolved.Config,
		Auth:       resolved.Auth,
	}, 64*1024)
	require.NoError(t, err)
	require.Equal(t, "legacy-config-secret", stdioConnection.Env["MCP_TOKEN"])

	var po mcpToolServerPO
	require.NoError(t, db.Where("server_id = ?", 100).First(&po).Error)
	require.NotContains(t, string(po.Config), "legacy-config-secret")
	require.JSONEq(t, `{"command":"npx","auth_args":{"0":"args.0"},"auth_env":{"MCP_TOKEN":"env.MCP_TOKEN"}}`, string(po.Config))
	require.NotContains(t, string(po.Auth), "legacy-config-secret")
	assertAESGCMEnvelope(t, string(po.Auth))
	require.Greater(t, po.UpdatedAt, int64(20))
}

type catalogBoundaryRecordingCodec struct {
	encodeCalls int
	decodeCalls int
	encoded     string
	decoded     string
	encodeErr   error
	decodeErr   error
}

func (c *catalogBoundaryRecordingCodec) EncodeMCPAuth(_ context.Context, _ string) (string, error) {
	c.encodeCalls++
	return c.encoded, c.encodeErr
}

func (c *catalogBoundaryRecordingCodec) DecodeMCPAuth(_ context.Context, _ string) (string, error) {
	c.decodeCalls++
	return c.decoded, c.decodeErr
}

func TestMySQLCatalogAuthBoundsRejectBeforeCodec(t *testing.T) {
	t.Parallel()

	codec := &catalogBoundaryRecordingCodec{}
	catalog := &MySQLCatalog{authCodec: codec}

	_, err := catalog.encodeAuth(context.Background(), mcpAuthObjectAtSize(t, maxMCPAuthPlaintextBytes+1))
	require.Error(t, err)
	require.Equal(t, 0, codec.encodeCalls)

	oversizedStored := strings.Repeat("x", maxMCPAuthEnvelopeBytes+1)
	_, err = catalog.decodeAuth(context.Background(), oversizedStored)
	require.Error(t, err)
	require.Equal(t, 0, codec.decodeCalls)

	_, _, err = catalog.decodeAuthForRead(context.Background(), oversizedStored)
	require.Error(t, err)
	require.Equal(t, 0, codec.decodeCalls)
}

func TestCatalogAuthRejectsDuplicatePlaintextBeforeCodec(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"token":"one","token":"two"}`,
		`{"credentials":{"token":"one","token":"two"}}`,
	}
	for _, auth := range tests {
		codec := &catalogBoundaryRecordingCodec{encoded: `{}`}
		catalog := &MySQLCatalog{authCodec: codec}

		_, err := catalog.encodeAuth(context.Background(), auth)
		require.Error(t, err)
		require.Equal(t, 0, codec.encodeCalls)
	}
}

func TestMySQLCatalogAuthRejectsInvalidCustomDecodeOutput(t *testing.T) {
	t.Parallel()

	invalidUTF8 := string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	tests := []struct {
		name    string
		decoded string
	}{
		{name: "null", decoded: `null`},
		{name: "scalar", decoded: `"token"`},
		{name: "array", decoded: `[{"token":"synthetic"}]`},
		{name: "duplicate top level", decoded: `{"token":"one","token":"two"}`},
		{name: "duplicate nested", decoded: `{"credentials":{"token":"one","token":"two"}}`},
		{name: "trailing data", decoded: `{"token":"synthetic"}{}`},
		{name: "invalid utf8", decoded: invalidUTF8},
		{name: "oversized", decoded: mcpAuthObjectAtSize(t, maxMCPAuthPlaintextBytes+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec := &catalogBoundaryRecordingCodec{decoded: tt.decoded}
			catalog := &MySQLCatalog{authCodec: codec}

			_, err := catalog.decodeAuth(context.Background(), `{"stored":true}`)
			require.EqualError(t, err, "mcp tool auth decode failed")
			require.Equal(t, 1, codec.decodeCalls)
		})
	}

	codec := &catalogBoundaryRecordingCodec{decoded: `{"token":"synthetic","nested":{"name":"unique"}}`}
	catalog := &MySQLCatalog{authCodec: codec}
	decoded, err := catalog.decodeAuth(context.Background(), `{"stored":true}`)
	require.NoError(t, err)
	require.JSONEq(t, codec.decoded, decoded)
}

func TestCatalogAuthClassificationPreservesEmptyLegacyAndReservedMarker(t *testing.T) {
	t.Parallel()

	empty, err := parseBoundedMCPAuthObjectString(`{}`, maxMCPAuthEnvelopeBytes)
	require.NoError(t, err)
	require.True(t, isEmptyCatalogMCPAuth(empty))
	require.False(t, isLegacyCatalogMCPAuth(empty))

	legacy, err := parseBoundedMCPAuthObjectString(`{"token":"synthetic"}`, maxMCPAuthEnvelopeBytes)
	require.NoError(t, err)
	require.False(t, isEmptyCatalogMCPAuth(legacy))
	require.True(t, isLegacyCatalogMCPAuth(legacy))

	reserved, err := parseBoundedMCPAuthObjectString(preSecureAEADExtractionMCPFixture, maxMCPAuthEnvelopeBytes)
	require.NoError(t, err)
	require.False(t, isEmptyCatalogMCPAuth(reserved))
	require.False(t, isLegacyCatalogMCPAuth(reserved))
}
