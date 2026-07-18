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

package appdev

import "time"

type ProjectStatus string

type RuntimeStatus string

const (
	ProjectStatusCreating ProjectStatus = "creating"
	ProjectStatusReady    ProjectStatus = "ready"
	ProjectStatusArchived ProjectStatus = "archived"
	ProjectStatusError    ProjectStatus = "error"

	RuntimeStatusStopped    RuntimeStatus = "stopped"
	RuntimeStatusStarting   RuntimeStatus = "starting"
	RuntimeStatusRunning    RuntimeStatus = "running"
	RuntimeStatusRestarting RuntimeStatus = "restarting"
	RuntimeStatusError      RuntimeStatus = "error"
)

type Project struct {
	ID                       string              `json:"id"`
	SpaceID                  string              `json:"space_id"`
	Name                     string              `json:"name"`
	Description              string              `json:"description,omitempty"`
	Prompt                   string              `json:"prompt,omitempty"`
	Status                   ProjectStatus       `json:"status"`
	RuntimeStatus            RuntimeStatus       `json:"runtime_status"`
	PreviewURL               string              `json:"-"`
	LastBuildStatus          string              `json:"last_build_status,omitempty"`
	LastBuildType            string              `json:"last_build_type,omitempty"`
	LastBuildArtifact        string              `json:"-"`
	LastBuildMessage         string              `json:"last_build_message,omitempty"`
	LastBuildAt              time.Time           `json:"last_build_at,omitempty"`
	SourceVersion            int64               `json:"-"`
	SourceUpdatedAt          time.Time           `json:"source_updated_at,omitempty"`
	ArchiveState             ProjectArchiveState `json:"-"`
	ArchiveIntentVersion     uint64              `json:"-"`
	ArchiveSourceVersion     int64               `json:"-"`
	ArchiveRuntimeGeneration uint64              `json:"-"`
	CreatorID                string              `json:"creator_id"`
	CreatorName              string              `json:"creator_name,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
}

type FileNode struct {
	ID        string      `json:"id"`
	Path      string      `json:"path"`
	Name      string      `json:"name"`
	Type      string      `json:"type"`
	Size      int64       `json:"size,omitempty"`
	Children  []*FileNode `json:"children,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type FileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Version  string `json:"version"`
	Language string `json:"language"`
}

type ProjectSnapshot struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

type RuntimeInfo struct {
	Status          RuntimeStatus `json:"status"`
	PreviewURL      string        `json:"-"`
	Message         string        `json:"message,omitempty"`
	LastKeepAliveAt time.Time     `json:"last_keep_alive_at,omitempty"`
}

type RuntimeLog struct {
	ID        string    `json:"id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Model struct {
	ID                         string `json:"id"`
	Name                       string `json:"name"`
	Provider                   string `json:"provider,omitempty"`
	Protocol                   string `json:"protocol,omitempty"`
	SupportsMultiModal         bool   `json:"supports_multi_modal"`
	SupportsImageUnderstanding bool   `json:"supports_image_understanding"`
	EnableBase64URL            bool   `json:"enable_base64_url"`
}
