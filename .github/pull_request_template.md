## 变更说明

<!-- 说明用户可见变化、内部结构变化和原因。 -->

## 文档对齐

- [ ] 已阅读 `ARCHITECTURE.md`、`PLAN.md` 和相关开发/部署文档
- [ ] 已同步更新受影响的文档
- [ ] 无需更新文档，并已在提交说明中说明原因
- [ ] 未重新引入已完成任务或历史重构清单

## 验证

- [ ] `COVERAGE_STAGE=p2 bash scripts/test-all.sh`
- [ ] 涉及前端时执行 `RUN_WEB=1 COVERAGE_STAGE=p2 bash scripts/test-all.sh`
- [ ] 涉及卡片/WebUI 时已人工检查或提供截图

## 风险与回滚

<!-- 说明数据迁移、兼容性、部署顺序和回滚方式；无风险时写“无”。 -->
