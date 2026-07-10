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

package mysql

import (
	"context"
	"strings"
	"testing"
)

type queryParamsFilter interface {
	ParamsFilter(ctx context.Context, sql string, params ...interface{}) (string, []interface{})
}

func TestNewGormLoggerOmitsQueryParameters(t *testing.T) {
	dbLogger := newGormLogger()
	filter, ok := dbLogger.(queryParamsFilter)
	if !ok {
		t.Fatal("gorm logger must support query parameter filtering")
	}

	const query = "SELECT * FROM user WHERE session_key = ?"
	filteredQuery, filteredParams := filter.ParamsFilter(context.Background(), query, "sensitive-session-key")
	if filteredQuery != query {
		t.Fatalf("query changed unexpectedly: %q", filteredQuery)
	}
	if len(filteredParams) != 0 {
		t.Fatalf("query parameters were not omitted: %#v", filteredParams)
	}
}

func TestNewDoesNotExposeDSNInError(t *testing.T) {
	const dsn = "audit-user:audit-secret@tcp(127.0.0.1:invalid-port)/opencoze"
	t.Setenv("MYSQL_DSN", dsn)

	_, err := New()
	if err == nil {
		t.Fatal("expected mysql open to fail")
	}
	for _, secret := range []string{dsn, "audit-user", "audit-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("mysql error exposed sensitive DSN content %q: %v", secret, err)
		}
	}
}
