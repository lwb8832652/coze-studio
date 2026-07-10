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

package workspace

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAppDevAccess(t *testing.T) {
	t.Run("development disabled", func(t *testing.T) {
		err := validateAppDevAccess(&WorkspaceDetail{
			AllowDevelop:    false,
			CurrentUserRole: workspaceRoleOwner,
		}, false)
		require.ErrorContains(t, err, "workspace does not allow development")
	})

	t.Run("member can develop", func(t *testing.T) {
		err := validateAppDevAccess(&WorkspaceDetail{
			AllowDevelop:    true,
			CurrentUserRole: workspaceRoleMember,
		}, false)
		require.NoError(t, err)
	})

	t.Run("member cannot manage runtime", func(t *testing.T) {
		err := validateAppDevAccess(&WorkspaceDetail{
			AllowDevelop:    true,
			CurrentUserRole: workspaceRoleMember,
		}, true)
		require.ErrorContains(t, err, "workspace owner or admin role is required")
	})

	t.Run("admin can manage runtime", func(t *testing.T) {
		err := validateAppDevAccess(&WorkspaceDetail{
			AllowDevelop:    true,
			CurrentUserRole: workspaceRoleAdmin,
		}, true)
		require.NoError(t, err)
	})
}
