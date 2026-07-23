# Nuwax 生态 MCP 目录迁移实施计划

**Goal:** 将 Nuwax 生态市场 24 个 MCP 完整迁移到 Coze 现有官方 MCP 目录，
并以真实运行能力区分可安装和需适配条目。

**Architecture:** 使用代码内不可变官方目录作为平台元数据源，工作空间安装
继续落到现有 MCP Server 数据模型。目录合同新增来源、发布方、图标和可用性
字段；安装 API 对需适配条目 fail closed。

**Tech Stack:** Go、Hertz、React、TypeScript、Semi/Coze Design。

## 工作项

- [x] 从 Nuwax 运行页面提取 24 个目录条目的名称、说明和图标。
- [x] 核实公开部署模板与 Nuwax 私有网关依赖。
- [x] 扩展后端和前端目录数据合同。
- [x] 增加完整 Nuwax 目录快照与 5 个公开运行模板。
- [x] 增加需平台适配条目的服务端安装保护。
- [x] 完成官方服务卡片、搜索、状态和安装交互。
- [x] 补齐相关自动化测试与页面验收记录。
