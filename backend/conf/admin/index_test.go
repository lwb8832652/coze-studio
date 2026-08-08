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

package admin_test

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyAdminRetiresOnlyModelManagement(t *testing.T) {
	data, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, removed := range []string{
		`data-page="model-management"`,
		"loadModelManagementPage",
		"renderModelManagement",
		"openAddModelModalWithProvider",
		"saveNewModel",
	} {
		if strings.Contains(source, removed) {
			t.Fatalf("legacy model UI still contains %q", removed)
		}
	}
	for _, preserved := range []string{
		`data-page="basic-config"`,
		`data-page="knowledge-config"`,
		"initBuiltinModelSelect",
		"/api/admin/config/model/list",
	} {
		if !strings.Contains(source, preserved) {
			t.Fatalf("required legacy admin capability %q was removed", preserved)
		}
	}
}
