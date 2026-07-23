# 工作空间官方 MCP 目录规格

## 参照结论

Nuwax 本地实现通过 `/api/mcp/official/list` 读取 `space_id = -1` 的已部署记录，
但当前源码、初始化 SQL 与本地运行环境均没有官方 MCP 数据。因此本次对齐其
“平台目录 + 工作空间使用”的机制，不伪造不存在的 Nuwax 目录内容。

## 目录和安装边界

- 官方目录是代码所有、只读且全局一致的元数据，不保存租户凭据。
- 工作空间安装复用现有 MCP server 持久化合同，继续执行空间角色强校验。
- 同名自定义服务显示为“待迁移”，Owner/Admin 显式提交后原位转为官方实例，
  不创建重复记录。
- 凭据进入既有 stdio credential canonicalization 和加密存储链路；API 只返
  回 `configured` 状态，不回显 Token、数据库 URL 或 provider 原始响应。
- 安装或重新配置后默认保持停用，用户显式启用时再执行能力发现和健康检查，
  避免运行依赖缺失导致配置无法保存。

## 首批目录

- GitHub：使用现有 `@modelcontextprotocol/server-github` 运行模板，要求
  `GITHUB_TOKEN`。
- PostgreSQL：使用现有 `@modelcontextprotocol/server-postgres` 运行模板，
  要求合法的 `postgres`/`postgresql` 连接地址。

OpenMeteo 和 Weather 当前只有静态能力描述，没有经过验证的安全运行模板，
不纳入首批官方目录。
