# Coze Nuwax Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first usable loop for Coze Studio workspace switching, workspace member/settings management, and admin-only system management based on the confirmed Coze + `nuwax-ai` direction.

**Architecture:** Keep Coze's static React Router and Coze Design UI. Add backend-backed permission facts first, then wire workspace switching, workspace management pages, and the independent `/system` admin area. Do not introduce a full dynamic menu/RBAC platform in phase one.

**Tech Stack:** React, TypeScript, React Router, Zustand, Coze Design, Go, Hertz, GORM, Thrift/IDL-generated API models.

---

## Scope Guardrails

- Do not merge or push any branch as part of this feature plan.
- Do not restore the removed `工具` workspace menu item.
- Do not rename `工作空间` back to `成员与设置`.
- Do not add IM channel settings or workers.
- Do not implement full `nuwax-ai` RBAC/menu-permission/payment/subscription modules.
- Do not rely on frontend-only role checks for admin APIs or workspace member writes.

## File Structure Map

Backend contract and permissions:

- Modify: `idl/passport/passport.thrift`
- Modify: `backend/application/user/user.go`
- Modify: `backend/api/middleware/session.go`
- Create: `backend/application/user/admin_auth.go`
- Modify: `frontend/packages/foundation/account-base/src/types/index.ts`
- Modify: `frontend/packages/foundation/account-adapter/src/passport-api/index.ts`
- Test: `backend/application/user/user_test.go`
- Test: `backend/api/middleware/session_test.go`
- Test: `frontend/packages/foundation/account-adapter/src/passport-api/__tests__/index.test.ts`

Workspace role and store:

- Modify: `idl/playground/playground.thrift`
- Modify: `backend/application/user/user.go`
- Modify: `backend/domain/user/entity/space.go`
- Modify: `backend/domain/user/service/user.go`
- Modify: `backend/domain/user/service/user_impl.go`
- Modify: `backend/domain/user/repository/repository.go`
- Modify: `backend/domain/user/internal/dal/space.go`
- Modify: `frontend/packages/foundation/space-store-adapter/src/space/index.ts`
- Modify: `frontend/packages/common/auth-adapter/src/space/use-init-space-role.ts`
- Test: `frontend/packages/common/auth-adapter/__tests__/space/use-init-space-role.test.ts`
- Test: `frontend/packages/foundation/space-store-adapter/__tests__/space.test.ts`

Workspace switcher:

- Create: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-switcher.tsx`
- Create: `frontend/apps/coze-studio/src/components/workspace-sub-menu/create-team-space-modal.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`

Workspace management page:

- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/workspace-members-tab.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/workspace-settings-tab.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/workspace/__tests__/workspace-page.test.tsx`

Workspace member backend:

- Create: `idl/workspace/workspace.thrift`
- Modify: `idl/api.thrift`
- Create: `backend/application/workspace/workspace.go`
- Create: `backend/api/handler/coze/workspace_service.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/middleware.go`
- Modify: `backend/domain/user/repository/repository.go`
- Modify: `backend/domain/user/internal/dal/space.go`
- Test: `backend/application/workspace/workspace_test.go`
- Test: `backend/api/handler/coze/workspace_service_test.go`

System management:

- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/layout.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/system-sub-menu.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/workspaces.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/users.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/config.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/system-layout.test.tsx`
- Create: `idl/admin/management.thrift`
- Modify: `idl/api.thrift`
- Create: `backend/application/admin/management.go`
- Create: `backend/api/handler/coze/admin_management_service.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/middleware.go`
- Test: `backend/application/admin/management_test.go`
- Test: `backend/api/handler/coze/admin_management_service_test.go`

---

### Task 1: Add backend-backed system-admin status

**Files:**
- Modify: `idl/passport/passport.thrift`
- Create: `backend/application/user/admin_auth.go`
- Modify: `backend/application/user/user.go`
- Modify: `backend/api/middleware/session.go`
- Modify: `frontend/packages/foundation/account-base/src/types/index.ts`
- Modify: `frontend/packages/foundation/account-adapter/src/passport-api/index.ts`
- Test: `backend/api/middleware/session_test.go`
- Test: `backend/application/user/user_test.go`
- Test: `frontend/packages/foundation/account-adapter/src/passport-api/__tests__/index.test.ts`

- [ ] **Step 1: Write backend admin helper tests**

Create `backend/application/user/admin_auth.go` tests in `backend/application/user/user_test.go`:

```go
func TestIsSystemAdminEmail(t *testing.T) {
	t.Parallel()

	require.True(t, IsSystemAdminEmail("owner@example.com", "owner@example.com"))
	require.True(t, IsSystemAdminEmail("OWNER@example.com", "owner@example.com"))
	require.True(t, IsSystemAdminEmail("owner@example.com", "admin@example.com, owner@example.com"))
	require.False(t, IsSystemAdminEmail("member@example.com", "owner@example.com"))
	require.False(t, IsSystemAdminEmail("member@example.com", ""))
}
```

- [ ] **Step 2: Implement shared admin email helper**

Create `backend/application/user/admin_auth.go`:

```go
package user

import "strings"

func IsSystemAdminEmail(email string, adminEmails string) bool {
	if strings.TrimSpace(email) == "" || strings.TrimSpace(adminEmails) == "" {
		return false
	}

	for _, adminEmail := range strings.Split(adminEmails, ",") {
		if strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(adminEmail)) {
			return true
		}
	}

	return false
}
```

- [ ] **Step 3: Update admin middleware to use helper**

Modify `backend/api/middleware/session.go` inside `AdminAuthMW`:

```go
if user.IsSystemAdminEmail(session.UserEmail, baseConf.AdminEmails) {
	ctx.Next(c)
	return
}

httputil.Unauthorized(ctx, "the account does not have permission to access")
```

Keep existing fallback that fills `baseConf.AdminEmails` from env when empty.

- [ ] **Step 4: Add admin flag to user response contract**

Modify `idl/passport/passport.thrift` `struct User` by adding:

```thrift
    40: optional bool is_system_admin
```

Then regenerate backend and frontend API models using the repository's existing IDL generation command. If generation is not available in the current environment, manually update only the local generated model files needed by compile and record the blocker in the implementation notes.

- [ ] **Step 5: Populate admin flag in account info**

Modify `backend/application/user/user.go` in `PassportAccountInfoV2`:

```go
baseConf, err := config.Base().GetBaseConfig(ctx)
if err != nil {
	return nil, err
}

passportUser := userDo2PassportTo(userInfo)
passportUser.IsSystemAdmin = ptr.Of(IsSystemAdminEmail(userInfo.Email, baseConf.AdminEmails))

return &passport.PassportAccountInfoV2Response{
	Data: passportUser,
	Code: 0,
}, nil
```

If `config` is not yet imported in `backend/application/user/user.go`, add:

```go
"github.com/coze-dev/coze-studio/backend/bizpkg/config"
```

- [ ] **Step 6: Update frontend user type**

Modify `frontend/packages/foundation/account-base/src/types/index.ts`:

```ts
  is_system_admin?: boolean;
```

Place it near other account identity fields.

- [ ] **Step 7: Update frontend account adapter test**

Modify `frontend/packages/foundation/account-adapter/src/passport-api/__tests__/index.test.ts` to assert the field is preserved:

```ts
const mockUserInfo = { name: 'test', is_system_admin: true };
vi.mocked(passport.PassportAccountInfoV2).mockResolvedValueOnce({
  data: mockUserInfo,
} as never);

const result = await passportApi.checkLogin();
expect(result).toEqual(mockUserInfo);
```

- [ ] **Step 8: Verify targeted tests**

Run:

```bash
cd backend
go test ./application/user ./api/middleware -run 'TestIsSystemAdminEmail|TestAdmin' -count=1
```

Expected: PASS.

Run:

```bash
cd frontend/packages/foundation/account-adapter
npm run test -- src/passport-api/__tests__/index.test.ts
```

Expected: PASS.

---

### Task 2: Return and consume real workspace roles

**Files:**
- Modify: `backend/domain/user/entity/space.go`
- Modify: `backend/domain/user/service/user.go`
- Modify: `backend/domain/user/service/user_impl.go`
- Modify: `backend/domain/user/repository/repository.go`
- Modify: `backend/domain/user/internal/dal/space.go`
- Modify: `backend/application/user/user.go`
- Modify: `frontend/packages/foundation/space-store-adapter/src/space/index.ts`
- Modify: `frontend/packages/common/auth-adapter/src/space/use-init-space-role.ts`
- Test: `frontend/packages/common/auth-adapter/__tests__/space/use-init-space-role.test.ts`
- Test: `frontend/packages/foundation/space-store-adapter/__tests__/space.test.ts`

- [ ] **Step 1: Extend domain space entity with role metadata**

Modify `backend/domain/user/entity/space.go`:

```go
type Space struct {
	ID          int64
	Name        string
	Description string
	IconURL     string
	SpaceType   SpaceType
	OwnerID     int64
	CreatorID   int64
	RoleType    int32
	MemberCount int64
	CreatedAt   int64
	UpdatedAt   int64
}
```

- [ ] **Step 2: Add repository methods for member role and count**

Modify `backend/domain/user/repository/repository.go` `SpaceRepository`:

```go
	GetSpaceUsersBySpaceID(ctx context.Context, spaceID int64) ([]*model.SpaceUser, error)
	CountSpaceUsers(ctx context.Context, spaceIDs []int64) (map[int64]int64, error)
```

- [ ] **Step 3: Implement DAL member queries**

Modify `backend/domain/user/internal/dal/space.go`:

```go
func (dao *SpaceDAO) GetSpaceUsersBySpaceID(ctx context.Context, spaceID int64) ([]*model.SpaceUser, error) {
	return dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.Eq(spaceID),
	).Find()
}

func (dao *SpaceDAO) CountSpaceUsers(ctx context.Context, spaceIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(spaceIDs))
	if len(spaceIDs) == 0 {
		return result, nil
	}

	rows, err := dao.query.SpaceUser.WithContext(ctx).
		Select(dao.query.SpaceUser.SpaceID, dao.query.SpaceUser.ID.Count()).
		Where(dao.query.SpaceUser.SpaceID.In(spaceIDs...)).
		Group(dao.query.SpaceUser.SpaceID).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var spaceID int64
		var count int64
		if err := rows.Scan(&spaceID, &count); err != nil {
			return nil, err
		}
		result[spaceID] = count
	}

	return result, rows.Err()
}
```

- [ ] **Step 4: Preserve role_type in user space list**

Modify `backend/domain/user/service/user_impl.go` in `GetUserSpaceList`:

```go
roleBySpaceID := slices.ToMap(userSpaces, func(us *model.SpaceUser) (int64, int32) {
	return us.SpaceID, us.RoleType
})

memberCounts, err := u.SpaceRepo.CountSpaceUsers(ctx, spaceIDs)
if err != nil {
	return nil, err
}
```

Then set fields in `spacePo2Do` result:

```go
space := spacePo2Do(sm, urls[sm.IconURI])
space.RoleType = roleBySpaceID[sm.ID]
space.MemberCount = memberCounts[sm.ID]
return space
```

- [ ] **Step 5: Return roles in GetSpaceListV2**

Modify `backend/application/user/user.go`:

```go
return &playground.BotSpaceV2{
	ID:             space.ID,
	Name:           space.Name,
	Description:    space.Description,
	SpaceType:      playground.SpaceType(space.SpaceType),
	IconURL:        space.IconURL,
	RoleType:       space.RoleType,
	SpaceRoleType:  playground.SpaceRoleType(space.RoleType),
	OwnerUserID:    ptr.Of(space.OwnerID),
	TotalMemberNum: ptr.Of(space.MemberCount),
}
```

- [ ] **Step 6: Update frontend space role initialization**

Modify `frontend/packages/common/auth-adapter/src/space/use-init-space-role.ts` so it reads the current space from `useSpaceStore`:

```ts
const currentSpace = useSpaceStore(state =>
  state.space.id === spaceId ? state.space : undefined,
);

useEffect(() => {
  const role =
    currentSpace?.space_role_type ??
    currentSpace?.role_type ??
    SpaceRoleType.Default;
  setRoles(spaceId, [role]);
  setIsReady(spaceId, true);
}, [currentSpace?.role_type, currentSpace?.space_role_type, spaceId]);
```

Add import:

```ts
import { useSpaceStore } from '@coze-foundation/space-store';
```

- [ ] **Step 7: Update auth-adapter tests**

Modify `frontend/packages/common/auth-adapter/__tests__/space/use-init-space-role.test.ts` mock so `useSpaceStore` returns role metadata:

```ts
vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (selector: any) =>
    selector({
      space: {
        id: 'space-1',
        role_type: SpaceRoleType.Admin,
        space_role_type: SpaceRoleType.Admin,
      },
    }),
}));
```

Assert:

```ts
expect(mockSetRoles).toHaveBeenCalledWith('space-1', [SpaceRoleType.Admin]);
```

- [ ] **Step 8: Verify targeted tests**

Run:

```bash
cd frontend/packages/common/auth-adapter
npm run test -- __tests__/space/use-init-space-role.test.ts
```

Expected: PASS.

Run:

```bash
cd backend
go test ./application/user ./domain/user/service -run Test -count=1
```

Expected: PASS.

---

### Task 3: Add workspace menu entry and WorkspaceSwitcher

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Create: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-switcher.tsx`
- Create: `frontend/apps/coze-studio/src/components/workspace-sub-menu/create-team-space-modal.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`

- [ ] **Step 1: Add workspace menu key**

Modify `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`:

```ts
export const SPACE_SUB_MODULE = {
  WORKBENCH: 'chats/new',
  LIBRARY: 'library',
  SKILL: 'skill',
  DEVELOP: 'develop',
  WORKSPACE: 'workspace',
  TASKS: 'chats',
} as const;
```

Add menu item immediately before `全部任务`:

```ts
{
  label: '工作空间',
  path: SPACE_SUB_MODULE.WORKSPACE,
  dataTestId: 'navigation_workspace_settings',
},
```

- [ ] **Step 2: Create create-team modal**

Create `frontend/apps/coze-studio/src/components/workspace-sub-menu/create-team-space-modal.tsx`:

```tsx
import { useState } from 'react';

import { Modal, Form, Input, Toast } from '@coze-arch/coze-design';
import { useSpaceStore } from '@coze-foundation/space-store';
import { SpaceType } from '@coze-arch/bot-api/developer_api';

interface CreateTeamSpaceModalProps {
  visible: boolean;
  onCancel: () => void;
  onCreated: (spaceId: string) => void;
}

export const CreateTeamSpaceModal = ({
  visible,
  onCancel,
  onCreated,
}: CreateTeamSpaceModalProps) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const createSpace = useSpaceStore(state => state.createSpace);
  const fetchSpaces = useSpaceStore(state => state.fetchSpaces);

  const handleOk = async () => {
    const normalizedName = name.trim();
    if (!normalizedName) {
      Toast.warning({ content: '请输入团队空间名称' });
      return;
    }

    try {
      setSubmitting(true);
      const result = await createSpace({
        name: normalizedName,
        description: description.trim(),
        icon_uri: '',
        space_type: SpaceType.Team,
      });
      await fetchSpaces(true);
      setName('');
      setDescription('');
      onCreated(String(result?.id ?? ''));
    } catch (error) {
      Toast.error({ content: '创建团队空间失败' });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title="创建团队空间"
      visible={visible}
      okText="创建"
      cancelText="取消"
      confirmLoading={submitting}
      onOk={handleOk}
      onCancel={onCancel}
    >
      <Form>
        <Form.Slot label="空间名称" required>
          <Input
            value={name}
            maxLength={40}
            placeholder="请输入团队空间名称"
            onChange={setName}
          />
        </Form.Slot>
        <Form.Slot label="空间描述">
          <Input.TextArea
            value={description}
            maxLength={200}
            placeholder="请输入空间描述"
            onChange={setDescription}
          />
        </Form.Slot>
      </Form>
    </Modal>
  );
};
```

- [ ] **Step 3: Create WorkspaceSwitcher**

Create `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-switcher.tsx`:

```tsx
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Dropdown, Typography } from '@coze-arch/coze-design';
import {
  IconCozArrowDown,
  IconCozCheckMark,
  IconCozPlus,
  IconCozSetting,
} from '@coze-arch/coze-design/icons';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import { useSpaceStore } from '@coze-foundation/space-store';

import { CreateTeamSpaceModal } from './create-team-space-modal';

export const WorkspaceSwitcher = () => {
  const navigate = useNavigate();
  const { space_id: routeSpaceId } = useParams();
  const userInfo = useUserInfo();
  const [createVisible, setCreateVisible] = useState(false);

  const currentSpace = useSpaceStore(state => state.space);
  const spaceList = useSpaceStore(state => state.spaceList);

  const switchSpace = (spaceId: string) => {
    if (!spaceId || spaceId === routeSpaceId) {
      return;
    }
    localStorage.setItem('workspace-spaceId', spaceId);
    navigate(`/space/${spaceId}/chats/new`);
  };

  const menus = [
    ...spaceList.map(space => ({
      title: (
        <span className="flex items-center gap-[8px]">
          {space.id === currentSpace.id ? <IconCozCheckMark /> : null}
          <span>{space.name}</span>
        </span>
      ),
      onClick: () => switchSpace(String(space.id)),
    })),
    <Dropdown.Divider />,
    {
      prefixIcon: <IconCozPlus />,
      title: '创建团队空间',
      onClick: () => setCreateVisible(true),
    },
    ...(userInfo?.is_system_admin
      ? [
          <Dropdown.Divider key="system-divider" />,
          {
            prefixIcon: <IconCozSetting />,
            title: '系统管理',
            onClick: () => navigate('/system'),
          },
        ]
      : []),
  ];

  return (
    <>
      <Dropdown trigger="click" render={menus}>
        <button
          type="button"
          className="coze-prototype-sidebar-header"
          data-testid="workspace-switcher"
        >
          <Typography.Text
            ellipsis={{ showTooltip: true, rows: 1 }}
            className="coze-prototype-workspace-title"
          >
            {currentSpace?.name || '工作空间'}
          </Typography.Text>
          <IconCozArrowDown className="text-[14px]" />
        </button>
      </Dropdown>
      <CreateTeamSpaceModal
        visible={createVisible}
        onCancel={() => setCreateVisible(false)}
        onCreated={spaceId => {
          setCreateVisible(false);
          if (spaceId) {
            switchSpace(spaceId);
          }
        }}
      />
    </>
  );
};
```

- [ ] **Step 4: Replace static header in workspace submenu**

Modify `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`:

```tsx
import { WorkspaceSwitcher } from './workspace-switcher';
```

Replace the static title block in `headerNode` with:

```tsx
<WorkspaceSwitcher />
```

Keep assistant card below it.

- [ ] **Step 5: Add icon for workspace menu**

In `MENU_ICONS`, add:

```tsx
[SPACE_SUB_MODULE.WORKSPACE]: {
  icon: <IconCozSetting />,
  activeIcon: <IconCozSettingFill />,
},
```

- [ ] **Step 6: Update workspace submenu test**

Modify `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`:

```ts
expect(menuLabels).toEqual([
  '新建任务',
  '资源配置',
  '技能配置',
  '开发配置',
  '工作空间',
  '全部任务',
]);
```

Assert the old `工具` label is absent:

```ts
expect(markup).not.toContain('工具');
```

- [ ] **Step 7: Verify targeted frontend test**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: PASS.

---

### Task 4: Add workspace member/settings backend APIs

**Files:**
- Create: `idl/workspace/workspace.thrift`
- Modify: `idl/api.thrift`
- Create: `backend/application/workspace/workspace.go`
- Create: `backend/api/handler/coze/workspace_service.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/middleware.go`
- Modify: `backend/domain/user/repository/repository.go`
- Modify: `backend/domain/user/internal/dal/space.go`
- Test: `backend/application/workspace/workspace_test.go`
- Test: `backend/api/handler/coze/workspace_service_test.go`

- [ ] **Step 1: Define workspace API IDL**

Create `idl/workspace/workspace.thrift`:

```thrift
namespace go workspace

include "../base.thrift"

struct WorkspaceDetail {
    1: required i64 id (api.js_conv="true")
    2: required string name
    3: optional string description
    4: optional string icon_url
    5: required i64 owner_user_id (api.js_conv="true")
    6: required i32 current_user_role
    7: optional i64 total_member_num
}

struct WorkspaceMember {
    1: required i64 user_id (api.js_conv="true")
    2: required string name
    3: optional string user_unique_name
    4: optional string email
    5: optional string avatar_url
    6: required i32 role_type
    7: required i64 joined_at
}

struct GetWorkspaceDetailRequest {
    1: required i64 space_id (api.js_conv="true", api.query="space_id")
}

struct GetWorkspaceDetailResponse {
    1: required WorkspaceDetail data
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp
}

struct ListWorkspaceMembersRequest {
    1: required i64 space_id (api.js_conv="true")
    2: optional string keyword
    3: optional i32 role_type
}

struct ListWorkspaceMembersResponse {
    1: required list<WorkspaceMember> members
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp
}

struct AddWorkspaceMemberRequest {
    1: required i64 space_id (api.js_conv="true")
    2: required i64 user_id (api.js_conv="true")
    3: required i32 role_type
}

struct UpdateWorkspaceMemberRoleRequest {
    1: required i64 space_id (api.js_conv="true")
    2: required i64 user_id (api.js_conv="true")
    3: required i32 role_type
}

struct RemoveWorkspaceMemberRequest {
    1: required i64 space_id (api.js_conv="true")
    2: required i64 user_id (api.js_conv="true")
}

struct UpdateWorkspaceRequest {
    1: required i64 space_id (api.js_conv="true")
    2: optional string name
    3: optional string description
}

struct SearchWorkspaceUsersRequest {
    1: required string keyword
}

struct SearchWorkspaceUsersResponse {
    1: required list<WorkspaceMember> users
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp
}

struct WorkspaceBaseResponse {
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp
}

service WorkspaceService {
    GetWorkspaceDetailResponse GetWorkspaceDetail(1:GetWorkspaceDetailRequest req)(api.get="/api/workspace/space/detail")
    ListWorkspaceMembersResponse ListWorkspaceMembers(1:ListWorkspaceMembersRequest req)(api.post="/api/workspace/space/members")
    WorkspaceBaseResponse AddWorkspaceMember(1:AddWorkspaceMemberRequest req)(api.post="/api/workspace/space/members/add")
    WorkspaceBaseResponse UpdateWorkspaceMemberRole(1:UpdateWorkspaceMemberRoleRequest req)(api.post="/api/workspace/space/members/update_role")
    WorkspaceBaseResponse RemoveWorkspaceMember(1:RemoveWorkspaceMemberRequest req)(api.post="/api/workspace/space/members/remove")
    WorkspaceBaseResponse UpdateWorkspace(1:UpdateWorkspaceRequest req)(api.post="/api/workspace/space/update")
    SearchWorkspaceUsersResponse SearchWorkspaceUsers(1:SearchWorkspaceUsersRequest req)(api.post="/api/workspace/users/search")
}
```

- [ ] **Step 2: Include workspace service in root API IDL**

Modify `idl/api.thrift`:

```thrift
include "./workspace/workspace.thrift"

service WorkspaceService extends workspace.WorkspaceService {}
```

Regenerate API models and routes using the repository's standard generator.

- [ ] **Step 3: Implement application permission helpers**

Create `backend/application/workspace/workspace.go` with role constants:

```go
package workspace

const (
	RoleOwner  int32 = 1
	RoleAdmin  int32 = 2
	RoleMember int32 = 3
)

func canManageMembers(role int32) bool {
	return role == RoleOwner || role == RoleAdmin
}

func canUpdateSpace(role int32) bool {
	return role == RoleOwner || role == RoleAdmin
}
```

Add service methods for detail/list/add/update/remove/search. Each method must read current UID from `ctxutil.MustGetUIDFromCtx(ctx)` and fetch current user's `space_user.role_type` server-side.

- [ ] **Step 4: Enforce member write safety**

In application service, enforce:

```go
if !canManageMembers(currentRole) {
	return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "no workspace member permission"))
}
if req.UserID == currentUID && req.RoleType != RoleOwner {
	return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "owner cannot downgrade self"))
}
if targetRole == RoleOwner && req.UserID != currentUID {
	return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "owner cannot be removed"))
}
```

- [ ] **Step 5: Verify backend tests**

Run:

```bash
cd backend
go test ./application/workspace ./api/handler/coze ./api/router/coze -run Workspace -count=1
```

Expected: PASS.

---

### Task 5: Add `/space/:space_id/workspace` frontend page

**Files:**
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/workspace/workspace-members-tab.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/workspace-settings-tab.tsx`
- Create: `frontend/apps/coze-studio/src/pages/workspace/__tests__/workspace-page.test.tsx`

- [ ] **Step 1: Add lazy route component**

Modify `frontend/apps/coze-studio/src/routes/async-components.tsx`:

```ts
export const WorkspacePage = lazy(() => import('../pages/workspace'));
```

- [ ] **Step 2: Add route under `/space/:space_id`**

Modify `frontend/apps/coze-studio/src/routes/index.tsx` imports and child routes:

```tsx
{
  path: 'workspace',
  Component: WorkspacePage,
  loader: () => ({
    subMenuKey: SPACE_SUB_MODULE.WORKSPACE,
  }),
},
```

Place it before task routes.

- [ ] **Step 3: Create workspace service wrapper**

Create `frontend/apps/coze-studio/src/pages/workspace/service.ts`:

```ts
export interface WorkspaceDetail {
  id: string;
  name: string;
  description?: string;
  icon_url?: string;
  owner_user_id: string;
  current_user_role: number;
  total_member_num?: number;
}

export interface WorkspaceMember {
  user_id: string;
  name: string;
  user_unique_name?: string;
  email?: string;
  avatar_url?: string;
  role_type: number;
  joined_at: number;
}

const requestJSON = async <T>(url: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(url, {
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    ...init,
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
};

export const getWorkspaceDetail = (spaceId: string) =>
  requestJSON<{ data: WorkspaceDetail }>(
    `/api/workspace/space/detail?space_id=${encodeURIComponent(spaceId)}`,
  );

export const listWorkspaceMembers = (params: {
  space_id: string;
  keyword?: string;
  role_type?: number;
}) =>
  requestJSON<{ members: WorkspaceMember[] }>('/api/workspace/space/members', {
    method: 'POST',
    body: JSON.stringify(params),
  });

export const updateWorkspace = (params: {
  space_id: string;
  name?: string;
  description?: string;
}) =>
  requestJSON('/api/workspace/space/update', {
    method: 'POST',
    body: JSON.stringify(params),
  });
```

When generated API client is available, replace this wrapper with generated client calls in the same file and keep the component API stable.

- [ ] **Step 4: Create workspace page shell**

Create `frontend/apps/coze-studio/src/pages/workspace/index.tsx`:

```tsx
import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';

import { Tabs, Spin, Toast } from '@coze-arch/coze-design';

import { getWorkspaceDetail, type WorkspaceDetail } from './service';
import { WorkspaceMembersTab } from './workspace-members-tab';
import { WorkspaceSettingsTab } from './workspace-settings-tab';

const WorkspacePage = () => {
  const { space_id } = useParams();
  const [detail, setDetail] = useState<WorkspaceDetail>();
  const [loading, setLoading] = useState(true);

  const refresh = async () => {
    if (!space_id) {
      return;
    }
    setLoading(true);
    try {
      const response = await getWorkspaceDetail(space_id);
      setDetail(response.data);
    } catch (error) {
      Toast.error({ content: '加载工作空间失败' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refresh();
  }, [space_id]);

  if (loading) {
    return <Spin wrapperClassName="h-full w-full flex items-center justify-center" />;
  }

  if (!space_id || !detail) {
    return <div className="p-[24px]">工作空间不存在或无权限访问</div>;
  }

  return (
    <main className="h-full overflow-auto p-[24px]">
      <h1 className="text-[24px] font-[600] text-[#1d2333]">工作空间</h1>
      <p className="mt-[4px] text-[13px] text-[#6b7280]">
        管理当前工作空间的成员和基础信息
      </p>
      <Tabs
        className="mt-[20px]"
        tabs={[
          {
            itemKey: 'members',
            tab: '成员管理',
            children: <WorkspaceMembersTab spaceId={space_id} detail={detail} />,
          },
          {
            itemKey: 'settings',
            tab: '基础设置',
            children: (
              <WorkspaceSettingsTab
                spaceId={space_id}
                detail={detail}
                onUpdated={refresh}
              />
            ),
          },
        ]}
      />
    </main>
  );
};

export default WorkspacePage;
```

- [ ] **Step 5: Create settings tab**

Create `frontend/apps/coze-studio/src/pages/workspace/workspace-settings-tab.tsx`:

```tsx
import { useState } from 'react';

import { Button, Input, Toast } from '@coze-arch/coze-design';

import { updateWorkspace, type WorkspaceDetail } from './service';

export const WorkspaceSettingsTab = ({
  spaceId,
  detail,
  onUpdated,
}: {
  spaceId: string;
  detail: WorkspaceDetail;
  onUpdated: () => void;
}) => {
  const [name, setName] = useState(detail.name);
  const [description, setDescription] = useState(detail.description ?? '');
  const [saving, setSaving] = useState(false);
  const canEdit = detail.current_user_role === 1 || detail.current_user_role === 2;

  const save = async () => {
    try {
      setSaving(true);
      await updateWorkspace({
        space_id: spaceId,
        name: name.trim(),
        description: description.trim(),
      });
      Toast.success({ content: '工作空间已更新' });
      onUpdated();
    } catch (error) {
      Toast.error({ content: '保存工作空间失败' });
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="max-w-[640px]">
      <label className="block text-[13px] font-[500] text-[#1d2333]">名称</label>
      <Input className="mt-[8px]" value={name} disabled={!canEdit} onChange={setName} />
      <label className="mt-[20px] block text-[13px] font-[500] text-[#1d2333]">
        描述
      </label>
      <Input.TextArea
        className="mt-[8px]"
        value={description}
        disabled={!canEdit}
        onChange={setDescription}
      />
      {canEdit ? (
        <Button className="mt-[20px]" color="brand" loading={saving} onClick={save}>
          保存
        </Button>
      ) : null}
    </section>
  );
};
```

- [ ] **Step 6: Verify page tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/workspace/__tests__/workspace-page.test.tsx
```

Expected: PASS.

---

### Task 6: Add independent system management pages

**Files:**
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/layout.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/system-sub-menu.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/workspaces.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/users.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/config.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/system-layout.test.tsx`

- [ ] **Step 1: Add system lazy components**

Modify `frontend/apps/coze-studio/src/routes/async-components.tsx`:

```ts
export const SystemLayout = lazy(() => import('../pages/system/layout'));
export const SystemWorkspacesPage = lazy(() => import('../pages/system/workspaces'));
export const SystemUsersPage = lazy(() => import('../pages/system/users'));
export const SystemConfigPage = lazy(() => import('../pages/system/config'));
```

- [ ] **Step 2: Add `/system` routes**

Modify `frontend/apps/coze-studio/src/routes/index.tsx`:

```tsx
{
  path: 'system',
  Component: SystemLayout,
  loader: () => ({
    hasSider: false,
    requireAuth: true,
  }),
  children: [
    {
      index: true,
      element: <Navigate to="workspaces" replace />,
    },
    {
      path: 'workspaces',
      Component: SystemWorkspacesPage,
    },
    {
      path: 'users',
      Component: SystemUsersPage,
    },
    {
      path: 'config',
      Component: SystemConfigPage,
    },
  ],
},
```

- [ ] **Step 3: Create system submenu**

Create `frontend/apps/coze-studio/src/pages/system/system-sub-menu.tsx`:

```tsx
import { NavLink } from 'react-router-dom';

const items = [
  { label: '工作空间管理', path: '/system/workspaces' },
  { label: '用户管理', path: '/system/users' },
  { label: '系统配置', path: '/system/config' },
];

export const SystemSubMenu = () => (
  <aside className="h-full w-[240px] border-r border-[#e5e7eb] bg-white p-[16px]">
    <h1 className="mb-[16px] text-[18px] font-[600] text-[#1d2333]">
      系统管理
    </h1>
    <nav className="flex flex-col gap-[4px]">
      {items.map(item => (
        <NavLink
          key={item.path}
          to={item.path}
          className={({ isActive }) =>
            [
              'rounded-[8px] px-[12px] py-[8px] text-[14px]',
              isActive ? 'bg-[#eef2ff] text-[#335cff]' : 'text-[#4b5563]',
            ].join(' ')
          }
        >
          {item.label}
        </NavLink>
      ))}
    </nav>
  </aside>
);
```

- [ ] **Step 4: Create system layout with admin guard**

Create `frontend/apps/coze-studio/src/pages/system/layout.tsx`:

```tsx
import { Outlet } from 'react-router-dom';

import { useUserInfo } from '@coze-arch/foundation-sdk';
import { Empty } from '@coze-arch/coze-design';

import { SystemSubMenu } from './system-sub-menu';

const SystemLayout = () => {
  const userInfo = useUserInfo();

  if (!userInfo?.is_system_admin) {
    return (
      <Empty
        className="h-full justify-center"
        title="无权限访问系统管理"
        description="该页面仅系统管理员可见"
      />
    );
  }

  return (
    <div className="flex h-full w-full bg-[#f7f8fa]">
      <SystemSubMenu />
      <section className="min-w-0 flex-1 overflow-auto">
        <Outlet />
      </section>
    </div>
  );
};

export default SystemLayout;
```

- [ ] **Step 5: Create phase-one pages**

Create `frontend/apps/coze-studio/src/pages/system/workspaces.tsx`:

```tsx
const SystemWorkspacesPage = () => (
  <main className="p-[24px]">
    <h2 className="text-[24px] font-[600] text-[#1d2333]">工作空间管理</h2>
    <p className="mt-[4px] text-[13px] text-[#6b7280]">
      查看所有工作空间、owner、成员数和创建时间。
    </p>
  </main>
);

export default SystemWorkspacesPage;
```

Create `frontend/apps/coze-studio/src/pages/system/users.tsx`:

```tsx
const SystemUsersPage = () => (
  <main className="p-[24px]">
    <h2 className="text-[24px] font-[600] text-[#1d2333]">用户管理</h2>
    <p className="mt-[4px] text-[13px] text-[#6b7280]">
      查看用户基础信息和账号状态。
    </p>
  </main>
);

export default SystemUsersPage;
```

Create `frontend/apps/coze-studio/src/pages/system/config.tsx`:

```tsx
const SystemConfigPage = () => (
  <main className="p-[24px]">
    <h2 className="text-[24px] font-[600] text-[#1d2333]">系统配置</h2>
    <p className="mt-[4px] text-[13px] text-[#6b7280]">
      查看管理员邮箱、注册开关和服务地址等基础配置。
    </p>
  </main>
);

export default SystemConfigPage;
```

- [ ] **Step 6: Verify system layout test**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-layout.test.tsx
```

Expected: PASS.

---

### Task 7: Add system management backend list APIs

**Files:**
- Create: `idl/admin/management.thrift`
- Modify: `idl/api.thrift`
- Create: `backend/application/admin/management.go`
- Create: `backend/api/handler/coze/admin_management_service.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/middleware.go`
- Test: `backend/application/admin/management_test.go`
- Test: `backend/api/handler/coze/admin_management_service_test.go`

- [ ] **Step 1: Define admin management IDL**

Create `idl/admin/management.thrift`:

```thrift
namespace go admin.management

include "../base.thrift"

struct AdminWorkspace {
    1: required i64 id (api.js_conv="true")
    2: required string name
    3: optional string description
    4: required i64 owner_user_id (api.js_conv="true")
    5: optional string owner_name
    6: optional i64 total_member_num
    7: required i64 created_at
}

struct AdminUser {
    1: required i64 user_id (api.js_conv="true")
    2: required string name
    3: optional string email
    4: optional string user_unique_name
    5: optional i64 created_at
}

struct ListAdminWorkspacesRequest {
    1: optional string keyword
    2: optional i32 page
    3: optional i32 size
}

struct ListAdminWorkspacesResponse {
    1: required list<AdminWorkspace> workspaces
    2: required i64 total
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp
}

struct ListAdminUsersRequest {
    1: optional string keyword
    2: optional i32 page
    3: optional i32 size
}

struct ListAdminUsersResponse {
    1: required list<AdminUser> users
    2: required i64 total
    253: required i64 code
    254: required string msg
    255: required base.BaseResp BaseResp
}

service AdminManagementService {
    ListAdminWorkspacesResponse ListAdminWorkspaces(1:ListAdminWorkspacesRequest req)(api.post="/api/admin/workspaces/list", api.category="admin")
    ListAdminUsersResponse ListAdminUsers(1:ListAdminUsersRequest req)(api.post="/api/admin/users/list", api.category="admin")
}
```

- [ ] **Step 2: Include service and regenerate models/routes**

Modify `idl/api.thrift`:

```thrift
include "./admin/management.thrift"

service AdminManagementService extends management.AdminManagementService {}
```

Regenerate API models and routes using the repository's standard generator.

- [ ] **Step 3: Implement read-only application service**

Create `backend/application/admin/management.go` with methods:

```go
package admin

type ManagementApplicationService struct{}

var ManagementApplicationSVC = &ManagementApplicationService{}
```

Implement `ListAdminWorkspaces` and `ListAdminUsers` as read-only queries using existing user/space repositories. Keep deletes, disables, password resets, and config writes out of phase one.

- [ ] **Step 4: Ensure admin middleware wraps `/api/admin/*`**

Confirm generated route group remains under:

```go
_admin := _api.Group("/admin", _adminMw()...)
```

The route must inherit:

```go
func _adminMw() []app.HandlerFunc {
	return []app.HandlerFunc{middleware.AdminAuthMW()}
}
```

- [ ] **Step 5: Verify backend admin tests**

Run:

```bash
cd backend
go test ./application/admin ./api/handler/coze ./api/router/coze -run Admin -count=1
```

Expected: PASS.

---

### Task 8: Wire system pages to read APIs

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/workspaces.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/users.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/config.tsx`
- Test: `frontend/apps/coze-studio/src/pages/system/__tests__/system-layout.test.tsx`

- [ ] **Step 1: Create system service wrapper**

Create `frontend/apps/coze-studio/src/pages/system/service.ts`:

```ts
export interface AdminWorkspace {
  id: string;
  name: string;
  description?: string;
  owner_user_id: string;
  owner_name?: string;
  total_member_num?: number;
  created_at: number;
}

export interface AdminUser {
  user_id: string;
  name: string;
  email?: string;
  user_unique_name?: string;
  created_at?: number;
}

const postJSON = async <T>(url: string, body: unknown): Promise<T> => {
  const response = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
};

export const listAdminWorkspaces = (params: {
  keyword?: string;
  page?: number;
  size?: number;
}) =>
  postJSON<{ workspaces: AdminWorkspace[]; total: number }>(
    '/api/admin/workspaces/list',
    params,
  );

export const listAdminUsers = (params: {
  keyword?: string;
  page?: number;
  size?: number;
}) =>
  postJSON<{ users: AdminUser[]; total: number }>(
    '/api/admin/users/list',
    params,
  );

export const getAdminBasicConfig = async () => {
  const response = await fetch('/api/admin/config/basic/get', {
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<{
    configuration?: {
      admin_emails?: string;
      disable_user_registration?: boolean;
      server_host?: string;
    };
  }>;
};
```

When generated admin client is available, replace `fetch` internals in this file and keep page imports unchanged.

- [ ] **Step 2: Replace placeholder pages with loading/error/list states**

Update `workspaces.tsx`, `users.tsx`, and `config.tsx` to:

- show `Spin` while loading;
- show `Empty` when list is empty;
- show a visible error message on failure;
- render table-like rows with Coze Design primitives.

- [ ] **Step 3: Verify frontend system tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-layout.test.tsx
```

Expected: PASS.

---

### Task 9: Final targeted validation

**Files:**
- No implementation files.

- [ ] **Step 1: Run workspace submenu tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run workspace page tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/workspace/__tests__/workspace-page.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Run system page tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-layout.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Run backend targeted tests**

Run:

```bash
cd backend
go test ./application/user ./application/workspace ./application/admin ./api/handler/coze ./api/router/coze -run 'Test|Workspace|Admin' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run TypeScript check if generated API types changed**

Run:

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
```

Expected: no type errors.

## Self-Review Notes

- Spec coverage: plan covers left workspace switcher, create team space, workspace menu rename/position, workspace page, admin `/system` area, admin visibility, backend role facts, and phase-one API boundaries.
- Intentional deferral: full dynamic RBAC, payment/subscription, IM channels, destructive admin writes, and independent personal-center route are not included.
- Risk: exact IDL generation command is not captured in this plan because this repository's generator command was not re-inspected during plan writing. Implementation worker must use the repository's existing IDL generation workflow before manually editing generated files.
