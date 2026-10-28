# qqbot-course-schedule 架构

> 状态：当前实现基线。最后核对：2026-10-27，提交 `6ef3f04`。
> 本文只描述现在有效的架构、契约和验收方式。已完成的迁移过程见
> [REFACTOR-GUIDE.md](REFACTOR-GUIDE.md)，功能规格见 [PLAN.md](PLAN.md)。
> 变更和交付规则见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 1. 产品边界

- 产品作用域只有 QQ 群聊。
- 单聊、频道、Guild、OneBot、NapCat 和 AstrBot 插件形态都不是当前产品目标。
- Webhook 收到 `C2C_MESSAGE_CREATE` 时只 ACK 并忽略，不进入业务处理。
- Web 管理台、公开编辑链接和全部课表读写都围绕群内成员。
- `private:*`、`global-user:*`、`kv_data`、`schedule_members` 和
  `course_events` 保留给 v6 迁移、回滚和只读兼容，不是新业务入口。

## 2. 身份契约

### 2.1 唯一身份

```go
type GroupKey struct {
	GroupOpenID string
}

type GroupMemberKey struct {
	GroupOpenID  string
	MemberOpenID string
}
```

- 群成员身份唯一键：`(group_openid, member_openid)`。
- `member_openid` 是平台签发的群内标识，不是昵称、QQ 号或展示名。
- 两个群即使返回相同字面值的 `member_openid`，也必须保存为两个成员、两份课表。
- `display_name`、`qq`、`union_openid` 都只是 `group_members` 属性。
- `union_openid` 可空。隔离模式不跨群读取；全局模式只在显式开启后用于优先匹配同一成员的跨群课表。
- `scope_id` / `user_id` 只允许出现在 HTTP、Webhook DTO 和迁移边界；进入应用层后
  必须立即转换成 `GroupKey` / `GroupMemberKey`。

实现位置：

- `internal/schedule/identity_types.go`
- `internal/schedule/identity.go`
- `internal/schedule/group_member.go`
- `internal/store/group_repository.go`

### 2.2 不变量

1. 所有新查询和新写入必须带 `group_openid`。
2. 禁止只按 `member_openid` 更新或删除成员课表。
3. 课表、事件、编辑链接、成员标记必须验证成员属于同一群。
4. 删除成员时，课表、事件、编辑链接和成员标记由外键级联清理。
5. 群级标记使用 `group_day_overrides.member_openid IS NULL`，不使用 `*` 伪外键。
6. 全局模式是可切换的读取/编辑策略，不改变 `(group_openid, member_openid)` 主键。

### 2.3 隔离与全局模式

- `storage_mode=group`：默认隔离模式，只读取和写入当前群课表。
- `storage_mode=global`：群内成员没有可用课表时，允许跨群查找该成员已有课表。
- 全局查找顺序：
  1. 当前群已有事件课表。
  2. `union_openid` 精确匹配的跨群课表或 v6 `global-user:<union_openid>`。
  3. 平台未提供 union 时，按相同 `member_openid` 查找最近更新的跨群课表。
- 空课表（0 个事件）不能遮蔽可用的全局课表。
- 全局模式下，WebUI 编辑全局命中课表时写入其实际来源群；来源不存在时才在当前群创建。
- 删除课表时删除当前模式下命中的实际课表记录，保留群成员资料和 seen 状态。

## 3. 运行时与依赖

### 3.1 启动链路

```text
cmd/bot
  → config.Load
  → app.Run
      → store.Open + migrate
      → render.New
      → qqapi.New
      → schedule.NewService
      → bot.Env / bot.Handler
      → webhook.Verify / webhook.Dispatcher
      → gin router
      → HTTP server + workers
```

`cmd/bot` 只负责配置入口、日志级别、signal 和退出码。依赖构造、路由、worker、
HTTP shutdown 和数据库关闭统一由 `internal/app` 管理。

### 3.2 依赖方向

```text
cmd/bot ───────→ app
                   ├─→ config
                   ├─→ store ─────────→ schedule/admin ports
                   ├─→ schedule
                   ├─→ bot ───────────→ schedule
                   ├─→ webhook ───────→ bot
                   ├─→ server ────────→ admin
                   ├─→ render
                   ├─→ qqapi
                   └─→ wakeup

webhook → bot → schedule
server  → admin → schedule
store   → schedule/admin ports
schedule → standard library + rrule/text dependencies
```

禁止：

- `schedule` import gin、SQLite、gg、cron、qqapi、store 或 admin。
- `admin` import gin、SQLite、render 或 store 具体类型。
- `server` import webhook 或 store。
- `bot` 非测试代码 import store。
- `cmd` 直接 import store。
- 生产代码 import `tests/`。

执行 `bash scripts/check-architecture.sh` 检查这些边界。

## 4. 仓库结构

```text
cmd/
  bot/                       进程入口
  cardpreview/               卡片预览工具

internal/
  app/                       装配、路由、worker、生命周期
  admin/                     管理台、公开编辑、导入导出、备份
  bot/                       指令、群成员上下文、回复器、面板
  config/                    配置加载与校验
  qqapi/                    QQ API 客户端
  render/                    纯 Go 图片渲染
  schedule/                  领域模型、端口、课表/ICS/日期/榜单/标记
  server/                    HTTP 绑定、鉴权、错误映射、静态文件
  store/                     SQLite v7 适配器与 v6 兼容层
  timing/                    请求阶段计时
  wakeup/                    Wakeup 分享解析
  webhook/                   验签、Payload、幂等和事件分发

tests/
  unit/                      公开 API 和纯领域测试
  contract/                  端口契约
  integration/               SQLite、迁移、admin、QQ API、schedule
  e2e/                       入站消息到回复链路
  testutil/                  共享测试工具
  testdata/                  输入型 fixture

web/
  src/                       管理台源码
  dist/                      入库构建产物

assets/                      字体与预览资源
deploy/                      部署脚本与 systemd/Caddy 模板
scripts/                     架构、覆盖率和完整测试脚本
docs/                        QQ 官方文档快照
```

### 4.1 `schedule`

`schedule` 是领域与应用核心，不包含 HTTP、SQL 或平台 SDK。

| 文件 | 责任 |
| --- | --- |
| `identity_types.go` | `GroupKey`、`GroupMemberKey`、`Revision`、边界转换 |
| `ports.go` | 成员、课表、标记、设置、编辑链接、统计端口 |
| `errors.go` | validation/conflict/not-found/gone/internal 错误分类 |
| `service.go` | ICS/Wakeup 导入、日卡查询 |
| `repository_helpers.go` | 群坐标到仓储调用的显式转换 |
| `ics.go` / `occurrence.go` | iCalendar、RRULE/RDATE/EXDATE、发生时间 |
| `dayquery.go` / `dayoff.go` / `timerange.go` | 日期、时间、范围解析 |
| `daycard.go` / `rank.go` / `override.go` | 日卡、榜单、休假调休 |
| `settings.go` / `stats.go` / `editlink.go` | 设置、统计、公开编辑 token |

### 4.2 `store`

| 文件 | 责任 |
| --- | --- |
| `store.go` | 打开、DSN、关闭、迁移前备份 |
| `schema.go` | schemaVersion、v7 DDL、v6 → v7 迁移 |
| `group_repository.go` | groups、group_members、roster、seen、目录查询 |
| `schedule_repository.go` | 课表、事件、revision CAS |
| `override_repository.go` | 群级/成员级标记 |
| `settings_store.go` | 单例机器人设置 |
| `edit_link_store.go` | 哈希后的公开编辑 token |
| `kv_state.go` | 面板状态 |
| `stats_repository.go` | append-only 消息统计 |
| `legacy_scope_repository.go` | v6 scope/成员兼容读写 |
| `legacy_kv_repository.go` | v6 `kv_data` 兼容 |
| `member_codec.go` | v6 JSON 编解码 |

### 4.3 `admin`

| 文件 | 责任 |
| --- | --- |
| `service.go` | 显式窄端口、目录和成员创建 |
| `dto.go` | 管理台 HTTP DTO |
| `schedule_editor.go` | 成员课表读取与 revision 写入 |
| `day_override.go` | 管理台休假/调休 |
| `public_editor.go` | token 限定的公开编辑 |
| `transfer.go` | ZIP/manifest/ICS 导入导出 |
| `backup.go` | 原始备份与恢复 |
| `settings.go` | 设置用例转发 |

`admin.Service` 不嵌入 `*schedule.Service`，只持有显式 `domain` 和三个窄仓储接口。

### 4.4 `bot`

`env.go` 只保留依赖容器和媒体发送；`group_context.go` 负责群成员上下文与 roster。
命令注册、回复、卡片、附件、token 和面板分别放在独立文件。

## 5. 端口与错误契约

### 5.1 仓储端口

```go
type GroupMemberRepository interface {
	GetGroupMember(ctx context.Context, key GroupMemberKey) (GroupMember, bool, error)
	ListGroupMembers(ctx context.Context, group GroupKey) ([]GroupMember, error)
	RecordGroupMemberSeen(ctx context.Context, observation GroupMemberObservation) error
	ReplaceGroupRoster(ctx context.Context, group GroupKey, members []GroupMemberObservation) error
}

type GroupScheduleRepository interface {
	GetGroupMemberSchedule(ctx context.Context, key GroupMemberKey) (*Member, bool, error)
	ListGroupSchedules(ctx context.Context, group GroupKey) (map[string]*Member, error)
	PutGroupMemberSchedule(ctx context.Context, key GroupMemberKey, member *Member, expected Revision) (Revision, error)
}

type GlobalScheduleRepository interface {
	FindGlobalSchedule(ctx context.Context, memberOpenID, unionOpenID string, exclude *GroupMemberKey) (*Member, StorageTarget, bool, error)
	DeleteMemberSchedules(ctx context.Context, target StorageTarget, expected Revision, currentKey GroupMemberKey, unionOpenID string, global bool) (int, error)
}
```

其余端口：

- `DayOverrideRepository`
- `GlobalScheduleRepository`
- `SettingsStore`
- `EditLinkStore`
- `StatsRepository`
- `PanelStore`
- `LegacyRepository`

规则：

- 所有仓储 I/O 首参为 `context.Context`。
- 纯日期、ICS、RRULE 函数不机械添加 context。
- 不为不同能力继续扩展一个万能 `Storage`。
- 可选能力必须在构造期显式，不能靠运行时类型断言静默降级。

### 5.2 错误分类

| `ErrorKind` | HTTP | 用途 |
| --- | ---: | --- |
| `validation` | 400 | 参数、日期、格式、长度 |
| `unauthorized` | 401 | 未登录 |
| `forbidden` | 403 | 无权限 |
| `not_found` | 404 | 成员、课表、token 不存在 |
| `conflict` | 409 | revision 或唯一键冲突 |
| `gone` | 410 | 编辑链接过期 |
| `too_many_requests` | 429 | 限流 |
| `internal` | 500 | SQL、文件或未知故障 |

HTTP 响应同时包含 `error` 和稳定 `code`。客户端不应解析 SQL、路径或内部堆栈。
唯一映射点在 `internal/server/errors.go`。

### 5.3 revision

- `expectedRevision = 0`：只允许创建；已存在则 `conflict`。
- `expectedRevision > 0`：执行精确 CAS。
- update 影响 0 行时必须区分“课表不存在”和“revision 冲突”。
- 成功写入后 revision 加 1。
- 新业务不允许 `nil` revision 或无 revision 覆盖写。

## 6. SQLite v7

### 6.1 当前表

| 表 | 主键/关键约束 |
| --- | --- |
| `metadata` | `key`，含 `schema_version=7` |
| `groups` | `group_openid` 主键 |
| `group_members` | `(group_openid, member_openid)` 主键 |
| `group_member_schedules` | 复合主键并引用成员 |
| `group_member_course_events` | `(group, member, event_index)` 复合主键 |
| `group_day_overrides` | 两个部分唯一索引，区分群级与成员级 |
| `bot_settings` | 单例 `id=1`，JSON 校验 |
| `schedule_edit_links` | token SHA-256 主键，复合外键到成员 |
| `bot_panels` | `panel_scope` 主键 |
| `message_stats` | append-only 统计，不设成员外键 |

全局模式查询使用：

- `group_members(union_openid)` 非唯一索引。
- `group_member_schedules(member_openid, schedule_updated_at)` 跨群最近课表索引。
- `group_member_schedules(import_union_openid)` 部分索引。

### 6.2 连接与事务

- `MaxOpenConns(1)`、`MaxIdleConns(1)`。
- DSN 为每条连接设置 `foreign_keys=1`、`busy_timeout=30000`、`_txlock=immediate`。
- 写课表使用单个事务：成员 upsert、CAS、事件删除重插、提交。
- `foreign_keys` 是连接级 PRAGMA，不依赖一次性的启动命令。

### 6.3 v6 → v7 迁移

启动时：

1. 检测旧 schema。
2. 在数据目录创建 `<db>.v6.<timestamp>.bak`。
3. 在事务内创建 v7 表并迁移 `group:*` 数据。
4. 合并 `seen`、`identity`、`roster` 到 `group_members`。
5. 迁移 settings、edit links、panel。
6. 执行 `PRAGMA foreign_key_check`。
7. 写入 `schema_version=7`。
8. 任一步失败回滚；旧表和未知 KV 保留。

线上迁移结果由 `tests/integration/migration` 和实际部署备份共同保护。

## 7. 关键调用链

### 7.1 群消息

```text
POST /webhook
  → Ed25519 verify
  → dispatcher ACK 200
  → payload mapper
  → bot.Handler
  → GroupMemberKey
  → seen/union attribute
  → schedule use case
  → ReplyPlan / Replier
  → QQ API
```

Webhook 先 ACK，再创建带超时的 context 异步处理。重复事件按平台事件 ID 去重。

### 7.2 管理台

```text
/api/schedule
  → session auth
  → admin.Service
  → GroupMemberKey
  → v7 repository
  → DTO

/api/schedule/save
  → session auth
  → validation
  → global/isolated target resolution
  → expectedRevision CAS
  → 200 或 409

/api/schedule/delete
  → session auth
  → target resolution
  → revision-checked deletion
  → remove current/global shadow rows
  → retain member profile
```

### 7.3 公开编辑

```text
token
  → SHA-256 lookup
  → exact GroupMemberKey
  → expiry check
  → schedule read/write
```

公开请求体不包含、也不信任 scope 或 member 覆盖字段。

### 7.4 导入导出

ZIP、manifest、ICS 和 backup 工作流在 `internal/admin/transfer.go`。
`internal/server/transfer.go` 只负责 multipart、响应头和附件输出。

## 8. 测试、覆盖率与 CI

### 8.1 测试目录

- 黑盒测试放 `tests/`。
- 需要未导出状态的测试必须命名为 `_whitebox_test.go`。
- 生产代码不得 import `tests/`。
- 不为了迁移目录而导出生产符号。

### 8.2 当前覆盖率

| 包 | 覆盖率 |
| --- | ---: |
| Go 总计 | 75.1% |
| `internal/admin` | 75.7% |
| `internal/bot` | 68.9% |
| `internal/config` | 94.3% |
| `internal/qqapi` | 70.1% |
| `internal/render` | 79.8% |
| `internal/schedule` | 78.4% |
| `internal/server` | 77.6% |
| `internal/store` | 66.7% |
| `internal/timing` | 100.0% |
| `internal/wakeup` | 91.3% |
| `internal/webhook` | 74.6% |

当前 CI 使用 `COVERAGE_STAGE=p2`，总覆盖率下限 75%。后续门槛只允许上调：

`p2/p3/p5/final` 只是 `scripts/check-coverage.sh` 的门槛档位，不再对应未完成的重构任务。

| 档位 | 总覆盖率 |
| --- | ---: |
| 当前（`p2`） | 75% |
| 下一档 | 80% |
| 发布目标 | 85% |
| 最终 | 90% |

### 8.3 必须通过的检查

```bash
COVERAGE_STAGE=p2 bash scripts/test-all.sh
RUN_WEB=1 COVERAGE_STAGE=p2 bash scripts/test-all.sh
```

Go job：

- `gofmt`
- 架构守卫
- `go vet`
- `go test -race -shuffle=on -count=1`
- 总覆盖率和包级门槛
- `CGO_ENABLED=0 go build`
- `cmd/cardpreview` smoke test

Web job：

- `npm ci`
- ESLint
- Vitest
- `vue-tsc --noEmit`
- Vite build
- `web/dist` 一致性

## 9. 工程守卫

`scripts/check-architecture.sh` 当前检查：

- schedule/admin 禁止基础设施依赖。
- server 不依赖 webhook/store。
- bot 非测试代码不依赖 store。
- cmd 不直接依赖 store。
- 生产代码不 import tests。
- 源码目录只允许 `_whitebox_test.go`。
- 端口首参必须为 `context.Context`。
- 不出现旧 `Message` 和裸 KV 访问。

`scripts/check-coverage.sh` 解析 coverage profile，对重复测试二进制块取最大计数，
防止多包测试导致覆盖率被重复累计。

## 10. 明确不做

- 不拆分 `schedule` 为大量小包。
- 不为 `qqapi.Client` 再包同形接口。
- 不引入 ORM、第二数据库或分布式调度。
- 不把 `web/dist` 改成运行时构建。
- 不恢复单聊、主动推送或频道能力；全局模式必须由设置显式开启。
- 不通过文档中的“未来计划”暗示已完成能力尚未实现。
