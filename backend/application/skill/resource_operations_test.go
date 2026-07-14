/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package skill

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type denyingUserSpaceReader struct{}

func (denyingUserSpaceReader) GetUserSpaceList(context.Context, int64) ([]*userentity.Space, error) {
	return nil, nil
}

type recordingUserSpaceReader struct {
	userID int64
}

func (r *recordingUserSpaceReader) GetUserSpaceList(
	_ context.Context,
	userID int64,
) ([]*userentity.Space, error) {
	r.userID = userID
	return []*userentity.Space{{ID: 101, RoleType: 3, AllowDevelop: true}}, nil
}

func TestAuthorizeSpaceKeepsUnitServicesIsolatedWithoutReader(t *testing.T) {
	service := &ApplicationService{}
	require.NoError(t, service.authorizeSpace(context.Background(), 1))
}

func TestAuthorizeSpaceFailsClosedWithoutAuthenticatedUser(t *testing.T) {
	service := &ApplicationService{UserSpaceReader: denyingUserSpaceReader{}}
	require.ErrorIs(t, service.authorizeSpace(context.Background(), 1), ErrSkillAccessDenied)
}

func TestAuthorizeSpaceUsesSpaceThenUserContractOrder(t *testing.T) {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userentity.Session{UserID: 9})
	reader := &recordingUserSpaceReader{}
	service := &ApplicationService{UserSpaceReader: reader}

	require.NoError(t, service.authorizeSpace(ctx, 101))
	require.Equal(t, int64(9), reader.userID)
}

func TestAuthorizeSpaceWriteUsesRealDevelopmentPermission(t *testing.T) {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userentity.Session{UserID: 9})
	service := &ApplicationService{UserSpaceReader: &recordingUserSpaceReader{}}
	require.NoError(t, service.authorizeSpaceWrite(ctx, 101))

	service.UserSpaceReader = userSpaceReaderFunc(func(context.Context, int64) ([]*userentity.Space, error) {
		return []*userentity.Space{{ID: 101, RoleType: 3, AllowDevelop: false}}, nil
	})
	require.ErrorIs(t, service.authorizeSpaceWrite(ctx, 101), ErrSkillAccessDenied)
	require.NoError(t, service.authorizeSpace(ctx, 101), "read access remains available to a legal member")
}

type userSpaceReaderFunc func(context.Context, int64) ([]*userentity.Space, error)

func (f userSpaceReaderFunc) GetUserSpaceList(ctx context.Context, userID int64) ([]*userentity.Space, error) {
	return f(ctx, userID)
}

func TestManagedResourcePathRejectsTraversalAndDeclaration(t *testing.T) {
	_, err := managedResourcePath("../secret.txt")
	require.Error(t, err)

	_, err = managedResourcePath("SKILL.md")
	require.Error(t, err)

	path, err := managedResourcePath("references\\guide.md")
	require.NoError(t, err)
	require.Equal(t, "references/guide.md", path)
}

func TestMatchingResourcesIncludesDirectoryChildrenInStableOrder(t *testing.T) {
	resources := []*entity.SkillResource{
		{Path: "references/z.md"},
		{Path: "scripts/run.py"},
		{Path: "references/a.md"},
	}

	matches := matchingResources(resources, "references")
	require.Equal(t, []string{"references/a.md", "references/z.md"}, []string{
		matches[0].Path,
		matches[1].Path,
	})
}
