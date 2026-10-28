# qqbot-course-schedule 开发手册

> 适用范围：本项目全部 Go 代码、Web 管理台、部署与联调。
> 功能规格：[PLAN.md](PLAN.md)。当前架构：[ARCHITECTURE.md](ARCHITECTURE.md)。
> 本文只描述现在的开发、联调、测试和部署流程。

代码变更必须遵循 [CONTRIBUTING.md](CONTRIBUTING.md)：先核对文档契约，修改代码时同步更新
受影响的文档，验证通过后提交并推送。

---

## 1. 快速开始

```bash
# 1. 拉取代码
git clone git@github.com:mico-v/qqbot-course-schedule.git
cd qqbot-course-schedule

# 2. 依赖代理（本机 proxy.golang.org 不可达，必须设置）
go env -w GOPROXY=https://goproxy.cn,direct

# 3. 拉依赖并构建
go mod download
go build ./...

# 4. 配置
cp config.example.json config.json
$EDITOR config.json

# 5. 运行
go run ./cmd/bot
```

启动成功的关键日志：

```
启动 qqbot-course-schedule version=...
webhook 监听中 addr=127.0.0.1:18080 path=/webhook
指令面板同步完成 created=... updated=...
```

接入步骤见 [CONNECT.md](CONNECT.md)。

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
├── README.md / ARCHITECTURE.md / PLAN.md / DEV-GUIDE.md / CONNECT.md / COVERAGE-AstrBot.md
├── cmd/
│   ├── bot/main.go              # 配置加载、signal、调用 internal/app
│   └── cardpreview/main.go      # 样例卡片预览（-rank 预览榜单）
├── internal/
│   ├── app/                     # 依赖装配、路由、worker、生命周期
│   ├── config/                  # config.json 加载、默认值、校验
│   ├── qqapi/                   # 官方 API 客户端（自研，不用 botgo）
│   │   ├── client.go            #   Token 缓存刷新、请求、重试、flexInt
│   │   ├── message.go           #   文本/markdown（msg_id / event_id 两种被动回复）
│   │   ├── media.go             #   富媒体 URL 上传（业务只使用群接口）
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
│   │   ├── handler.go           #   指令路由、别名、args、观察成员记录
│   │   ├── commands.go          #   指令注册与处理器
│   │   ├── inbound.go           #   Inbound 入站事实、Args/Command、收到时刻
│   │   ├── replier.go           #   被动回复、event_id 回复、5 次计数
│   │   ├── env.go               #   Env 依赖与卡片发送（媒体 / markdown+键盘）
│   │   ├── import.go            #   .ics 附件下载与导入、失败留存
│   │   ├── export.go            #   .ics 导出与公开文件
│   │   ├── avatars.go           #   成员头像缓存与 QQ 头像
│   │   ├── settings.go          #   回复策略与管理员判定
│   │   ├── nickname.go          #   成员展示名
│   │   ├── panel.go             #   指令面板同步
│   │   ├── group_context.go     #   群坐标、roster、观察成员
│   │   ├── menu.go              #   QQ API 历史能力，不在启动路径
│   │   └── cardui.go            #   卡片键盘（日期/榜单切换按钮）
│   ├── store/                   # SQLite v7 适配器与 v6 兼容层
│   │   ├── store.go             #   Open/DSN/Close 与迁移前备份
│   │   ├── schema.go            #   v7 DDL、v6 → v7 迁移与校验
│   │   ├── group_repository.go  #   群、成员、roster、seen
│   │   ├── schedule_repository.go # 课表、事件、revision CAS
│   │   └── *_store.go           #   设置、编辑链接、面板、统计
│   ├── admin/                   # 管理台应用层（无 Gin / SQLite / 渲染依赖）
│   │   ├── service.go           #   显式窄依赖与目录用例
│   │   ├── dto.go               #   HTTP 共用 DTO
│   │   ├── schedule_editor.go   #   管理台课表编辑与 revision
│   │   ├── public_editor.go     #   公开 token 编辑
│   │   ├── day_override.go      #   管理台休假/调休
│   │   ├── transfer.go          #   ZIP/manifest/ICS 导入导出
│   │   ├── settings.go          #   设置用例
│   │   └── backup.go            #   原始备份/恢复与限额
│   ├── schedule/                # 课表领域（纯逻辑，无框架依赖）
│   │   ├── ports.go             #   窄仓储端口
│   │   ├── identity_types.go    #   GroupKey / GroupMemberKey / Revision
│   │   ├── errors.go            #   稳定错误分类
│   │   ├── ics.go               #   VEVENT 解析/序列化、RAW_ICAL、嵌套组件
│   │   ├── encoding.go          #   UTF-8/UTF-16/GBK 解码与内容嗅探
│   │   ├── occurrence.go        #   RRULE/RDATE/EXDATE 展开 + 休假调休
│   │   ├── dayoff.go            #   中文日期/范围解析
│   │   ├── academicyear.go      #   学期（8/1 分界）计算与同季判断
│   │   ├── timerange.go         #   区间表达式解析
│   │   ├── daycard.go           #   每日状态行、收纳拆分、区间合并
│   │   ├── rank.go              #   时长榜口径
│   │   ├── override.go          #   休假/调休目标解析与写入
│   │   ├── panelstore.go        #   面板状态 DTO 与存储接口
│   │   ├── seen.go              #   群成员观察记录（管理台候选来源）
│   │   └── service.go           #   ICS 导入与日卡数据
│   ├── render/
│   │   ├── font.go              #   内嵌字体、字素簇、单色 emoji 回退、富文本测量
│   │   ├── card.go              #   日卡/榜单共用卡片（含图例、键盘无关）
│   │   ├── avatar.go            #   机器人头像缓存 + 成员首字底色
│   │   └── output.go            #   JPEG 输出、TTL 清理
│   └── server/
│       ├── admin.go             #   /admin 页面 + /api/*（会话校验）
│       ├── auth.go              #   /login 登录页 + 会话 Cookie + 登录限流
│       ├── public_schedule.go   #   公开 token 编辑 API
│       ├── overrides.go         #   管理台标记 API
│       ├── settings.go          #   管理台设置 API
│       ├── transfer.go          #   导入/导出 HTTP 绑定
│       ├── images.go            #   公开卡片图床 /images/:name
│       ├── files.go             #   公开导出文件 /files/:name
│       └── errors.go            #   错误分类到 HTTP 的唯一映射
├── tests/                       # 黑盒单元、契约、集成和 E2E 测试
├── web/                         # 管理台前端（Vue 3 + Vite + TS + Element Plus）
│   ├── src/                     #   源码（views/ components/ store/ utils/）
│   ├── dist/                    #   npm run build 产物，入库后 go:embed
│   └── embed.go
├── assets/
│   ├── embed.go                 # go:embed 字体
│   ├── fonts/                   # Noto Sans CJK SC / Noto Emoji + LICENSE
│   └── preview/                 # 卡片预览图（README 引用）
├── deploy/
│   ├── deploy.sh                # 一键编译上传部署
│   ├── qqbot-course-schedule.service
│   ├── qqbot.caddy              # 单域名反代模板（回调 + 管理台同域）
│   └── README.md
├── docs/                        # 仅官方文档快照（autogen/dev-prepare/...）
├── config.example.json
└── go.mod                       # module github.com/mico-v/qqbot-course-schedule
```

上图是当前生产布局。`tests/` 已承担黑盒测试，详见
[ARCHITECTURE.md 测试目录](ARCHITECTURE.md#81-测试目录)；业务源码不再作为新测试的默认归属地。

**依赖规则**（禁止反向依赖）：

```
webhook → bot → schedule
              ↘ qqapi
store   → schedule/admin
admin   → schedule
render 只依赖 schedule 的数据结构
server 依赖 admin/schedule，不依赖 webhook/store
schedule 包内不得 import gin/qqapi/store/admin
admin 包内不得 import gin/sqlite/render/store
```

`bot` 的非测试代码不得 import `store`；`Env` 只暴露 `schedule.PanelStore` 窄接口。
`store.Store` 是适配器，显式实现 `schedule.ServiceStorage` 与 `admin.Storage`；HTTP 路由只拿到
`*admin.Service`，不得直接访问 SQLite。边界由 `scripts/check-architecture.sh` 强制检查。

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
   - 互动事件 `INTERACTION_CREATE`（按钮）
   - 群消息接收开关 `GROUP_MSG_RECEIVE`
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

### 5.4 被动回复

| 场景 | 规则 |
| --- | --- |
| 被动回复 | 携带 `msg_id`（事件 `d.id`），5 分钟内有效，同一 `msg_id` 最多 5 条，`msg_seq` 从 1 递增 |
| 超限 | 客户端计数达 5 时返回明确错误，不静默失败 |

---

## 6. 模块开发指南

### 6.1 新增一条指令

```go
// internal/bot/commands.go 中的新指令示意
func newScheduleCommand(env *Env) *Command {
    return &Command{
        Prefix:      "/课表",
        Description: "查询指定日期课程表",
        Ready:       true,
        Handle:      handleSchedule,
    }
}

func handleSchedule(ctx context.Context, in *Inbound, r *Replier) error {
    query, problem := schedule.ParseDayQuery(
        strings.TrimSpace(in.Args),
        time.Now().In(schedule.LocalTZ),
    )
    if query == nil {
        return r.Reply(ctx, problem)
    }
    target := query.Day
    card, ok, err := env.RenderDayCard(ctx, in, r, target, query.At)
    if err != nil || !ok {
        return r.Reply(ctx, "当前会话还没有可展示的课程表。")
    }
    return env.SendCard(ctx, in, r, card, nil)
}
```

约定：

- `Handle` 返回的 `error` 只写日志；用户可见错误必须显式 `r.Reply`，或返回已分类的领域错误。
- 指令参数用 `in.Args`（`Dispatch` 已剥离命中的前缀），不要自己 `TrimPrefix`。
- 别名在 `Command.Aliases` 声明，会注册到同一处理器；`Commands()` 会去重。`panelOrder` 只决定面板排序，
  新指令标 `Ready: true` 会自动追加进面板（见 6.7），无需手改列表。
- 群消息的成员坐标是 `GroupMemberKey{GroupOpenID: in.GroupOpenID, MemberOpenID: in.UserOpenID}`。
  `in.UserOpenID` 在群事件中承载平台 `member_openid`，它既不是昵称也不是 QQ 号，只有与 `group_openid`
  组合后才构成完整业务身份。@ 目标从 `in.Mentions` 解析，不能按显示名反查身份。
- 两个群即使返回相同字面值的 `member_openid`，也必须是两条存储记录。隔离模式只读当前群；
  全局模式在当前群无有效课表时，可跨群解析同一成员。
- 全局解析优先使用 `union_openid`。缺失或没有匹配时，才按 `member_openid` 查找最近课表。
  `union_openid` 仍不是主键，也不会改变群成员坐标。

### 6.2 新增一个事件

1. 在 `webhook/payload.go` 增加 DTO（只加需要字段，保留 `json.RawMessage` 兜底）。
2. 在 `webhook/dispatch.go` 的 `switch payload.T` 增加分支；未知事件默认忽略。
3. 若是开关类事件（如 `GROUP_MSG_RECEIVE`），只更新对应的窄设置端口，不回复；新代码不得新增通用 KV。
4. 补 `testdata/payload/<event>.json` 与分发测试。

### 6.3 存储使用

目标是让应用层只接收显式身份坐标；下面是目标签名示意，现有 `scope_id`/`user_id` 参数只允许出现在 HTTP
绑定和迁移兼容层，并应立即转换：

```go
key := schedule.GroupMemberKey{
    GroupOpenID:  in.GroupOpenID,
    MemberOpenID: in.UserOpenID,
}

// 管理台读取
page, found, err := adminService.PageSchedule(ctx, key)

// 管理台写入：回传页面读到的 revision，由 admin.Service 调用领域服务
_, err = adminService.SavePageSchedule(ctx, admin.SavePageCommand{
    Key:              key,
    ExpectedRevision: page.Revision,
    Events:           events,
}, "webui")
```

规则：

- HTTP 边界只调用 `admin.Service`；不得直接调用 `store` 或拼接 SQL。
- 事件变更复用 `schedule.Service` / `schedule.SortEvents`，由服务层负责排序、ICS 重建与派生字段。
- 成员课表写操作必须带 `expected_revision`：0 只允许创建，正数执行精确 CAS；冲突在管理台转为 409，
  在聊天侧转为中文提示，禁止裸写。
- 一次业务操作 = 一个事务；不得跨事务拼接。
- 迁移用 `metadata.schema_version` + 顺序脚本，禁止直接改线上表结构。

### 6.4 按钮与互动回调

> 课表卡片默认携带日期切换按钮（前一天 / 今天 / 后一天），交互注册见 6.7。

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

### 6.5 发送消息与媒体

| 类型 | 方式 |
| --- | --- |
| 文本 | `msg_type=0`，`content` |
| Markdown | `msg_type=2`，`markdown.content`；图片用公网 URL（`![alt #Wpx #Hpx](url)`） |
| 图片/文件 | 先上传拿 `file_info`，再 `msg_type=7` |
| 上传方式 | **已定：公网 URL 上传**（配合 `public_base_url` 图床）；不实现分片上传 |
| 注意 | 群接口上传的 `file_info` 只能用于群消息，不能使用单聊上传接口；>20MB 图片会被降级为文件 |

禁止使用未在官方文档出现的 `file_data` 字段（历史实现依赖此字段，已失效）。

### 6.6 Web 管理台

- 前端在 `web/`（Vue 3 + Vite + TypeScript + Element Plus，源码 `web/src/`，构建产物 `web/dist/` 入库），
  `go:embed` 进二进制，经 `/admin` 提供；接口在 `/api/*`（`internal/server/admin.go`）。
- 鉴权：`admin_password` 为空时仅回环地址可访问；设置后走独立登录页 `/login`，登录成功下发
  HttpOnly + SameSite=Lax 会话 Cookie（7 天，内存态，重启失效）；未登录 API 返回 401 JSON，
  页面跳转 `/login`；密码错误次数过多会临时限流。
- 公网域名只放行 `/webhook`、`/healthz`、`/images/*`、`/files/*`（QQ 平台需要拉取卡片图与导出文件）；
  回调与管理台使用同一个域名（如 `kb.example.com`）整站反代，登录页负责管理台鉴权，见 `deploy/qqbot.caddy`。
- 导出的 `.ics` 写在 `data/files`，随机文件名 + 24h 清理；平台错误码分类见 `qqapi/errors.go`。
- 前端不使用 `window.AstrBotPluginPage`，统一 `fetch('/api/...')`，错误统一 `{error: "..."}`。
- 保存流程：读取时拿 `revision` → 提交时回传 → 409 时提示刷新，**不要自动重试覆盖**。
- 设置页提供隔离/全局模式切换。全局模式会为当前群成员补查其最近的非空课表。
- 全局模式下编辑或删除命中的跨群课表时，作用于实际来源课表；空课表不能遮蔽全局课表。
- `/api/schedule/delete` 使用 revision CAS 删除成员课表并保留成员资料；WebUI 删除前必须二次确认。
- 所有输入在服务端重新校验（长度、时间、RRULE），前端校验只是体验。
- 观察成员：`Handler.Dispatch` 对**每条收到的群消息**调用
  `schedule.Service.RecordSeenMemberContext`
  （指令、附件、全量模式下的普通发言都算）；官方群成员列表内邀不可用，这是降级方案。
- 会话列表除"已有课表"的群外，还包含"只有发言记录、还没有课表"的群
  （`groups` / `group_members`），并返回 `pending_count`，管理台显示"待添加 N 位成员"引导建表。
- 成员编辑器可填 QQ 号（可选）：保存进成员记录（随备份/恢复一起走），用于卡片头像；
  输入时用 `q1.qlogo.cn` 做小圆预览，校验 5-11 位数字。
- 休假/调休标记（`internal/server/overrides.go`）：
  - `GET /api/overrides?scope_id=` 列出标记（带成员显示名，`*` 显示为"全体成员"）
  - `POST /api/overrides/set`（`scope_id`/`user_id`/`day`/`kind`/`source_day`）与
    `POST /api/overrides/delete`（`scope_id`/`user_id`/`day`）
  - 服务端复用 `admin.Service.SetWebDayOverride`/`DeleteWebDayOverride`，与机器人指令共享同一张表与校验
    （类型、来源日期、上限），`created_by` 记为 `webui`
- 机器人设置面板含「机器人昵称」：写入 `settings/bot` 的 `nickname`，渲染时由
  `render.DayCardData.BotName` 画在卡片右上角（空值只显示头像）
- 批量导入/导出（工作流在 `internal/admin/transfer.go`，`internal/server/transfer.go` 只做 HTTP 绑定）：
  - `GET /api/export?scope_id=&format=ics|backup`：ICS 压缩包（每成员 `schedule_<OpenID>.ics`
    + `manifest.json`）或原始备份 JSON（成员 + 休假/调休标记）
  - `GET /api/export?scope_id=&user_id=&format=ics`：单成员 `.ics` 直下（`text/calendar`；
    空课表 400、成员不存在 404），对应编辑器里的「导入 .ics / 导出 .ics」按钮
  - `POST /api/import`（multipart：`scope_id`、可选 `user_id`、`file`）：`.zip` 按 manifest 或
    文件名约定导入、`.json` 备份恢复（清空并重建标记）、`.ics` 导入到选中成员
  - 上限：总文件 20 MiB、压缩包条目 ≤ 510、单 ICS 2 MiB；空课表成员不进入 ICS 压缩包

以上 `scope_id`/`user_id` 是现有 HTTP 兼容字段，不是新的领域身份。group scope 的入口必须校验
`scope_id == "group:"+group_openid`，并把 `user_id` 解释为 `member_openid`；不得只凭 `user_id` 读写。

### 6.7 群指令面板

指令集合是单一事实来源：面板从已注册指令生成，不手写列表。实现见 `internal/bot/panel.go`、`internal/qqapi/panel.go`。

```go
// 启动后异步同步；失败只记日志。管理员可用 /同步面板 手动触发。
created, updated, err := bot.SyncPanels(ctx, env, handler)
```

| 规则 | 说明 |
| --- | --- |
| 面板数量 | 只创建 1 个：`scope=group`，`target_type=all`；ID + items hash 存 `bot_panels` |
| 面板元素 | 来自 `panelOrder` 中标记 `Ready` 的指令；`name` ≤ 14 显示列、`desc` ≤ 30 显示列（CJK 算 2） |
| 认领与重建 | 用 `remark=qqbot-course-schedule` 认领旧面板；本地状态丢失时不重复创建；平台侧被删除时重建 |
| 变更检测 | items 的 SHA-256 前 8 字节存 `bot_panels`，未变化不调 PUT |
| 平台行为 | 条目名会去掉前导 `/`，因此 `Dispatch` 同时接受 `/课表` 与 `课表` |
| 失败隔离 | 面板错误不影响启动与消息链路，仅记日志 |

---

## 7. 数据模型与迁移

### 7.1 表结构

旧版 schema v6 使用 `schedule_members(scope_id, user_id, data_json)`、`course_events` 和 `kv_data`。
启动时会自动迁移到 schema v7；旧表保留用于回滚和只读兼容。

当前 schema v7 以 `(group_openid, member_openid)` 为群成员唯一键，并拆出 `groups`、`group_members`、
`group_member_schedules`、`group_member_course_events`、`group_day_overrides`、`bot_settings`、
`schedule_edit_links`、`bot_panels`、`message_stats`、`metadata`。v6 的 `course_events` 保持原名作为兼容快照，
避免建表冲突。
字段、复合外键和迁移规则见
[ARCHITECTURE.md 身份契约](ARCHITECTURE.md#2-身份契约) 与
[SQLite v7](ARCHITECTURE.md#6-sqlite-v7)。

- `member_openid` 是群内用户标识，不是昵称或 QQ 号。
- 同一个 `member_openid` 出现在不同群时必须保存为不同记录。
- `union_openid` 是可空成员属性。隔离模式不参与跨群查找；全局模式仅把它作为精确匹配线索，
  不改变成员主键，也不会产生跨群共享课表行。
- 设置支持 `storage_mode=group`（隔离）和 `storage_mode=global`（全局）。
  `private:*` / `global-user:*` 只作为 v6 兼容数据；新全局模式基于群内成员记录跨群查找。

### 7.2 写入语义

- `PutMember`：`begin immediate` → 按 `(group_openid, member_openid, revision)` CAS 更新成员课表 →
  删除旧事件 → 批量插入新事件 → 提交。
- revision CAS 必须同时带 `group_openid` 与 `member_openid`，禁止只按 `member_openid` 更新；
  `expected_revision=0` 只允许插入，不提供无条件覆盖。
- 可查询字段由服务层规范化后写入结构化列；事件数通过事件表计算，不另存容易漂移的计数列。
  `raw_json` 只保留无法结构化且需无损往返的 iCalendar 数据。
- 覆盖标记读取顺序：先取 `member_openid IS NULL` 的全群行，再用成员行覆盖同 `day`。

### 7.3 迁移策略

- `metadata.schema_version` 当前为 7；v6 → v7 迁移已实现。
- 迁移前备份数据库，迁移在事务中完成，并校验行数、revision、事件数和失败坐标。
- v7 首版保留旧表用于回滚或双读校验，确认稳定后再单独提交删除。
- **不自动导入旧 Python 插件数据**：其数据坐标与当前 Go 版不保证可对应，用户重新导入 ICS 即可；
  当前 Go v6 → schema v7 的结构迁移仍按上述契约自动执行。

---

## 8. 官方 API 客户端规范

| 项 | 规则 |
| --- | --- |
| Token | 启动/过期前 50s 刷新；并发刷新加锁；失败指数退避，最多重试 3 次 |
| 请求头 | `Authorization: QQBot <token>`，`Content-Type: application/json` |
| 域名 | 默认 `api.bot.qq.com`；`domain` 可配置 |
| 超时 | 普通请求 10s；媒体上传 60s；Token 10s |
| 重试 | 网络错误与 5xx 重试（幂等请求）；4xx 不重试，返回结构化错误 |
| 限频 | 识别 `429`/频控错误码，退避后重试 |
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
| 头像 | 机器人：`GET /users/@me` 的 `avatar`；启动时**同步读磁盘缓存**（保证首张卡片就有头像）+ 后台刷新，失败后由渲染路径 `EnsureBotAvatar` 每 5 分钟节流重试。`circleImage` 必须先按 cover 缩放再裁圆（直接 `DrawImage` 只会显示大图中心一块）。成员：官方无头像接口（OpenID，成员信息接口为内邀且无 avatar 字段），因此采用**自愿绑定 QQ 号**（`/绑定QQ` 或 WebUI）→ `q1.qlogo.cn` 公开接口取头像；`Env.MemberAvatars` 并发预取（≤6 并发）、内存 24h / 失败 10min、磁盘 `data/avatars/qq/<qq>.png`，失败或未绑定回退昵称首字 + 稳定底色（openid 哈希）。QQ 号是自报数据，不做真实性校验 |
| 输出 | JPEG quality 80、4:2:0、optimize；写入 `data/images`；>24h 清理 |
| 性能 | 合并连续同字体绘制；单张卡片目标 <300ms（不含上传） |
| 可测试性 | 布局计算与绘制分离：`layout.go` 输出纯数据结构，`draw.go` 只负责画 |
| 预览 | `go run ./cmd/cardpreview -o /tmp/card.jpg` 生成样例卡片；`-avatar path.png` 可带上真实头像，改版式时先看预览 |

---

## 10. 领域逻辑规范

- **时区**：`schedule.LocalTZ = Asia/Shanghai`，全项目唯一；日界 = 本地 00:00（`[start, end)`）。
- **日期解析只有两个入口**：`dayoff`（天/范围，含相对词）与 `timerange`（区间表达式）；新需求必须扩展这两个，不得新写解析器。
- **学期制度**：以本地 8 月 1 日为分界（`academicyear.go`）；不带年份的日期解析到参考日期所在学期，调休的来源/目标必须 `SameAcademicYear`。
- **ICS**：解析保留 `RAW_ICAL` 与未知属性；序列化时复用原文件非 VEVENT 部分。
- **occurrence**：展开使用 `rrule.Between(start, end, true)` 有界展开；应用覆盖后再返回；跨天事件在统计与展示时都按窗口裁剪。
- **course_id**：位置索引，任何写入前必须重排并按 DTSTART 顺序；对外接口只保证"查询结果里的 id 在最近一次写入前有效"。
- **文案**：用户可见文案保持中文、稳定并可测试；修改时同步对应测试和功能规格。

---

## 11. 测试规范

```bash
bash scripts/check-architecture.sh # 依赖边界与重构守卫
gofmt -l .                       # 应为空
go vet ./cmd/... ./internal/... ./tests/... ./web
go test -race -shuffle=on ./internal/... ./tests/...
go test ./internal/schedule -run TestDayoff -v
```

当前黑盒测试目录：

```text
tests/unit/          公开 API 可验证的纯领域测试
tests/contract/      端口契约对 fake 与 SQLite 运行同一套用例
tests/integration/   SQLite、migration、admin、qqapi、schedule
tests/e2e/           入站 payload 到 ReplyPlan/HTTP 的完整链路
tests/testutil/      fake、临时数据库、fixture 工厂
tests/testdata/      输入型 ICS、payload、备份
```

只有必须访问未导出函数的白盒测试保留为同目录 `_whitebox_test.go`。禁止为了搬目录而导出生产符号。

`.github/workflows/ci.yml` 在 push 到 `main` 和所有 PR 上执行 Go 与 Web 两个必需 job。本地使用：

```bash
COVERAGE_STAGE=p2 bash scripts/test-all.sh
RUN_WEB=1 COVERAGE_STAGE=p2 bash scripts/test-all.sh
```

完整门禁见 [ARCHITECTURE.md CI](ARCHITECTURE.md#83-必须通过的检查)。

| 类型 | 要求 |
| --- | --- |
| 表格驱动 | 日期解析、ICS、occurrence、rank、权限矩阵必须表格化 |
| 测试数据 | 输入型 fixture 放 `tests/testdata/`；小型 ICS 可在测试代码内联 |
| 存储 | 临时目录；必须覆盖 revision=0 创建、revision=N 更新、CAS 冲突与取消 |
| HTTP | `httptest` 假官方服务器；覆盖 Token 刷新、重试、409、频控 |
| Webhook | 构造真实签名；覆盖验签失败、Op=13、重复 id |
| 渲染 | 尺寸断言 + 像素抽样；性能守卫用例（20 人卡片） |
| 迁移 | v6 fixture → v7、幂等、坏数据中止、跨群同 `member_openid` 不合并 |
| 身份 | 群模式无 union、union 后补、显示名变化、两群同值 |
| 回归 | 修复线上行为时先补最小失败用例，再修实现 |

覆盖率采用不回退的阶梯门槛，最终 Go 总覆盖率 ≥ 90%；`schedule/store/server/webhook`
≥ 90%，`admin/bot/render` ≥ 85%，`qqapi` ≥ 80%。不允许使用 golden 文件或 `-update` 流程。

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

脚本行为：架构检查 → `gofmt` 检查 → `go test` → 构建 linux/amd64 → 首次自动创建用户/目录并上传 config.json 与 systemd unit → 原子替换二进制 → 重启服务 → 健康检查（失败打印 journalctl）。完整参数见脚本头部与 `deploy/README.md`。

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

- 遵循 `gofmt` / `go vet`；提交前跑 `scripts/test-all.sh`。
- 行为或架构变化必须在同一提交中更新对应文档。
- 检查通过后按 `type(scope): 中文描述` 提交，并推送 `origin/main`。
- 代码注释与日志用英文；用户可见文案与 Go error 用中文。
- 包名小写单词；导出符号必须有 doc comment。
- 配置与常量集中在 `internal/config` 与各领域包，避免魔法数字；上限与 [PLAN.md](PLAN.md#33-上限) 一致。
- 提交信息：`type(scope): 描述`，type ∈ `feat/fix/docs/refactor/test/chore`，描述用中文，例如：

```
feat(schedule): 实现 /课表 日期解析与卡片渲染
fix(render): 修正头像裁圆边界溢出
docs(plan): 补充 ICS 导入失败排查说明
```

- 不提交 `config.json`、`data/`、`bin/`（`.gitignore` 覆盖）。

---

## 15. FAQ / 常见坑

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 平台提示回调验证失败 | 端口不在 80/443/8080/8443；secret 错误；回包缺 `signature` | 对照 5.3；本地先手动构造 Op=13 验证 |
| 发消息返回 `msg_id` 过期 | 超过 5 分钟被动窗口 | 忽略；提示用户重新发送指令 |
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
