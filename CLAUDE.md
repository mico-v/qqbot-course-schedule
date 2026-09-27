# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

QQ 官方机器人 API v2 + Go 的独立课表机器人（不依赖 AstrBot / OneBot / NapCat）。
功能抽象自 `astrbot_plugin_CourseSchedule`（Python），**用户可见文案与该 Python 版逐字对齐**，便于验收。

文档分工（改代码时同步更新）：

| 文件 | 内容 |
| --- | --- |
| [PLAN.md](PLAN.md) | 功能规格（F1–F10）、里程碑、风险、**决策记录 D1–D9**（改设计前先读） |
| [DEV-GUIDE.md](DEV-GUIDE.md) | 最详细的手册：目录、配置、各模块规范、FAQ。**改动后请同步本文档** |
| [CONNECT.md](CONNECT.md) | QQ 开放平台接入与验证步骤 |
| `docs/` | **仅**官方文档快照，不得混入本项目文档 |

## Commands

```bash
go env -w GOPROXY=https://goproxy.cn,direct   # 本机直连 proxy.golang.org 超时，必须先设
go build ./...
go vet ./... && gofmt -l .                    # deploy.sh 会检查 gofmt
go test ./...                                 # 全量（含 web/、assets/ 编译）
go test ./internal/schedule/ -run TestDayoff -v
go test ./internal/schedule/ -run TestParseICS -count=1   # 缓存干扰时

go run ./cmd/bot                              # 读 config.json（可用 BOT_CONFIG 覆盖），默认 :18080，回调 /webhook
curl -s localhost:18080/healthz
go run ./cmd/cardpreview -o /tmp/card.jpg     # 改卡片版式时先看预览（-rank 看榜单，-avatar 带真实头像）

./deploy/deploy.sh                            # gofmt → go test → 构建 → 上传 → 重启 → 健康检查
./deploy/deploy.sh --skip-tests --caddy       # 需 CADDY_DOMAIN；HOST 选 SSH 主机（默认 as）
```

测试全部是标准库 `testing`，无 fake 框架、无 golden 文件、无 `-update` 开关：
渲染用尺寸断言 + 像素抽样，HTTP 用 `httptest` 假官方服务器，存储用临时目录。
引用测试断言失败时打印大量中文差异，属于正常现象。

## 依赖方向（禁止反向依赖）

```
webhook → bot → schedule
              ↘ qqapi
render  只依赖 schedule 的数据结构
server  依赖 store/schedule/render，不依赖 webhook
schedule 包内不得 import gin / qqapi / store（只认 types.go 里的 Storage 接口）
```

`main` 只做装配：`config → store → render → qqapi → bot.Env → webhook.Dispatcher → gin 路由 → scheduler`（见 `cmd/bot/main.go`）。
`bot.Env` 不持有通用 `schedule.Storage`：推送订阅与面板状态分别只依赖 `schedule.PushStore` /
`schedule.PanelStore`，非测试代码不得 import `store`。

## 关键约定

**所有课表写路径都经过 `schedule.Service`**（`internal/schedule/service.go`、`web.go`）。它负责排序、ICS 重建、派生字段、`course_id` 重排。机器人指令、WebUI `/api/*`、批量导入导出共用同一层，不要绕过它直接写 store。推送订阅/面板状态属于独立的 KV 配置，只能走 `schedule.PushStore` / `schedule.PanelStore`。

**乐观锁**：`Storage.PutMember(scopeID, userID, member, expectedRevision)`，`expectedRevision=nil` 表示新建。
`store.ErrConflict` → WebUI 返回 409（提示刷新，**不自动重试覆盖**）；机器人侧回中文提示。
裸写 SQL（不带 revision）是禁止的。

**指令注册**（`internal/bot/commands.go` 的 `NewDefaultHandler`）：一个 `Command{Prefix, Aliases, Description, Ready, Handle}`。
- 新指令标 `Ready: true` 就会自动进入平台指令面板，**不需要改 `panelOrder`**（`panel.go` 用 `panelOrder` 仅做排序，未列出的按前缀顺序追加）。
- `panel.go` 用 items 的 SHA-256 存 KV 做变更检测；面板错误只记日志，不影响启动与消息链路，管理员可用 `/同步面板` 手动触发。
- `Handler.Dispatch` 已剥离命中的前缀并放入 `in.Args`（群内平台面板会去掉前导 `/`，`Dispatch` 同时接受 `/课表` 与 `课表`）。

**`Dispatch` 的处理顺序**（改动消息链路前必读 `internal/bot/handler.go`）：去 @ 前缀 → 总开关开时 `recordSeenMember` + `.ics` 附件自动导入（直接 return，不再走指令）→ 查指令 → `/设置` 可绕过总开关 → 回复策略过滤（无斜杠/斜杠/@机器人，提到 @ 优先于斜杠判定）→ 执行。
`Handler.currentSettings()` 读设置失败时回退默认值，绝不因此静默静音机器人。

**会话作用域**：`group:<group_openid>` / `private:<user_openid>`（`schedule.ScopeGroup` / `ScopePrivate` / `ParseScope`）。
成员标识用 `in.UserOpenID`（群内为 member_openid），**不与 QQ 号对应**；QQ 号只是成员记录里的自报字段（`/绑定QQ` 或 WebUI），用于 `q1.qlogo.cn` 头像，不做真实性校验。

**时区**：全项目唯一 `schedule.LocalTZ = Asia/Shanghai`（`_now_iso` 等记账字段才用 UTC）。日界 = 本地 00:00，区间 `[start, end)`。写时间相关代码若出现 8 小时偏差，就是这里错了。

**日期解析只有两个入口**：`schedule/dayoff.go`（天/范围，含相对词）与 `schedule/timerange.go`（区间表达式）。新需求必须扩展这两个，**不要新写解析器**。

**KV 命名空间**（表 `kv_data`，`store.GetKV/SetKV/ListKV/DeleteKV`，`global` scope 下）：
`settings/bot`（总开关+回复策略）、`push/<scopeID>`（推送订阅）、`seen/<scopeID>`（观察成员）、
`panel/<scope>`（面板 ID 与 items hash）。新增命名空间时集中定义常量。

**SQLite**：`modernc.org/sqlite` 纯 Go，必须保持 `MaxOpenConns(1)`（多连接写会 `database is locked`）；事务短小；`PutMember` 用 `begin immediate` + 删旧事件 + 批量插入。迁移走 `metadata.schema_version` + `store.migrate()` 顺序幂等语句，禁止直接改线上表结构；**不做** Python 数据自动迁移（OpenID 无法对应 QQ 号，用户重新导入 ICS）。

**QQ API 客户端**（`internal/qqapi/`，自研不用 botgo）：
- 被动回复带 `msg_id`，5 分钟窗口内同一 `msg_id` 最多 5 条（`msg_seq` 递增，超限返回明确错误）；主动推送不带 `msg_id` 且必须 `msg.SetInitiative()`。
- 图片/文件一律**公网 URL 上传**拿 `file_info`（配合 `public_base_url` 图床），**不实现分片上传**；禁止使用未文档化的 `file_data` 字段（历史实现依赖它，已失效）。
- 群接口的 `file_info` 只能用于群消息，单聊同理。
- 错误码分类见 `internal/qqapi/errors.go`：频控退避重试，`40034105`（主动消息无权限）暂停该订阅并记录原因。

**渲染**（`internal/render/`，纯函数无副作用，`gg` + 内嵌 Noto 字体）：
文本测量必须用 `render.Measure`/`WrapFit`，禁止直接 `font.MeasureString` 处理中英混排；
**不做彩色 emoji**（按字素簇拆分，缺字形回退单色 emoji 或占位）；
头像必须先按 cover 缩放再裁圆；JPEG quality 80 写 `data/images`，24h TTL 清理，单卡目标 <300ms。

**Web 管理台**（`web/`，`go:embed`，`internal/server/`）：`/admin` + `/api/*`，`admin_password` 为空时仅回环可访问，设置后为 Basic Auth（用户名 `admin`）。
前端不用 `window.AstrBotPluginPage`，统一 `fetch('/api/...')`，错误统一 `{error: "..."}`。
服务端必须重新校验所有输入，前端校验只是体验。公网只放行 `/webhook`、`/healthz`、`/images/*`、`/files/*`。

**日志与错误**：`log/slog` 字段化（`event`/`group`/`user`(脱敏短 ID)/`cmd`/`err`）。请求路径禁止 `panic`；启动配置错误可 `log.Fatal` + 中文指引。用户可见错误必须显式回复（`r.Reply(...)` 或 `UserError`），`Handle` 返回的裸 error 只写日志、用户看不到。

## 代码风格

- 代码注释与日志用**英文**；**用户可见文案用中文**（与 Python 版逐字对齐）；Go 错误消息多为中文。
- 遵循 `gofmt` / `go vet`；导出符号必须有 doc comment；不引入魔法数字（上限常量放各领域包，与 PLAN F1.4 一致）。
- 提交信息：`type(scope): 中文描述`，type ∈ `feat/fix/docs/refactor/test/chore`，如 `feat(schedule): 实现 /课表 日期解析与卡片渲染`。
- 不提交 `config.json`、`data/`、`bin/`（`.gitignore` 已覆盖）。

## 常见坑

| 现象 | 原因 / 处理 |
| --- | --- |
| 平台回调验证失败 | 端口不在 80/443/8080/8443；secret 错误；Op=13 回包缺 `signature`（验签细节见 DEV-GUIDE 5.3） |
| `database is locked` | 连接数 >1，保持 `MaxOpenConns(1)` |
| 时间差 8 小时 | 用了 UTC，统一 `schedule.LocalTZ` |
| `go build` 卡住 | GOPROXY 未设 `https://goproxy.cn,direct` |
| 中文显示方框 | `assets/fonts` 未被 embed 或路径错 |
| 事件重复收到 | 平台会重复推送，按 `payload.id` 幂等（`webhook/dispatch.go`） |
| 群成员列表不可用 | 官方接口为内邀能力；降级方案是 `recordSeenMember` 观察名单 + WebUI「待添加 N 位成员」 |
