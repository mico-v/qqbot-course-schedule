# qqbot-course-schedule

基于 **QQ 官方机器人 API v2 + Go** 的独立课表机器人。

功能抽象自 [astrbot_plugin_CourseSchedule](https://github.com/mico-v/astrbot_plugin_CourseSchedule)，
框架思路参考 [Polarix](https://github.com/YearnstudioYangyi/Polarix)，不依赖 AstrBot / OneBot / NapCat。

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/PLAN.md](docs/PLAN.md) | 项目计划：功能抽象、范围、里程碑、验收标准、风险 |
| [docs/DEV-GUIDE.md](docs/DEV-GUIDE.md) | 开发手册：环境、目录、配置、编码规范、测试、部署 |
| [docs/CONNECT.md](docs/CONNECT.md) | 接入清单：平台配置、服务器配置、验证步骤、常见失败 |
| [docs/COVERAGE-AstrBot.md](docs/COVERAGE-AstrBot.md) | QQ 官方 API v2 × AstrBot 适配度研究（既有） |
| [docs/INDEX.md](docs/INDEX.md) | QQ 官方文档快照索引（既有） |
| [docs/autogen/](docs/autogen/) | QQ 官方 API / 事件文档快照（既有） |

## 快速开始

```bash
cp config.example.json config.json   # 填写 appid / secret
go run ./cmd/bot                    # 监听 :8080，回调路径 /webhook
curl -s localhost:8080/healthz       # {"ok":true,"version":"dev"}
```

已实现指令：`/今日课表` `/明日课表` `/课表 [日期]` `/导入课表`（发送 `.ics` 文件自动导入）`/ping` `/help`。
卡片预览（开发用）：`go run ./cmd/cardpreview -o /tmp/card.jpg`。

![课表卡片预览](docs/assets/card-preview.jpg)

接入 QQ 平台的完整步骤见 [docs/CONNECT.md](docs/CONNECT.md)。

一键编译上传部署（systemd + Caddy）：

```bash
./deploy/deploy.sh                  # 测试 → 构建 → 上传 → 重启 → 健康检查
```

## 状态

- [x] 需求调研与官方文档快照
- [x] 项目计划与开发手册
- [x] M0 骨架：webhook 验签 + Op=13 + Token + 文本收发 + `/ping` `/help` + 一键部署脚本
- [x] M1 数据与图片：SQLite / ICS / RRULE / 图片渲染（`/课表` 全图片）
- [ ] M2 休假调休 / 时长榜（图片）
- [ ] M3 Web 管理台
- [x] M4 指令面板（c2c + group 已上线，`/同步面板` 可手动同步）
- [ ] M4 按钮交互 / 定时推送

## 许可证

待定。功能设计源自作者本人的 AGPL-3.0 项目，框架参考 MIT 项目 Polarix。
