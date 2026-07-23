// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package base

import "testing"

func TestBasicConfigurationPatchRecognizesSiteBrandFields(t *testing.T) {
	siteName := "NewX Enterprise"
	siteDescription := "Enterprise AI workspace"
	siteLogoURI := "site-brand/logo/logo.png"
	faviconURI := "site-brand/favicon/favicon.png"

	tests := []struct {
		name  string
		patch BasicConfigurationPatch
	}{
		{name: "site name", patch: BasicConfigurationPatch{SiteName: &siteName}},
		{name: "site description", patch: BasicConfigurationPatch{SiteDescription: &siteDescription}},
		{name: "site logo", patch: BasicConfigurationPatch{SiteLogoURI: &siteLogoURI}},
		{name: "favicon", patch: BasicConfigurationPatch{FaviconURI: &faviconURI}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.patch.IsEmpty() {
				t.Fatalf("brand patch %q must not be treated as empty", tt.name)
			}
		})
	}
}
