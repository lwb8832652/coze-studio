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

package search

import (
	"context"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/infra/es"
)

const (
	projectSearchIndex  = "project_draft"
	resourceSearchIndex = "coze_resource"
)

func ensureSearchIndices(ctx context.Context, client es.Client) error {
	indices := []struct {
		name       string
		properties map[string]any
	}{
		{name: projectSearchIndex, properties: projectSearchProperties()},
		{name: resourceSearchIndex, properties: resourceSearchProperties()},
	}

	for _, index := range indices {
		exists, err := client.Exists(ctx, index.name)
		if err != nil {
			return fmt.Errorf("check search index %q: %w", index.name, err)
		}
		if exists {
			continue
		}

		if err := client.CreateIndex(ctx, index.name, index.properties); err != nil {
			// Another replica may have created the index after our existence check.
			existsAfterCreate, existsErr := client.Exists(ctx, index.name)
			if existsErr == nil && existsAfterCreate {
				continue
			}
			return fmt.Errorf("create search index %q: %w", index.name, err)
		}
	}

	return nil
}

func projectSearchProperties() map[string]any {
	return map[string]any{
		"create_time":        longProperty(),
		"has_published":      keywordProperty(),
		"id":                 keywordProperty(),
		"name":               searchableNameProperty(),
		"owner_id":           keywordProperty(),
		"publish_time":       longProperty(),
		"space_id":           keywordProperty(),
		"status":             keywordProperty(),
		"type":               keywordProperty(),
		"update_time":        longProperty(),
		"fav_time":           longProperty(),
		"recently_open_time": longProperty(),
		"is_fav":             keywordProperty(),
		"is_recently_open":   keywordProperty(),
	}
}

func resourceSearchProperties() map[string]any {
	return map[string]any{
		"res_type":       keywordProperty(),
		"app_id":         map[string]any{"type": "keyword", "null_value": "NULL"},
		"res_id":         keywordProperty(),
		"res_sub_type":   keywordProperty(),
		"name":           searchableNameProperty(),
		"owner_id":       keywordProperty(),
		"space_id":       keywordProperty(),
		"biz_status":     keywordProperty(),
		"publish_status": keywordProperty(),
		"create_time":    longProperty(),
		"update_time":    longProperty(),
		"publish_time":   longProperty(),
	}
}

func keywordProperty() map[string]any {
	return map[string]any{"type": "keyword"}
}

func longProperty() map[string]any {
	return map[string]any{"type": "long"}
}

func searchableNameProperty() map[string]any {
	return map[string]any{
		"type": "text",
		"fields": map[string]any{
			"raw": keywordProperty(),
		},
	}
}
