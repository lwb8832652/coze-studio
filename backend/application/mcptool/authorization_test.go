// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"testing"

	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

type stubUserSpaceRoleReader struct {
	userID int64
	spaces []*userentity.Space
	members []*userentity.SpaceMember
	err    error
}

func (s *stubUserSpaceRoleReader) GetUserSpaceList(
	_ context.Context,
	userID int64,
) ([]*userentity.Space, error) {
	s.userID = userID
	return s.spaces, s.err
}

func (s *stubUserSpaceRoleReader) GetSpaceMembers(
	_ context.Context,
	_ int64,
) ([]*userentity.SpaceMember, error) {
	return s.members, s.err
}

func TestSpaceAuthorizerRequiresAuthenticatedUser(t *testing.T) {
	authorizer := newSpaceAuthorizer(&stubUserSpaceRoleReader{}, func(context.Context) (int64, bool) {
		return 0, false
	})

	err := authorizer.Authorize(context.Background(), 10, MCPAccessRead)
	if !errors.Is(err, ErrMCPUnauthenticated) {
		t.Fatalf("expected unauthenticated error, got %v", err)
	}
}

func TestSpaceAuthorizerAllowsMembersToRead(t *testing.T) {
	reader := &stubUserSpaceRoleReader{
		spaces: []*userentity.Space{{ID: 10, RoleType: 3}},
	}
	authorizer := newSpaceAuthorizer(reader, fixedUserID(7))

	if err := authorizer.Authorize(context.Background(), 10, MCPAccessRead); err != nil {
		t.Fatalf("authorize member read: %v", err)
	}
	if reader.userID != 7 {
		t.Fatalf("expected authenticated user 7, got %d", reader.userID)
	}
}

func TestSpaceAuthorizerRestrictsWritesToOwnerAndAdmin(t *testing.T) {
	tests := []struct {
		name      string
		roleType  int32
		wantError bool
	}{
		{name: "owner", roleType: 1},
		{name: "admin", roleType: 2},
		{name: "member", roleType: 3, wantError: true},
		{name: "unknown", roleType: 0, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := newSpaceAuthorizer(
				&stubUserSpaceRoleReader{spaces: []*userentity.Space{{ID: 10, RoleType: tt.roleType}}},
				fixedUserID(7),
			)
			err := authorizer.Authorize(context.Background(), 10, MCPAccessManage)
			if tt.wantError && !errors.Is(err, ErrMCPForbidden) {
				t.Fatalf("expected forbidden error, got %v", err)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("authorize write: %v", err)
			}
		})
	}
}

func TestSpaceAuthorizerRejectsNonMemberAndDependencyFailure(t *testing.T) {
	authorizer := newSpaceAuthorizer(&stubUserSpaceRoleReader{}, fixedUserID(7))
	if err := authorizer.Authorize(context.Background(), 10, MCPAccessRead); !errors.Is(err, ErrMCPForbidden) {
		t.Fatalf("expected non-member to be forbidden, got %v", err)
	}

	dependencyErr := errors.New("database unavailable")
	authorizer = newSpaceAuthorizer(&stubUserSpaceRoleReader{err: dependencyErr}, fixedUserID(7))
	if err := authorizer.Authorize(context.Background(), 10, MCPAccessRead); !errors.Is(err, dependencyErr) {
		t.Fatalf("expected dependency error, got %v", err)
	}
}

func fixedUserID(userID int64) currentUserIDResolver {
	return func(context.Context) (int64, bool) {
		return userID, userID > 0
	}
}
