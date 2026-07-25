// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package safetext

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAllowsOrdinaryPromptAndCheckpointWords(t *testing.T) {
	value, err := Normalize(
		"Prompt 配置和 checkpoint 状态将在维护后保留，token 统计不会改变。",
		Rules{MaxRunes: 128, Required: true},
	)

	require.NoError(t, err)
	require.Equal(
		t,
		"Prompt 配置和 checkpoint 状态将在维护后保留，token 统计不会改变。",
		value,
	)
}

func TestNormalizeRejectsExecutableMarkupControlsAndSecrets(t *testing.T) {
	for _, value := range []string{
		"<script>alert(1)</script>",
		"api_key=do-not-store",
		`{"password":"hunter2"}`,
		`{"client_secret":"value"}`,
		`{"api_key":"value"}`,
		`{"token":"value"}`,
		`{"DB_PASSWORD":"hunter2"}`,
		`{"GITHUB_TOKEN":"value"}`,
		`{"openai_api_key":"value"}`,
		`{"my_client_secret":"value"}`,
		`'access_token': 'value'`,
		`'vendor.password': 'value'`,
		"PASSWORD: hunter2",
		"CLIENT_SECRET=value",
		"DB_PASSWORD=hunter2",
		"GITHUB_TOKEN=value",
		"OPENAI_API_KEY=value",
		"MY_CLIENT_SECRET=value",
		"Authorization: Bearer abcdefghijklmnop",
		"-----BEGIN PRIVATE KEY-----",
		"object at s3://private-bucket/key",
		"bad\x00value",
	} {
		_, err := Normalize(value, Rules{MaxRunes: 128, Required: true})
		require.True(t, errors.Is(err, ErrUnsafe), value)
	}
}

func TestNormalizeAllowsOrdinaryWordsAndNonSensitiveSuffixes(t *testing.T) {
	for _, value := range []string{
		"prompt 和 checkpoint 的状态正常，token 用量保持稳定。",
		"prompt=customer support",
		"checkpoint: ready",
		"token_count=10",
		"password_hint=未设置",
	} {
		normalized, err := Normalize(
			value,
			Rules{MaxRunes: 128, Required: true},
		)
		require.NoError(t, err, value)
		require.Equal(t, value, normalized)
	}
}
