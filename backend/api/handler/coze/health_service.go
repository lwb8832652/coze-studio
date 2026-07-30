// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func GetHealthz(ctx context.Context, c *app.RequestContext) {
	revision := strings.TrimSpace(os.Getenv("APP_REVISION"))
	c.JSON(consts.StatusOK, map[string]string{
		"status":   "ok",
		"revision": revision,
	})
}
