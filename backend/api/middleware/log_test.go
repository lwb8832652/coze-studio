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

package middleware

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func TestAccessLogMWDoesNotLogRequestOrResponsePayloads(t *testing.T) {
	var output bytes.Buffer
	logs.SetOutput(&output)
	logs.SetLevel(logs.LevelDebug)
	t.Cleanup(func() {
		logs.SetLevel(logs.LevelInfo)
		logs.SetOutput(os.Stderr)
	})

	h := server.Default()
	h.Use(AccessLogMW())
	h.POST("/api/passport/web/email/login/", func(_ context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, map[string]string{
			"session": "response-sensitive-marker",
		})
	})

	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/passport/web/email/login/?token=query-sensitive-marker",
		&ut.Body{
			Body: bytes.NewBufferString(`{"password":"request-sensitive-marker"}`),
			Len:  39,
		},
	)

	assert.DeepEqual(t, http.StatusOK, response.Code)
	logged := output.String()
	assert.Assert(t, bytes.Contains([]byte(logged), []byte("/api/passport/web/email/login/")))
	assert.Assert(t, !bytes.Contains([]byte(logged), []byte("query-sensitive-marker")))
	assert.Assert(t, !bytes.Contains([]byte(logged), []byte("request-sensitive-marker")))
	assert.Assert(t, !bytes.Contains([]byte(logged), []byte("response-sensitive-marker")))
}
