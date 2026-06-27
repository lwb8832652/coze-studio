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

package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type RuntimeFileRepository interface {
	UpsertRuntimeFile(
		ctx context.Context,
		file *entity.AgentFile,
	) (*entity.AgentFile, bool, error)
	GetFileByID(
		ctx context.Context,
		id int64,
	) (*entity.AgentFile, error)
	GetRuntimeFile(
		ctx context.Context,
		runID int64,
		virtualPath string,
	) (*entity.AgentFile, error)
}

func NewRuntimeFileRepository(db *gorm.DB) RuntimeFileRepository {
	return &threadRepository{db: db}
}
