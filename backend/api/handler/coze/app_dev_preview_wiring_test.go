// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppDevPreviewProjectorAllowsOnlyExplicitLoopbackDebugHTTP(t *testing.T) {
	projector, err := NewAppDevLoopbackPreviewURLProjector("http://127.0.0.1:8080")
	require.NoError(t, err)
	projected, err := projector.ProjectAppDevPreviewURL(context.Background(), "1001", "project-a", "/preview/opaque")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080/preview/opaque", projected)
	require.True(t, validProjectedAppDevPreviewURL(projected))

	for _, raw := range []string{
		"http://preview.example.test",
		"http://127.0.0.1.evil.test",
		"http://0.0.0.0:8080",
		"https://preview.example.test",
	} {
		_, err := NewAppDevLoopbackPreviewURLProjector(raw)
		require.Error(t, err, raw)
	}
}
