// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import "testing"

func TestProviderExecutionProjectScopeValidationIsStrict(t *testing.T) {
	for _, value := range []string{"", " project-a", "project-a ", "project/a", "\tproject-a"} {
		if ValidProviderExecutionProjectID(value) {
			t.Fatalf("project scope %q was accepted", value)
		}
	}
	if !ValidProviderExecutionProjectID("project-a_1") {
		t.Fatal("canonical project scope was rejected")
	}
}
