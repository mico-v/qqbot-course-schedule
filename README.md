# qqbot-course-schedule

基于 **QQ 官方机器人 API v2 + Go** 的独立课表机器人。

功能抽象自 [astrbot_plugin_CourseSchedule](https://github.com/mico-v/astrbot_plugin_CourseSchedule)，
框架思路参考 [Polarix](https://github.com/YearnstudioYangyi/Polarix)，不依赖 AstrBot / OneBot / NapCat。

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/PLAN.md](docs/PLAN.md) | 项目计划：功能抽象、范围、里程碑、验收标准、风险 |
| [docs/DEV-GUIDE.md](docs/DEV-GUIDE.md) | 开发手册：环境、目录、配置、编码规范、测试、部署 |
| [docs/COVERAGE-AstrBot.md](docs/COVERAGE-AstrBot.md) | QQ 官方 API v2 × AstrBot 适配度研究（既有） |
| [docs/INDEX.md](docs/INDEX.md) | QQ 官方文档快照索引（既有） |
| [docs/autogen/](docs/autogen/) | QQ 官方 API / 事件文档快照（既有） |

## 状态

- [x] 需求调研与官方文档快照
- [x] 项目计划与开发手册（待评审）
- [ ] M0 骨架：webhook 验签 + Token + 文本收发
- [ ] M1 存储 / ICS / 文字课表
- [ ] M2 卡片图片
- [ ] M3 休假调休 / 时长榜
- [ ] M4 Web 管理台
- [ ] M5 定时推送 / 按钮交互

## 许可证

待定。功能设计源自作者本人的 AGPL-3.0 项目，框架参考 MIT 项目 Polarix。
