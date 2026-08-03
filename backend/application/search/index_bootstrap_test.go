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
	"testing"

	infraes "github.com/coze-dev/coze-studio/backend/infra/es"
)

func TestEnsureSearchIndicesCreatesMissingIndices(t *testing.T) {
	client := &recordingESClient{
		existing: make(map[string]bool),
		created:  make(map[string]map[string]any),
	}

	if err := ensureSearchIndices(context.Background(), client); err != nil {
		t.Fatalf("ensureSearchIndices() error = %v", err)
	}

	for _, index := range []string{"project_draft", "coze_resource"} {
		if _, ok := client.created[index]; !ok {
			t.Fatalf("missing CreateIndex call for %q", index)
		}
	}

	project := client.created["project_draft"]
	assertPropertyType(t, project, "space_id", "keyword")
	assertPropertyType(t, project, "update_time", "long")

	name, ok := project["name"].(map[string]any)
	if !ok {
		t.Fatalf("project name mapping = %#v, want map", project["name"])
	}
	if got := name["type"]; got != "text" {
		t.Fatalf("project name type = %#v, want text", got)
	}
	if _, hasAnalyzer := name["analyzer"]; hasAnalyzer {
		t.Fatalf("project name mapping must not require an optional analyzer: %#v", name)
	}
	fields, ok := name["fields"].(map[string]any)
	if !ok {
		t.Fatalf("project name fields = %#v, want map", name["fields"])
	}
	raw, ok := fields["raw"].(map[string]any)
	if !ok || raw["type"] != "keyword" {
		t.Fatalf("project name.raw mapping = %#v, want keyword", fields["raw"])
	}

	resource := client.created["coze_resource"]
	assertPropertyType(t, resource, "res_id", "keyword")
	assertPropertyType(t, resource, "publish_time", "long")
}

func assertPropertyType(t *testing.T, properties map[string]any, field, want string) {
	t.Helper()

	property, ok := properties[field].(map[string]any)
	if !ok {
		t.Fatalf("%s mapping = %#v, want map", field, properties[field])
	}
	if got := property["type"]; got != want {
		t.Fatalf("%s type = %#v, want %s", field, got, want)
	}
}

type recordingESClient struct {
	existing map[string]bool
	created  map[string]map[string]any
}

func (c *recordingESClient) Create(context.Context, string, string, any) error { return nil }

func (c *recordingESClient) Update(context.Context, string, string, any) error { return nil }

func (c *recordingESClient) Delete(context.Context, string, string) error { return nil }

func (c *recordingESClient) Search(context.Context, string, *infraes.Request) (*infraes.Response, error) {
	return nil, nil
}

func (c *recordingESClient) Exists(_ context.Context, index string) (bool, error) {
	return c.existing[index], nil
}

func (c *recordingESClient) CreateIndex(_ context.Context, index string, properties map[string]any) error {
	c.created[index] = properties
	c.existing[index] = true
	return nil
}

func (c *recordingESClient) DeleteIndex(context.Context, string) error { return nil }

func (c *recordingESClient) Types() infraes.Types { return nil }

func (c *recordingESClient) NewBulkIndexer(string) (infraes.BulkIndexer, error) { return nil, nil }
