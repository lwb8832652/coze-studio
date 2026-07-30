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

package coze

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/api/model/passport"
	"github.com/coze-dev/coze-studio/backend/application/user"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestPassportWebEmailLoginPostDoesNotLogSessionKey(t *testing.T) {
	const sessionKey = "test-session-secret"

	patch := mockey.Mock((*user.UserApplicationService).PassportWebEmailLoginPost).
		Return(&passport.PassportWebEmailLoginPostResponse{Code: 0}, sessionKey, nil).
		Build()
	t.Cleanup(func() { patch.UnPatch() })

	var output bytes.Buffer
	logs.SetOutput(&output)
	t.Cleanup(func() { logs.SetOutput(os.Stderr) })

	c := app.NewContext(1)
	c.Request.SetMethod(http.MethodPost)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))
	c.Request.SetBody([]byte(`{"email":"test@example.com","password":"password"}`))

	PassportWebEmailLoginPost(context.Background(), c)

	if got := c.Response.StatusCode(); got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
	cookieHeader := string(c.Response.Header.Peek("Set-Cookie"))
	cookies := (&http.Response{Header: http.Header{"Set-Cookie": {cookieHeader}}}).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set-cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != entity.SessionKey || cookie.Value != sessionKey {
		t.Fatalf("session cookie identity = %q, want %q", cookie.Name, entity.SessionKey)
	}
	if cookie.Path != "/" || cookie.MaxAge != consts.SessionMaxAgeSecond ||
		!cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteDefaultMode {
		t.Fatal("login response changed the session cookie policy")
	}
	if strings.Contains(output.String(), sessionKey) {
		t.Fatal("login handler logged the session key")
	}
}
