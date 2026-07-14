/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package service

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	"github.com/coze-dev/coze-studio/backend/domain/skill/repository"
)

func (s *skillService) MutateVersionResources(ctx context.Context, skillID, versionID int64, mutations []ResourceMutation) (*entity.SkillVersion, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if skillID <= 0 || versionID <= 0 {
		return nil, InvalidArgumentErrorf("skill id and version id are required")
	}
	if len(mutations) == 0 {
		return nil, InvalidArgumentErrorf("resource mutation is required")
	}

	current, err := s.Get(ctx, skillID)
	if err != nil {
		return nil, err
	}
	latest, err := s.components.Repo.GetLatestVersion(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if latest == nil || latest.ID != versionID {
		latestID := int64(0)
		if latest != nil {
			latestID = latest.ID
		}
		return nil, ConflictErrorf("expected version %d is stale; latest version is %d", versionID, latestID)
	}
	resources, err := s.components.Repo.ListResources(ctx, skillID, versionID)
	if err != nil {
		return nil, err
	}
	nextResources, err := applyResourceMutations(skillID, resources, mutations)
	if err != nil {
		return nil, err
	}

	restored, err := skillFromVersionSnapshot(current, latest)
	if err != nil {
		return nil, err
	}
	restored.UpdatedAt = time.Now().UnixMilli()
	newVersion, err := s.newVersionSnapshot(ctx, restored, latest.SkillMD)
	if err != nil {
		return nil, err
	}
	for _, resource := range nextResources {
		resource.VersionID = newVersion.ID
	}
	if err := s.components.Repo.UpdateWithVersionCAS(ctx, restored, versionID, newVersion, nextResources); err != nil {
		if errors.Is(err, repository.ErrVersionConflict) {
			return nil, ConflictErrorf("skill resources changed while saving")
		}
		return nil, err
	}
	return newVersion, nil
}

func applyResourceMutations(skillID int64, resources []*entity.SkillResource, mutations []ResourceMutation) ([]*entity.SkillResource, error) {
	items := make(map[string]*entity.SkillResource, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		items[resource.Path] = cloneResource(skillID, resource)
	}

	for _, mutation := range mutations {
		source, err := editableResourcePath(mutation.Path)
		if err != nil {
			return nil, err
		}
		switch mutation.Operation {
		case ResourceMutationUpsert:
			if int64(len(mutation.Content)) > maxSkillArchiveFileBytes {
				return nil, InvalidArgumentErrorf("skill resource %s exceeds %d bytes", source, maxSkillArchiveFileBytes)
			}
			items[source] = &entity.SkillResource{SkillID: skillID, Path: source, Content: append([]byte(nil), mutation.Content...), Size: int64(len(mutation.Content)), SHA256: contentSHA256(mutation.Content)}
		case ResourceMutationDelete:
			matches := resourcePathsUnder(items, source)
			if len(matches) == 0 {
				return nil, NotFoundErrorf("skill resource %s not found", source)
			}
			for _, match := range matches {
				delete(items, match)
			}
		case ResourceMutationMove:
			target, err := editableResourcePath(mutation.TargetPath)
			if err != nil {
				return nil, err
			}
			if target == source || strings.HasPrefix(target+"/", source+"/") {
				return nil, InvalidArgumentErrorf("target path must be outside the source path")
			}
			matches := resourcePathsUnder(items, source)
			if len(matches) == 0 {
				return nil, NotFoundErrorf("skill resource %s not found", source)
			}
			moving := make(map[string]struct{}, len(matches))
			for _, match := range matches {
				moving[match] = struct{}{}
			}
			destinations := make(map[string]string, len(matches))
			for _, match := range matches {
				relative := strings.TrimPrefix(strings.TrimPrefix(match, source), "/")
				destination := target
				if relative != "" {
					destination = path.Join(target, relative)
				}
				if _, duplicate := destinations[destination]; duplicate {
					return nil, InvalidArgumentErrorf("target path already exists: %s", destination)
				}
				if _, exists := items[destination]; exists {
					if _, isMoving := moving[destination]; !isMoving {
						return nil, InvalidArgumentErrorf("target path already exists: %s", destination)
					}
				}
				destinations[destination] = match
			}
			for _, match := range matches {
				delete(items, match)
			}
			for destination, sourcePath := range destinations {
				cloned := cloneResource(skillID, resourcesByPath(resources)[sourcePath])
				cloned.Path = destination
				items[destination] = cloned
			}
		default:
			return nil, InvalidArgumentErrorf("unsupported resource mutation %d", mutation.Operation)
		}
	}

	paths := make([]string, 0, len(items))
	for resourcePath := range items {
		paths = append(paths, resourcePath)
	}
	sort.Strings(paths)
	result := make([]*entity.SkillResource, 0, len(paths))
	for _, resourcePath := range paths {
		result = append(result, items[resourcePath])
	}
	return result, nil
}

func resourcePathsUnder(items map[string]*entity.SkillResource, source string) []string {
	prefix := strings.TrimSuffix(source, "/") + "/"
	paths := make([]string, 0)
	for resourcePath := range items {
		if resourcePath == source || strings.HasPrefix(resourcePath, prefix) {
			paths = append(paths, resourcePath)
		}
	}
	sort.Strings(paths)
	return paths
}

func resourcesByPath(resources []*entity.SkillResource) map[string]*entity.SkillResource {
	result := make(map[string]*entity.SkillResource, len(resources))
	for _, resource := range resources {
		if resource != nil {
			result[resource.Path] = resource
		}
	}
	return result
}

func cloneResource(skillID int64, resource *entity.SkillResource) *entity.SkillResource {
	if resource == nil {
		return &entity.SkillResource{SkillID: skillID}
	}
	return &entity.SkillResource{SkillID: skillID, Path: resource.Path, Content: append([]byte(nil), resource.Content...), Size: resource.Size, SHA256: resource.SHA256}
}
