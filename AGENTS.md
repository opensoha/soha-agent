# Soha Agent 仓库入口

- 本仓负责独立 Agent、runner 和集群执行能力；不得 import `soha/internal/**`，通过公开 API 和 contracts 与控制面交互。
- 在 OpenSoha 多仓工作区中读取 `../AGENTS.md` 一次；独立克隆时使用本仓规则，不要求初始化相邻仓库或规划工具。
- Agent 实现或审查按需使用 [soha-agent](.agents/skills/soha-agent/SKILL.md)，保持诊断脱敏、执行权限与任务取消边界。
- 普通 Go 修复执行受影响包的行为回归；主入口为 `GOWORK=off go test ./...`。架构、契约、依赖、安全、并发/runner、打包或发布变更执行技能及 [CI](.github/workflows/ci.yml) 对应完整门禁。
- 文档和技能改动只检查内容、链接与差异；相关代码和环境未变化时复用成功验证，保留用户未提交改动。
