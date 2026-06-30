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

package skill

import "github.com/coze-dev/coze-studio/backend/api/model/base"

const (
	SkillType_DeerSkill   SkillType = 3
	SkillType_PublicSkill SkillType = 4
	SkillType_CustomSkill SkillType = 5
)

type SkillVersion struct {
	ID           int64  `thrift:"id,1,required" form:"id,required" json:"id,string,required" query:"id,required"`
	SkillID      int64  `thrift:"skill_id,2,required" form:"skill_id,required" json:"skill_id,string,required" query:"skill_id,required"`
	Version      string `thrift:"version,3,required" form:"version,required" json:"version,required" query:"version,required"`
	SkillMD      string `thrift:"skill_md,4,required" form:"skill_md,required" json:"skill_md,required" query:"skill_md,required"`
	InputSchema  string `thrift:"input_schema,5,required" form:"input_schema,required" json:"input_schema,required" query:"input_schema,required"`
	OutputSchema string `thrift:"output_schema,6,required" form:"output_schema,required" json:"output_schema,required" query:"output_schema,required"`
	Executor     string `thrift:"executor,7,required" form:"executor,required" json:"executor,required" query:"executor,required"`
	Permissions  string `thrift:"permissions,8,required" form:"permissions,required" json:"permissions,required" query:"permissions,required"`
	CreatedAt    int64  `thrift:"created_at,9,required" form:"created_at,required" json:"created_at,required" query:"created_at,required"`
}

type ListSkillVersionsRequest struct {
	SkillID int64      `thrift:"skill_id,1,required" json:"skill_id,string,required" path:"skill_id,required"`
	Base    *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type ListSkillVersionsData struct {
	Versions []*SkillVersion `thrift:"versions,1,required,list<SkillVersion>" form:"versions,required" json:"versions,required" query:"versions,required"`
}

type ListSkillVersionsResponse struct {
	Data     *ListSkillVersionsData `thrift:"data,1,optional" form:"data" json:"data,omitempty" query:"data"`
	Code     int64                  `thrift:"code,253,required" form:"code,required" json:"code,required" query:"code,required"`
	Msg      string                 `thrift:"msg,254,required" form:"msg,required" json:"msg,required" query:"msg,required"`
	BaseResp *base.BaseResp         `thrift:"BaseResp,255,optional" form:"-" json:"-" query:"-"`
}

type SkillVersionResponse struct {
	Data     *SkillVersion  `thrift:"data,1,optional" form:"data" json:"data,omitempty" query:"data"`
	Code     int64          `thrift:"code,253,required" form:"code,required" json:"code,required" query:"code,required"`
	Msg      string         `thrift:"msg,254,required" form:"msg,required" json:"msg,required" query:"msg,required"`
	BaseResp *base.BaseResp `thrift:"BaseResp,255,optional" form:"-" json:"-" query:"-"`
}

type SkillResource struct {
	ID            int64  `thrift:"id,1,required" form:"id,required" json:"id,string,required" query:"id,required"`
	SkillID       int64  `thrift:"skill_id,2,required" form:"skill_id,required" json:"skill_id,string,required" query:"skill_id,required"`
	VersionID     int64  `thrift:"version_id,3,required" form:"version_id,required" json:"version_id,string,required" query:"version_id,required"`
	Path          string `thrift:"path,4,required" form:"path,required" json:"path,required" query:"path,required"`
	ContentBase64 string `thrift:"content_base64,5,required" form:"content_base64,required" json:"content_base64,required" query:"content_base64,required"`
	Size          int64  `thrift:"size,6,required" form:"size,required" json:"size,required" query:"size,required"`
	SHA256        string `thrift:"sha256,7,required" form:"sha256,required" json:"sha256,required" query:"sha256,required"`
	CreatedAt     int64  `thrift:"created_at,8,required" form:"created_at,required" json:"created_at,required" query:"created_at,required"`
}

type ListSkillVersionResourcesRequest struct {
	SkillID   int64      `thrift:"skill_id,1,required" json:"skill_id,string,required" path:"skill_id,required"`
	VersionID int64      `thrift:"version_id,2,required" json:"version_id,string,required" path:"version_id,required"`
	Base      *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type ListSkillVersionResourcesData struct {
	Resources []*SkillResource `thrift:"resources,1,required,list<SkillResource>" form:"resources,required" json:"resources,required" query:"resources,required"`
}

type ListSkillVersionResourcesResponse struct {
	Data     *ListSkillVersionResourcesData `thrift:"data,1,optional" form:"data" json:"data,omitempty" query:"data"`
	Code     int64                          `thrift:"code,253,required" form:"code,required" json:"code,required" query:"code,required"`
	Msg      string                         `thrift:"msg,254,required" form:"msg,required" json:"msg,required" query:"msg,required"`
	BaseResp *base.BaseResp                 `thrift:"BaseResp,255,optional" form:"-" json:"-" query:"-"`
}

type UpdateSkillVersionResourceRequest struct {
	SkillID       int64      `thrift:"skill_id,1,required" json:"skill_id,string,required" path:"skill_id,required"`
	VersionID     int64      `thrift:"version_id,2,required" json:"version_id,string,required" path:"version_id,required"`
	Path          string     `thrift:"path,3,required" form:"path,required" json:"path,required" query:"path,required"`
	ContentBase64 string     `thrift:"content_base64,4,required" form:"content_base64,required" json:"content_base64,required" query:"content_base64,required"`
	Base          *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type UpdateSkillVersionContentRequest struct {
	SkillID   int64      `thrift:"skill_id,1,required" json:"skill_id,string,required" path:"skill_id,required"`
	VersionID int64      `thrift:"version_id,2,required" json:"version_id,string,required" path:"version_id,required"`
	SkillMD   string     `thrift:"skill_md,3,required" form:"skill_md,required" json:"skill_md,required" query:"skill_md,required"`
	Base      *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type ExportSkillVersionRequest struct {
	SkillID   int64      `thrift:"skill_id,1,required" json:"skill_id,string,required" path:"skill_id,required"`
	VersionID int64      `thrift:"version_id,2,required" json:"version_id,string,required" path:"version_id,required"`
	Base      *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type ExportSkillVersionData struct {
	FileName      string `thrift:"file_name,1,required" form:"file_name,required" json:"file_name,required" query:"file_name,required"`
	ContentBase64 string `thrift:"content_base64,2,required" form:"content_base64,required" json:"content_base64,required" query:"content_base64,required"`
	ContentType   string `thrift:"content_type,3,required" form:"content_type,required" json:"content_type,required" query:"content_type,required"`
}

type ExportSkillVersionResponse struct {
	Data     *ExportSkillVersionData `thrift:"data,1,optional" form:"data" json:"data,omitempty" query:"data"`
	Code     int64                   `thrift:"code,253,required" form:"code,required" json:"code,required" query:"code,required"`
	Msg      string                  `thrift:"msg,254,required" form:"msg,required" json:"msg,required" query:"msg,required"`
	BaseResp *base.BaseResp          `thrift:"BaseResp,255,optional" form:"-" json:"-" query:"-"`
}

type RollbackSkillVersionRequest struct {
	SkillID   int64      `thrift:"skill_id,1,required" json:"skill_id,string,required" path:"skill_id,required"`
	VersionID int64      `thrift:"version_id,2,required" json:"version_id,string,required" path:"version_id,required"`
	Base      *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type ListSkillToolCandidatesRequest struct {
	SpaceID int64      `thrift:"space_id,1,required" form:"space_id,required" json:"space_id,string,required" query:"space_id,required"`
	Base    *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type SkillToolCandidate struct {
	Name        string `thrift:"name,1,required" form:"name,required" json:"name,required" query:"name,required"`
	DisplayName string `thrift:"display_name,2,required" form:"display_name,required" json:"display_name,required" query:"display_name,required"`
	Description string `thrift:"description,3,required" form:"description,required" json:"description,required" query:"description,required"`
	Category    string `thrift:"category,4,required" form:"category,required" json:"category,required" query:"category,required"`
	Visibility  string `thrift:"visibility,5,required" form:"visibility,required" json:"visibility,required" query:"visibility,required"`
	Source      string `thrift:"source,6,optional" form:"source" json:"source,omitempty" query:"source"`
	SourceID    string `thrift:"source_id,7,optional" form:"source_id" json:"source_id,omitempty" query:"source_id"`
	SourceName  string `thrift:"source_name,8,optional" form:"source_name" json:"source_name,omitempty" query:"source_name"`
}

type ListSkillToolCandidatesData struct {
	Tools []*SkillToolCandidate `thrift:"tools,1,required,list<SkillToolCandidate>" form:"tools,required" json:"tools,required" query:"tools,required"`
}

type ListSkillToolCandidatesResponse struct {
	Data     *ListSkillToolCandidatesData `thrift:"data,1,optional" form:"data" json:"data,omitempty" query:"data"`
	Code     int64                        `thrift:"code,253,required" form:"code,required" json:"code,required" query:"code,required"`
	Msg      string                       `thrift:"msg,254,required" form:"msg,required" json:"msg,required" query:"msg,required"`
	BaseResp *base.BaseResp               `thrift:"BaseResp,255,optional" form:"-" json:"-" query:"-"`
}

type InstallSkillFromArtifactRequest struct {
	SpaceID    int64      `thrift:"space_id,1,required" form:"space_id,required" json:"space_id,string,required" query:"space_id,required"`
	ThreadID   int64      `thrift:"thread_id,2,required" form:"thread_id,required" json:"thread_id,string,required" query:"thread_id,required"`
	ArtifactID int64      `thrift:"artifact_id,3,required" form:"artifact_id,required" json:"artifact_id,string,required" query:"artifact_id,required"`
	Base       *base.Base `thrift:"Base,255,optional" json:"-" query:"-" form:"-"`
}

type InstallSkillFromArtifactData struct {
	Success   bool   `thrift:"success,1,required" form:"success,required" json:"success,required" query:"success,required"`
	SkillName string `thrift:"skill_name,2,required" form:"skill_name,required" json:"skill_name,required" query:"skill_name,required"`
	Message   string `thrift:"message,3,required" form:"message,required" json:"message,required" query:"message,required"`
	Skill     *Skill `thrift:"skill,4,optional" form:"skill" json:"skill,omitempty" query:"skill"`
}

type InstallSkillFromArtifactResponse struct {
	Data     *InstallSkillFromArtifactData `thrift:"data,1,optional" form:"data" json:"data,omitempty" query:"data"`
	Code     int64                         `thrift:"code,253,required" form:"code,required" json:"code,required" query:"code,required"`
	Msg      string                        `thrift:"msg,254,required" form:"msg,required" json:"msg,required" query:"msg,required"`
	BaseResp *base.BaseResp                `thrift:"BaseResp,255,optional" form:"-" json:"-" query:"-"`
}
