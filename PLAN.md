# qqbot-course-schedule 项目计划

> 版本：v1.0（M0–M5 已实现）
> 来源：`astrbot_plugin_CourseSchedule` 全量功能抽象
> 基座：Polarix 框架思路 + QQ 官方机器人 API v2 + Go
> 配套文档：[DEV-GUIDE.md](DEV-GUIDE.md)（开发手册）、[CONNECT.md](CONNECT.md)（接入清单）

---

## 0. 实现进度（2026-09-21）

| 里程碑 | 状态 | 交付 |
| --- | --- | --- |
| M0 骨架 | ✅ 已上线 | webhook 验签 + Op=13、Token、文本收发、`deploy/deploy.sh` |
| M1 数据与图片 | ✅ 已上线 | SQLite 三表 + KV、ICS/RRULE/RDATE/EXDATE、中文日期解析、gg 图片渲染、`/images` 图床、`/导入课表` |
| M2 休假调休 + 榜单 | ✅ 已上线 | `/休假` `/调休` `/销假` `/假期`、`/上课时长榜`（union 口径） |
| M3 Web 管理台 | ✅ 已上线 | `/admin` + 5 个 API + Basic Auth + revision 409 |
| M4 面板/菜单/推送/按钮 | ✅ 已上线 | 指令面板（14 项）、自定义菜单（5 项）、定时推送（默认 07:30）、按钮框架（内邀默认关闭） |
| M5 收尾 | ✅ 已上线 | `/导出课表`、错误码分类、文档与测试补齐 |
| M6 增强 | ✅ 已上线 | WebUI 批量导入/导出：ICS 压缩包（含 manifest）、原始备份 JSON、单文件导入 |
| M7 增强 | ✅ 已上线 | 按会话自定义推送时间（`/推送时间`，调度器每分钟按订阅 cron 触发）、WebUI 休假/调休标记管理 |
| M8 增强 | ✅ 已上线 | 成员页单成员 `.ics` 导入/导出（`/api/export?...&user_id=`，编辑器内一键完成） |
| M9 修复 | ✅ 已上线 | 卡片头像：修复 `circleImage` 未缩放导致大图只显示中心一块；启动同步读缓存 + 渲染路径节流重试，首张卡片即带头像 |
| M10 增强 | ✅ 已上线 | QQ 号绑定（`/绑定QQ` `/解绑QQ` + WebUI 字段）→ 用 qlogo 公开接口取成员真实头像（并发预取 + 内存/磁盘缓存 + 失败回退） |

线上形态：`/opt/qqbot-course-schedule` + systemd（开机自启）+ Caddy（公网只放行
`/webhook` `/healthz` `/images/*` `/files/*`，管理台走 ZeroTier 内网入口）。

**已知平台限制与降级**：自定义按钮为内邀能力（`buttons=false`）；群成员列表为内邀
（管理台批量添加只列"与机器人互动过且无课表"的成员）；主动推送需群管理员在资料页开启
（失败自动暂停订阅）；彩色 emoji 不做（单色回退）。

---

## 1. 项目定位

### 1.1 一句话

把 AstrBot 课表插件的能力完整迁移到一个**独立运行、只依赖 QQ 官方接口**的 Go 机器人上，
不做 AstrBot / OneBot / NapCat 兼容层。

### 1.2 目标

1. 功能对等：插件现有的课程表存储、查询、图片、榜单、休假调休、ICS 导入、Web 管理台全部保留；**课表与榜单展示统一为图片渲染**，不提供文字版课表。
2. 官方接口：事件走 Webhook，消息/媒体/按钮走 QQ 开放平台 v2 HTTP API。
3. 独立部署：单二进制 + SQLite + 静态资源，一条 systemd 服务即可运行。
4. 可演进：服务层与框架解耦，后续可加 HTTP API、更多平台，不接 LLM 工具。
**已定边界（2026-09-19 评审）：渲染用纯 Go 最快方案（不做彩色 emoji）；图片发送走部署后的公网 URL 上传；不做 AI 工具。**

### 1.3 非目标

- 频道 / Guild / 子频道相关能力（用户群体在 QQ 群与单聊）。
- OneBot v11 协议、群文件上传/下载、`get_group_member_list`。
- AstrBot Star 插件形态、AstrBot WebUI Pages 桥接。
- 多机器人实例、分布式部署（单实例设计，必要时再扩展）。

---

## 2. 功能总览

从原插件抽象出 10 个功能域：

| 编号 | 功能域 | 优先级 | 现状（插件） |
| --- | --- | --- | --- |
| F1 | 会话作用域与存储 | P0 | SQLite 三表 + revision 乐观锁 |
| F2 | 成员与课程事件管理 | P0 | create/update/delete + ICS 重建 |
| F3 | 课表查询与当日卡片 | P0 | `/今日课表` `/明日课表` `/课表` + Pillow 渲染 |
| F4 | 上课时长榜 | P1 | `/上课时长榜` + union 去重口径 |
| F5 | 休假 / 调休 | P1 | `/休假` `/调休` `/销假` `/假期` |
| F6 | ICS 导入 / 导出 | P0 | 文件消息自动导入 + `schedule<QQ号>.ics` 约定 |
| F7 | Web 管理台 | P1 | 5 个 Web API + 单页应用 |
| F8 | 对话交互与权限 | P0 | 指令 + @ 提及 + 群成员角色 |
| F9 | ~~AI 查询/编辑工具~~ **不做** | — | 查询/编辑逻辑保留在服务层，仅由指令与 WebUI 使用 |
| F10 | 定时推送与主动消息 | P2 | 插件无，来自 Polarix 定时任务 |

---

## 3. 功能规格（全量抽象）

规格记述方式：
- **行为**：用户可见的输入输出。
- **规则**：边界、顺序、去重、上限。
- **权限**：谁能对谁做什么。
- **验收**：可写成测试或手工验证的条目。

### F1 会话作用域与存储

#### F1.1 作用域

| 项 | 规格 |
| --- | --- |
| 群聊作用域 | `group:<group_openid>` |
| 单聊作用域 | `private:<user_openid>` |
| 成员主键 | 群聊 `member_openid`；单聊 `user_openid`（缺失时退 `union_openid`） |
| 说明 | 官方接口不提供 QQ 号，成员身份只能用 OpenID；展示名来自事件 `author.username`，首次互动落库 |

#### F1.2 数据表（照搬原插件 schema，语义等价）

```
metadata(key, value)                                    -- schema_version
schedule_members(scope_id, user_id, data_json,
                 updated_at, revision)                  -- PK(scope_id,user_id)
course_events(scope_id, user_id, event_index, uid, summary,
              location, description, dtstart, dtend,
              dtstart_tzid, dtend_tzid, rrule, dtstamp,
              event_json)                               -- PK(scope,user,index)
schedule_day_overrides(scope_id, user_id, day, kind,
                       source_day, created_by, created_at) -- PK(scope,user,day)
```

- `schedule_members.data_json`：成员元数据（name、source、event_count、时间戳等）。
- `course_events.event_json`：规范化事件对象（含 `RAW_ICAL`），按 `event_index` 排序。
- `schedule_day_overrides.user_id='*'` 表示"全体成员标记"；读取时**成员标记覆盖全体标记**。

#### F1.3 写入一致性与并发

| 规则 | 说明 |
| --- | --- |
| revision | 每次成功写入 +1 |
| `expected_revision=None` | upsert（新增或覆盖） |
| `expected_revision=0` | 仅允许新增，已存在则冲突 |
| `expected_revision=N` | CAS，版本不符抛冲突 |
| 冲突表现 | 聊天侧返回"课表已更新，请刷新后重试"；WebUI 返回 HTTP 409 |
| 事务 | `BEGIN IMMEDIATE`；事件表整体删除重插；进程内单写锁（Go: `sync.Mutex` + `SetMaxOpenConns(1)`） |
| 存储位置 | 默认 `data/course_schedule.sqlite3`（WAL、`busy_timeout=30s`、`synchronous=NORMAL`） |

#### F1.4 全局上限（沿用插件常量）

| 常量 | 值 | 用途 |
| --- | --- | --- |
| `MAX_ICS_BYTES` | 2 MiB | 单次导入 ICS 大小 |
| `MAX_EVENTS_PER_FILE` | 120 | 单成员事件数；WebUI 单次保存节数 |
| `MAX_MEMBERS_PER_CREATE` | 200 | 批量建课表上限 |
| `MAX_DAY_OVERRIDES_PER_SCOPE` | 1000 | 单会话标记总数 |
| `MAX_DAY_OVERRIDE_RANGE_DAYS` | 180 | 单次 `/休假` 可标记天数 |
| `MAX_DAY_OVERRIDE_SPAN_DAYS` | 366 | 调休来源日与目标日最大间隔 |
| 文本长度 | course/location/name ≤200，description ≤2000，rrule ≤500 | 创建/修改/WebUI 统一校验 |

#### F1.5 时间规则

| 项 | 规格 |
| --- | --- |
| 本地时区 | `Asia/Shanghai`，所有解析/展示/日界都用它（Go 用 `time/tzdata` 内嵌） |
| 事件内部格式 | `DTSTART/DTEND` 归一化为 `YYYYMMDDTHHMMSS`，另存 `*_TZID` |
| 时间戳字段 | `updated_at` 等记账字段用 UTC ISO（`_now_iso`） |
| 日期解析入口 | **只有两个**：`day_off`（按天，支持相对词/范围）与 `time_range`（区间表达式），不得新增第三个 |

### F2 成员与课程事件管理

#### F2.1 事件对象

- 内部规范形：UPPERCASE iCalendar 键（`SUMMARY/DTSTART/DTEND/RRULE/LOCATION/DESCRIPTION/UID/DTSTAMP/*_TZID`）**加** `RAW_ICAL`。
- `RAW_ICAL` 保存原始 VEVENT 文本，用于保留 RDATE/EXDATE 及未建模属性；**任何重建都不得丢弃**。
- `course_id` 是 `events` 数组中 1 起的位置序号，不是稳定 ID；每次写入按 `DTSTART` 重排，位置会变。

#### F2.2 新增（create）

| 项 | 规格 |
| --- | --- |
| 必填 | `course`、`start_time`、`end_time` |
| 校验 | 时间可解析；`end > start`；`rrule` 可解析且含 `FREQ`；文本长度上限 |
| 目标 | 默认发送者；管理员可指定 QQ 号/昵称/@；目标不存在时自动创建成员记录 |
| UID | 传入则复用，否则 `uuid4@astrbot-course-schedule`（Go 版换成 `@qqbot-course-schedule`） |
| 结果文案 | `已新增 <名>(<openid>) 的课程"<课名>"，当前共有 N 个课程事件。` |

#### F2.3 修改（update）

| 项 | 规格 |
| --- | --- |
| 定位 | `course_id`（来自查询结果）；仅改昵称可省略 |
| 留空字段 | 保持原值 |
| 清空字段 | `clear_fields` 支持 `location` / `description` / `rrule`（含中文名） |
| UID | 保持原 UID；`RAW_ICAL` 保留 |
| 结果文案 | `已更新 <名>(<openid>) 的课程 course_id=N，当前共有 N 个课程事件。` |

#### F2.4 删除（delete）

- 按 `course_id` 删除；越界返回 `course_id 必须是有效的课程编号，当前有效范围为 1..N。`
- 结果文案：`已删除 <名>(<openid>) 的课程"<课名>"，剩余 N 个课程事件。`

#### F2.5 昵称

- 首次创建成员时按 `member_name` → 当前 @ 提及昵称 → 发送者昵称 → OpenID 取值。
- 修改昵称不改变成员身份；`member_name` ≤200 字符。
- Go 版新增：每次收到消息若 `author.username` 与库中不一致，可后台刷新（保留人工改名优先级，需在实现时定开关）。

#### F2.6 每次写入后的派生数据

写入事件列表后同步更新：`ics`（完整 ICS 文本）、`schedule`（事件行文本）、`event_count`、
`source="ics"`、`updated_at`、`schedule_updated_at`、`last_modified_by`、`revision`。

### F3 课表查询与当日卡片

> **展示形式（已定）**：所有课表与榜单结果一律渲染为图片发送；文本只用于错误提示、权限拒绝与操作结果。
> 图片经公网 URL 上传（`public_base_url`）；上传失败时返回文本错误提示，**不降级为文字课表**。

#### F3.1 指令

| 指令 | 行为 |
| --- | --- |
| `/今日课表` | 当前会话今日卡片 |
| `/明日课表` | 当前会话明日卡片 |
| `/课表 [日期]` | 指定日期卡片；不带参数等于今天 |

日期写法：`2026-09-17`、`2026/9/17`、`2026年9月17日`、`9.17`、`9月17日`、`前天/昨天/今天/明天/后天/大后天`，带虚词（"明天的课"）也可。
规则：不带年份取离今天最近的一次；一次只能查一天；日期范围或多个日期报错并提示单日写法。

#### F3.2 成员状态机（`daily_member_rows`）

| 状态 key | 条件 | 卡片状态文案 | 倒计时 |
| --- | --- | --- | --- |
| `holiday` | 当天标记休假 | 今日休假 / 当天休假 | 当天课程全部取消 |
| `active` | 今天且在课中 | 正在上课 | 距下课 + 进度条 |
| `upcoming` | 今天未来最近一节 / 未来日期第一节 | 下一节即将上课 | 距上课 |
| `finished` | 有课且目标日 ≤ 今天 | 今日课程已结束 / 当天课程已结束 | 已上完 |
| `none` | 当天无课 | 今日无课 / 当天无课 | 休息日 |

排序：`sort_priority`（active 0 → upcoming 1 → finished 2 → none/holiday 3）→ `sort_time` → 昵称 → OpenID。

#### F3.3 收纳条带（`split_folded_rows`）

- `finished` / `none` / `holiday` 的成员收进卡片下方"没有课的群友"网格，用小头像（首字/emoji 底色）+昵称排列。
- 判断依据是"那天还有没有剩余课程"，与查看哪一天无关；查过去日期时所有人都会收纳。
- 条带标题：当天 `今天已经没有课的群友`，其它日期 `MM-DD 没有课的群友`。
- 条带人数计入副标题总数；图例只列卡片实际出现的状态。

#### F3.4 页面文案

| 元素 | 规则 |
| --- | --- |
| 标题 | `课程表 · YYYY-MM-DD 周X` |
| 副标题（非今天） | `昨天/明天/N 天后 · 共 N 位成员 · M 人有课` |
| 页脚 | 过去日期显示"历史课表"，其余按 `schedule_footer` 规则 |
| 调休提示 | 时间行前缀 `调休 · 按 MM-DD 的课表`；休假显示 `休假 · 无课程安排` |
| 无数据 | `当前会话还没有可展示的今日课程表。` 等 |

#### F3.5 渲染规范（原插件 `render.py`）

- 字体：内置 Noto Sans CJK SC（Regular/Bold）绘制中英数符号；CJK 缺字形时按**字素簇**回退 Noto Color Emoji。
- 富文本测量必须走 `_rich_width/_fit_rich_text/_wrap_rich_text` 等宽高统一函数；emoji 昵称不得破坏布局。
- 绘制时合并连续同字体字素为一次 `draw.text` 调用（性能约束，见原测试）。
- 输出 JPEG（quality 80、4:2:0、optimize），目录 `data/images`，超过 24h 自动清理。
- 头像：
  - **机器人自身**：`GET /users/@me` 返回 `avatar`（`thirdqq.qlogo.cn/g?b=oidb&k=...`），启动时拉取并缓存（内存 + 磁盘，TTL 24h），用于卡片页眉/帮助图；
  - **群成员**：官方群成员接口（列表/详情）为内邀且不返回 `avatar`，消息事件 `author` 也无头像字段，成员头像继续用"首字/首 emoji + 稳定底色"，不发起网络请求；
  - 原实现 `q1.qlogo.cn` 依赖 QQ 号，官方接口不可用。

### F4 上课时长榜

| 项 | 规格 |
| --- | --- |
| 指令 | `/上课时长榜`，别名 `/上课排行` `/本周上课排行` `/学习时长榜` |
| 默认范围 | 本周；支持 今日/明日/昨天/本周/上周/本月/上月/下周/下月/`YYYY-MM-DD..YYYY-MM-DD` |
| 范围上限 | 366 天 |
| 指标 | 默认 `union`：重叠时段只计一次（`merge_intervals`） |
| 裁剪 | 展开后的 occurrence 先裁剪到统计窗口，跨天课程只计窗口内部分 |
| 全天事件 | 只带日期的事件不计入 |
| 展示 | `minutes` 全窗口（榜单稳定），`elapsed_minutes` 已上部分；节数、门数（去重课程名）、相对榜首进度 |
| 名次 | 无课时长不排名；同分钟数并列 |
| 展示数 | 默认前 20 名 |
| 页脚 | `重复课程按 RRULE 展开 · 时间以本地时区为准 · 仅展示前 20 名` |
| 空结果 | `当前会话还没有可统计的课程。` |
| 设计依据 | `astrbot_plugin_CourseSchedule/docs/rank-board-design.md`（历史 bug 清单，移植前必读） |

### F5 休假 / 调休

#### F5.1 指令与日期

| 指令 | 别名 | 语法 |
| --- | --- | --- |
| `/休假` | `/放假` | `/休假 <日期|范围> [成员]` |
| `/调休` | `/补课` `/调课` | `/调休 <被覆盖日期> <来源日期> [成员]` |
| `/销假` | `/取消休假` `/取消调休` | `/销假 <日期|范围> [成员]` |
| `/假期` | `/假期列表` `/调休列表` `/休假列表` | 列出本会话标记（最多 50 条） |

日期支持相对词、`10月1日至10月8日`、`10月1日 .. 10月8日`、`9.17` 等；范围两侧可带空格与填充词
（`10月11日上10月8日的课` 也能解析），解析入口见 `split_day_override_args`。

#### F5.2 语义

| 项 | 规格 |
| --- | --- |
| 休假 | 目标成员当天的全部 occurrence 取消 |
| 调休 | 目标日期的课程替换为来源日期的课程；来源日期本身不受影响；来源日期即使是休假也照样提供课程 |
| 覆盖 | 同一天重复标记直接覆盖，提示"原有标记已被覆盖" |
| 优先级 | 成员个人标记优先于 `*` 全体标记 |
| 过去日期 | 允许标记，提示"包含已过去的日期，只影响查询与统计" |
| 重新导入 ICS | **不清除**标记 |
| 生效范围 | 卡片、`/课表`、时长榜、find 查询、SQL 查询全部一致 |

#### F5.3 权限

| 场景 | 规则 |
| --- | --- |
| 群聊管理员 | 不指定成员 = 全体（`*`）；可 QQ 号/完整昵称/@ 指定个人或 `全体` |
| 群聊普通成员 | 只能标记自己；标记他人报权限错误 |
| 单聊 | 只能标记自己 |
| `/销假` | 普通成员不能取消 `*` 全体标记，提示请管理员操作 |
| 上限 | 超过 `MAX_DAY_OVERRIDES_PER_SCOPE` 拒绝 |

### F6 ICS 导入 / 导出

#### F6.1 导入入口（三条）

1. `/导入课表` + `.ics` 附件；
2. 直接发送 `.ics` 文件消息（自动导入，带单次标记防止重复）；
3. 原插件的 `schedule<QQ号>.ics` 群文件约定（**官方接口不提供 QQ 号，Go 版重设计**：仅支持导入自己，或管理员在 WebUI 指定成员）。

#### F6.2 解析与序列化

| 项 | 规格 |
| --- | --- |
| 解析 | icalendar 解析 VEVENT → 大写键字典 + `RAW_ICAL`；保留 RDATE/EXDATE 等未建模属性 |
| 大小 | 单文件 ≤ 2 MiB；> 120 事件拒绝 |
| 序列化 | 复用原文件的非 VEVENT 部分（`base_ics`），只重建 VEVENT |
| 时区 | TZID 保留；无时区按本地时区处理 |
| 失败文案 | 返回字符串给用户，不抛异常 |

#### F6.3 导出（Go 版新增，可选）

- `/导出课表 [成员]`：把某成员当前事件序列化为 `.ics`，通过富媒体接口以文件消息发送（公网 URL 上传，见第 4 节）。

### F7 Web 管理台

#### F7.1 接口

| 方法 | 路径 | 入参 | 出参 |
| --- | --- | --- | --- |
| GET | `/api/scopes` | — | `{scopes:[{scope_id,kind,target_id,label,member_count,event_count,members:[{user_id,name,event_count,revision}]}]}` |
| GET | `/api/schedule` | `scope_id`,`user_id` | `{scope_id,user_id,name,revision,events:[{id,uid,course,location,description,start,end,rrule}]}` |
| POST | `/api/schedule/save` | `{scope_id,user_id,revision,name?,events[]}` | `{scope_id,user_id,name,revision,event_count}`；revision 不符返回 409 |
| GET | `/api/members` | `scope_id` | `{scope_id,members,member_count}`；平台不支持时返回提示 |
| POST | `/api/schedule/create` | `{scope_id,members:[{user_id,name}]}` | `{scope_id,created,created_count}` |

#### F7.2 页面功能

- 左侧作用域列表（群/私聊）+ 搜索；右侧成员列表与课程编辑器。
- 课程卡片增删改：课程名、开始/结束时间、地点、备注、重复规则。
- 成员昵称编辑；"有未保存修改"提示与离开确认。
- 保存时携带 revision，409 时提示"课表已更新，请刷新后再保存"。
- "批量建课表"：选群 → 拉取未建档成员 → 勾选 → 创建空白课表（官方群成员接口内邀，需降级方案）。

#### F7.3 鉴权

- 独立管理页走 Basic Auth（沿用 Polarix：`admin_password`，未设置仅本机可访问）。
- 管理台页面与图片静态资源使用 `embed.FS` 内嵌。

### F8 对话交互与权限

#### F8.1 指令汇总

| 指令 | 参数 | 说明 |
| --- | --- | --- |
| `/今日课表` `/明日课表` | — | 卡片 |
| `/课表` | `[日期]` | 卡片 |
| `/上课时长榜` | `[范围]` | 榜单卡片 |
| `/休假` `/调休` `/销假` `/假期` | 见 F5 | 标记管理 |
| `/导入课表` | 附件 | ICS 导入 |
| `/导出课表`（新增） | `[成员]` | ICS 导出 |
| `/启用推送` `/关闭推送`（新增） | — | 主动推送开关 |

#### F8.2 输入解析

- 群聊：`GROUP_AT_MESSAGE_CREATE`（@机器人，content 已去除 @ 前缀）；开启全量模式后为 `GROUP_MESSAGE_CREATE`。
- 单聊：`C2C_MESSAGE_CREATE`。
- 指令前缀 + 首个空格后的完整尾巴作为参数（原插件的 `_full_command_tail` 问题在 Go 版不存在，需自定义多参数解析）。
- @ 目标：解析事件 `mentions`（排除机器人与发送者），支持"完整昵称精确匹配 + 当前消息 @ 唯一"两条解析路径。

#### F8.3 权限矩阵

| 操作 | 群管理员 | 群成员 | 单聊 |
| --- | --- | --- | --- |
| 查询/看卡片 | ✅ | ✅ | ✅ |
| 编辑自己课表 | ✅ | ✅ | ✅ |
| 编辑他人课表 | ✅ | ❌ | ❌ |
| 休假/调休自己 | ✅ | ✅ | ✅ |
| 休假/调休他人/全体 | ✅ | ❌ | ❌ |
| 销假全体标记 | ✅ | ❌ | ❌ |
| Web 管理台 | 独立 Basic Auth，与群角色无关 | | |

管理员判定：官方事件 `author.member_role ∈ {admin, owner}`（`owner` 视为管理员）。

#### F8.4 文案约定

- 所有用户可见文案为中文；失败以字符串返回，不抛异常到聊天层。
- 未知成员、越界 course_id、非法日期、超限等均有独立文案（沿用插件措辞）。

#### F8.5 指令面板与自定义菜单（已定，指令面板已上线）

| 项 | 规格 |
| --- | --- |
| 指令面板 | `POST /v2/panels`（10 QPM）：`scope=c2c` 与 `scope=group` 各一个，`target_type=all`；机器人上限 20 个面板、每面板 20 个元素 |
| 面板元素 | 每个指令一个 item：`type=command`，`name` 为点击后填入输入框的指令文本（≤14 显示列，CJK 算 2），`desc` 说明（≤30 显示列） |
| 同步时机 | 启动后异步同步一次；以 `remark=qqbot-course-schedule` 认领旧面板，KV 存 `panel_id + items_hash`，指令集合变化时 PUT 更新，平台侧被删除时重建 |
| 注册范围 | 只注册标记 `Ready` 的已实现指令（当前 5 个）；`/同步面板`（管理员）可手动触发 |
| 自定义菜单 | `PUT /v2/menu`（5 QPM）：仅单聊全局；一级最多 10 项、二级最多 5 项；`send_message` 点击填入输入框，`link` 需 HTTPS（待实现） |
| 失败处理 | 面板/菜单失败只记日志，不影响启动；频控退避重试 |
| 已知平台行为 | 面板条目名会去掉前导 `/`，因此指令匹配同时接受带/不带 `/` 两种写法 |

#### F8.6 消息内嵌按钮（已定）

| 项 | 规格 |
| --- | --- |
| 载体 | 卡片消息附带 `keyboard`（每行最多 5 个、最多 5 行）；`action.type=1` 回调或 `type=2` 指令 |
| 典型按钮 | 日期切换（前一天 / 今天 / 后一天）、榜单范围切换、刷新、导出课表 |
| 回调 | 收到 `INTERACTION_CREATE` 后必须 `PUT /interactions/{id}` 应答（3 秒内、只能一次）；button data 编码 `action:param` |
| 权限 | 日期切换类按钮用 `permission.specify_user_ids` 限定消息接收者 |
| 降级 | 旧客户端不支持时展示 `unsupport_tips` 文案 |

### F9 ~~AI 查询 / 编辑工具~~（不做）

**决策：不向任何 LLM/Agent 暴露工具接口。** 原插件的 `find`（按人/时间/字段查询）与
`edit`（增删改）行为仍保留在服务层 `schedule.Service` 内部，供指令、WebUI 与测试复用；
`course_id`、字段映射、状态枚举等语义继续遵守 F2 与 F9 原有的行为定义（作为内部接口契约）。

### F10 定时推送与主动消息（P2，新增）

| 项 | 规格 |
| --- | --- |
| 用途 | 每天定时向已订阅的群/用户推送当日课表卡片，可选课前提醒 |
| 调度 | 调度器每分钟 tick，逐订阅判断 `cron`（会话级覆盖全局 `push_cron`，5 段表达式） |
| 订阅 | `/启用推送` `/关闭推送` 记录意愿，`/推送时间` 调整时间；平台侧还需群管理员在资料页开启（`GROUP_MSG_RECEIVE` / `C2C_MSG_RECEIVE` 事件） |
| 频控 | 群 20 qpm、单群 1000 条/天、机器人按认证等级 30~60 qpm；失败必须有降级与提示 |
| 目标 | 按 scope 存储（群/单聊），推送时读取最新课表渲染 |
| 错误 | `40034100`/`40034105` 记录并暂停该目标，避免持续失败 |

---

## 4. 官方 API 能力映射

| 功能 | 事件 / 接口 | 要点 |
| --- | --- | --- |
| 指令接收 | `GROUP_AT_MESSAGE_CREATE` / `GROUP_MESSAGE_CREATE` / `C2C_MESSAGE_CREATE` | 需订阅 Intent `GROUP_AND_C2C_EVENT (1<<25)`；相同 `msg_id` 可能重复推送，按 `id` 去重 |
| 被动回复 | `POST /v2/groups/{openid}/messages`、`/v2/users/{openid}/messages` | 5 分钟内、同一 `msg_id` 最多 5 条、`msg_seq` 去重 |
| 主动推送 | 同上，不带 `msg_id` | 需群管理员开启接收；受频控；错误码 `40034100`/`40034105` |
| 卡片图片 | 富媒体上传 + `msg_type=7`，或 Markdown 内嵌公网图片 | **已定：部署后服务器以公网 URL 提供图片，走 URL 上传**（`public_base_url`），不实现分片上传；Markdown 内嵌同理 |
| ICS 文件 | 事件 `attachments[]`（`content_type=file`）下载；导出走 `file_type=4` 上传 | 附件 URL 为临时地址，需实测直连下载 |
| 群信息 | `GET /v2/groups/{openid}/info` | 群名/人数，用于管理台展示 |
| 群成员列表 | `GET /v2/groups/{openid}/members` | **内邀能力**，公开机器人不可用；降级见 F7.2；不返回头像 |
| 机器人详情 | `GET /users/@me` | 机器人自身 `username`/`avatar`（thirdqq.qlogo.cn）；启动拉取并缓存 24h |
| 按钮交互 | `INTERACTION_CREATE` + `PUT /interactions/{id}` | 需 Intent `1<<26`；必须应答且只能一次 |
| 指令面板 | `GET/POST /v2/panels`、`PUT /v2/panels/{id}`、`PUT /v2/panels/{id}/target` | 最多 20 个面板 / 每面板 20 个元素；c2c、group 支持 all/specific；2026-08 新增能力 |
| 自定义菜单 | `GET/PUT /v2/menu` | 仅单聊全局；最多 10 项；send_message 点击填入输入框 |
| 主动接收开关 | `GROUP_MSG_RECEIVE` / `C2C_MSG_RECEIVE` | 用于更新本地订阅状态 |
| 事件订阅变更 | `SUBSCRIBE_MESSAGE_STATUS` | 订阅消息模板授权，P2 可选 |
| 验签 | Webhook Ed25519 + Op=13 回包 | 端口仅 80/443/8080/8443 |
| 域名 | `api.bot.qq.com`（2026-08-10 起统一） | 旧 `api.sgroup.qq.com` 仅作兼容 |

**身份差异（相对原插件）**

| 原插件（OneBot） | Go 版（官方 API） |
| --- | --- |
| QQ 号 | OpenID（群 `member_openid` / 单聊 `user_openid` / `union_openid`） |
| 群成员列表 `get_group_member_list` | 内邀接口，不可用 |
| 头像 `q1.qlogo.cn`（按 QQ 号） | 仅 `/users/@me` 提供机器人自身头像；群成员无头像接口 |
| 群文件上传/下载 | 无；只有富媒体临时文件 |
| `event.is_admin()` | `member_role` |

---

## 5. 技术方案

### 5.1 架构分层

```
cmd/bot            启动装配
internal/qqapi     官方 API 客户端（Token、消息、媒体、互动）
internal/webhook   验签、Payload、事件分发、幂等
internal/bot       上下文、消息构造、按钮、指令注册、权限、定时任务
internal/config    配置加载/保存
internal/store     SQLite（课表三表 + KV）
internal/schedule  领域：ics / occurrences / dayoff / timerange / rank / service
internal/render    卡片渲染
internal/server    Web 管理台 API + 静态资源 + 图床
assets/ fonts、模板
web/    管理台前端
```

### 5.2 技术选型

| 项 | 选择 | 理由 |
| --- | --- | --- |
| HTTP | `gin` | Polarix 同栈，webhook + 管理台复用 |
| 存储 | `modernc.org/sqlite` | 纯 Go 无 CGO，Polarix 同栈 |
| RRULE | `github.com/teambition/rrule-go` | 支持 `between` 有界展开 |
| ICS | 自研极简 VEVENT 解析（保留 RAW_ICAL）+ `arran4/golang-ical` 参考 | 插件语义依赖原始文本 |
| 渲染 | `fogleman/gg` + `x/image/font/opentype` | **已定：追求速度**，纯 Go 同步渲染，无外部进程；彩色 emoji 不做，见 5.3 |
| 字素簇 | `rivo/uniseg` | emoji 昵称排版 |
| 时区 | 标准库 `time/tzdata` | 免宿主 tzdata |
| 静态资源 | 标准库 `embed` | 单二进制部署 |

### 5.3 渲染方案（已定）

选择 `gg` + `x/image/font/opentype` 的纯 Go 同步渲染：单张卡片预计 <100ms，进程内零外部依赖。

| 项 | 决策 |
| --- | --- |
| 彩色 emoji | **不做**。昵称中的 emoji 降级为单色回退（Noto Emoji 变体）或首字占位，颜色不做 |
| 字体 | 只用 Noto Sans CJK SC Regular/Bold（约 33MB，内嵌） |
| 备选 | `chromedp` 已否决（需装 Chromium、启动慢，与“尽可能快”冲突） |
| 头像 | 机器人：`GET /users/@me` 的 `avatar`，启动拉取 + 24h 缓存；成员：无接口，用"首字/首 emoji + 稳定底色"，不发网络请求（快） |

---

## 6. 里程碑

### M0 骨架打通（已完成，待平台联调验收）

- Go module、配置、gin、`/webhook` 验签 + Op=13、Token 获取、发送一条文本消息。
- 已交付：`/ping`、`/help`、M1–M5 占位指令、事件幂等、互动事件应答、单元测试（config/verify/dispatch/bot）、`deploy/deploy.sh` 一键部署脚本（已实测）。
- 验收：QQ 群里 @机器人 得到文本回复；平台回调验证通过（需真实 AppID/域名，见 `CONNECT.md`）。

### M1 数据与图片（已完成，待真机验收）

- store（三表 + revision）、ICS 解析/序列化、occurrence 展开、日期解析。
- 渲染（gg 纯 Go）+ 公网 URL 上传；`/今日课表` `/明日课表` `/课表` 全部输出图片。
- 机器人头像：`GET /users/@me` 拉取并缓存（24h），用于卡片页眉。
- 已交付：`internal/store`（SQLite 三表 + KV + 乐观锁）、`internal/schedule`（ICS/RRULE/RDATE/EXDATE、休假调休展开、中文日期/区间解析、日卡数据）、`internal/render`（内嵌 Noto 字体、卡片/收纳条带/头像）、`/images/:name` 公网图床、`/导入课表` 与附件自动导入、`cmd/cardpreview` 预览工具。
- 验收：导入样例 ICS → 三个指令收到卡片图片；状态/排序与 Python 用例一致；`test_ics/test_ics_import/test_schedule_day` 对应用例通过。

### M2 休假调休 + 时长榜（已完成，待真机验收）

- F5 全部指令与权限（别名、@ 提及解析、管理员默认全体）；F4 榜单（图片输出）。
- 已交付：`internal/schedule/override.go`（目标解析、设置/取消/列表、上限与过去日期提示）、
  `rank.go`（union 去重、窗口裁剪、全天忽略、已上/总时长、并列名次）、
  `/上课时长榜` 及别名图片输出、`cmd/cardpreview -rank` 预览。
- 验收：行为与文案对齐；榜单口径用例（重叠去重、跨天裁剪、全天忽略）通过。

### M3 Web 管理台（已完成，待真机验收）

- 5 个 API + 单页应用改造 + Basic Auth + 409 冲突。
- 已交付：`internal/server/admin.go`（`/admin` 页面与 `/api/*`，Basic Auth，未设密码仅本机可访问）、
  `internal/schedule/web.go`（会话汇总、成员读取/保存、批量建课表、revision 乐观锁、RAW_ICAL 保留、
  观察成员记录）、`web/`（改造自插件 Pages，`fetch` 直连 API）。
- 降级：官方群成员列表内邀不可用，`/api/members` 只返回与机器人互动过且无课表的成员，并在页面提示。
- 验收：浏览器可增删改课程并持久化；并发保存出现 409 提示。

### M4 面板 / 按钮 / 定时推送（已完成）

- [x] F8.5 指令面板（c2c + group 全局面板，启动同步，`/同步面板` 手动触发）
- [x] 自定义菜单 `/v2/menu`（单聊全局，启动同步，5 个 send_message 项）
- [x] F10 定时推送：`push_cron`（默认每天 07:30）+ `/启用推送` `/关闭推送`（群内限管理员），
      按 scope 主动推送当日卡片，无课跳过，失败记日志
- [x] F8.6 卡片按钮与 `INTERACTION_CREATE` 应答：前一天/今天/后一天、本周/上周/本月，
      回调被动回复；**官方自定义按钮为内邀能力**，`buttons=false` 时仍走媒体图片
- 验收：面板可见；到点推送成功；按钮在开通内邀并置 `buttons=true` 后可用。

### M5 收尾（已完成）

- [x] `/导出课表 [成员]`：成员 ICS 导出，写入 `data/files` 并以富媒体文件消息发送（`file_type=4`）
- [x] 错误码处理：`qqapi/errors.go` 分类（主动消息无权限/频控/被动过期/媒体转存/键盘超限），
      推送遇到 40034105 自动暂停该订阅并在下次成功时恢复
- [x] 部署脚本、文档、README 完善；测试补齐（导出/错误码/files 路由/推送暂停）
- 验收：群里 `/导出课表` 收到 .ics 文件；平台错误在聊天里给出可读原因。

**总计约 3~4 周**（单人）。

---

## 7. 测试策略

| 层 | 方式 |
| --- | --- |
| 纯逻辑 | Go 表格驱动测试，逐条翻译 Python 测试（`test_daily_schedule/test_day_override/test_rank/test_ics/test_ics_import/test_schedule_day`） |
| 存储 | 临时目录 SQLite；覆盖 revision 冲突、全体/成员标记合并、级联删除 |
| 渲染 | 尺寸/像素抽样断言 + 人工预览；保留"批量绘制性能"守卫用例 |
| HTTP | `httptest` 假官方服务器：Token 刷新、消息发送、媒体上传、409/重试 |
| Webhook | 构造带签名的 payload，覆盖验签失败、Op=13、重复 `msg_id`、未知事件 |
| 端到端 | 官方沙箱群手工验收（M0/M2/M5） |

**验收总则**：Python 版行为即规格；出现分歧时以"Python 测试 + README 语义"为准，并在 PR 说明。

---

## 8. 风险与开放问题

| # | 风险 | 影响 | 应对 |
| --- | --- | --- | --- |
| R1 | 群成员列表内邀 | 批量建课表不可用 | 降级为"已互动/已导入成员"；提示管理员手动导入 |
| R2 | 无 QQ 号、群成员无头像 | 卡片视觉与原版不一致、数据不可迁移 | 机器人头像用 `/users/@me`；成员用首字/emoji 底色头像；数据迁移靠重新导入 ICS |
| R3 | 主动推送受限 | 定时推送可能失败 | 订阅开关 + 频控退避 + 错误码区分提示 |
| R4 | 本地图片发送 | 需要公网 URL | **已定**：webhook 服务器兼作图床（`public_base_url`），部署后 URL 上传；R2 的“前缀”风险同步降低 |
| R5 | 附件 URL 可下载性未实测 | ICS 导入可能不通 | M0 阶段实测；必要时提示用户改用 WebUI |
| R6 | ~~彩色 emoji 渲染~~ | **已定**：不做彩色 emoji，降级单色/占位 | — |
| R7 | 域名/接口变更 | 调用失败 | 域名配置化；跟进 `docs/changelog.md` |
| R8 | 限频与重试 | 消息丢失 | 客户端统一重试 + 429/5xx 退避 |
| R9 | 许可证 | 开源合规 | 业务源自自有 AGPL 项目可改写；Polarix MIT 需保留声明；字体 OFL |
| R10 | 构建环境无外网 | 依赖拉取失败 | `GOPROXY=https://goproxy.cn,direct`（已验证可用） |
| R11 | 面板/菜单为 2026-08 新能力 | 接口可能调整 | 面板注册失败不影响机器人运行；接口与字段集中在 `qqapi/panel.go` 便于跟进 |
| R12 | 面板配额与限频 | 最多 20 面板 / 20 元素，10 QPM | 只建 2 个面板（c2c/group）；指令变化时合并更新，不重复创建 |

---

## 9. 交付物（已交付）

1. 可执行二进制 + systemd unit + 示例 `config.json` + `deploy/deploy.sh`。
2. 数据库 schema（`internal/store/store.go`，`metadata.schema_version=2`）。
3. 管理台前端（`web/`，内嵌二进制）。
4. 本计划、[DEV-GUIDE.md](DEV-GUIDE.md)、[CONNECT.md](CONNECT.md)、`deploy/README.md`。
5. Go 测试套件（`go test ./...` 覆盖领域逻辑、存储、渲染、指令、面板、推送、管理台、验签）。

---

## 10. 决策记录

| # | 问题 | 结论 |
| --- | --- | --- |
| D1 | 渲染方案 | **已定**：`gg` 纯 Go 最快方案，彩色 emoji 不做 |
| D2 | 图片发送 | **已定**：部署后公网 URL 直接上传（图床），不实现分片上传 |
| D3 | Web 管理台路径与鉴权 | **已定并实现**：`/admin` + `/api/*`，`admin_password` Basic Auth；未设密码仅回环可访问；公网不放行 |
| D4 | AI 工具 | **已定**：不做；查询/编辑保留为内部服务层 |
| D5 | 是否需要历史数据迁移/绑定流程？ | 不做自动迁移，提供"重新导入 ICS"路径 |
| D6 | 定时推送的默认策略 | **已定并实现**：默认关闭，`/启用推送` 后按 `push_cron`（默认 30 7 * * *）推送；会话可用 `/推送时间` 覆盖（调度器每分钟按订阅 cron 触发）；无权限自动暂停 |
| D7 | 许可证 | 建议 MIT（业务代码为自有版权） |
| D8 | 是否需要频道（Guild）支持 | 不需要，明确排除 |
| D9 | 指令注册方式 | **已定**：用官方指令面板（`/v2/panels`，c2c+group 全局）+ 自定义菜单（`/v2/menu`，仅单聊）+ 卡片内嵌按钮；启动时自动同步 |
