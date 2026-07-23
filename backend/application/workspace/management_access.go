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
	"context"
	"fmt"
)

func (s *ApplicationService) CanManageWorkspace(
	ctx context.Context,
	req *CheckWorkspaceMembershipRequest,
) (bool, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return false, fmt.Errorf("invalid workspace management request")
	}
	resp, err := s.GetWorkspaceDetail(ctx, &GetWorkspaceDetailRequest{
		SpaceID:       req.SpaceID,
		CurrentUserID: req.CurrentUserID,
	})
	if err != nil {
		return false, err
	}
	if resp == nil || resp.Data == nil {
		return false, fmt.Errorf("workspace access is unavailable")
	}
	return resp.Data.CurrentUserRole == workspaceRoleOwner || resp.Data.CurrentUserRole == workspaceRoleAdmin, nil
}
