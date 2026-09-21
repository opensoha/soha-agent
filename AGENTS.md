# Soha Agent 仓库入口

- 本仓负责独立 Agent、runner 和集群执行能力；不得 import `soha/internal/**`，通过公开 API 和 contracts 与控制面交互。
- 在 OpenSoha 多仓工作区中读取 `../AGENTS.md` 一次；独立克隆时使用本仓规则，不要求初始化相邻仓库或规划工具。
- Agent 实现或实质审查前读取 [soha-agent](.agents/skills/soha-agent/SKILL.md) 及本次相关参考，不只依赖技能自动匹配；已读且未变化的内容可复用。保持诊断脱敏、执行权限与任务取消边界。
- 普通 Go 修复执行受影响包的行为回归；主入口为 `GOWORK=off go test ./...`。架构、契约、依赖、安全、并发/runner、打包或发布变更执行技能及 [CI](.github/workflows/ci.yml) 对应完整门禁。
- 文档和技能改动只检查内容、链接与差异；相关代码和环境未变化时复用成功验证，保留用户未提交改动。

## 变更与验收边界

- 修改前明确 API、Kubernetes adapter、runner、environment、Outpost 或打包的已有所有者、受影响调用者与验证入口；不因功能相似而复制另一套执行循环。
- 当前实现用于核实现状，不自动证明它符合协议或有效规范。冲突需明确说明；局部修复不顺手改变公开协议、action allowlist、并发默认值或工作目录策略。
- 共享 runner/lifecycle 修改验证相关消费者及取消、超时、重试、终态幂等、迟到回调、租约与资源清理；新增外部操作保持有界超时和现有审计路径。
- 对照本仓已有测试和 CI 选择验证层级，保留完整安全、race、镜像和发布门禁；不通过改宽策略或忽略失败实现通过。
- 记录 Agent/Core/contracts 实际提交或发布版本、执行模式、命令与结果。Mock/包测试、真实 runtime/cluster 和镜像验收分别报告；环境缺失、跳过、未运行不能记为通过。
- 跨仓只更新受影响的公开契约和消费者；本仓说明与历史计划不构成生产操作或发布授权。
