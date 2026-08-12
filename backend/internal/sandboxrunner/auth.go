// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func authenticateBearer(request *http.Request, token string) bool {
	if request == nil || token == "" {
		return false
	}
	value := request.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(value, "Bearer ")
	return len(provided) == len(token) && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}
