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
	"context"
	"errors"
	"net/http"
	"net/mail"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/api/internal/httputil"
	"github.com/coze-dev/coze-studio/backend/application/user"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

const systemAdminBootstrapEmailsEnv = "COZE_SYSTEM_ADMIN_EMAILS"

var loadAdminAuthEmailConfig = func(ctx context.Context) (string, error) {
	baseConf, revision, err := config.Base().GetBaseConfigWithRevision(ctx)
	if err != nil {
		return "", err
	}
	return resolveAdminAuthEmailConfig(baseConf.AdminEmails, revision, os.Getenv), nil
}

func resolveAdminAuthEmailConfig(configured, revision string, getenv func(string) string) string {
	if configured != "" || revision != kvstore.MissingRevision || getenv == nil {
		return configured
	}
	return getenv(systemAdminBootstrapEmailsEnv)
}

var loadAdminAuthEmails = func(ctx context.Context) (string, error) {
	raw, err := loadAdminAuthEmailConfig(ctx)
	if err != nil {
		return "", err
	}
	adminEmails, valid := canonicalAdminAuthEmails(raw)
	if !valid {
		logs.CtxWarnf(ctx, "[AdminAuthMW] admin email configuration is invalid")
		return "", nil
	}
	return adminEmails, nil
}

var validateSession = func(ctx context.Context, sessionID string) (*entity.Session, error) {
	return user.UserApplicationSVC.ValidateSession(ctx, sessionID)
}

var noNeedSessionCheckPath = map[string]bool{
	"/api/passport/web/email/login/":       true,
	"/api/passport/web/email/register/v2/": true,
}

func SessionAuthMW() app.HandlerFunc {
	return SessionAuthMWWithValidator(validateSession)
}

func SessionAuthMWWithValidator(validator func(context.Context, string) (*entity.Session, error)) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		requestAuthType := ctx.GetInt32(RequestAuthTypeStr)
		if requestAuthType != int32(RequestAuthTypeWebAPI) {
			ctx.Next(c)
			return
		}

		if noNeedSessionCheckPath[string(ctx.GetRequest().URI().Path())] {
			ctx.Next(c)
			return
		}

		s := ctx.Cookie(entity.SessionKey)
		if len(s) == 0 {
			logs.Errorf("[SessionAuthMW] session id is nil")
			authenticationRequired(ctx)
			return
		}

		if validator == nil {
			logs.Errorf("[SessionAuthMW] session validator is nil")
			authenticationRequired(ctx)
			return
		}
		session, err := validator(c, string(s))
		if err != nil {
			logs.Errorf("[SessionAuthMW] validate session failed")
			authenticationRequired(ctx)
			return
		}
		if session == nil || session.UserID <= 0 {
			logs.Errorf("[SessionAuthMW] validated session is empty")
			authenticationRequired(ctx)
			return
		}

		ctxcache.Store(c, consts.SessionDataKeyInCtx, session)
		ctx.Next(c)
	}
}

func canonicalAdminAuthEmails(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	seen := make(map[string]struct{})
	canonical := make([]string, 0, strings.Count(raw, ",")+1)
	for _, item := range strings.Split(raw, ",") {
		candidate := strings.TrimSpace(item)
		address, err := mail.ParseAddress(candidate)
		if err != nil || address == nil || address.Name != "" || !strings.EqualFold(candidate, address.Address) {
			return "", false
		}
		email := strings.ToLower(strings.TrimSpace(address.Address))
		if email == "" {
			return "", false
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		canonical = append(canonical, email)
	}
	return strings.Join(canonical, ","), true
}

func authenticationRequired(ctx *app.RequestContext) {
	ctx.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"code":       http.StatusUnauthorized,
		"error_code": "AUTHENTICATION_REQUIRED",
		"msg":        "authentication required",
	})
}

func AdminAuthMW() app.HandlerFunc {
	return AdminAuthMWWithEmailLoader(loadAdminAuthEmails)
}

func AdminAuthMWWithEmailLoader(loader func(context.Context) (string, error)) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		session, ok := ctxcache.Get[*entity.Session](c, consts.SessionDataKeyInCtx)
		if !ok || session == nil || session.UserID <= 0 {
			logs.Errorf("[AdminAuthMW] session data is nil")
			authenticationRequired(ctx)
			return
		}

		if loader == nil {
			logs.Errorf("[AdminAuthMW] admin email loader is nil")
			httputil.InternalError(c, ctx, errors.New("admin authentication configuration unavailable"))
			return
		}
		adminEmails, err := loader(c)
		if err != nil {
			logs.Errorf("[AdminAuthMW] get base config failed")
			httputil.InternalError(c, ctx, err)
			return
		}

		if adminEmails == "" {
			logs.CtxWarnf(c, "[AdminAuthMW] admin emails are empty")
		}

		if user.IsSystemAdminEmail(session.UserEmail, adminEmails) {
			ctx.Next(c)
			return
		}

		ctx.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
			"code":       http.StatusForbidden,
			"error_code": "ADMIN_PERMISSION_DENIED",
			"msg":        "the account does not have permission to access",
		})
	}
}
