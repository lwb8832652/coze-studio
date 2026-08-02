// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/require"
)

func TestGetHealthz(t *testing.T) {
	const revision = "0123456789abcdef0123456789abcdef01234567"

	t.Run("returns deployment revision", func(t *testing.T) {
		t.Setenv("APP_REVISION", revision)
		c := app.NewContext(0)

		GetHealthz(context.Background(), c)

		require.Equal(t, http.StatusOK, c.Response.StatusCode())
		require.JSONEq(t, `{
			"status": "ok",
			"revision": "0123456789abcdef0123456789abcdef01234567"
		}`, string(c.Response.Body()))
	})

	t.Run("returns empty revision when environment is unset", func(t *testing.T) {
		t.Setenv("APP_REVISION", "")
		require.NoError(t, os.Unsetenv("APP_REVISION"))
		c := app.NewContext(0)

		GetHealthz(context.Background(), c)

		require.Equal(t, http.StatusOK, c.Response.StatusCode())
		require.JSONEq(t, `{
			"status": "ok",
			"revision": ""
		}`, string(c.Response.Body()))
	})
}
