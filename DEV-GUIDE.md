# qqbot-course-schedule 开发手册

> 适用范围：本项目全部 Go 代码、Web 管理台、部署与联调。
> 对应计划：[PLAN.md](PLAN.md)。手册描述的是 **M0–M5 已实现**的代码结构与接口；改动代码时请同步本文档。

---

## 1. 快速开始

```bash
# 1. 拉取代码
git clone git@github.com:mico-v/qqbot-course-schedule.git
cd qqbot-course-schedule

# 2. 依赖代理（本机 proxy.golang.org 不可达，必须设置）
go env -w GOPROXY=https://goproxy.cn,direct

# 3. 拉依赖并构建（M0 后）
go mod download
go build ./...

# 4. 配置
cp config.example.json config.json
$EDITOR config.json

# 5. 运行
go run ./cmd/bot
```

启动成功的标志：

```
[INFO] webhook listening on :8080
[INFO] qq access token refreshed, expires in ...
[INFO] registered commands: /今日课表 /明日课表 /课表 /上课时长榜 /休假 /调休 /销假 /假期 /导入课表
```

首次接入 QQ 开放平台的步骤见第 5.1 节。

---

## 2. 环境与工具链

| 项 | 要求 | 说明 |
| --- | --- | --- |
| Go | ≥ 1.25（本机 1.27） | 使用 `embed`、`log/slog`、`time/tzdata` |
| GOPROXY | `https://goproxy.cn,direct` | 本机直连 proxy.golang.org 超时 |
| 公网地址 | HTTPS，端口 80/443/8080/8443 | QQ 平台只允许这四个回调端口 |
| SQLite | 无需安装 | `modernc.org/sqlite` 纯 Go |
| 调试工具 | `curl`、`jq`、`sqlite3`（可选）、`cloudflared`/`frp`（本地联调） | |
| 可选 | 无 | 渲染为纯 Go（已定），不需要 Chromium |

---

## 3. 仓库结构

```
qqbot-course-schedule/
├── README.md / PLAN.md / DEV-GUIDE.md / CONNECT.md / COVERAGE-AstrBot.md
├── cmd/
│   ├── bot/main.go              # 装配：config → store → qqapi → bot → server → scheduler
│   └── cardpreview/main.go      # 样例卡片预览（-rank 预览榜单）
├── internal/
│   ├── config/                  # config.json 加载、默认值、校验
│   ├── qqapi/                   # 官方 API 客户端（自研，不用 botgo）
│   │   ├── client.go            #   Token 缓存刷新、请求、重试、flexInt
│   │   ├── message.go           #   文本/markdown（msg_id / event_id 两种被动回复）
│   │   ├── media.go             #   富媒体 URL 上传（图片/文件，群/单聊）
│   │   ├── keyboard.go          #   内联键盘 DTO（内邀能力）
│   │   ├── menu.go              #   自定义菜单 /v2/menu
│   │   ├── panel.go             #   指令面板 /v2/panels
│   │   ├── user.go              #   机器人详情 /users/@me（头像）
│   │   ├── errors.go            #   错误码分类与中文提示
│   │   └── model.go             #   APIError / SendResult
│   ├── webhook/
│   │   ├── verify.go            #   Ed25519 验签、Op=13 回包
│   │   ├── payload.go           #   事件 Payload DTO
│   │   └── dispatch.go          #   事件分发、msg_id 幂等、互动回调
│   ├── bot/
│   │   ├── handler.go           #   指令路由、别名、msg.Args、观察成员记录
│   │   ├── commands.go          #   全部指令处理器（课表/榜单/休假/推送/面板）
│   │   ├── message.go           #   Message、被动/主动回复、event_id 回复、5 次计数
│   │   ├── env.go               #   Env 依赖与卡片发送（媒体 / markdown+键盘）
│   │   ├── import.go            #   .ics 附件下载与导入、失败留存
│   │   ├── export.go            #   .ics 导出与公开文件
│   │   ├── panel.go             #   指令面板同步
│   │   ├── menu.go              #   自定义菜单同步
│   │   ├── push.go              #   推送订阅、每日推送、卡片键盘
│   │   └── scheduler.go         #   robfig/cron 调度
│   ├── store/store.go           # SQLite：三表 + KV + revision 乐观锁
│   ├── schedule/                # 课表领域（纯逻辑，无框架依赖）
│   │   ├── types.go             #   Event/Member/DayOverride/Storage 接口
│   │   ├── ics.go               #   VEVENT 解析/序列化、RAW_ICAL、嵌套组件
│   │   ├── encoding.go          #   UTF-8/UTF-16/GBK 解码与内容嗅探
│   │   ├── occurrence.go        #   RRULE/RDATE/EXDATE 展开 + 休假调休
│   │   ├── dayoff.go            #   中文日期/范围解析
│   │   ├── timerange.go         #   区间表达式解析
│   │   ├── daycard.go           #   每日状态行、收纳拆分、区间合并
│   │   ├── rank.go              #   时长榜口径
│   │   ├── override.go          #   休假/调休目标解析与写入
│   │   ├── web.go               #   管理台服务层（汇总/读写/建表/观察成员）
│   │   └── service.go           #   ICS 导入与日卡数据
│   ├── render/
│   │   ├── font.go              #   内嵌字体、字素簇、单色 emoji 回退、富文本测量
│   │   ├── card.go              #   日卡/榜单共用卡片（含图例、键盘无关）
│   │   ├── avatar.go            #   机器人头像缓存 + 成员首字底色
│   │   └── output.go            #   JPEG 输出、TTL 清理
│   └── server/
│       ├── admin.go             #   /admin 页面 + /api/*（Basic Auth）
│       ├── images.go            #   公开卡片图床 /images/:name
│       └── files.go             #   公开导出文件 /files/:name
├── web/                         # 管理台前端（改造自插件 Pages，embed）
│   ├── index.html / app.js / style.css
│   └── embed.go
├── assets/
│   ├── embed.go                 # go:embed 字体
│   ├── fonts/                   # Noto Sans CJK SC / Noto Emoji + LICENSE
│   └── preview/                 # 卡片预览图（README 引用）
├── deploy/
│   ├── deploy.sh                # 一键编译上传部署
│   ├── qqbot-course-schedule.service
│   ├── qqbot.caddy              # 公网白名单 + ZeroTier 管理入口模板
│   └── README.md
├── docs/                        # 仅官方文档快照（autogen/dev-prepare/...）
├── config.example.json
└── go.mod                       # module github.com/mico-v/qqbot-course-schedule
```

**依赖规则**（禁止反向依赖）：

```
webhook → bot → schedule → store
              ↘ qqapi
render 只依赖 schedule 的数据结构
server 依赖 store/schedule/render，不依赖 webhook
schedule 包内不得 import gin/qqapi/store
```

---

## 4. 配置

`config.json`（示例见 `config.example.json`）：

| 键 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `port` | int | ✅ | 8080 | HTTP 端口，必须为 80/443/8080/8443 之一 |
| `bind` | string | | 空（所有网卡） | 监听地址；生产建议 `127.0.0.1`，由反向代理对外 |
| `appid` | string | ✅ | — | 机器人 AppID |
| `secret` | string | ✅ | — | AppSecret，同时用于 Webhook 验签密钥派生 |
| `domain` | string | | `https://api.bot.qq.com` | API 域名，可配置 |
| `token_endpoint` | string | | `https://bots.qq.com/app/getAppAccessToken` | 取 Token 地址 |
| `public_base_url` | string | | — | 对外 HTTPS 地址，用于图床与分享链接 |
| `database` | string | | `data/course_schedule.sqlite3` | SQLite 路径 |
| `admin_password` | string | | 空 | 管理台密码；空时仅本机可访问 |
| `push_cron` | string | | `30 7 * * *` | 推送默认时间（5 段 cron，本地时区）；`/启用推送` 后生效，会话可用 `/推送时间` 覆盖 |
| `buttons` | bool | | false | 卡片按钮（markdown+keyboard）；官方为内邀能力，默认关闭 |
| `log_level` | string | | `info` | `debug/info/warn/error` |
| `data_dir` | string | | `data` | 图片、缓存根目录 |

加载规则：

- 启动时读取失败 → 打印明确的中文提示并退出（不要 panic 堆栈）。
- 管理台保存配置时原子写回（临时文件 + rename），保留未知字段。
- 敏感值日志脱敏（`secret`、`admin_password`、Token 只显示前后 4 位）。

---

## 5. 运行与联调

### 5.1 平台配置

1. [QQ 开放平台](https://q.qq.com/) 创建机器人，取得 `AppID` / `AppSecret`。
2. 「开发设置 → 回调配置」填写 `https://<你的域名>:<端口>/webhook`（端口限 80/443/8080/8443）。
3. 勾选事件：
   - 群 @ 消息 `GROUP_AT_MESSAGE_CREATE`
   - 单聊消息 `C2C_MESSAGE_CREATE`
   - 互动事件 `INTERACTION_CREATE`（按钮）
   - 群消息接收开关 `GROUP_MSG_RECEIVE`、单聊 `C2C_MSG_RECEIVE`
   - 全量群消息 `GROUP_MESSAGE_CREATE`（申请通过后）
   - 群成员事件（如平台开放）
4. 保存时平台会发送 `op=13` 验证请求，服务必须正确回包（见 5.3）。
5. 沙箱环境用于联调，正式环境需发布并配置 IP 白名单（如有）。

### 5.2 本地联调

- 用 `cloudflared tunnel --url http://localhost:8080` 得到临时 HTTPS 地址（443 端口），填到平台回调。
- 本地起服务：`go run ./cmd/bot`，观察日志确认收到 `op=13` 与消息事件。
- 事件重放：`testdata/payload/*.json` 保存真实 payload，配合 `cmd/replay`（可选工具）本地重放，无需真机。

### 5.3 Webhook 验签（必须完全对齐官方文档）

| 步骤 | 规则 |
| --- | --- |
| 公钥派生 | `seed = secret`，不足 32 字节则重复拼接，取前 32 字节，`ed25519.GenerateKey(strings.NewReader(seed))` |
| 校验体 | `X-Signature-Timestamp + rawBody` |
| 头 | `X-Signature-Ed25519`、`X-Signature-Timestamp` |
| 失败 | 返回 401，不进入业务 |
| Op=13 | 用同一私钥对 `event_ts + plain_token` 签名，返回 `{plain_token, signature}` |
| 幂等 | `op` 非 13 且事件类型未知 → 直接 200 忽略；业务处理按 `payload.id` 去重 |

### 5.4 被动回复与主动推送

| 场景 | 规则 |
| --- | --- |
| 被动回复 | 携带 `msg_id`（事件 `d.id`），5 分钟内有效，同一 `msg_id` 最多 5 条，`msg_seq` 从 1 递增 |
| 超限 | 客户端计数达 5 时返回明确错误，不静默失败 |
| 主动推送 | 不带 `msg_id`；仅 `GROUP_MSG_RECEIVE`/`C2C_MSG_RECEIVE` 后允许；受 `40034100`（频控）/`40034105`（无权限）限制 |

---

## 6. 模块开发指南

### 6.1 新增一条指令

```go
// internal/bot/command/schedule.go（示意 API，M0 冻结）
func init() {
    command.Register(&command.Command{
        Prefix:      "/课表",
        Role:        role.Member,
        Description: "查询指定日期课程表",
        Handle:      handleSchedule,
    })
}

func handleSchedule(ctx *context.MessageContext) error {
    target, err := dayoff.SingleDay(strings.TrimSpace(ctx.Args), time.Now().In(schedule.LocalTZ))
    if err != nil {
        return ctx.Text(err.Error()).Send()
    }
    path, err := schedule.RenderDayCard(ctx.Scope(), target)
    if err != nil {
        return ctx.Text("当前会话还没有可展示的课程表。").Send()
    }
    return ctx.Image(path).Send() // 内部完成媒体上传
}
```

约定：

- `Handle` 返回的 `error` 只写日志；用户可见错误必须显式 `ctx.Text(...).Send()` 或返回带用户文案的错误类型。
- 指令参数用 `msg.Args`（`Dispatch` 已剥离命中的前缀），不要自己 `TrimPrefix`。
- 别名在 `Command.Aliases` 声明，会注册到同一处理器；`Commands()` 会去重，面板只展示 `panelOrder` 中的主指令。
- 群成员身份用 `msg.UserOpenID`（群内为 member_openid），@ 目标从 `msg.Mentions` 解析。

### 6.2 新增一个事件

1. 在 `webhook/payload.go` 增加 DTO（只加需要字段，保留 `json.RawMessage` 兜底）。
2. 在 `webhook/dispatch.go` 的 `switch payload.T` 增加分支；未知事件默认忽略。
3. 若是开关类事件（如 `GROUP_MSG_RECEIVE`），只更新 KV，不回复。
4. 补 `testdata/payload/<event>.json` 与分发测试。

### 6.3 存储使用

```go
// 读取
member, found, err := store.GetMember(ctx, scopeID, userID)
// 写入（乐观锁）
err = store.PutMember(ctx, scopeID, userID, updated, store.ExpectRevision(member.Revision))
if errors.Is(err, store.ErrConflict) {
    return ctx.Text("课表已更新，请刷新后重试。").Send()
}
```

规则：

- 所有写操作走带 `expected_revision` 的接口；禁止裸写。
- 事件变更必须走服务层 `schedule.Service`（负责排序、ICS 重建、派生字段）。
- 一次业务操作 = 一个事务；不得跨事务拼接。
- 迁移用 `metadata.schema_version` + 顺序脚本，禁止直接改线上表结构。

### 6.4 定时任务

```go
// StartScheduler 校验默认 push_cron 后，注册一个每分钟触发的 tick。
scheduler.AddFunc("* * * * *", func() {
    env.PushDue(ctx, env.now()) // 每个订阅按自己的 cron 判断是否到点
})
```

- 推送时间逐会话存储：订阅里的 `cron` 优先，缺省用全局 `push_cron`；
  `/推送时间 HH:MM`（或 5 段 cron）修改，`/推送时间 默认` 恢复。
- `cronDue(spec, now)` 用 `cron.Schedule.Next(minute-1s)` 判断本分钟是否命中，
  支持任意 5 段表达式；订阅里的 `last_run`（分钟精度）防止重启/重复 tick 二次发送。
- 任务回调在独立 goroutine 执行，注意并发安全（store 已串行化）。
- 主动推送必须 `msg.SetInitiative()`；失败按错误码分类：频控退避、无权限则暂停该目标订阅并记录原因。
- 修改推送时间立即生效，无需重建 cron 条目；重启后以订阅存储为准。

### 6.5 按钮与互动回调

> 课表卡片默认携带日期切换按钮（前一天 / 今天 / 后一天），交互注册见 6.8。

```go
kb := buttons.NewKeyboard()
btn, _ := kb.Append("prev", "前一天", "前一天", buttons.StyleBlue, 0)
btn.SetCallback("day:-1")
msg := ctx.Markdown(content)
msg.Keyboard(kb)
```

- 回调 id 唯一；注册处理函数后，收到 `INTERACTION_CREATE` 必须调用 `PUT /interactions/{id}` 应答（只能一次）。
- 回调处理在 3 秒内完成，超时会使用户端持续 loading。
- 按钮 `permission` 用 `specify_user_ids` 限制为消息接收者（原插件同类场景）。
- 按钮数量限制：每行最多 5 个、最多 5 行；button data 用 `action:param` 编码，参数保持短小。

### 6.6 发送消息与媒体

| 类型 | 方式 |
| --- | --- |
| 文本 | `msg_type=0`，`content` |
| Markdown | `msg_type=2`，`markdown.content`；图片用公网 URL（`![alt #Wpx #Hpx](url)`） |
| 图片/文件 | 先上传拿 `file_info`，再 `msg_type=7` |
| 上传方式 | **已定：公网 URL 上传**（配合 `public_base_url` 图床）；不实现分片上传 |
| 注意 | 群接口上传的 `file_info` 只能用于群消息，单聊同理；>20MB 图片会被降级为文件 |

禁止使用未在官方文档出现的 `file_data` 字段（历史实现依赖此字段，已失效）。

### 6.7 Web 管理台

- 前端在 `web/`，`go:embed` 进二进制，经 `/admin` 提供；接口在 `/api/*`（`internal/server/admin.go`）。
- 鉴权：`admin_password` 为空时仅回环地址可访问；设置后要求 HTTP Basic Auth（用户名 `admin`）。
- 公网只放行 `/webhook`、`/healthz`、`/images/*`、`/files/*`（QQ 平台需要拉取卡片图与导出文件）；
  管理台建议走内网入口（如 ZeroTier）或反代白名单，见 `deploy/qqbot.caddy`。
- 导出的 `.ics` 写在 `data/files`，随机文件名 + 24h 清理；平台错误码分类见 `qqapi/errors.go`。
- 前端不使用 `window.AstrBotPluginPage`，统一 `fetch('/api/...')`，错误统一 `{error: "..."}`。
- 保存流程：读取时拿 `revision` → 提交时回传 → 409 时提示刷新，**不要自动重试覆盖**。
- 所有输入在服务端重新校验（长度、时间、RRULE），前端校验只是体验。
- 观察成员：`Handler.Dispatch` 在指令/附件消息上调用 `Service.RecordSeenMember`，
  管理台用它给"没有课表"的成员建空表；官方群成员列表内邀不可用，这是降级方案。
- 休假/调休标记（`internal/server/overrides.go`）：
  - `GET /api/overrides?scope_id=` 列出标记（带成员显示名，`*` 显示为"全体成员"）
  - `POST /api/overrides/set`（`scope_id`/`user_id`/`day`/`kind`/`source_day`）与
    `POST /api/overrides/delete`（`scope_id`/`user_id`/`day`）
  - 服务端复用 `SetWebDayOverride`/`DeleteWebDayOverride`，与机器人指令共享同一张表与校验
    （类型、来源日期、上限），`created_by` 记为 `webui`
- 批量导入/导出（`internal/server/transfer.go`）：
  - `GET /api/export?scope_id=&format=ics|backup`：ICS 压缩包（每成员 `schedule_<OpenID>.ics`
    + `manifest.json`）或原始备份 JSON（成员 + 休假/调休标记）
  - `POST /api/import`（multipart：`scope_id`、可选 `user_id`、`file`）：`.zip` 按 manifest 或
    文件名约定导入、`.json` 备份恢复（清空并重建标记）、`.ics` 导入到选中成员
  - 上限：总文件 20 MiB、压缩包条目 ≤ 510、单 ICS 2 MiB；空课表成员不进入 ICS 压缩包

### 6.8 指令面板与自定义菜单

指令集合是单一事实来源：面板从已注册指令生成，不手写列表。实现见 `internal/bot/panel.go`、`internal/qqapi/panel.go`。

```go
// 启动后异步同步；失败只记日志。管理员可用 /同步面板 手动触发。
created, updated, err := bot.SyncPanels(ctx, env, handler)
```

| 规则 | 说明 |
| --- | --- |
| 面板数量 | 只创建 2 个：`scope=c2c`、`scope=group`，`target_type=all`；ID + items hash 存 KV `global/panel/<scope>` |
| 面板元素 | 来自 `panelOrder` 中标记 `Ready` 的指令；`name` ≤ 14 显示列、`desc` ≤ 30 显示列（CJK 算 2） |
| 认领与重建 | 用 `remark=qqbot-course-schedule` 认领旧面板；KV 丢失时不重复创建；平台侧被删除时重建 |
| 变更检测 | items 的 SHA-256 前 8 字节存 KV，未变化不调 PUT |
| 平台行为 | 条目名会去掉前导 `/`，因此 `Dispatch` 同时接受 `/课表` 与 `课表` |
| 自定义菜单 | `PUT /v2/menu` 仅单聊全局；一级 ≤10 项、二级 ≤5 项；待实现 |
| 失败隔离 | 面板错误不影响启动与消息链路，仅记日志 |

---

## 7. 数据模型与迁移

### 7.1 表结构

见 [PLAN.md F1.2](PLAN.md#f12-数据表照搬原插件-schema语义等价)。Go 版字段与 Python 版保持一致，
JSON 键名沿用，便于对照与手工排查。

### 7.2 写入语义

- `PutMember`：`begin immediate` → 更新成员行（revision CAS）→ 删除旧事件 → 批量插入新事件 → 提交。
- 派生字段（`ics/schedule/event_count/updated_at/...`）由服务层计算后一并写入。
- 覆盖标记读取顺序：先取 `*` 行，再用成员行覆盖同 `day`。

### 7.3 迁移策略

- `metadata.schema_version` 从 2 起（对齐 Python 版），每次升级递增。
- 提供 `store.Migrate()`，启动时执行；迁移必须幂等。
- **不做** Python 数据自动迁移：OpenID 与 QQ 号无法对应，用户重新导入 ICS 即可。

---

## 8. 官方 API 客户端规范

| 项 | 规则 |
| --- | --- |
| Token | 启动/过期前 50s 刷新；并发刷新加锁；失败指数退避，最多重试 3 次 |
| 请求头 | `Authorization: QQBot <token>`，`Content-Type: application/json` |
| 域名 | 默认 `api.bot.qq.com`；`domain` 可配置 |
| 超时 | 普通请求 10s；媒体上传 60s；Token 10s |
| 重试 | 网络错误与 5xx 重试（幂等请求）；4xx 不重试，返回结构化错误 |
| 限频 | 识别 `429`/频控错误码，退避后重试；主动推送失败要记录目标 |
| 错误结构 | `{code, message, err_code}` 统一解析，错误码表见官方 `dev-prepare/api-call-guide.md` |
| 日志 | 请求打点：method、路径（脱敏 openid）、耗时、结果码；body 仅在 `debug` 级别 |

---

## 9. 渲染规范

| 项 | 规则 |
| --- | --- |
| 入口 | `render.DayCard(rows, opts)` / `render.RankCard(rows, opts)`，纯函数无副作用 |
| 字体 | `assets/fonts/NotoSansCJKsc-{Regular,Bold}.otf`；emoji 仅单色回退，不做彩色 |
| 文本测量 | 必须使用 `render.Measure/WrapFit`，禁止直接 `font.MeasureString` 处理混排 |
| emoji | **已定：不做彩色 emoji**；按字素簇（`uniseg`）拆分，CJK 缺字形时回退单色 emoji 字体或占位 |
| 头像 | 机器人：`GET /users/@me` 的 `avatar`，启动拉取 + 24h 缓存；成员：无接口，用昵称首字/首 emoji + 稳定底色（openid 哈希），不发网络请求 |
| 输出 | JPEG quality 80、4:2:0、optimize；写入 `data/images`；>24h 清理 |
| 性能 | 合并连续同字体绘制；单张卡片目标 <300ms（不含上传） |
| 可测试性 | 布局计算与绘制分离：`layout.go` 输出纯数据结构，`draw.go` 只负责画 |
| 预览 | `go run ./cmd/cardpreview -o /tmp/card.jpg` 生成样例卡片，改版式时先看预览 |

---

## 10. 领域逻辑规范

- **时区**：`schedule.LocalTZ = Asia/Shanghai`，全项目唯一；日界 = 本地 00:00（`[start, end)`）。
- **日期解析只有两个入口**：`dayoff`（天/范围，含相对词）与 `timerange`（区间表达式）；新需求必须扩展这两个，不得新写解析器。
- **ICS**：解析保留 `RAW_ICAL` 与未知属性；序列化时复用原文件非 VEVENT 部分。
- **occurrence**：展开使用 `rrule.Between(start, end, true)` 有界展开；应用覆盖后再返回；跨天事件在统计与展示时都按窗口裁剪。
- **course_id**：位置索引，任何写入前必须重排并按 DTSTART 顺序；对外接口只保证"查询结果里的 id 在最近一次写入前有效"。
- **文案**：所有用户可见文案与 Python 版逐字对齐（便于验收）；修改文案需同步 PLAN 与测试。

---

## 11. 测试规范

```bash
go test ./...                    # 全量
go test ./internal/schedule/... -run TestDayoff -v
go test ./internal/render/... -update   # 更新 golden（仅限有意改版式）
```

| 类型 | 要求 |
| --- | --- |
| 表格驱动 | 日期解析、ICS、occurrence、rank、权限矩阵必须表格化 |
| 测试数据 | `testdata/ics/*.ics` 覆盖 RRULE/RDATE/EXDATE/时区/跨天/全天 |
| 存储 | 临时目录；必须覆盖 revision=0/None/N 三种写入与冲突 |
| HTTP | `httptest` 假官方服务器；覆盖 Token 刷新、重试、409、频控 |
| Webhook | 构造真实签名；覆盖验签失败、Op=13、重复 id |
| 渲染 | 尺寸断言 + 像素抽样；性能守卫用例（20 人卡片） |
| 对齐 | 每条 Python 测试在 Go 侧有对应用例，命名加 `TestPy_` 前缀便于追踪（可选约定） |

覆盖率目标：`internal/schedule` ≥ 85%，`internal/store` ≥ 80%，其余 ≥ 60%。

---

## 12. 日志与错误处理

- 使用 `log/slog`，字段化输出：`event`、`group`、`user`（脱敏）、`cmd`、`cost`、`err`。
- 用户可见错误与内部错误分离：

```go
type UserError struct{ Msg string }
func (e *UserError) Error() string { return e.Msg }
```

- 指令处理函数：`UserError` → 回复用户；其他 error → 回复通用文案并打日志（含堆栈）。
- 禁止在请求路径 `panic`；启动阶段配置错误可以 `log.Fatal` 并给出中文指引。
- 所有外部调用（官方 API、附件下载）都要有超时与错误包装（`fmt.Errorf("...: %w", err)`）。

---

## 13. 构建与部署

### 13.1 一键部署（推荐）

```bash
./deploy/deploy.sh                 # 测试 → 构建 → 上传 → 重启 → 健康检查
./deploy/deploy.sh --skip-tests    # 跳过测试
./deploy/deploy.sh --caddy         # 同时更新 Caddy 反代（需 CADDY_DOMAIN）
HOST=myserver ./deploy/deploy.sh   # 换 SSH 主机
```

脚本行为：`gofmt` 检查 → `go test` → 构建 linux/amd64 → 首次自动创建用户/目录并上传 config.json 与 systemd unit → 原子替换二进制 → 重启服务 → 健康检查（失败打印 journalctl）。完整参数见脚本头部与 `deploy/README.md`。

### 13.2 手动构建

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$(git describe --tags --always)" \
  -o bin/qqbot-course-schedule ./cmd/bot
```

systemd（模板见 `deploy/qqbot-course-schedule.service`）：

```ini
[Unit]
Description=QQ Course Schedule Bot
After=network-online.target

[Service]
WorkingDirectory=/opt/qqbot-course-schedule
ExecStart=/opt/qqbot-course-schedule/bin/qqbot-course-schedule
Restart=always
RestartSec=5
User=qqbot

[Install]
WantedBy=multi-user.target
```

反代（Caddy，模板见 `deploy/qqbot.caddy`；机器人保持 `bind=127.0.0.1`）：

```
bot.example.com {
    reverse_proxy 127.0.0.1:18080
}
```

运维要点：

- 回调必须 HTTPS 且端口合规；证书自动续期。
- 图床 `public_base_url` 指向反代下的 `/images/`，图片只读、不可枚举（随机文件名）。
- 备份：每日 `sqlite3 data/course_schedule.sqlite3 ".backup 'backup/xxx.db'"`；备份前无需停服。
- 清理：`data/images` 24h TTL 由进程内定时任务清理；`data/avatars` 仅缓存机器人自身头像（24h TTL）。
- 升级：替换二进制 + `systemctl restart`；schema 迁移在启动时自动执行。

---

## 14. 代码风格与提交规范

- 遵循 `gofmt` / `go vet`；提交前跑 `go test ./...`。
- 代码注释与日志用英文；用户可见文案用中文（与 Python 版一致）。
- 包名小写单词；导出符号必须有 doc comment。
- 配置与常量集中在 `internal/config` 与各领域包，避免魔法数字；上限常量与 PLAN F1.4 一致。
- 提交信息：`type(scope): 描述`，type ∈ `feat/fix/docs/refactor/test/chore`，描述用中文，例如：

```
feat(schedule): 实现 /课表 日期解析与卡片渲染
fix(qqapi): 修正分片上传 part_index 从 0 开始
docs(plan): 补充主动推送错误码处理
```

- 不提交 `config.json`、`data/`、`bin/`（`.gitignore` 覆盖）。

---

## 15. FAQ / 常见坑

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 平台提示回调验证失败 | 端口不在 80/443/8080/8443；secret 错误；回包缺 `signature` | 对照 5.3；本地先手动构造 Op=13 验证 |
| 发消息返回 `msg_id` 过期 | 超过 5 分钟被动窗口 | 改用主动推送（需接收开关）或忽略 |
| 同一 `msg_id` 回复 5 条后失败 | 被动回复上限 | 合并消息；多余内容走主动或按钮 |
| `40034105 主动消息发送失败，无权限` | 群未开启消息接收 | 记录并提示管理员在机器人资料页开启 |
| 事件重复收到 | 平台为保证可达会重复推送 | 按 `payload.id` 幂等 |
| 图片发送失败 | 用了未文档化的 `file_data` | 改用 URL 上传或分片上传 |
| 群成员列表 403/不可用 | 内邀能力 | 降级：只列出已互动/已建档成员 |
| SQLite `database is locked` | 多连接并发写 | 保持 `MaxOpenConns(1)`，事务短小 |
| 中文显示为方框 | 字体缺失 | 确认 `assets/fonts` 被 `embed` 且路径正确 |
| 时间差 8 小时 | 用了 UTC | 统一 `schedule.LocalTZ`；`_now_iso` 仅记账 |
| `go build` 卡住 | proxy.golang.org 不可达 | `go env -w GOPROXY=https://goproxy.cn,direct` |
| 导入课表提示解析失败 | 文件不是 iCalendar / 编码异常 | 查看 `journalctl -u qqbot-course-schedule`，失败原件留存于 `data/failed_ics/`；支持 UTF-8/GBK/UTF-16 |
