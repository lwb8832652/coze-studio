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
