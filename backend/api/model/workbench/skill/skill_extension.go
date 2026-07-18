//go:build hz_fallback_models

/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package skill

import "github.com/coze-dev/coze-studio/backend/api/model/base"

// This file keeps Workbench skill extensions available when the local hz
// generator is not installed. The IDL remains the source of truth.
type SkillResourceOperation int64

const (
	SkillResourceOperation_Upsert          SkillResourceOperation = 1
	SkillResourceOperation_Delete          SkillResourceOperation = 2
	SkillResourceOperation_Move            SkillResourceOperation = 3
	SkillResourceOperation_CreateDirectory SkillResourceOperation = 4
)

type SkillVersion struct {
	ID           int64  `json:"id,string,required"`
	SkillID      int64  `json:"skill_id,string,required"`
	Version      string `json:"version,required"`
	SkillMD      string `json:"skill_md,required"`
	InputSchema  string `json:"input_schema,required"`
	OutputSchema string `json:"output_schema,required"`
	Executor     string `json:"executor,required"`
	Permissions  string `json:"permissions,required"`
	CreatedAt    int64  `json:"created_at,required"`
}

type ListSkillVersionsRequest struct {
	SkillID int64      `path:"skill_id,required" json:"skill_id,string,required"`
	Base    *base.Base `json:"-"`
}

type ListSkillVersionsData struct {
	Versions []*SkillVersion `json:"versions,required"`
}
type ListSkillVersionsResponse struct {
	Data     *ListSkillVersionsData `json:"data,omitempty"`
	Code     int64                  `json:"code,required"`
	Msg      string                 `json:"msg,required"`
	BaseResp *base.BaseResp         `json:"-"`
}
type SkillVersionResponse struct {
	Data     *SkillVersion  `json:"data,omitempty"`
	Code     int64          `json:"code,required"`
	Msg      string         `json:"msg,required"`
	BaseResp *base.BaseResp `json:"-"`
}

type SkillResource struct {
	ID            int64  `json:"id,string,required"`
	SkillID       int64  `json:"skill_id,string,required"`
	VersionID     int64  `json:"version_id,string,required"`
	Path          string `json:"path,required"`
	ContentBase64 string `json:"content_base64,required"`
	Size          int64  `json:"size,required"`
	SHA256        string `json:"sha256,required"`
	CreatedAt     int64  `json:"created_at,required"`
}
type ListSkillVersionResourcesRequest struct {
	SkillID   int64      `path:"skill_id,required" json:"skill_id,string,required"`
	VersionID int64      `path:"version_id,required" json:"version_id,string,required"`
	Base      *base.Base `json:"-"`
}
type ListSkillVersionResourcesData struct {
	Resources []*SkillResource `json:"resources,required"`
}
type ListSkillVersionResourcesResponse struct {
	Data     *ListSkillVersionResourcesData `json:"data,omitempty"`
	Code     int64                          `json:"code,required"`
	Msg      string                         `json:"msg,required"`
	BaseResp *base.BaseResp                 `json:"-"`
}
type UpdateSkillVersionResourceRequest struct {
	SkillID       int64                  `path:"skill_id,required" json:"skill_id,string,required"`
	VersionID     int64                  `path:"version_id,required" json:"version_id,string,required"`
	Path          string                 `form:"path,required" json:"path,required"`
	ContentBase64 string                 `form:"content_base64,required" json:"content_base64,required"`
	Operation     SkillResourceOperation `form:"operation" json:"operation,omitempty"`
	TargetPath    string                 `form:"target_path" json:"target_path,omitempty"`
	Base          *base.Base             `json:"-"`
}
type UpdateSkillVersionContentRequest struct {
	SkillID   int64      `path:"skill_id,required" json:"skill_id,string,required"`
	VersionID int64      `path:"version_id,required" json:"version_id,string,required"`
	SkillMD   string     `form:"skill_md,required" json:"skill_md,required"`
	Base      *base.Base `json:"-"`
}
type ExportSkillVersionRequest struct {
	SkillID   int64      `path:"skill_id,required" json:"skill_id,string,required"`
	VersionID int64      `path:"version_id,required" json:"version_id,string,required"`
	Base      *base.Base `json:"-"`
}
type ExportSkillVersionData struct {
	FileName      string `json:"file_name,required"`
	ContentBase64 string `json:"content_base64,required"`
	ContentType   string `json:"content_type,required"`
}
type ExportSkillVersionResponse struct {
	Data     *ExportSkillVersionData `json:"data,omitempty"`
	Code     int64                   `json:"code,required"`
	Msg      string                  `json:"msg,required"`
	BaseResp *base.BaseResp          `json:"-"`
}
type RollbackSkillVersionRequest struct {
	SkillID           int64      `path:"skill_id,required" json:"skill_id,string,required"`
	VersionID         int64      `path:"version_id,required" json:"version_id,string,required"`
	ExpectedVersionID int64      `form:"expected_version_id,required" json:"expected_version_id,string,required"`
	Base              *base.Base `json:"-"`
}

type ListSkillToolCandidatesRequest struct {
	SpaceID int64      `query:"space_id,required" json:"space_id,string,required"`
	Base    *base.Base `json:"-"`
}
type SkillToolCandidate struct {
	Name        string `json:"name,required"`
	DisplayName string `json:"display_name,required"`
	Description string `json:"description,required"`
	Category    string `json:"category,required"`
	Visibility  string `json:"visibility,required"`
	Source      string `json:"source,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	SourceName  string `json:"source_name,omitempty"`
}
type ListSkillToolCandidatesData struct {
	Tools []*SkillToolCandidate `json:"tools,required"`
}
type ListSkillToolCandidatesResponse struct {
	Data     *ListSkillToolCandidatesData `json:"data,omitempty"`
	Code     int64                        `json:"code,required"`
	Msg      string                       `json:"msg,required"`
	BaseResp *base.BaseResp               `json:"-"`
}

type InstallSkillFromArtifactRequest struct {
	SpaceID    int64      `form:"space_id,required" json:"space_id,string,required"`
	ThreadID   int64      `form:"thread_id,required" json:"thread_id,string,required"`
	ArtifactID int64      `form:"artifact_id,required" json:"artifact_id,string,required"`
	Base       *base.Base `json:"-"`
}
type InstallSkillFromArtifactData struct {
	Success   bool   `json:"success,required"`
	SkillName string `json:"skill_name,required"`
	Message   string `json:"message,required"`
	Skill     *Skill `json:"skill,omitempty"`
}
type InstallSkillFromArtifactResponse struct {
	Data     *InstallSkillFromArtifactData `json:"data,omitempty"`
	Code     int64                         `json:"code,required"`
	Msg      string                        `json:"msg,required"`
	BaseResp *base.BaseResp                `json:"-"`
}
