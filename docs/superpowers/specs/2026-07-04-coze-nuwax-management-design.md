# Coze Studio 个人中心、工作空间和系统管理一期设计

## 状态

- 日期：2026-07-04
- 状态：设计待评审
- 当前目标：在 Coze Studio 基础上参考 `/Users/liuwenbo/code/BuildingAI/nuwax-ai`，完成个人中心、工作空间、系统管理的一期重要功能闭环。

## 背景

Coze Studio 当前工作空间侧边栏已经有工作空间标题区域、账号下拉和静态工作空间菜单，但左上角空间标题尚未承担完整的空间切换和创建能力。`nuwax-ai` 已有空间下拉、团队空间创建、工作空间成员设置和系统管理后台形态，可以作为产品和接口参考。

本设计只面向当前 Coze Studio 管理能力迭代，不引入旧主线项目上下文，避免实现时产生错误联想。

## 参考范围

- Coze 当前路由：`frontend/apps/coze-studio/src/routes/index.tsx`
- Coze 当前工作空间菜单：`frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Coze 当前工作空间侧栏：`frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
- Coze 当前账号下拉：`frontend/packages/foundation/global-adapter/src/components/account-dropdown/index.tsx`
- Coze 当前个人资料面板：`frontend/packages/foundation/account-ui-base/src/components/user-info-panel/index.tsx`
- Coze 当前空间 store：`frontend/packages/foundation/space-store-adapter/src/space/index.ts`
- Coze 当前空间角色初始化：`frontend/packages/common/auth-adapter/src/space/use-init-space-role.ts`
- Coze 后端用户和空间服务：`backend/application/user/user.go`
- Coze 后端空间表：`backend/domain/user/internal/dal/model/space.gen.go`
- Coze 后端空间成员表：`backend/domain/user/internal/dal/model/space_user.gen.go`
- Coze 后端管理员校验：`backend/api/middleware/session.go`
- `nuwax-ai` 路由参考：`/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/routes/index.ts`
- `nuwax-ai` 菜单参考：`/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/services/menuService.ts`
- `nuwax-ai` 工作空间设置参考：`/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/pages/TeamSetting/index.tsx`
- `nuwax-ai` 系统用户管理参考：`/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/pages/UserManage/index.tsx`
- `nuwax-ai` 系统管理接口参考：`/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/services/systemManage.ts`

## 产品原则

- 一级菜单保持不变，不新增系统管理一级入口。
- 左上角承担空间上下文，不承担账号身份能力。
- 账号下拉继续承担个人资料、API 授权、退出登录等账号能力。
- 系统管理是管理员后台域，使用独立 `/system` 页面和二级菜单。
- 工作空间菜单使用 `工作空间`，不是 `成员与设置`。
- `工作空间` 菜单放在 `全部任务` 上方。
- 不恢复已移动到设置下的 `工具` 菜单。
- 一期优先做可用闭环，不一次性搬完整 `nuwax-ai` 后台。

## 一期功能范围

一期包含：

- 左上角空间下拉。
- 个人空间和团队空间切换。
- 创建团队空间。
- 系统管理员可见的系统管理入口。
- 工作空间侧边菜单新增 `工作空间`。
- 工作空间页面包含成员管理和基础设置。
- 系统管理独立页面。
- 系统管理二级菜单至少包含工作空间管理、用户管理、系统配置。
- 系统管理一期完成任意几个重要闭环，优先工作空间管理、用户管理、系统配置。

一期不包含：

- 完整动态菜单权限系统。
- 完整角色组、菜单权限、数据权限后台。
- 支付、收益、订阅、积分管理。
- IM channel 配置。
- 远程分支、测试环境或发布流程策略。
- 仅靠前端本地字段控制管理员权限。

## 导航设计

左上角工作空间下拉：

```text
当前空间名称
当前选中空间
个人空间
团队空间列表
创建团队空间
系统管理（仅系统管理员可见）
```

工作空间侧边菜单：

```text
新建任务
资源配置
技能配置
开发配置
工作空间
全部任务
```

系统管理二级菜单：

```text
系统管理
工作空间管理
用户管理
系统配置
```

## 路由设计

工作空间：

```text
/space/:space_id/workspace
```

系统管理：

```text
/system
/system/workspaces
/system/users
/system/config
```

路由行为：

- `/system` 默认跳转 `/system/workspaces`。
- 非系统管理员访问 `/system/*` 时返回无权限页面或服务端 403。
- 进入 `/space/:space_id/workspace` 前必须已完成真实空间角色初始化。

## 个人中心设计

个人中心一期不强制改成独立页面。保留现有账号下拉结构，继续承载：

- 个人资料。
- API 授权。
- 退出登录。

如后续需要独立个人中心页，可以新增 `/account` 或 `/personal-center`，但不阻塞本期空间和系统管理闭环。

## 工作空间下拉设计

左上角当前工作空间标题改造成 `WorkspaceSwitcher`。

`WorkspaceSwitcher` 负责：

- 展示当前空间名称。
- 展示当前选中标记。
- 切换个人空间。
- 切换团队空间。
- 创建团队空间。
- 为系统管理员展示 `系统管理` 入口。

切换空间后：

- 优先保持当前工作空间子路径。
- 当前子路径不适用于目标空间时，回退到新建任务页。
- 记录最近选择的空间。

创建团队空间后：

- 刷新空间列表。
- 自动切换到新团队空间。
- 跳转到新团队空间默认页。

## 工作空间页面设计

`/space/:space_id/workspace` 页面包含两个一期 tab：

- 成员管理。
- 基础设置。

成员管理功能：

- 成员列表。
- 按关键词搜索成员。
- 搜索用户并添加成员。
- 修改成员角色。
- 移除成员。
- 展示 owner、admin、member。

基础设置功能：

- 查看空间名称、描述、图标。
- owner 或 admin 可编辑允许编辑的基础字段。
- owner 专属危险操作后续再加。

权限建议：

- owner 可添加、移除、修改成员角色、编辑空间基础信息。
- admin 可添加、移除、修改普通成员角色。
- member 只读或隐藏管理操作。
- 个人空间不展示成员管理操作，或只展示当前账号和只读说明。

## 系统管理页面设计

系统管理使用独立布局，不进入普通工作空间菜单。

一期建议闭环：

- 工作空间管理：列表、搜索、查看 owner、成员数、创建时间、空间类型。
- 用户管理：列表、搜索、启用状态查看、基础信息查看。
- 系统配置：读取基础配置，展示管理员邮箱、注册开关、ServerHost 等安全字段。

危险操作建议后置：

- 删除工作空间。
- 转让工作空间 owner。
- 禁用用户。
- 重置用户密码。
- 修改系统配置。

若一期必须包含写操作，必须有服务端权限校验、二次确认、错误态和最小测试覆盖。

## 后端合同设计

系统管理员判断：

- 复用 `AdminAuthMW()` 的管理员邮箱判断逻辑。
- 新增共享 helper，避免中间件和用户信息接口各自实现。
- `PassportAccountInfoV2` 返回 `is_system_admin` 或等价字段。
- 前端只用该字段隐藏入口，后端系统管理 API 仍必须强校验。

空间列表：

- `GetSpaceListV2` 应返回当前用户在每个空间的真实角色。
- 字段优先使用既有 `role_type` 和 `space_role_type`。
- 返回 `owner_user_id`、`owner_name`、`total_member_num` 等可支撑 UI 的字段。

空间角色：

- `useInitSpaceRole` 不再硬编码 owner。
- 前端从空间列表或空间详情中获取当前用户真实角色。
- 权限计算继续复用 `@coze-common/auth` 的 `ESpacePermisson`。

工作空间成员 API：

```text
GET 或 POST /api/workspace/space/detail
GET 或 POST /api/workspace/space/members
POST /api/workspace/space/members/add
POST /api/workspace/space/members/update_role
POST /api/workspace/space/members/remove
POST /api/workspace/space/update
GET 或 POST /api/workspace/users/search
```

系统管理 API：

```text
GET 或 POST /api/admin/workspaces/list
GET 或 POST /api/admin/users/list
GET /api/admin/config/basic/get
```

具体路径可在实现时按 Coze IDL/生成 client 规范调整。原则是优先使用 IDL 和生成 client，不新增分散手写 fetch。

## 数据模型建议

现有表可支撑一期：

- `space`
- `space_user`
- `user`

一期优先补 repository/service 方法，而不是新增表。

需要补充的能力：

- 根据 space_id 查询空间详情。
- 根据 space_id 查询成员列表。
- 根据关键词搜索用户。
- 添加空间成员。
- 更新空间成员角色。
- 移除空间成员。
- 更新空间基础信息。
- 查询所有空间列表。
- 查询所有用户列表。

需要注意：

- 所有写操作必须基于服务端 session user 判断权限。
- 不能信任客户端提交的 `user_id`、`space_id`、owner 字段。
- owner 不能被普通删除。
- 最后一个 owner 不能被移除。
- 个人空间不允许添加团队成员。

## 前端组件建议

新增或调整组件：

- `WorkspaceSwitcher`
- `CreateTeamSpaceModal`
- `WorkspacePage`
- `WorkspaceMembersTab`
- `WorkspaceSettingsTab`
- `SystemLayout`
- `SystemSubMenu`
- `SystemWorkspaceManagementPage`
- `SystemUserManagementPage`
- `SystemConfigPage`

优先使用：

- `@coze-arch/coze-design`
- `@coze-arch/coze-design/icons`
- 现有路由、store、toast、empty、loading、error 风格

不建议：

- 直接搬 Ant Design Pro Table。
- 直接引入 `nuwax-ai` 的动态菜单模型。
- 把系统管理塞进当前 workspace 页面。

## 状态和错误体验

必须覆盖：

- 空间列表 loading。
- 空间列表为空。
- 创建团队空间失败。
- 切换空间失败。
- 成员列表 loading。
- 成员列表为空。
- 搜索用户为空。
- 添加重复成员。
- 权限不足。
- 系统管理员入口不可见。
- 直接访问 `/system/*` 被拒绝。
- 系统配置读取失败。

## 验证建议

前端：

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
npm run test -- src/pages/workspace/__tests__/workspace-page.test.tsx
npm run test -- src/pages/system/__tests__/system-layout.test.tsx
```

后端：

```bash
cd backend
go test ./application/user ./api/handler/coze ./api/router/coze -run Test -count=1
```

如果改 IDL 或生成 client：

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
```

## 实施顺序建议

第一阶段：底座合同。

- 补系统管理员共享判断。
- 当前用户信息返回管理员标识。
- 空间列表返回真实空间角色。
- 前端空间角色初始化不再硬编码 owner。

第二阶段：工作空间切换。

- 左上角 `WorkspaceSwitcher`。
- 创建团队空间 modal。
- 切换空间和最近空间记忆。

第三阶段：工作空间页面。

- 新增 `工作空间` 菜单。
- 新增 `/space/:space_id/workspace`。
- 成员管理和基础设置页面。
- 成员 API 接入。

第四阶段：系统管理。

- 新增 `/system` 独立布局。
- 新增系统管理二级菜单。
- 管理员入口和守卫。
- 工作空间管理、用户管理、系统配置基础闭环。

## 开放问题

- 系统配置一期是否只读，还是允许保存。
- 用户管理一期是否只读，还是允许启用/禁用。
- 工作空间管理一期是否允许删除。
- 团队空间创建是否需要上传图标，还是先使用默认图标。
- 个人中心是否后续升级为独立页面。
