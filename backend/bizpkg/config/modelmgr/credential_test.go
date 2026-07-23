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

package modelmgr

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadModelCredentialCodecRequiresCompleteDedicatedKeyRing(t *testing.T) {
	codec, err := LoadModelCredentialCodec(func(string) string { return "" })
	require.NoError(t, err)
	require.Nil(t, codec)

	_, err = LoadModelCredentialCodec(func(key string) string {
		if key == ModelCredentialKeysJSONEnv {
			return modelCredentialKeysJSON("model-key-v1", 0x41)
		}
		return ""
	})
	require.ErrorContains(t, err, "incomplete")
}

func TestModelCredentialCodecEncryptsWithEndpointBoundAAD(t *testing.T) {
	codec := mustModelCredentialCodec(t, "model-key-v1", modelCredentialKeysJSON("model-key-v1", 0x42))

	envelope, fingerprint, err := codec.Encrypt(1001, 2001, "secret-value")
	require.NoError(t, err)
	require.NotEmpty(t, envelope)
	require.NotContains(t, envelope, "secret-value")
	require.NotEmpty(t, fingerprint)

	plaintext, err := codec.Decrypt(1001, 2001, envelope)
	require.NoError(t, err)
	require.Equal(t, "secret-value", plaintext)

	_, err = codec.Decrypt(1001, 2002, envelope)
	require.Error(t, err)
	_, err = codec.Decrypt(1002, 2001, envelope)
	require.Error(t, err)
}

func TestModelCredentialCodecRejectsInvalidInputs(t *testing.T) {
	codec := mustModelCredentialCodec(t, "model-key-v1", modelCredentialKeysJSON("model-key-v1", 0x43))

	_, _, err := codec.Encrypt(0, 1, "secret")
	require.ErrorIs(t, err, ErrModelCredentialCodecMissing)
	_, _, err = codec.Encrypt(1, 0, "secret")
	require.ErrorIs(t, err, ErrModelCredentialCodecMissing)
	_, _, err = codec.Encrypt(1, 1, "")
	require.ErrorIs(t, err, ErrModelCredentialCodecMissing)
	_, err = codec.Decrypt(1, 1, "")
	require.ErrorIs(t, err, ErrModelCredentialCodecMissing)
}

func TestModelCredentialCodecRewrapsOldEnvelope(t *testing.T) {
	oldKeys := modelCredentialKeysJSON("model-key-v1", 0x44)
	oldCodec := mustModelCredentialCodec(t, "model-key-v1", oldKeys)
	envelope, _, err := oldCodec.Encrypt(1001, 2001, "rotating-secret")
	require.NoError(t, err)

	newKeys := "{" +
		"\"model-key-v1\":\"" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x44}, 32)) + "\"," +
		"\"model-key-v2\":\"" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x45}, 32)) + "\"}"
	newCodec := mustModelCredentialCodec(t, "model-key-v2", newKeys)

	rewrapped, fingerprint, changed, err := newCodec.Rewrap(1001, 2001, envelope)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEqual(t, envelope, rewrapped)
	require.NotEmpty(t, fingerprint)

	plaintext, err := newCodec.Decrypt(1001, 2001, rewrapped)
	require.NoError(t, err)
	require.Equal(t, "rotating-secret", plaintext)

	_, _, changed, err = newCodec.Rewrap(1001, 2001, rewrapped)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestNilModelCredentialCodecFailsClosed(t *testing.T) {
	var codec *modelCredentialCodec
	_, _, err := codec.Encrypt(1, 1, "secret")
	require.True(t, errors.Is(err, ErrModelCredentialCodecMissing))
}

func mustModelCredentialCodec(t *testing.T, activeKeyID, keysJSON string) ModelCredentialCodec {
	t.Helper()
	codec, err := LoadModelCredentialCodec(func(key string) string {
		switch key {
		case ModelCredentialKeysJSONEnv:
			return keysJSON
		case ModelCredentialActiveKeyIDEnv:
			return activeKeyID
		default:
			return ""
		}
	})
	require.NoError(t, err)
	require.NotNil(t, codec)
	return codec
}

func modelCredentialKeysJSON(keyID string, fill byte) string {
	return "{\"" + keyID + "\":\"" +
		base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32)) + "\"}"
}
