// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"net/http/httptest"
	"testing"
)

func TestAuthenticateBearerRequiresExactBearerCredential(t *testing.T) {
	const credential = "runner-auth-token-0123456789"
	for name, authorization := range map[string]string{
		"missing":       "",
		"wrong scheme":  "Basic " + credential,
		"wrong token":   "Bearer runner-auth-token-0123456780",
		"extra spacing": "Bearer  " + credential,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set("Authorization", authorization)
			if authenticateBearer(request, credential) {
				t.Fatal("authenticateBearer() unexpectedly accepted request")
			}
		})
	}

	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+credential)
	if !authenticateBearer(request, credential) {
		t.Fatal("authenticateBearer() rejected exact bearer credential")
	}
}
