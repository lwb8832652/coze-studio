# 订阅与积分系统实施计划

> 设计依据：`docs/superpowers/specs/2026-07-22-subscription-credit-system-design.md`
>
> 目标：在 Coze Studio 中完整引入 Nuwax 的订阅、积分、订单、用量与模型计费能力，覆盖系统管理员和 C 端用户，同时保持 Coze 现有任务、模型与工作空间边界。

## 1. 交付范围

本次完整方案包含：

- 系统管理：基础配置、订阅套餐、积分包、用户积分、积分记录、订单管理。
- 用户中心：我的订阅、积分明细、用量统计、我的订单。
- 模型计费：模型价格版本、输入/输出/缓存 Token 价格、计费快照。
- 运行时结算：预占、结算、释放、失败补偿、幂等和并发控制。
- 订单与支付：订单状态机、支付网关抽象、支付回调幂等、订阅激活。
- 安全与审计：敏感字段保护、租户隔离、管理员鉴权、账本审计。
- 可观测性：余额、扣费、补偿、支付和对账指标。

本次明确不包含：

- Nuwax 独立的“支付与收益”菜单。
- 商户入驻、创作者收益、提现和分账。
- 默认启用团队共享钱包；一期以用户为计费主体，预留工作空间账单主体。
- 未配置生产支付网关时的模拟付费成功。

## 2. 实施原则

- 金额和积分统一使用整数微单位，禁止浮点数参与结算。
- 积分账本只追加，不直接改写历史流水。
- 扣减使用“最早到期优先”，并通过数据库事务与乐观锁保证并发一致性。
- 每次计费必须保存价格快照，历史账单不受后续价格调整影响。
- 所有消费、支付回调和补偿接口必须具备业务幂等键。
- 运行时计费失败默认 fail closed，不允许绕过余额或静默免费执行。
- 管理端权限以后端认证上下文为准，不能只依赖前端菜单隐藏。
- 前端优先复用现有 Coze Design/Semi 组件和系统管理页面布局。

## 3. 交付阶段

### 阶段一：积分账本与数据库基线

#### 目标

建立不可变积分账本、余额批次和预占能力，作为所有订阅与计费功能的基础。

#### 计划文件

- 新增 `docker/atlas/migrations/20260722000200_subscription_credit_billing.sql`
- 新增 `backend/domain/billing/entity.go`
- 新增 `backend/domain/billing/repository.go`
- 新增 `backend/domain/billing/service.go`
- 新增 `backend/domain/billing/service_test.go`
- 新增 `backend/infra/billing/mysql_repository.go`
- 新增 `backend/infra/billing/mysql_repository_test.go`
- 新增 `backend/application/billing/init.go`
- 新增 `backend/application/billing/service.go`
- 修改 `backend/application/application.go`

#### 数据表

- `billing_accounts`
- `credit_batches`
- `credit_ledger_entries`
- `credit_reservations`
- `billing_audit_logs`

#### 核心约束

- `billing_accounts` 唯一键为 `subject_type + subject_id`。
- 用户账单主体必须绑定认证用户，不能接受客户端传入任意用户 ID。
- `credit_batches.remaining_micros` 不得小于零。
- `credit_ledger_entries.business_no` 全局幂等。
- `credit_reservations` 支持 `reserved/settled/released/expired` 状态。
- 扣减按 `expires_at ASC, id ASC` 锁定批次。
- 余额为批次汇总投影，账本为审计事实源。

#### TDD 顺序

1. 编写余额授予、过期过滤、最早到期优先扣减测试。
2. 编写重复业务号、余额不足、并发版本冲突测试。
3. 编写预占、部分结算、释放和超时回收测试。
4. 实现领域服务和仓储。
5. 增加数据库约束与索引。

#### 验证命令

```bash
cd backend
go test ./domain/billing ./infra/billing ./application/billing -run Test -count=1

cd ..
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

### 阶段二：订阅、套餐、积分包与订单

#### 目标

建立套餐生命周期、周期性积分发放、积分包购买和订单支付状态机。

#### 计划文件

- 扩展 `backend/domain/billing/entity.go`
- 扩展 `backend/domain/billing/repository.go`
- 新增 `backend/domain/billing/subscription_service.go`
- 新增 `backend/domain/billing/subscription_service_test.go`
- 新增 `backend/domain/billing/order_service.go`
- 新增 `backend/domain/billing/order_service_test.go`
- 扩展 `backend/infra/billing/mysql_repository.go`
- 新增 `backend/application/billing/payment_gateway.go`
- 新增 `backend/application/billing/payment_service.go`
- 新增 `backend/application/billing/payment_service_test.go`

#### 数据表

- `subscription_plans`
- `subscription_plan_versions`
- `user_subscriptions`
- `subscription_cycle_grants`
- `credit_packages`
- `billing_orders`
- `billing_order_items`
- `payment_transactions`

#### 核心状态机

- 订阅：`pending -> active -> past_due/cancelled/expired`。
- 订单：`pending_payment -> paid -> fulfilled`，失败进入 `failed`，超时进入 `closed`。
- 支付交易：`created -> processing -> succeeded/failed/refunded`。
- 只有已验证的支付成功事件才能激活订阅或发放积分。
- 套餐和积分包修改生成版本快照，已购订单继续引用原快照。

#### 支付网关边界

- 定义 `PaymentGateway` 接口，不在领域层绑定具体供应商。
- 未配置生产网关时，仅允许创建不可支付订单并返回明确配置错误。
- 支付回调必须验签、校验金额/币种/订单状态并记录原始事件摘要。
- 原始密钥、完整回调体和供应商凭据不得返回前端或写入普通日志。

### 阶段三：模型定价、用量采集与运行时计费

#### 目标

将现有任务 Token 用量转为可审计积分消费，并补齐模型定价和监控数据。

#### 计划文件

- 新增 `backend/domain/billing/pricing.go`
- 新增 `backend/domain/billing/pricing_test.go`
- 新增 `backend/domain/billing/metering.go`
- 新增 `backend/domain/billing/metering_test.go`
- 新增 `backend/application/billing/metering_service.go`
- 新增 `backend/application/billing/metering_service_test.go`
- 修改 Agent/Task 完成路径中的用量落账适配器
- 扩展现有模型管理应用服务，关联价格版本与监控聚合

#### 数据表

- `model_price_versions`
- `model_usage_records`
- `billing_settlements`
- `billing_reconciliation_runs`

#### 计价合同

- 输入 Token、输出 Token、缓存写入和缓存命中分别定价。
- 每条用量记录保存 provider、model、task、run、价格版本和价格快照。
- 预估阶段按最大安全预算预占，执行完成后按真实用量结算并释放差额。
- Provider 已扣费但业务落账失败时进入可重试补偿队列。
- 同一 `run_id + usage_sequence` 不得重复扣费。

#### 运行时接入顺序

1. 只记录影子用量，不影响任务执行。
2. 比较现有 `CostMicros` 与新计费结果并输出差异指标。
3. 开启预占但不真实扣减。
4. 对内部测试账号开启真实结算。
5. 完成对账后逐步全量。

### 阶段四：系统管理 API 与页面

#### 目标

在现有 `/system` 管理控制台中完整承载订阅与积分管理，不引入独立后台壳。

#### 计划文件

- 新增 `idl/admin/billing.thrift`
- 修改 `idl/api.thrift`
- 生成 `backend/api/model/admin/*`
- 生成或扩展 `backend/api/router/coze/api.go`
- 新增 `backend/api/handler/coze/admin_billing_service.go`
- 新增 `backend/api/handler/coze/admin_billing_service_test.go`
- 修改 `frontend/apps/coze-studio/src/pages/system/content.ts`
- 修改 `frontend/apps/coze-studio/src/pages/system/view-model.ts`
- 修改 `frontend/apps/coze-studio/src/pages/system/index.tsx`
- 修改 `frontend/apps/coze-studio/src/pages/system/service.ts`
- 修改 `frontend/apps/coze-studio/src/pages/system/newx-system-ui.less`
- 新增 `frontend/apps/coze-studio/src/pages/system/billing/*`

#### 管理菜单

- `订阅与积分 / 基础配置`
- `订阅与积分 / 套餐管理`
- `订阅与积分 / 积分包管理`
- `订阅与积分 / 用户积分`
- `订阅与积分 / 积分记录`
- `订阅与积分 / 订单管理`

#### 管理功能闭环

- 基础配置：积分名称、显示精度、过期策略、欠费策略和结算开关。
- 套餐管理：新增、编辑、上下架、版本、周期、额度和适用范围。
- 积分包：新增、编辑、上下架、售价、积分、有效期和购买限制。
- 用户积分：余额查询、管理员赠送、扣减、冻结、解冻和原因审计。
- 积分记录：按用户、业务类型、时间、方向和业务号筛选导出。
- 订单管理：订单详情、支付状态、履约状态、关闭和异常重试。

#### 权限

- 所有 `/api/admin/billing/*` 路由复用服务端系统管理员校验。
- 管理员手工调账必须填写原因，并记录操作者、目标用户和前后余额。
- 删除套餐和价格版本采用停用/归档，不物理删除已被引用的数据。

### 阶段五：C 端订阅、积分、用量和订单

#### 目标

按 Nuwax 的用户路径接入现有账号菜单与页面体系，形成购买、查看和追溯闭环。

#### 计划文件

- 新增用户侧 Billing IDL 与生成 client
- 新增 `backend/api/handler/coze/billing_service.go`
- 新增 `backend/api/handler/coze/billing_service_test.go`
- 修改 `frontend/apps/coze-studio/src/routes/index.tsx`
- 修改 `frontend/apps/coze-studio/src/components/workspace-account-dropdown.tsx`
- 修改 `frontend/packages/foundation/global-adapter/src/components/account-dropdown/account-settings/index.tsx`
- 新增 `frontend/apps/coze-studio/src/pages/billing/subscriptions/*`
- 新增 `frontend/apps/coze-studio/src/pages/billing/credits/*`
- 新增 `frontend/apps/coze-studio/src/pages/billing/usage/*`
- 新增 `frontend/apps/coze-studio/src/pages/billing/orders/*`
- 新增共享账单组件、服务和样式测试

#### 用户页面

- `/billing/subscriptions`：当前套餐、周期、续费状态、套餐对比和订阅操作。
- `/billing/credits`：可用积分、即将过期积分、收入/支出明细和业务来源。
- `/billing/usage`：按日期、模型、任务和用量类型查看 Token 与积分消耗。
- `/billing/orders`：订单列表、订单详情、支付状态和继续支付入口。

#### 交互要求

- 账号下拉提供“订阅与积分”入口，不新增左侧一级业务菜单。
- 余额不足时提供明确原因和可操作入口，不能只显示通用失败。
- 价格、额度、有效期和自动续费状态在确认购买前完整展示。
- 支付处理中可刷新查询，重复点击不得生成重复订单。
- 所有列表覆盖 loading、empty、error、refresh 和分页状态。

### 阶段六：补偿、对账、灰度与生产验收

#### 目标

完成生产级故障恢复、财务一致性、灰度开关和端到端验收。

#### 后台任务

- 过期预占释放。
- 到期积分批次失效。
- 订阅周期结转和额度发放。
- 支付状态主动查询与异常补偿。
- 用量记录和积分账本对账。
- 余额投影与批次汇总对账。

#### 运行开关

- `BILLING_CONTROL_PLANE_ENABLED`
- `BILLING_SHADOW_METERING_ENABLED`
- `BILLING_RESERVATION_ENABLED`
- `BILLING_SETTLEMENT_ENABLED`
- `BILLING_PAYMENT_ENABLED`

#### 生产验收清单

- 普通用户无法访问管理 API，服务端返回 `403`。
- 同一支付回调重复投递不会重复激活或发放积分。
- 同一任务重试不会重复扣费。
- 并发消费不会出现负余额。
- 预占失败时任务不进入付费执行。
- 执行失败时按策略结算实际消耗并释放剩余额度。
- 价格调整后历史订单和历史账单保持原价格快照。
- 用户可从订单追溯到订阅/积分包，并从积分流水追溯到任务用量。
- 管理员调账、套餐变更和订单处置均有审计记录。
- 未配置支付网关时付费功能明确不可用，不伪造成功。
- 管理端六个页面与用户端四个页面通过页面验收。
- 页面验收记录 URL、账号、关键状态、交互结果和控制台错误。

## 4. API 分组

### 管理端

- `/api/admin/billing/config`
- `/api/admin/billing/plans`
- `/api/admin/billing/credit-packages`
- `/api/admin/billing/accounts`
- `/api/admin/billing/ledger`
- `/api/admin/billing/orders`
- `/api/admin/billing/prices`
- `/api/admin/billing/monitoring`
- `/api/admin/billing/reconciliation`

### 用户端

- `/api/billing/account`
- `/api/billing/subscription`
- `/api/billing/plans`
- `/api/billing/credit-packages`
- `/api/billing/ledger`
- `/api/billing/usage`
- `/api/billing/orders`
- `/api/billing/payments`

### 内部运行时

- `ReserveCredits`
- `SettleCredits`
- `ReleaseReservation`
- `RecordModelUsage`
- `RetrySettlement`

## 5. 提交拆分

建议按以下顺序形成可独立审核和回滚的提交：

1. `feat(billing): add immutable credit ledger and reservations`
2. `feat(billing): add subscription plans packages and orders`
3. `feat(billing): add model pricing metering and settlement`
4. `feat(admin): add subscription and credit management`
5. `feat(account): add customer billing center`
6. `feat(billing): add reconciliation rollout and observability`

每个提交必须保持数据库、领域合同和调用方一致；不能提交只有 UI 壳、无服务端约束的中间状态。

## 6. 回滚边界

- 页面或管理能力异常：关闭控制面开关，保留账本和历史数据。
- 运行时计费异常：关闭结算开关，继续影子计量，不删除用量记录。
- 支付异常：关闭支付开关，停止新订单支付，继续处理已验签成功回调。
- 对账不一致：冻结受影响账户的新扣费，执行补偿，不直接修改历史账本。
- 迁移只允许前向修复，不通过破坏性回滚删除账务数据。

## 7. 完成定义

以下条件全部满足后，才可声明完整方案交付完成：

- 阶段一至阶段六代码和迁移全部落地。
- 领域、仓储、应用、Handler 和前端关键路径测试通过。
- Atlas hash 和 migration validate 通过。
- 管理端六个页面与用户端四个页面完成真实数据验收。
- 任务运行完成一次真实预占、结算、释放和账本追溯。
- 支付网关沙箱完成下单、回调、激活、重复回调和失败补偿验收。
- 管理员与普通用户权限边界验证通过。
- 账本、余额、用量和订单对账无未解释差异。
- 安全检查确认 API Key、支付密钥、回调原文和内部审计载荷未泄露。
- 运维文档、灰度开关、监控告警和回滚步骤已记录。

## 2026-07-23 最终交付验收

- 数据层：Atlas 已应用至 `20260723000400`，积分账户、不可变流水、批次、预占、套餐、积分包、订单、模型定价、用量和基础配置均使用持久化表。
- 一致性：账户首次并发创建使用幂等插入和行锁；人工积分调整、流水和审计位于同一数据库事务；失败回归用例确认不会留下部分入账。
- 运行时：结算关闭或账单服务未初始化时兼容原调用；结算开启后在模型调用前预占，缺少身份、价格或余额不足时按 fail-closed 处理。
- 管理端：积分基础配置、订阅套餐、积分包、模型定价、模型监控、用户积分、积分记录和订单管理 8 个页面均完成终态验收。
- 用户端：我的订阅、积分明细、用量统计和我的订单 4 个页面均完成桌面验收；订阅页完成 390x844 窄屏验收。
- 交互：基础配置保存成功；维护与对账成功，核对 1 个账户且差异 0；验收账户可用积分和预占积分均恢复为 0。
- 自动化：`go test ./application/billing ./infra/billing ./application/agentthread -count=1`、管理员 handler/router 编译、账单 Vitest、`tsc --noEmit` 和 `atlas migrate validate` 均通过。
- 浏览器：最终源码重建并启动后，12 个账单路由均无请求失败、无加载残留；修复后的验收时间窗内控制台新增错误为 0。
- 上线开关：在线支付和运行时结算保持默认关闭；只有支付网关、回调签名、模型价格和运营配置就绪后才允许显式开启。
