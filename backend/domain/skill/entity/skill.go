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

package entity

type Type string

const (
	TypeScript       Type = "script"
	TypeWorkflow     Type = "workflow"
	TypeDeerSkill    Type = "deer_skill"
	TypePublicSkill  Type = "public_skill"
	TypeCustomSkill  Type = "custom_skill"
	TypeCozeScript   Type = "coze_script"
	TypeCozeWorkflow Type = "coze_workflow"
)

type Skill struct {
	ID           int64
	SpaceID      int64
	Name         string
	Description  string
	Type         Type
	Version      string
	Enabled      bool
	InputSchema  string
	OutputSchema string
	Executor     string
	Permissions  string
	CreatedAt    int64
	UpdatedAt    int64
}

type SkillVersion struct {
	ID           int64
	SkillID      int64
	Version      string
	SkillMD      string
	InputSchema  string
	OutputSchema string
	Executor     string
	Permissions  string
	CreatedAt    int64
}
