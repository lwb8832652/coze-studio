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

package impl

import (
	"context"

	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/coze-dev/coze-studio/backend/infra/document/searchstore"
)

func newNoopVectorManager() searchstore.Manager {
	return noopVectorManager{}
}

type noopVectorManager struct{}

func (noopVectorManager) Create(context.Context, *searchstore.CreateRequest) error {
	return nil
}

func (noopVectorManager) Drop(context.Context, *searchstore.DropRequest) error {
	return nil
}

func (noopVectorManager) GetType() searchstore.SearchStoreType {
	return searchstore.TypeVectorStore
}

func (noopVectorManager) GetSearchStore(context.Context, string) (searchstore.SearchStore, error) {
	return noopVectorSearchStore{}, nil
}

type noopVectorSearchStore struct{}

func (noopVectorSearchStore) Store(_ context.Context, docs []*schema.Document, _ ...indexer.Option) ([]string, error) {
	return make([]string, len(docs)), nil
}

func (noopVectorSearchStore) Retrieve(context.Context, string, ...retriever.Option) ([]*schema.Document, error) {
	return nil, nil
}

func (noopVectorSearchStore) Delete(context.Context, []string) error {
	return nil
}
