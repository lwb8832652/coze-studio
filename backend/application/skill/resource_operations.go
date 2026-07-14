/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package skill

import (
	"context"
	"encoding/base64"
	"path"
	"sort"
	"strings"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

func (s *ApplicationService) mutateSkillVersionResource(ctx context.Context, req *skillapi.UpdateSkillVersionResourceRequest) (*skillapi.SkillVersionResponse, error) {
	cleanPath, err := managedResourcePath(req.Path)
	if err != nil {
		return nil, err
	}

	switch req.Operation {
	case skillapi.SkillResourceOperation_Delete:
		return s.deleteSkillVersionResources(ctx, req.SkillID, req.VersionID, cleanPath)
	case skillapi.SkillResourceOperation_Move:
		targetPath, err := managedResourcePath(req.TargetPath)
		if err != nil {
			return nil, err
		}
		if targetPath == cleanPath || strings.HasPrefix(targetPath+"/", cleanPath+"/") {
			return nil, domain.InvalidArgumentErrorf("target path must be outside the source path")
		}
		return s.moveSkillVersionResources(ctx, req.SkillID, req.VersionID, cleanPath, targetPath)
	case skillapi.SkillResourceOperation_CreateDirectory:
		return s.upsertManagedResource(
			ctx,
			req.SkillID,
			req.VersionID,
			path.Join(cleanPath, entity.ResourceDirectoryMarker),
			[]byte(entity.ResourceDirectoryMarker),
		)
	default:
		return nil, domain.InvalidArgumentErrorf("unsupported resource operation %d", req.Operation)
	}
}

func (s *ApplicationService) deleteSkillVersionResources(ctx context.Context, skillID, versionID int64, sourcePath string) (*skillapi.SkillVersionResponse, error) {
	version, err := s.DomainSVC.MutateVersionResources(ctx, skillID, versionID, []domain.ResourceMutation{{
		Operation: domain.ResourceMutationDelete,
		Path:      sourcePath,
	}})
	if err != nil {
		return nil, err
	}
	return &skillapi.SkillVersionResponse{Code: 0, Msg: "success", Data: skillVersionToAPI(version)}, nil
}

func (s *ApplicationService) moveSkillVersionResources(ctx context.Context, skillID, versionID int64, sourcePath, targetPath string) (*skillapi.SkillVersionResponse, error) {
	version, err := s.DomainSVC.MutateVersionResources(ctx, skillID, versionID, []domain.ResourceMutation{{
		Operation:  domain.ResourceMutationMove,
		Path:       sourcePath,
		TargetPath: targetPath,
	}})
	if err != nil {
		return nil, err
	}
	return &skillapi.SkillVersionResponse{Code: 0, Msg: "success", Data: skillVersionToAPI(version)}, nil
}

func (s *ApplicationService) upsertManagedResource(ctx context.Context, skillID, versionID int64, resourcePath string, content []byte) (*skillapi.SkillVersionResponse, error) {
	return s.UpdateSkillVersionResource(ctx, &skillapi.UpdateSkillVersionResourceRequest{
		SkillID:       skillID,
		VersionID:     versionID,
		Path:          resourcePath,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		Operation:     skillapi.SkillResourceOperation_Upsert,
	})
}

func managedResourcePath(value string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	cleaned := path.Clean(trimmed)
	if trimmed == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", domain.InvalidArgumentErrorf("invalid skill resource path")
	}
	if strings.EqualFold(cleaned, "SKILL.md") {
		return "", domain.InvalidArgumentErrorf("SKILL.md must be edited through the declaration editor")
	}
	return cleaned, nil
}

func matchingResourcePaths(resources []*entity.SkillResource, sourcePath string) []string {
	matches := matchingResources(resources, sourcePath)
	paths := make([]string, 0, len(matches))
	for _, resource := range matches {
		paths = append(paths, resource.Path)
	}
	return paths
}

func matchingResources(resources []*entity.SkillResource, sourcePath string) []*entity.SkillResource {
	matches := make([]*entity.SkillResource, 0)
	prefix := strings.TrimSuffix(sourcePath, "/") + "/"
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		if resource.Path == sourcePath || strings.HasPrefix(resource.Path, prefix) {
			matches = append(matches, resource)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Path < matches[j].Path })
	return matches
}
