# 变更与交付规则

本项目采用文档驱动变更。文档中的当前契约优先于代码习惯和个人偏好。

## 强制流程

每次代码修改、升级、重构或行为调整必须按以下顺序完成：

1. **先读文档**
   - 架构与依赖：`ARCHITECTURE.md`
   - 功能行为与边界：`PLAN.md`
   - 开发、测试和部署：`DEV-GUIDE.md`
   - 平台接入：`CONNECT.md`（涉及平台配置时）
   - 部署：`deploy/README.md`（涉及部署时）

2. **确认文档契约**
   - 代码实现必须符合当前文档。
   - 如果需求与文档冲突，先更新规格/架构，再修改代码。
   - 不允许通过代码绕过文档中的身份、作用域、权限、存储或错误契约。

3. **同批更新文档**
   - 行为、限制、命令或接口变化：更新 `PLAN.md`。
   - 模块、依赖、端口、数据库、调用链变化：更新 `ARCHITECTURE.md`。
   - 开发流程、测试命令、目录或部署步骤变化：更新 `DEV-GUIDE.md`。
   - 平台事件、回调或接入步骤变化：更新 `CONNECT.md`。
   - 部署脚本、服务或运维步骤变化：更新 `deploy/README.md`。
   - 用户入口或文档索引变化：更新 `README.md`。
   - 纯内部实现且不改变任何现有契约时，也必须在提交说明中确认无需更新文档。

4. **实现并验证**
   ```bash
   COVERAGE_STAGE=p2 bash scripts/test-all.sh
   ```
   涉及前端时再运行：
   ```bash
   RUN_WEB=1 COVERAGE_STAGE=p2 bash scripts/test-all.sh
   ```

5. **提交并推送**
   ```bash
   git add -A
   git commit -m "type(scope): 中文描述"
   git push origin main
   ```
   - 只有检查通过后才能提交。
   - 推送失败必须明确报告原因，不能声称已完成交付。
   - 部署是独立动作；未明确要求部署时，不自动执行远端部署。

## 文档映射

| 变更类型 | 必须检查/更新 |
| --- | --- |
| 群成员身份、作用域 | `ARCHITECTURE.md`、`PLAN.md` |
| 数据库表、迁移、revision | `ARCHITECTURE.md`、`PLAN.md`、`CONNECT.md` |
| 聊天指令与用户行为 | `PLAN.md`、`DEV-GUIDE.md` |
| HTTP API/错误码 | `PLAN.md`、`DEV-GUIDE.md` |
| 模块、端口、依赖方向 | `ARCHITECTURE.md`、`DEV-GUIDE.md`、`AGENTS.md` |
| 文件目录、测试目录 | `ARCHITECTURE.md`、`DEV-GUIDE.md`、`AGENTS.md` |
| 测试与 CI | `ARCHITECTURE.md`、`DEV-GUIDE.md`、`README.md` |
| Web 管理台 | `PLAN.md`、`DEV-GUIDE.md` |
| 配置、平台回调 | `DEV-GUIDE.md`、`CONNECT.md` |
| systemd、Caddy、部署脚本 | `deploy/README.md` |

## 禁止事项

- 不提交与代码行为不一致的文档。
- 不把已完成任务、历史迁移或临时计划重新写回当前架构文档。
- 不把 `REFACTOR-GUIDE.md` 当作新的待办清单。
- 不通过删除约束来让测试通过。
- 不在未运行验证的情况下提交或推送。
- 不在提交说明中使用 `update`、`fix stuff` 等无范围描述。
