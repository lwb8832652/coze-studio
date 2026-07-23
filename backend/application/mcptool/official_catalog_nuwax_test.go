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

package mcptool

import (
	"strings"
	"testing"
)

func TestOfficialMCPCatalogIncludesCompleteNuwaxEcosystemSnapshot(t *testing.T) {
	definitions := officialMCPCatalogDefinitions()
	if len(definitions) != 26 {
		t.Fatalf("expected 26 official MCP entries, got %d", len(definitions))
	}

	ids := make(map[string]struct{}, len(definitions))
	names := make(map[string]struct{}, len(definitions))
	nuwaxCount := 0
	installableNuwaxCount := 0
	for _, definition := range definitions {
		if definition.CatalogID == "" || definition.Name == "" || definition.Description == "" {
			t.Fatalf("catalog entry has incomplete identity: %#v", definition)
		}
		if _, exists := ids[definition.CatalogID]; exists {
			t.Fatalf("duplicate catalog ID %q", definition.CatalogID)
		}
		if _, exists := names[definition.Name]; exists {
			t.Fatalf("duplicate catalog name %q", definition.Name)
		}
		ids[definition.CatalogID] = struct{}{}
		names[definition.Name] = struct{}{}

		if definition.Source != officialMCPSourceNuwaxEcosystem {
			continue
		}
		nuwaxCount++
		if definition.Publisher != "女娲官方" || definition.IconURL == "" {
			t.Fatalf("Nuwax catalog metadata is incomplete for %q", definition.Name)
		}
		if string(definition.Availability) == "installable" {
			installableNuwaxCount++
		}
	}

	if nuwaxCount != 24 {
		t.Fatalf("expected 24 Nuwax entries, got %d", nuwaxCount)
	}
	if installableNuwaxCount != 5 {
		t.Fatalf("expected 5 verified installable Nuwax entries, got %d", installableNuwaxCount)
	}
}

func TestOfficialMCPConfigRejectsAdapterRequiredNuwaxEntry(t *testing.T) {
	definition := findOfficialMCPCatalogDefinition("nuwax-cc8fadd0f55c")
	if definition == nil {
		t.Fatal("expected Nuwax image service in official catalog")
	}
	if _, err := buildOfficialMCPConfig(definition, nil); err == nil ||
		!strings.Contains(err.Error(), "requires a platform adapter") {
		t.Fatalf("expected platform-adapter rejection, got %v", err)
	}
}

func TestOfficialMCPConfigBuildsVerifiedNuwaxPublicTemplates(t *testing.T) {
	tests := []struct {
		catalogID string
		needle    string
	}{
		{officialMCPCatalogIDNuwaxFetch, "mcp-server-fetch"},
		{officialMCPCatalogIDNuwax12306, "12306-mcp"},
		{officialMCPCatalogIDNuwaxTime, "mcp-server-time"},
		{officialMCPCatalogIDNuwaxScholarly, "mcp-scholarly"},
	}
	for _, tt := range tests {
		t.Run(tt.catalogID, func(t *testing.T) {
			definition := findOfficialMCPCatalogDefinition(tt.catalogID)
			if definition == nil {
				t.Fatalf("catalog entry %q not found", tt.catalogID)
			}
			config, err := buildOfficialMCPConfig(definition, nil)
			if err != nil {
				t.Fatalf("build official config: %v", err)
			}
			if !strings.Contains(config, tt.needle) {
				t.Fatalf("expected %q in config %s", tt.needle, config)
			}
		})
	}

	newsNow := findOfficialMCPCatalogDefinition(officialMCPCatalogIDNuwaxNewsNow)
	if newsNow == nil {
		t.Fatal("NewsNow entry not found")
	}
	if _, err := buildOfficialMCPConfig(newsNow, nil); err == nil {
		t.Fatal("expected missing NewsNow URL to be rejected")
	}
	config, err := buildOfficialMCPConfig(newsNow, map[string]string{
		"base_url": "https://news.example.com/",
	})
	if err != nil {
		t.Fatalf("build NewsNow config: %v", err)
	}
	if !strings.Contains(config, `"BASE_URL":"https://news.example.com"`) {
		t.Fatalf("expected normalized NewsNow URL in config: %s", config)
	}
}
