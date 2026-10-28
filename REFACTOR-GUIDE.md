# 重构历史归档

> 状态：已结束。本文只记录历史结论，不再包含待办事项。
> 当前架构以 [ARCHITECTURE.md](ARCHITECTURE.md) 为准。

## 结论

2026-10-27 的架构迁移已在提交 `6ef3f04` 完成：

- 群成员身份固定为 `(group_openid, member_openid)`。
- 新业务只支持群聊；全局模式作为同一群成员在不同群之间的显式查找策略恢复，
  单聊与 v6 `global-user:*` 存储仍只保留兼容。
- SQLite 从 scope + JSON + KV 迁移到 schema v7 群成员实体模型。
- `Storage` 拆为显式仓储端口，I/O 使用 `context.Context`。
- `admin.Service` 不再嵌入 `*schedule.Service`。
- 启动、路由、worker 和生命周期迁入 `internal/app`。
- 黑盒测试迁入 `tests/`，源码目录仅保留 `_whitebox_test.go`。
- 前端拆分为 client、endpoint、domain 和 state。
- CI 覆盖 race、shuffle、覆盖率、构建、卡片 smoke test 和前端产物一致性。

当时的详细债务清单、迁移步骤和回滚记录已经由 Git 历史保留，不再作为当前开发入口。
如果未来需要再次重构，应新建 ADR 或计划文档，并从当前 `ARCHITECTURE.md` 出发，而不是复活本文件中的旧任务。
