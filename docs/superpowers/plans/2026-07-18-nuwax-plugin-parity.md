# NuWAX 插件能力对齐实施计划

> **For agentic workers:** 后续仅执行本文“待完成事项”，不得恢复已排除的生态
> 市场、分组、文件树或多文件方案。

**目标：** 在 Coze 原插件页面内交付 `FUNC + IDE` 单源码代码插件闭环。

**架构：** 复用 Coze 插件 ID、路由、权限、发布和资源生命周期；代码草稿与
版本由独立持久化合同承载，执行统一经过 Sandbox `plugin` scope。前端使用
生成 API client，并以服务端 revision 和 `debug_ready` 为权威事实。

**技术栈：** React、TypeScript、Coze Design、Go、Hertz、GORM、Thrift、
MySQL、Atlas、Sandbox Control Plane。

---

## 范围锁定

- 保留 Coze 原插件列表、创建入口、详情页、发布入口和既有路由。
- 代码插件固定为 `FUNC + IDE`。
- 只支持单源码，不支持文件树、多文件或源码包。
- 支持 Python、JavaScript、输入/输出 schema、CAS 保存和试运行。
- 发布资格只使用服务端 `debug_ready`。
- 完成发布、复制、删除、权限和 Sandbox 生产闭环。
- 不引入生态市场、插件分组、文件树或新一级菜单。

## 已完成任务

### 1. Sandbox 插件执行边界

- [x] 增加 plugin scope、workload 和 purpose。
- [x] 固定代码插件入口为 `plugin/plugin/plugin/code/run`。
- [x] 代码插件只允许 remote Provider，禁止 local debug 和 host fallback。
- [x] 对非法 scope、workload、entrypoint 和未知 purpose fail closed。

### 2. 单源码持久化与 IDL

- [x] 增加代码草稿、版本和 revision CAS 仓储。
- [x] 保存单份源码、runtime、entry file、输入/输出 schema 和 bundle hash。
- [x] schema 修改进入 hash、提升 revision 并使旧调试状态失效。
- [x] 复制和发布版本完整快照源码及两份 schema。
- [x] 补齐草稿读取、保存、试运行和版本读取的生成 API 合同。

### 3. 创建、保存与试运行

- [x] 创建代码插件提交 `FUNC + IDE` 和 Python/JavaScript runtime。
- [x] 普通 HTTP 插件继续使用 `PLUGIN + COZE`，不回归原有 URL/鉴权约束。
- [x] 保存使用 revision CAS，冲突不覆盖服务端新草稿。
- [x] 服务端验证输入 schema，执行后验证输出 schema。
- [x] 调试结果使用受控字段和脱敏公共错误。

### 4. 原插件页面内的代码工作区

- [x] 在原 Coze 插件详情页内按 `FUNC + IDE` 分流。
- [x] 提供单源码编辑器、运行时、输入结构、输出结构和试运行区域。
- [x] 支持保存快捷键、loading、error、readonly、conflict 和响应式布局。
- [x] 使用 identity、epoch 和 sequence 防止加载、保存、调试响应跨插件污染。
- [x] 发布按钮只接受服务端当前 revision 的 `debug_ready=true`。
- [x] 未新增生态市场、分组、文件树或多文件编辑能力。

### 5. 生命周期、权限与安全

- [x] 发布前锁定并校验当前 revision、bundle hash 和调试 revision。
- [x] 发布版本固化源码、runtime、入口文件和输入/输出 schema。
- [x] 复制保留代码和 schema，重置 revision 语义和调试状态，不复制凭据。
- [x] 删除接入插件生命周期并清理代码数据。
- [x] 创建、读取、保存、调试、发布、复制和删除由服务端执行权限校验。
- [x] 代码插件拒绝 HTTP URL、OAuth 和 Service Auth 配置。

## 已完成验证

- [x] 前端 workspace/schema 定向测试：50 个通过。
- [x] 前端创建提交定向测试：2 个通过。
- [x] `bot-plugin/entry` TypeScript 检查通过。
- [x] `plugin-form-adapter` TypeScript 检查通过。
- [x] 后端 `application/plugin`、`domain/plugin/repository` 和
  `api/handler/coze` 插件相关定向测试通过。
- [x] 后端 `domain/sandbox`、`application/sandbox`、`infra/sandbox`、
  `infra/coderunner/impl`、`infra/coderunner/impl/controlplane` 相关定向测试
  通过。
- [x] `atlas migrate validate --dir file://docker/atlas/migrations` 通过。

说明：`api/handler/coze` 全包存在与本次无关的既有 `TestValidateTree` panic。
本次以插件相关定向测试作为验证证据，不把该既有问题列为本次功能失败。

## 待完成事项

### 6. 应用数据库 migration

- [ ] 在确认的本地或测试数据库执行 migration apply。
- [ ] 确认代码草稿、版本、schema 默认值、索引和外键级联生效。
- [ ] migration apply 属于数据库变更，执行前必须取得用户明确授权。

### 7. 使用内置浏览器完成页面验收

- [ ] 在 Coze 原插件入口创建 Python 代码插件，确认提交为 `FUNC + IDE`。
- [ ] 保存源码和输入/输出 schema，确认刷新后数据一致。
- [ ] 制造 CAS 冲突，确认页面保留本地内容且不静默覆盖。
- [ ] 分别验证合法输入、输入 schema 失败、运行错误和输出 schema 失败。
- [ ] 确认只有服务端当前 revision 的 `debug_ready=true` 时可发布。
- [ ] 发布并查看版本快照；修改草稿后确认发布资格撤销。
- [ ] 复制插件，确认源码/schema 被复制且必须重新调试。
- [ ] 验证删除、只读用户、跨空间和无权限操作由服务端拒绝。
- [ ] 关闭或移除可用 plugin Provider，确认执行 fail closed 且无 host fallback。
- [ ] 创建 JavaScript 代码插件并完成同等最小闭环。
- [ ] 记录具体 URL、账号/空间、关键可见状态、交互结果和控制台错误。

## 完成定义

- [x] 代码层实现符合单源码、`FUNC + IDE`、Python/JavaScript 和 schema 边界。
- [x] 保存 CAS、试运行、服务端 `debug_ready`、发布、复制、删除、权限和
  Sandbox 闭环已有定向测试证据。
- [x] 明确不引入生态市场、分组、文件树和多文件能力。
- [ ] 数据库 migration 已 apply 并确认结构生效。
- [ ] Codex 内置浏览器验收完成且无本次功能阻断问题。

当前结论：代码实现及定向验证完成；数据库和页面环境验收未完成，因此尚不标
记为生产验收完成。
