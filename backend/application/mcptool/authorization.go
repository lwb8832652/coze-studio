// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

var (
	ErrMCPUnauthenticated          = errors.New("MCP request requires authentication")
	ErrMCPForbidden                = errors.New("MCP space access is forbidden")
	ErrMCPAuthorizationUnavailable = errors.New("MCP authorization is unavailable")
)

type MCPAccess int

const (
	MCPAccessRead MCPAccess = iota
	MCPAccessManage
)

const (
	spaceRoleOwner int32 = 1
	spaceRoleAdmin int32 = 2
)

type UserSpaceRoleReader interface {
	GetUserSpaceList(ctx context.Context, userID int64) ([]*userentity.Space, error)
}

type currentUserIDResolver func(ctx context.Context) (int64, bool)

type spaceAuthorizer struct {
	reader        UserSpaceRoleReader
	currentUserID currentUserIDResolver
}

func newSpaceAuthorizer(reader UserSpaceRoleReader, resolver currentUserIDResolver) *spaceAuthorizer {
	if resolver == nil {
		resolver = authenticatedUserID
	}
	return &spaceAuthorizer{reader: reader, currentUserID: resolver}
}

func authenticatedUserID(ctx context.Context) (int64, bool) {
	userID := ctxutil.GetUIDFromCtx(ctx)
	if userID == nil || *userID <= 0 {
		return 0, false
	}
	return *userID, true
}

func (a *spaceAuthorizer) Authorize(ctx context.Context, spaceID int64, access MCPAccess) error {
	if a == nil || a.reader == nil || a.currentUserID == nil {
		return ErrMCPAuthorizationUnavailable
	}
	userID, ok := a.currentUserID(ctx)
	if !ok || userID <= 0 {
		return ErrMCPUnauthenticated
	}
	if spaceID <= 0 {
		return ErrMCPForbidden
	}

	spaces, err := a.reader.GetUserSpaceList(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve MCP space role: %w", err)
	}
	for _, space := range spaces {
		if space == nil || space.ID != spaceID {
			continue
		}
		if access == MCPAccessRead {
			return nil
		}
		if space.RoleType == spaceRoleOwner || space.RoleType == spaceRoleAdmin {
			return nil
		}
		return ErrMCPForbidden
	}

	return ErrMCPForbidden
}
