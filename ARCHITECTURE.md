# qqbot-course-schedule 架构基线

> 状态：P0-P6 结构改造已完成，覆盖率按阶段继续收紧；最后更新 2026-10-27。
> 范围：Go 服务、Web 管理台、持久化端口、平台边界与工程验收。
> 本文是当前架构和后续重构的首要依据；[REFACTOR-GUIDE.md](REFACTOR-GUIDE.md) 保留历史评估与已完成阶段记录。

## 1. 结论

本项目的基础方向是正确的，不需要推翻顶层目录或重写业务：

- `schedule` 保持为不依赖 Gin、SQLite、渲染库和 QQ API 的领域/用例内核。
- `store` 作为 SQLite 适配器，依赖 `schedule` / `admin` 定义的端口。
- 聊天入口保持 `webhook → bot → schedule`，管理台保持 `server → admin → schedule`。
- 目标产品作用域只有群聊：群内用户身份使用 `(group_openid, member_openid)` 唯一确定。
- `member_openid` 是平台签发的群内用户标识，不是昵称或 QQ 号；显示名统一保存在 `display_name`，不得参与身份判定。
- `union_openid` 只是可空平台属性，不参与默认主键，也不得把两个群的课表自动合并成一份。
- `private:*`、`global-user:*` 与现有全局存储模式只属于 v6 只读兼容，不是 v7 的新业务契约。
- `web/dist` 入库，使 Go 构建不依赖 Node 工具链。
- `MaxOpenConns(1)` 继续用于当前单实例 SQLite 部署。

真正需要治理的是六类“未来改动成本”：

1. **端口过宽且隐含可选能力**：`schedule.Storage` 混合成员、标记、统计和通用 KV；身份解析通过运行时类型断言探测可选接口，漏实现时只会静默降级。
2. **应用层契约不独立**：`admin.Service` 重复定义存储端口，并直接嵌入 `*schedule.Service`，把整个领域导出面变成管理台 API。
3. **函数编排混杂**：指令 handler、卡片生成发送、管理台 ZIP 转换、启动装配都承担了多个层次的责任。
4. **边界契约不统一**：仓储没有 `context.Context`；错误缺少统一分类；部分校验错误返回 500；前端 transport、状态、校验和弹窗混在一个模块。
5. **作用域实现漂移**：目标只要群聊，但现有设置、服务签名和全局 resolver 仍把 private/global-user 当作一等模式，增加身份合并风险。
6. **身份数据与验收不足**：群成员、身份映射、roster 和 seen 数据仍混在 KV；测试与源码同目录且整体覆盖率只有 69.4%；CI 没有 race、覆盖率门禁和前端校验。

优化策略是“先固化身份与测试护栏，再收紧契约、迁移数据库，最后拆文件并收拢装配”，不进行无收益的大规模包迁移。

### 实施状态

| 阶段 | 状态 | 已落地内容 |
| --- | --- | --- |
| P0 | 已完成 | 群复合身份、稳定错误分类、HTTP error code、`tests/` 根目录、race/shuffle/覆盖率 CI |
| P1 | 已完成 | schedule/admin 窄端口、显式依赖、仓储 `context.Context` 与 context 调用路径 |
| P2 | 已完成 | schema v7、迁移备份/回滚、群成员/课表/事件/覆盖/设置/链接/面板实体化 |
| P3 | 已完成 | store/admin/bot/server 关键职责拆分，ZIP/backup 工作流迁入 admin，白盒测试统一命名 |
| P4 | 已完成 | `internal/app` 负责装配、路由、worker 和生命周期，`cmd/bot` 只保留启动 |
| P5 | 已完成 | 前端 client/endpoint/domain/store 拆分与 ESLint/Vitest/typecheck/build 门禁 |
| P6 | 已完成 | cmd/tests/context/白盒测试/裸 KV 架构守卫与文档收口 |

当前 Go 总覆盖率为 75.1%，已达到 P2 门槛；P3/P5/final 的 80%/85%/90% 门槛继续作为后续补测目标。

## 2. 架构原则

### 2.1 依赖方向

```text
cmd/bot ───────→ app
                   ├─→ config
                   ├─→ store ─────────→ schedule/admin ports
                   ├─→ bot ───────────→ schedule
                   ├─→ webhook ───────→ bot
                   ├─→ server ────────→ admin
                   ├─→ render ────────→ schedule display DTO
                   ├─→ qqapi
                   └─→ wakeup

webhook ───────→ bot
server  ───────→ admin
bot     ───────→ schedule
admin   ───────→ schedule
schedule ──────→ Go/rrule/text only
```

允许适配器依赖端口，禁止端口依赖适配器。具体禁止项：

- `schedule` 不得依赖 `gin`、SQLite、`gg`、`cron`、`qqapi`、`store`、`admin`。
- `admin` 不得依赖 HTTP 框架、SQLite、渲染器或 `store` 具体类型。
- `server` 不得依赖 `webhook` 或 `store`。
- `webhook` 只负责平台边界，不包含课程规则。
- `cmd` 不承载业务规则、后台任务实现或路由细节。

### 2.2 分层责任

| 层 | 责任 | 不应承担 |
| --- | --- | --- |
| Transport / Edge | 验签、HTTP/JSON、Cookie、上传下载、平台 DTO 映射 | 领域规则、SQL、卡片布局 |
| Application | 用例校验、身份解析、事务编排、权限策略、错误分类 | Gin 上下文、直接发送 QQ 消息 |
| Domain | 日期、RRULE、课表、休假、榜单、revision 语义 | 网络、数据库、日志框架 |
| Ports | 应用层需要的最小能力与稳定错误语义 | SQL、通用 KV 泄漏、可选魔法 |
| Adapters | SQLite、QQ API、渲染、Wakeup、文件系统 | 业务决策 |
| Composition Root | 构造依赖、注册路由、启动 worker、生命周期 | 用例逻辑 |

### 2.3 契约原则

- 仓储和外部 I/O 方法第一个参数为 `context.Context`。
- 纯领域计算不机械增加 `context` 参数。
- 所有端口按调用方需要收窄，不暴露通用 KV。
- 可失败操作通过包装错误保留底层原因，调用方用 `errors.Is` / `errors.As`。
- HTTP 层集中把错误分类映射为状态码；响应不得泄漏 SQL、路径或内部堆栈。
- 用例返回回复意图或 DTO，发送器负责调用 QQ API。
- “群隔离”“群内复合主键”“乐观锁”是不可破坏的数据不变量，不能藏在文件名约定或拼接字符串里。

## 3. 重构前基线

### 3.1 已验证的优点

| 设计 | 现状 |
| --- | --- |
| 纯领域内核 | `schedule` 只依赖标准库、`rrule-go` 和文本编码库 |
| 存储适配器 | `store.Store` 同时实现当前 `schedule.Storage` 与 `admin.Storage` |
| Web 与存储分离 | `server` 只接收 `admin.Service`，路由不直接访问 SQLite |
| 命令单一来源 | 指令注册同时驱动 help、面板和菜单 |
| revision 乐观锁 | 成员课表写入携带 expected revision，冲突可映射为 409 |
| Webhook 边界 | Ed25519 验签、`op=13`、事件幂等、互动先 ACK 再处理 |
| 前端产物可构建 | `go:embed web/dist`，Go 构建不需要 Node |

### 3.2 重构前结构性债务

| 优先级 | 问题 | 当前证据 | 影响 |
| --- | --- | --- | --- |
| P0 | 错误契约不一致 | `internal/server/settings.go` 把设置校验失败返回 500；多处直接比较错误值 | 客户端无法可靠区分校验、冲突和内部故障 |
| P1 | 存储端口过宽 | `internal/schedule/types.go` 的 `Storage` 同时包含成员、标记、统计和 KV | 测试替身臃肿，修改一个能力影响所有调用方 |
| P1 | 可选能力是隐式的 | `internal/schedule/shared.go` 用类型断言探测 `ListMemberScopes` 与 `ListMembersByImportUnionOpenID` | 适配器少实现能力仍可编译，身份解析静默降级 |
| P1 | 基础设施无取消语义 | 仓储和多数领域服务方法不接收 `context.Context` | 超时、关机、请求取消无法传到 SQLite 操作 |
| P1 | `admin.Service` 暴露面过大 | `internal/admin/service.go` 嵌入 `*schedule.Service` 并重复定义 `Storage` | 管理台可意外调用任意领域方法，端口演进重复 |
| P0 | 群成员身份没有成为一等数据模型 | `group_members` 不存在；身份、roster、seen 分别塞在 `kv_data` 的 JSON 中 | 无法用复合主键和外键约束群内用户，课表可能脱离成员存在 |
| P0 | 测试与覆盖率不足 | 总覆盖率 69.4%；`admin` 54.1%、`store` 55.2%、`qqapi` 43.4%；测试几乎全部与源码同目录 | 高风险适配器和迁移路径缺少等价行为保护 |
| P2 | 大文件职责混合 | `store/store.go` 836 行、`admin/service.go` 821 行、`bot/env.go` 743 行、`server/transfer.go` 348 行 | 阅读、review、测试定位成本高 |
| P2 | 指令 handler 编排较重 | `internal/bot/commands.go` 中 handler 同时做解析、身份解析、领域调用、渲染和发送 | 单个用例难复用，测试必须穿过完整发送链 |
| P3 | 启动装配过重 | `cmd/bot/main.go` 同时做依赖构造、路由、后台任务与优雅退出 | 无法复用装配，测试生命周期和 worker 很困难 |
| P3 | 转换逻辑位于 HTTP 层 | `internal/server/transfer.go` 包含 ZIP/manifest/ICS/备份工作流 | HTTP 路由难以独立测试和复用 |
| P4 | 前端边界混合 | `web/src/api.ts` 重复响应解析；`web/src/store/admin.ts` 混合请求、状态、校验、下载和 Element Plus 弹窗 | 领域状态和交互提示耦合，复用与测试困难 |
| P4 | CI 验收不完整 | `.github/workflows/ci.yml` 只有格式化、架构、vet、普通测试和构建；无 race、覆盖率、前端 typecheck/build/dist 一致性检查 | 并发问题、覆盖回退和已入库前端产物漂移无法被阻止 |

## 4. 目标模块

### 4.1 顶层结构

保留现有顶层目录，新增一个装配包：

```text
cmd/bot/                   进程入口、version、退出码
cmd/cardpreview/           本地卡片预览工具
internal/app/              依赖装配、路由注册、worker、生命周期
internal/schedule/         领域规则、应用用例、端口、稳定错误
internal/admin/            管理台与公开编辑链接应用层
internal/bot/              聊天应用层、回复计划、QQ 平台编排
internal/webhook/          验签、Payload、幂等、DTO 映射
internal/server/           HTTP 绑定、鉴权、错误映射、响应
internal/store/            SQLite 适配器与迁移
internal/qqapi/            QQ API 基础设施
internal/render/           卡片与图片基础设施
internal/config/           配置加载与校验
internal/timing/           请求阶段计时
internal/wakeup/           Wakeup 获取与解析适配器
tests/                     跨包契约、集成、E2E 与测试工具
web/src/                   管理台源码
web/dist/                  入库构建产物
assets/                    字体与预览图
deploy/                    部署文件
scripts/                   架构与 CI 辅助
docs/                      QQ 官方文档快照
```

### 4.2 模块职责

| 模块 | 输入 | 输出 | 核心职责 |
| --- | --- | --- | --- |
| `app` | config、version、进程信号 | 运行中的服务 | 构造依赖、注册路由、启动/停止 worker |
| `webhook` | HTTP 请求、平台 Payload | `bot.Inbound` 或 `bot.Callback` | 验签、去重、ACK、异步处理、DTO 转换 |
| `bot` | Inbound、Callback、设置 | 回复计划/平台发送 | 指令策略、聊天用例、卡片响应编排 |
| `schedule` | 用例命令、端口 | 领域结果/卡片 DTO | 课表、RRULE、身份坐标、休假、榜单、revision |
| `admin` | 管理台/公开编辑命令 | 管理台 DTO | 管理台用例、公开 token 授权、批量工作流 |
| `server` | HTTP/JSON/表单 | HTTP 响应 | 参数绑定、鉴权、错误映射、文件响应 |
| `store` | 端口调用、SQL | 领域对象/分页/统计 | SQLite 事务、索引、迁移、乐观锁 |
| `render` | 展示 DTO、头像、字体 | JPEG/尺寸 | 像素布局，不读取业务存储 |
| `qqapi` | 发送/平台 API 参数 | 平台响应/错误 | Token、HTTP、重试、媒体上传 |
| `config` | `config.json`、环境 | 配置对象 | 默认值、解析、基本合法性 |
| `tests` | 公开包、真实 SQLite、`httptest`、假平台 | 测试结果/覆盖率 | 契约、集成、迁移、E2E；不进入生产构建 |

### 4.3 包内目标文件树

这些是职责拆分目标，不要求一次完成。优先做“包内拆文件”，暂不创建大量新包。

```text
internal/app/
  app.go                     Run、App 结构、入口
  build.go                   依赖构造
  routes.go                  /healthz、/webhook、/api、静态资源
  workers.go                 头像、面板、菜单同步
  lifecycle.go               signal、Shutdown、WaitGroup

internal/schedule/
  ports.go                   窄端口与 Dependencies
  errors.go                  稳定错误类别和 sentinel
  service.go                 用例服务构造与依赖校验
  group_member.go            GroupMemberKey、群成员身份与显示名
  import.go                  ICS/Wakeup 导入用例
  daycard.go                 日卡 DTO 与构建
  identity.go                群内身份与可选 union 关联
  storage_target.go          群内坐标与旧 scope 兼容适配
  day_override.go            休假/调休用例
  settings.go                设置读写与规范化
  stats.go                   统计端口调用与聚合
  edit_link.go               编辑 token 生命周期
  roster.go                  群成员快照
  seen.go                    已观察成员
  ics.go / occurrence.go     纯领域

internal/admin/
  service.go                 Service 结构与显式依赖
  schedule.go                成员课表读取/保存
  override.go                管理台标记用例
  public_editor.go           token 授权与公开编辑
  directory.go               scope/member/pending 查询
  transfer.go                ZIP/ICS/backup 工作流
  dto.go                     HTTP 共用 DTO
  backup.go                  原始备份

internal/bot/
  env.go                     依赖容器与构造器
  handler.go                 路由、开关、策略、附件分流
  inbound.go                 入站事实
  replier.go                 纯发送与被动回复计数
  response.go                文本/图片/文件/键盘回复计划
  usecase_schedule.go        课表查询编排
  usecase_override.go        休假/调休编排
  usecase_import.go          ICS/Wakeup 导入编排
  usecase_rank.go            榜单编排
  identity.go                身份补查
  roster.go                  群成员同步
  card.go                    日卡/榜单渲染与媒体准备
  media.go                   图片/文件落盘和 URL
  commands.go                只做注册
  command_*.go               各功能到用例的薄适配
  panel.go / menu.go         平台管理

internal/store/
  store.go                   Open、Close、PRAGMA、迁移入口
  schema.go                  schemaVersion 与版本化迁移注册
  migrations/v7_identity.go  群成员与规范化 scope 迁移
  group_repository.go        群、群成员、roster、seen
  schedule_repository.go     课表、事件、revision CAS
  override_repository.go     休假/调休
  stats_repository.go        统计
  settings_store.go          设置
  edit_link_store.go         编辑 token
  panel_store.go             面板状态
  legacy_scope_adapter.go    v6 private/global-user 只读兼容
  legacy_kv_adapter.go       迁移期未知 KV 只读兼容
  codec.go                   原始事件与兼容数据编解码

internal/server/
  router.go                  路由装配
  admin.go                   管理台页面与 session API
  public_schedule.go         公开编辑 API
  auth.go                    登录、会话、限流
  errors.go                  错误分类到 HTTP 的唯一映射
  transfer.go                只绑定/响应文件传输
  images.go / files.go       静态公开文件

tests/
  unit/schedule/             纯领域公开 API 测试
  contract/store/            仓储端口对 fake 与 SQLite 的同一行为套件
  integration/migration/     v6 → v7 数据迁移、回滚、幂等
  integration/admin/         管理台用例与 SQLite
  integration/server/        HTTP 状态码、鉴权、错误 envelope、文件
  integration/webhook/       验签、ACK、幂等、异步边界
  e2e/chat/                  入站事件 → 用例 → 回复计划的完整链路
  e2e/admin/                 登录 → 读取/保存 → revision 冲突
  testutil/                  fake、SQLite、fixture 工厂、脱敏断言
  testdata/                  ICS、备份、平台 payload
```

## 5. 契约设计

### 5.1 持久化端口

不要继续向一个 `Storage` 增加方法。由具体用例组合需要的端口：

```go
type GroupKey struct {
	GroupOpenID string
}

type GroupMemberKey struct {
	GroupOpenID  string
	MemberOpenID string
}

type Revision int64

// LegacyScopeRef is accepted only at HTTP and migration boundaries.
// New ports use GroupKey / GroupMemberKey and never infer identity from it.
type LegacyScopeRef struct {
	Kind string // group / private / global-user
	ID   string
}

type GroupMemberRepository interface {
	GetGroupMember(ctx context.Context, key GroupMemberKey) (GroupMember, bool, error)
	ListGroupMembers(ctx context.Context, group GroupKey) ([]GroupMember, error)
	RecordGroupMemberSeen(ctx context.Context, member GroupMemberObservation) error
	ReplaceGroupRoster(ctx context.Context, group GroupKey, members []GroupMemberObservation) error
}

type GroupScheduleRepository interface {
	GetMember(ctx context.Context, key GroupMemberKey) (*Member, bool, error)
	ListGroupSchedules(ctx context.Context, group GroupKey) (map[string]*Member, error)
	PutMember(
		ctx context.Context,
		key GroupMemberKey,
		member *Member,
		expectedRevision Revision,
	) (Revision, error)
}

type DayOverrideRepository interface {
	ListDayOverrides(ctx context.Context, group GroupKey) ([]DayOverrideRow, error)
	SetDayOverride(ctx context.Context, row DayOverrideRow) error
	DeleteDayOverride(ctx context.Context, group GroupKey, memberOpenID, day string) (bool, error)
}

type StatsRepository interface {
	InsertMessageStats(ctx context.Context, record MessageStats) error
	ListMessageStats(ctx context.Context, since time.Time, group *GroupKey) ([]MessageStats, error)
	PruneMessageStats(ctx context.Context, before time.Time) (int, error)
}
```

其余窄端口：

- `SettingsStore`：读取/保存 `Settings`。
- `GroupMemberRepository`：以 `(group_openid, member_openid)` 读写群成员、显示名、可选 union 关联、seen 和 roster 状态。
- `EditLinkStore`：保存、读取、删除哈希后的编辑 token。
- `PanelStore`：面板 ID 与内容指纹。

关键规则：

- `member_openid` 是群内用户主键，不是昵称；绝不解析为 QQ 号。
- 两个群的 `member_openid` 即使字面相同，也是两条不同的群成员记录，禁止按该字段单独合并。
- `union_openid` 是可空属性；群模式不要求它存在，新端口不提供“按 union 找所有群课表”的能力。
- 旧 private/global-user 数据由独立、只读的 compatibility adapter 读取；该适配器不属于群聊用例依赖，也不能被新业务代码调用。
- `scope_id` / `user_id` 只作为旧 HTTP 请求和 v6 数据迁移的兼容字段；进入应用层后立即转换为 `GroupKey` / `GroupMemberKey`。
- 组合根应在构造时校验依赖；不满足时启动失败，不做运行时类型断言后静默降级。
- KV 仅保留为 SQLite 内部实现细节。新的应用用例不得依赖通用 `GetKV` / `SetKV`。
- `store.Store` 可同时实现多个端口，但每个用例只接收自己需要的接口。
- `PutMember` 的 key 是唯一权威坐标；payload 中不得出现另一个可覆盖它的 `user_id`。
- `expectedRevision=0` 表示“仅允许创建”，大于 0 表示“仅允许在 revision 精确匹配时更新”。不允许 `nil` 或无
  revision 的覆盖写。
- `ListGroupSchedules` 返回的 map key 是 `MemberOpenID`，只在给定 `GroupOpenID` 内有效，不得跨群合并。

### 5.2 context 契约

- HTTP/Webhook 请求上下文传到用例，再传到仓储和外部 I/O。
- Webhook 先 ACK 后可脱离原请求上下文，但必须创建带超时的新 context。
- 后台任务从 `app` 接收根 context，使用 `WaitGroup` 等待退出。
- 纯函数如 `ParseDayQuery`、`ExpandOccurrences`、`SplitFoldedRows` 不需要 context。
- 已取消的 context 不得继续启动新的外部调用。

目标签名示例：

```go
func (s *Service) BuildDayCard(
	ctx context.Context,
	member GroupMemberKey,
	selected, now, at time.Time,
) (*DayCard, bool, error)
```

### 5.3 错误契约

内部统一分类，HTTP 边界统一映射：

| 类别 | 含义 | HTTP |
| --- | --- | --- |
| `validation` | 参数、日期、格式、范围不合法 | 400 |
| `unauthorized` | 未登录、token 无效 | 401 |
| `forbidden` | 已认证但无权操作 | 403 |
| `not_found` | 成员、课表、scope、token 不存在 | 404 |
| `conflict` | revision/乐观锁冲突 | 409 |
| `gone` | 编辑链接已过期 | 410 |
| `too_many_requests` | 登录限流等 | 429，附 `Retry-After` |
| `internal` | SQL、文件、意外故障 | 500，对外使用通用文案 |

推荐响应保持兼容并增加稳定代码：

```json
{
  "error": "课表已被其他操作更新，请刷新后重试。",
  "code": "conflict"
}
```

实现约束：

- 领域 sentinel 错误通过 `%w` 保留，调用方使用 `errors.Is`，禁止 `err == Err...`。
- 适配器错误补充操作上下文，例如“保存成员课表失败”，同时保留 cause。
- `server/errors.go` 是 HTTP 状态码的唯一决策点。
- `internal` 错误只进日志；客户端只收到安全文案。
- `schedule.ErrConflict` 继续作为乐观锁冲突 sentinel，但应被归类为 `conflict`。
- Webhook 明确返回 200 的已接收/忽略分支属于平台协议例外，应在 dispatcher 注释和测试中固定。

### 5.4 身份与存储不变量

身份字段的职责必须固定：

| 字段 | 含义 | 使用边界 |
| --- | --- | --- |
| `group_openid` | 平台签发的群会话标识 | 群数据隔离键；原样保存 |
| `member_openid` | 平台签发的群内成员标识 | 与 `group_openid` 组成成员主键；不是显示名 |
| `display_name` | 成员昵称或管理员设置的展示名 | 只用于展示；变化不影响身份 |
| `union_openid` | 可空的平台属性 | 只作为成员属性保存，不用于自动合并不同群课表 |
| `qq` | 可选手工绑定的 QQ 号 | 仅用于头像等附加能力；不是身份主键 |

不可破坏的规则：

1. 群成员身份和群课表统一使用 `GroupMemberKey{GroupOpenID, MemberOpenID}`。
2. `member_openid` 是平台签发的群内用户标识，不是昵称、QQ 号或展示名。昵称只存 `display_name`，不得参与身份判定。
3. `group_openid` 与 `member_openid` 都是不透明字符串，必须原样保存，不拆分、不大小写转换、不尝试反解。
4. 即使两个群返回相同的 `member_openid` 字面值，也必须保存为两条独立成员和课表记录；禁止按 `member_openid` 单独合并。
5. 当前产品契约以群消息为主。新功能不得从私聊或未带 `group_openid` 的消息推导群成员身份。
6. `union_openid` 是可空属性。群模式的所有读取、写入、导入、渲染和管理台操作都不得要求平台返回它。
7. `union_openid` 不得作为新业务 API 的默认查找键，也不得据此自动合并不同群的课表。跨群共享若未来重新立项，必须另写 ADR 和数据迁移，不能沿用当前全局模式偷渡。
8. scope 编码只保留为 API/迁移兼容边界：`group:<group_openid>`、`private:<user_openid>`、`global-user:<union_openid>`。新代码只接受群坐标；后两种 scope 仅供迁移报告和只读兼容。
9. 新导入在无 union OpenID 时仍必须成功并保存在当前群，不能因权限缺失阻断导入。
10. 渲染和管理台编辑在 union 缺失时仍可用；错误提示必须说明能力限制而不是返回 500。
11. 每次成员课表写入必须携带 expected revision；冲突返回 `ErrConflict`。
12. 时间解析、展示和日界线统一使用 `schedule.LocalTZ`。
13. `RAW_ICAL` 只在能匹配原事件时保留，不能用 UI 字段覆盖未知 iCalendar 属性。
14. v7 设置模型不再提供 `storage_mode=global`。旧值只作为迁移输入；切换后所有新查询和新写入都走群坐标，
    不从该值推断或合并跨群课表。

### 5.5 SQLite 目标数据模型

当前 `schedule_members(scope_id, user_id, data_json)` 能作为迁移起点，但不应继续承载全部身份语义。目标 schema
以群成员为聚合根，用复合主键和复合外键表达“成员属于群、课表属于成员、事件属于课表”：

```text
groups
  group_openid TEXT PRIMARY KEY CHECK (group_openid <> '')
  created_at TEXT NOT NULL
  updated_at TEXT NOT NULL

group_members
  group_openid TEXT NOT NULL
  member_openid TEXT NOT NULL
  display_name TEXT NOT NULL DEFAULT ''
  qq TEXT NOT NULL DEFAULT ''
  union_openid TEXT NULL
  first_seen_at TEXT NOT NULL
  last_seen_at TEXT NOT NULL
  last_roster_at TEXT NULL
  is_in_roster INTEGER NOT NULL DEFAULT 0 CHECK (is_in_roster IN (0, 1))
  PRIMARY KEY (group_openid, member_openid)
  FOREIGN KEY (group_openid) REFERENCES groups(group_openid) ON DELETE CASCADE
  CHECK (group_openid <> '' AND member_openid <> '')

group_member_schedules
  group_openid TEXT NOT NULL
  member_openid TEXT NOT NULL
  source TEXT NOT NULL DEFAULT ''
  source_file TEXT NOT NULL DEFAULT ''
  uploader_openid TEXT NOT NULL DEFAULT ''
  import_group_openid TEXT NOT NULL DEFAULT ''
  import_member_openid TEXT NOT NULL DEFAULT ''
  import_union_openid TEXT NOT NULL DEFAULT ''
  created_at TEXT NOT NULL
  updated_at TEXT NOT NULL
  schedule_updated_at TEXT NOT NULL
  last_modified_at TEXT NOT NULL
  last_modified_by TEXT NOT NULL DEFAULT ''
  revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1)
  PRIMARY KEY (group_openid, member_openid)
  FOREIGN KEY (group_openid, member_openid)
    REFERENCES group_members(group_openid, member_openid) ON DELETE CASCADE

group_member_course_events
  group_openid TEXT NOT NULL
  member_openid TEXT NOT NULL
  event_index INTEGER NOT NULL CHECK (event_index >= 0)
  uid TEXT NOT NULL DEFAULT ''
  summary TEXT NOT NULL DEFAULT ''
  location TEXT NOT NULL DEFAULT ''
  description TEXT NOT NULL DEFAULT ''
  dtstart TEXT NOT NULL DEFAULT ''
  dtend TEXT NOT NULL DEFAULT ''
  dtstart_tzid TEXT NOT NULL DEFAULT ''
  dtend_tzid TEXT NOT NULL DEFAULT ''
  rrule TEXT NOT NULL DEFAULT ''
  dtstamp TEXT NOT NULL DEFAULT ''
  raw_json TEXT NOT NULL DEFAULT '{}'
  PRIMARY KEY (group_openid, member_openid, event_index)
  FOREIGN KEY (group_openid, member_openid)
    REFERENCES group_member_schedules(group_openid, member_openid) ON DELETE CASCADE

group_day_overrides
  group_openid TEXT NOT NULL
  member_openid TEXT NULL
  day TEXT NOT NULL
  kind TEXT NOT NULL CHECK (kind IN ('holiday', 'shift'))
  source_day TEXT NOT NULL DEFAULT ''
  actor_openid TEXT NOT NULL DEFAULT ''
  created_at TEXT NOT NULL
  FOREIGN KEY (group_openid) REFERENCES groups(group_openid) ON DELETE CASCADE
  FOREIGN KEY (group_openid, member_openid)
    REFERENCES group_members(group_openid, member_openid) ON DELETE CASCADE
  CHECK (member_openid IS NULL OR member_openid <> '')
```

`group_day_overrides.member_openid IS NULL` 表示全群标记；非空值必须属于同一群。使用两个部分唯一索引表达
“全群每天一行”和“成员每天一行”，避免把 `*` 当作伪外键：

```sql
CREATE UNIQUE INDEX ux_group_day_overrides_all
  ON group_day_overrides(group_openid, day)
  WHERE member_openid IS NULL;

CREATE UNIQUE INDEX ux_group_day_overrides_member
  ON group_day_overrides(group_openid, member_openid, day)
  WHERE member_openid IS NOT NULL;
```

`display_name` 与 `qq` 属于群成员资料，只有 `group_members` 一个事实来源；它们变化不得伪造课表 revision。
`group_member_schedules` 只保存课表内容、来源、时间和 revision。成员成功做第一次课表导入时，同一事务内 upsert
`groups` / `group_members` 再插入课表，避免课表脱离成员。
`group_member_course_events` 使用独立表名，避免与 v6 的通用 `course_events` 冲突，使旧表可以原样保留为回滚
快照。事件数由 `COUNT(group_member_course_events)` 或同一事务内维护的派生查询得到，不增加容易漂移的冗余计数列。

必建索引与约束：

- `group_members(union_openid) WHERE union_openid IS NOT NULL`：非唯一索引，只用于诊断或未来独立能力，
  不得借此把同一 union 的多群成员行折叠成一个用户。
- `group_member_schedules(updated_at)`：按本群最近更新时间列出成员。
- `group_member_course_events(group_openid, dtstart)`：按群和开始时间过滤事件。
- `group_day_overrides(group_openid, day)`：全群标记和成员覆盖的统一读取。
- `schedule_edit_links(expires_at)` 与 `message_stats(received_at)`：清理任务。
- 所有文本 ID 原样保存，不拆分、不拼接、不大小写折叠；空值在入口校验，数据库仅用非空约束做最后防线。

revision 写入必须遵守：

1. `expectedRevision=0` 只执行 insert；唯一键冲突映射为 `ErrConflict`。
2. `expectedRevision>0` 只执行 `UPDATE ... WHERE group_openid=? AND member_openid=? AND revision=?`。
3. update 影响 0 行时，同一事务内区分“成员课表不存在”（稳定 not-found）和“revision 冲突”
   （`ErrConflict`），不得一律报 500。
4. 成功后 `revision=revision+1`，返回新 revision；课表元数据和事件快照在同一事务提交。
5. 任何路径都不得执行只带 `member_openid` 的 update/delete，也不得提供无条件覆盖写。

其余数据也必须从通用 `kv_data` 实体化：

- `bot_settings`：单例配置，`id=1` 约束；JSON 仅作为聚合配置载荷。
- `schedule_edit_links`：`token_hash` 主键、精确 `(group_openid, member_openid)`、过期时间，并以复合外键关联
  成员；禁止保存明文 token。
- `bot_panels`：`panel_scope` 主键、平台 panel ID、内容指纹。
- `message_stats`：append-only 观测数据，显式记录可空 `group_openid`、`member_openid` 和旧 scope 兼容字段；
  不设成员外键，避免删除成员时破坏历史统计。
- `metadata`：schema version，仅存系统元数据。

`group_member_schedules` 不保存整份成员 JSON。可查询、可约束的字段使用列；`raw_json` 只保留 iCalendar
未知属性等无法结构化且需要无损往返的内容。

完整辅助表 DDL 如下，迁移时一并创建：

```sql
CREATE TABLE bot_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  settings_json TEXT NOT NULL CHECK (json_valid(settings_json)),
  updated_at TEXT NOT NULL
);

CREATE TABLE schedule_edit_links (
  token_hash TEXT PRIMARY KEY CHECK (length(token_hash) = 64),
  group_openid TEXT NOT NULL,
  member_openid TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (group_openid, member_openid)
    REFERENCES group_members(group_openid, member_openid) ON DELETE CASCADE,
  CHECK (group_openid <> '' AND member_openid <> '')
);
CREATE INDEX idx_schedule_edit_links_expires
  ON schedule_edit_links(expires_at);

CREATE TABLE bot_panels (
  panel_scope TEXT PRIMARY KEY CHECK (panel_scope <> ''),
  panel_id TEXT NOT NULL,
  items_hash TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
  updated_at TEXT NOT NULL
);

CREATE TABLE message_stats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_openid TEXT NULL,
  member_openid TEXT NULL,
  legacy_scope_id TEXT NOT NULL DEFAULT '',
  legacy_user_id TEXT NOT NULL DEFAULT '',
  origin TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL DEFAULT '',
  received_at TEXT NOT NULL,
  render_ms INTEGER NOT NULL DEFAULT 0 CHECK (render_ms >= 0),
  upload_ms INTEGER NOT NULL DEFAULT 0 CHECK (upload_ms >= 0),
  send_ms INTEGER NOT NULL DEFAULT 0 CHECK (send_ms >= 0),
  server_ms INTEGER NOT NULL DEFAULT 0 CHECK (server_ms >= 0),
  ok INTEGER NOT NULL DEFAULT 1 CHECK (ok IN (0, 1)),
  err_code TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_message_stats_received
  ON message_stats(received_at);

CREATE TABLE metadata (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
```

`message_stats` 是有保留期的 append-only 观测表，不设成员外键，避免成员离开或改名后破坏历史统计。新写入只填
`group_openid` / `member_openid`；`legacy_*` 只为导出旧数据保留。

SQLite `foreign_keys` 是连接级 PRAGMA。目标实现必须在 DSN 中为每条新连接启用，例如
`_pragma=foreign_keys(1)`、`_pragma=busy_timeout(30000)`，而不能只依赖 `Open` 时执行一次 PRAGMA。
写事务使用 `BEGIN IMMEDIATE`；仍保留 `MaxOpenConns(1)`，并继续在进程内串行写，避免单实例与外部工具竞争时
把 CAS 冲突误判为 I/O 错误。

`private:` / `global-user:` 是现有安装的兼容数据，不是 v7 群模型的聚合根。首版迁移只把
`group:<group_openid>` 行提升为一等群成员数据；非群 scope 原样保留在 v6 `schedule_members` /
`course_events` / `schedule_day_overrides` 中，只提供读取和导出适配。`group:*` 组内行也保留一份旧快照，供首个
v7 版本回滚和双读校验，但新代码不得再写旧表。非群 scope 不暴露在群管理台，也不参与新身份解析；若后续明确
彻底废弃，再用单独版本归档，不能在本轮静默删除。

### 5.6 schema v7 迁移契约

v6 与 v7 的表映射固定如下，避免同名表互相覆盖：

| v6 | v7 | 处理 |
| --- | --- | --- |
| `schedule_members` 的 `group:*` 行 | `groups`、`group_members`、`group_member_schedules` | 结构化并校验 revision |
| `course_events` 的群事件 | `group_member_course_events` | 按原 `event_index` 复制 |
| `schedule_day_overrides` 的群行 | `group_day_overrides` | 原 `user_id='*'` 映射为 `member_openid=NULL` |
| `kv_data/settings/settings` | `bot_settings` | 解析后写单例行 |
| `kv_data/seen`、`identity`、`roster` | `group_members` | 按 `(group_openid, member_openid)` 合并 |
| `kv_data/edit_links` | `schedule_edit_links` | 保留 token 摘要和过期时间 |
| `kv_data/panel` | `bot_panels` | 保留平台 panel ID 与内容指纹 |
| 非 group scope 行 | 原 v6 表 | 保留读取/导出兼容，不伪装成群成员 |
| 未知 KV namespace | `kv_data` | 原样保留，不做猜测性删除 |

迁移必须在单个事务内完成，并在执行前通过 SQLite backup API 或 `VACUUM INTO` 创建可恢复备份：

1. 校验 `scope_id`；非法值停止迁移并报告表名、行号、scope 和无法解析原因。
2. 将 `group:<group_openid>` 行写入 `groups`、`group_members` 和 `group_member_schedules`；成员行缺失时先创建最小成员记录。
3. 将 `schedule_members.data_json` 中的 revision、时间、来源和事件元数据解码到结构化列；坏 JSON 必须中止，不得跳过。
4. 将 `seen`、`identity`、`roster` 三个 KV namespace 合并到 `group_members`，按字段更新时间与来源优先级解决
   显示名/union 冲突；合并后 union 只是成员属性，不生成跨群共享关系。
5. 将全局 `settings`、`edit_links`、`panel` 迁移到具名表；未知 KV namespace 保留只读兼容，不静默丢弃。
6. 同一个 `member_openid` 出现在多个群时，必须生成多条 `group_members`、多张课表和多组事件，不得按 `user_id` 去重。
7. 非 group scope 原样保留在 v6 表，不伪装成群成员数据；迁移报告必须单列其行数。
8. 完成后执行 `PRAGMA foreign_key_check` 和 `PRAGMA integrity_check`，校验每群成员数、课表数、事件数、revision、
   覆盖标记数、非群 scope 数和无法解析行数，再写入 `schema_version=7`。
9. 同一 fixture 连续执行两次必须得到相同行数和内容哈希；任一步失败都回滚事务并保留迁移前备份。
10. 首个版本保留 v6 表和双读校验能力；确认线上稳定后再删除兼容路径，禁止在迁移提交中直接丢表。
11. 回滚到旧二进制前必须停止写入并确认旧快照边界，或直接恢复迁移前备份；不能声称旧二进制会自动看到 v7 新写入。

### 5.7 回复契约

聊天用例不直接调用 `qqapi.Client`。推荐由用例返回“回复计划”，发送器执行：

```go
type ReplyPlan struct {
	Kind     ReplyKind // text/markdown/image/file/none
	Text     string
	Markdown string
	Image    *MediaRef
	File     *FileRef
	Keyboard *ReplyKeyboard
}
```

`Replier` 继续负责：

- 被动回复 `msg_id` / `event_id` 的选择。
- `msg_seq` 计数和 5 次上限。
- 上传和发送错误归类。
- 统计阶段记录。

业务用例只决定“回复什么”，不决定“如何发”。

## 6. 函数编排

### 6.1 聊天指令

群消息的身份编排固定为：

```text
Inbound{group_openid, member_openid}
  → GroupMemberKey{GroupOpenID, MemberOpenID}
  → RecordGroupMemberSeen（显示名与 union 仅作可选属性）
  → GroupMemberKey 直接作为成员与课表坐标
```

`bot` 不自行拼接数据库键，`admin` 不根据显示名猜成员；所有身份到存储坐标的转换由 `schedule` 的显式类型完成。
HTTP 在过渡期继续接受 `scope_id` / `user_id` 以兼容现有前端，但服务入口必须校验 `group:` 前缀并立即转换为
`GroupKey` / `GroupMemberKey`。非群 scope 只能进入迁移/导出专用函数。

```text
HTTP /webhook
  → webhook.Verify
  → Dispatcher.Handle            ACK 200、按 payload.id 去重
  → Dispatcher.process           新建超时 context
  → payload mapper               生成 bot.Inbound
  → Handler.Dispatch             开关、@/slash 策略、附件分流、指令匹配
  → Command.Handle               薄适配：Inbound → UseCase Request
  → ChatUseCase.Execute          身份解析、权限、领域调用、回复计划
  → schedule.Service             纯规则与端口调用
  → store adapter                SQLite
  → ReplyPlan
  → Replier                      上传/发送/计数/统计
```

职责边界：

- `Handler` 不解析业务日期、不查成员课表、不拼卡片。
- `Command.Handle` 只做输入适配和调用一个用例，不跨多步编排。
- `ChatUseCase` 可以用多个端口，但不得接收 Gin context 或平台原始 JSON。
- `schedule.Service` 负责可复用领域规则和领域写入，不发送消息。
- `Replier` 不决定权限、不读取设置、不构造业务文案。

### 6.2 Webhook

```text
Verify(raw body + signature)
  → decode envelope
  → op=13: sign plain_token and answer immediately
  → business event: ACK 200, dedup by id
  → bounded worker context
  → platform DTO mapper
  → bot handler
```

要求：

- 验签依赖原始 body；下游只能读取恢复后的 body。
- 去重表必须有 TTL 和容量上限。
- 业务处理 panic 必须在本 goroutine 内 recover。
- 长任务不能占用 HTTP 请求 goroutine，但其 context、超时和退出等待必须明确。

### 6.3 管理台编辑

```text
POST /api/schedule/save
  → auth middleware
  → bind JSON
  → admin.SaveSchedule(ctx, command)
  → resolve storage target
  → load current member
  → validate + make domain events
  → preserve RAW_ICAL
  → PutMember(expected revision)
  → save result DTO
  → server error/response mapper
```

`server` 不知道 SQL，也不直接拼领域事件；`admin` 不接收 `*gin.Context`。

### 6.4 公开编辑链接

```text
GET/POST /api/public/schedule
  → token digest lookup
  → validate expiry
  → resolve exact GroupMemberKey from token record
  → load/save only that member
  → invalid=404, expired=410, conflict=409
```

安全约束：

- 请求体不接受也不信任 scope/user 覆盖字段。
- token 只保存摘要，不保存明文。
- token 过期后读取可触发删除，但删除失败不能把“已过期”误报为“无效”。

### 6.5 导入与导出

```text
HTTP multipart/query
  → server 解析文件、格式、大小
  → admin.Transfer
      → ZIP/manifest 校验
      → 单成员/批量用例
      → schedule.SaveICS / ImportBackup
  → ImportResult
  → HTTP JSON 或附件响应
```

ZIP、manifest、备份格式属于应用层工作流，不属于 HTTP 绑定。`server/transfer.go` 最终只保留：

- 请求解析。
- Content-Type/Disposition。
- 调用 `admin.Transfer`。
- 响应和错误映射。

## 7. 关键用例的推荐签名

以下签名表达依赖方向，不要求一次性改完：

```go
func (a *Admin) SaveSchedule(
	ctx context.Context,
	cmd SaveScheduleCommand,
) (SaveScheduleResult, error)

func (b *Chat) QueryDay(
	ctx context.Context,
	cmd QueryDayCommand,
) (ReplyPlan, error)

func (t *Transfer) Import(
	ctx context.Context,
	cmd ImportCommand,
) (ImportResult, error)

func (a *App) Run(ctx context.Context, cfg config.Config) error
```

命令对象由 transport 层构造，应用层校验；领域函数只接收规范化后的值。结果对象是应用 DTO，不直接嵌入 HTTP/`gin` 类型。

## 8. 测试与架构守卫

### 8.1 当前覆盖率

2026-10-27 执行完整覆盖率脚本后的结果：

| 范围 | 当前 |
| --- | ---: |
| Go 总计 | 75.1% |
| `internal/admin` | 76.1% |
| `internal/bot` | 68.7% |
| `internal/config` | 94.3% |
| `internal/qqapi` | 70.1% |
| `internal/render` | 71.7% |
| `internal/schedule` | 78.2% |
| `internal/server` | 78.1% |
| `internal/store` | 65.7% |
| `internal/timing` | 100.0% |
| `internal/wakeup` | 91.3% |
| `internal/webhook` | 74.0% |

`cmd` 和 `main` 不靠覆盖率数字评价；它们应保持极薄，由 `app`、用例和 HTTP 集成测试覆盖真正的生命周期逻辑。

### 8.2 测试目录政策

生产代码目录不再承载大部分测试：

```text
tests/
  unit/              可仅通过公开 API 验证的纯函数
  contract/          同一端口契约对 fake 与 SQLite 运行同一套测试
  integration/       SQLite、migration、admin、server、webhook
  e2e/               从入站 payload 到 ReplyPlan/HTTP 响应的完整链路
  testutil/          fake、临时数据库、fixture 工厂、请求构造器
  testdata/          输入型 ICS、payload、备份文件
```

规则：

- 业务行为、端口契约、HTTP、迁移和 E2E 测试统一放 `tests/`，通过导出 API 和 `internal` 包边界验证。
- 只有必须访问未导出函数或内部状态的白盒测试可以保留为同目录 `_test.go`，且文件名需明确 `_whitebox_test.go`。
- 不为了搬目录而导出生产符号，不添加 `export_test.go` 作为常规手段。
- `tests/testutil` 可包含非 `_test.go` 文件供其他测试包 import，但不得被生产代码引用。
- 测试使用标准库 `testing`；期望值写在代码中。`tests/testdata` 只放输入数据，不放需要 `-update` 的 golden 输出。
- 每次目录迁移前先确认测试能在新路径运行；迁移提交不夹带业务行为变化。

### 8.3 测试层次

| 层次 | 必测契约 |
| --- | --- |
| 领域单测 | ICS、RRULE、日期范围、休假/调休、榜单、revision、时间边界 |
| 身份单测 | 同群唯一键、两群同 `member_openid`、union 缺失、union 出现、显示名变化 |
| 端口契约 | member、group member、override、stats、settings、edit link、panel 对 fake 与 SQLite 行为一致 |
| SQLite 集成 | 外键、索引路径、事务回滚、CAS、并发写、WAL、`MaxOpenConns(1)` |
| 迁移测试 | v6 fixture → v7、幂等、重复执行、坏数据中止、备份恢复、行数和 revision 校验 |
| 应用用例 | 群模式、v6 非群数据只读兼容、权限、公开 token、导入导出、错误分类 |
| HTTP | 状态码、错误 envelope、鉴权、上传下载头、请求取消 |
| Webhook | Ed25519、`op=13`、重复 id、ACK 后异步处理、panic recovery |
| E2E | 群消息查询/导入/标记、管理台编辑、公开编辑链接、备份恢复 |
| 前端 | transport 错误映射、纯表单转换、store 状态变化、构建与 dist 一致性 |

高风险路径必须覆盖正例、边界、失败和取消四类情况。覆盖率是门槛，不替代对状态转换和数据库不变量的断言。
主 CI 必须跑完全部确定性测试；ICS、scope 解析、ZIP manifest 和日期解析另设夜间限时 fuzz，并把发现的
最小输入提交为普通回归用例。不能只依赖随机 fuzz 或 golden 结果。

### 8.4 覆盖率门禁

采用逐步收紧，避免当前基线无法提交，同时禁止覆盖率回退：

| 阶段 | Go 总覆盖率 |
| --- | ---: |
| P0 建立基线 | ≥ 69%，此后不得下降 |
| P2 数据库迁移完成 | ≥ 75% |
| P3 用例编排完成 | ≥ 80% |
| P5 完整 CI 完成 | ≥ 85% |
| 最终目标 | ≥ 90% |

最终包级目标：

- `schedule`、`store`、`server`、`webhook`：≥ 90%。
- `admin`、`bot`、`render`：≥ 85%。
- `qqapi`：≥ 80%。
- `config`、`timing`、`wakeup`：≥ 90%。

CI 通过 `scripts/check-coverage.sh coverage.out` 解析总覆盖率和包级覆盖率。新增或修改核心逻辑原则上必须带测试；不能通过给 `cmd` 增加无意义测试来抬高总数。

### 8.5 完整 CI

CI 分为 Go 与 Web 两个必需 job：

```text
Go
  test -z "$(gofmt -l .)"
  bash scripts/check-architecture.sh
  go vet ./...
  go test -race -shuffle=on -count=1 \
    -covermode=atomic -coverpkg=./internal/... \
    -coverprofile=coverage.out ./...
  bash scripts/check-coverage.sh coverage.out
  CGO_ENABLED=0 go build ./...
  go run ./cmd/cardpreview -o /tmp/card.jpg     渲染 smoke test
  test -s /tmp/card.jpg
  上传 coverage.out 与覆盖率报告 artifact

Web
  npm ci
  npm run lint
  npm run test -- --run                         Vitest 纯函数/transport/store
  npm run typecheck
  npm run build
  git diff --exit-code -- web/dist
```

要求：

- push 到 `main` 和所有 PR 都执行完整 CI。
- `go test -race` 是主门禁，不只放在夜间任务。
- 测试失败时保留可复现命令和失败包名，不吞掉 stderr。
- 前端测试依赖进入 `devDependencies`，锁定版本并使用 `npm ci`。
- CI 与本地统一调用 `scripts/test-all.sh`，该脚本只编排命令，不复制架构或覆盖率断言。
- `check-coverage.sh` 必须同时检查总覆盖率和包级门槛；不能只读 `go test` 日志中的最后一行。
- Web 构建必须在干净工作区完成；任何源码改动导致 `web/dist` 漂移都算失败。
- CI 通过后才能执行部署，部署脚本不得提供绕过测试的默认路径。

### 8.6 架构守卫

`scripts/check-architecture.sh` 继续强制：

- `schedule` / `admin` 禁止基础设施依赖。
- `server` 不依赖 `webhook` / `store`。
- `bot` 非测试代码不依赖 `store`。
- `tests` 不得被生产包 import。
- 不再出现旧 `Message` 或裸 KV 访问。

已增加：

- 检查 `cmd` 不直接依赖 `store`。
- 检查端口方法首参为 `context.Context`（AST guard）。
- 检查生产代码不 import `tests`。
- 检查源码目录普通测试文件只能命名为 `_whitebox_test.go`。
- 检查前端构建产物与 `web/dist` 一致。

### 8.7 每次迁移的完成标准

```bash
bash scripts/test-all.sh
CGO_ENABLED=0 go build ./...
```

涉及卡片时再执行 `go run ./cmd/cardpreview -o /tmp/card.jpg`，检查尺寸、像素采样和人工预览。

## 9. 分阶段优化计划

每个阶段必须可独立提交、独立发布、独立回滚。阶段之间不得夹带无关重命名或格式化 churn。

### P0：固化身份契约、错误契约与测试护栏

**目标**：先让身份语义、客户端错误和测试入口有唯一事实来源，不改变数据库结构和业务功能。

**改动**

- 新增 `GroupKey`、`GroupMemberKey`，旧 `ScopeRef` 只允许出现在 HTTP/迁移兼容层。
- 固化 `(group_openid, member_openid)` 复合身份；明确 `member_openid` 不是昵称，`union_openid` 是可空关联。
- 新建 `tests/`、`tests/testutil/`，先迁移 HTTP、store 契约、身份和迁移测试；白盒例外按 8.2 执行。
- 新增 `scripts/test-all.sh` 与 `scripts/check-coverage.sh`，CI 先以当前 69% 为不回退基线。
- 新增 `server/errors.go`，集中映射 validation/not-found/conflict/gone/unauthorized/internal。
- 设置保存的校验错误从 500 改为 400；直接错误比较改为 `errors.Is`；保留 `{"error":"..."}` 并增加可选 `code`。
- Go CI 加 `-race`、`-shuffle=on`、覆盖率报告和 artifact。

**验收**

- 身份测试断言：同一 `member_openid` 在两个群产生两个不同身份和课表坐标。
- 无 union 的导入、查询、渲染和管理台编辑全部成功。
- 设置非法 BaseURL/TTL 返回 400；公开链接 invalid/expired、revision conflict 分别返回 404/410/409。
- `go test -race ./...`、架构检查、覆盖率脚本和构建全绿。
- 总覆盖率不低于基线，测试目录中至少包含 HTTP、store 契约和身份链路。

**风险与回滚**

- 风险：测试迁移一次性过大。解决：按领域分批移动，纯移动提交与行为修改分开。
- 风险：已有前端按错误文本分支。解决：保留文本字段，只增加状态码和可选 code。
- 回滚：错误映射、类型引入、测试迁移各自独立提交。

### P1：窄化端口、显式依赖与 context

**目标**：让能力缺失在编译/启动期暴露，让所有 I/O 可取消。

**改动**

- 拆分 `Storage` 为 `GroupMemberRepository`、课表、标记、统计、设置、edit-link、panel 等窄端口。
- 群成员资料、roster、seen 统一通过 `GroupMemberRepository`，不再由三个应用层 KV namespace 表达。
- 所有群端口使用 `GroupKey` / `GroupMemberKey`，避免裸字符串和拼接 scope 坐标扩散。
- 用显式 `Dependencies`/构造函数替代运行时类型断言；删除应用层通用 KV。
- 所有仓储和外部 I/O 方法首参增加 `context.Context`。
- `store.Store` 逐端口写编译期断言，并补 fake/SQLite 共享契约测试。
- `admin.Service` 移除 `*schedule.Service` 嵌入，改为显式的小接口或函数依赖。

**验收**

- 组合根缺少任一必需群端口时编译或构造失败，启动日志明确指出缺失依赖。
- 不存在可选的跨群身份端口；fake 未实现必需能力时测试直接失败，不会静默改变身份解析行为。
- 取消 HTTP 请求/关停后，端口调用收到 canceled context。
- 契约测试只需编写一次即可同时跑 fake 和 SQLite。

**风险与回滚**

- 风险：一次修改所有签名触碰面大。解决：先加适配器方法和契约测试，再逐个用例迁移。
- 数据兼容：本阶段只通过 adapter 包装现有表，不改变磁盘 schema。

### P2：SQLite schema v7 与群成员实体化

**目标**：用数据库主键、外键和索引表达群内身份，不再依赖 JSON 与拼接字符串。

**改动**

- 新增 `groups`、`group_members`、`group_member_schedules`、`group_member_course_events` 与
  `group_day_overrides`。
- 将 `identity`、`roster`、`seen` 从 `kv_data` 迁入 `group_members`；将 settings、edit links、panel 迁入具名表。
- 通过 `(group_openid, member_openid)` 复合主键保证群成员唯一；`union_openid` 保持可空。
- 将 `group_member_schedules` 的可查询字段从 `data_json` 移为列，`raw_json` 只保存无损往返所需的未知 iCalendar 数据。
- 用 `(group_openid, member_openid)` 复合外键保证课表必须属于成员、事件必须属于课表；展示名和可选 `qq`
  只存 `group_members`。
- 在 v7 切换点停止新业务写入 `private:*` / `global-user:*`，并从新设置和新管理台流程移除全局存储选项；
  旧 scope 只能通过只读 compatibility adapter 查看或导出。
- 实现备份、事务迁移、行数/revision/事件数校验、幂等重跑和旧表兼容读取。
- 迁移测试使用 v6 fixture，覆盖两个群共享同一 `member_openid` 的场景。

**验收**

- v6 fixture 可完整迁移到 v7，连续执行两次结果一致。
- 坏 scope、坏 JSON、冲突主键会让迁移安全中止，原库和备份可恢复。
- 两个群相同 `member_openid` 的身份、课表、事件和覆盖标记互不覆盖。
- 删除成员时课表和事件随外键级联清理；直接插入孤立课表或孤立事件必须失败。
- 无 union 用户可完整使用；union 出现后只增加关联，不改变群内主键。
- 即使 v6 设置中 `storage_mode=global`，v7 群查询也不得跨群拼出成员课表；旧 global-user 数据只能只读导出。
- SQLite 契约测试和迁移测试通过，Go 总覆盖率不低于 75%。

**风险与回滚**

- 风险：线上数据是唯一事实来源。解决：启动前备份，迁移在事务内执行，第一版保留旧表与双读校验。
- 风险：把 `user_id` 当全局身份会错误合并数据。解决：所有查询显式带群坐标，并加入跨群同值测试。
- 回滚：保留 v6 库文件和旧表，二进制回退后可继续读取；不得在首个迁移版本删除旧表。

### P3：包内拆文件与薄化编排

**目标**：降低 review 和测试成本，不改变公开行为。

- `store/store.go` 按 schema、group、member、override、stats、settings、edit-link、panel 拆文件。
- `admin/service.go` 按 schedule/override/public/directory/transfer 拆文件。
- `bot/env.go` 拆出 identity、roster、card、media、stats 组件；`Env` 只保留装配职责。
- `commands.go` 只做注册；复杂 handler 改为薄适配 + `ChatUseCase` + `ReplyPlan`。
- `server/transfer.go` 的工作流迁入 `admin/transfer.go`，HTTP 文件只做绑定和响应。
- 将可黑盒验证的测试继续迁入 `tests/`，白盒测试数量逐步减少。

**验收**

- 行为测试不修改或仅做构造方式适配。
- 每个新文件职责可用一句话描述。
- 渲染预览与重构前一致。
- Go 总覆盖率不低于 80%。

**风险与回滚**

- 风险：机械拆文件导致隐式共享状态更难追踪。解决：先识别状态 owner（avatar/roster/stats），再拆。
- 回滚：文件移动和编排调整分开提交；纯移动 commit 可单独回退。

### P4：引入 `internal/app` 与生命周期管理

**目标**：让 `cmd/bot` 只负责进程入口。

- 新增 `app.Run(ctx, cfg, version)`。
- 将依赖构造、路由、后台头像/面板/菜单同步迁入 `app`。
- 所有 worker 使用根 context 和 `WaitGroup`；HTTP shutdown 后再等待或取消 worker。
- 统一 logger 初始化；按“停止接收 → 取消 worker → 等待 → 关闭 store”退出。
- `cmd/bot/main.go` 只解析入口、调用 app、打印启动错误。

**验收**

- SIGTERM 集成测试验证 HTTP、worker、数据库按顺序关闭。
- 启动失败时不需要经过 HTTP 层即可返回明确错误。
- `go build ./cmd/...` 和部署脚本通过。

**风险与回滚**

- 风险：关闭顺序变化造成 WAL 或 worker 写入失败。解决：先补生命周期测试和超时保护。
- 回滚：保留旧 main 装配直到新 app 全测通过。

### P5：前端 API/状态/交互拆分与完整 CI

**目标**：让网页边界与 Go 边界同样可测试、可演进，并让 CI 覆盖全部产品面。

- `web/src/api/client.ts`：统一 fetch、JSON 解析、401 跳转、`ApiError`。
- `web/src/api/admin.ts` / `public.ts`：只放有类型的 endpoint。
- `web/src/stores/scopes.ts` / `editor.ts` / `settings.ts`：状态与请求分离。
- `web/src/domain/`：表单校验、事件转换、导入摘要等纯函数。
- Element Plus 的 message/confirm 留在组件或 composable，不进入纯状态模块。
- 增加 ESLint 与 Vitest：静态规则、transport 错误映射、纯函数、store 状态和下载流程。
- CI 增加 Node 设置、`npm ci`、`npm test`、typecheck、build、`web/dist` 一致性检查。
- CI 的 Go 与 Web job 都成为部署前置条件。

**验收**

- 源码与入库 dist 一致。
- 401、409、410 和网络失败显示正确中文提示。
- 前端测试、typecheck、build 全部通过。
- 移动端和桌面端管理台无文本溢出或交互回归。

**风险与回滚**

- 风险：拆 store 时出现双份状态。解决：按页面引入唯一 store/composable，不引入额外状态库。
- 回滚：API transport、store、交互提示的拆分分别提交。

### P6：强化架构守卫与文档收口

**目标**：让规则持续可执行，而不是依赖 review 记忆。

- 扩展 `scripts/check-architecture.sh`：`cmd` 边界、应用层禁 Gin、旧端口/裸 KV、生产代码 import `tests`。
- 用 AST 守卫端口首参 `context.Context`，避免脆弱正则。
- 固化 `GroupKey` / `GroupMemberKey` / `Revision` 的类型边界，清理含糊的通用 `user_id` 新 API。
- 更新 README、DEV-GUIDE 的目录、身份语义、测试目录和 CI 说明。
- 删除已完成迁移的临时兼容层，确认没有未引用类型和方法。

**验收**

- CI 同时覆盖 Go、前端、race、覆盖率、迁移和产物一致性。
- 新成员可从 ARCHITECTURE.md 找到模块入口、身份主键、依赖规则和验收命令。
- 所有边界规则至少有一个命令或测试守护。

**风险与回滚**

- 风险：守卫过严导致正常演进需要频繁豁免。解决：只守护真正发生过或高风险的边界。
- 回滚：守卫脚本改动独立提交。

## 10. 不纳入重构的事项

以下选择有明确收益，不应为了“更漂亮”改掉：

- 不把 `schedule` 拆成大量小包；当前包级依赖干净，包内文件拆分足够。
- 不给 `qqapi.Client` 再包一层同形接口；已有 `httptest` 适配方式。
- 不把 `Env` 的所有字段拆成大量位置参数；拆分其方法的职责，而不是制造签名噪音。
- 不把 `web/dist` 改为运行时构建；Go 单二进制部署更重要。
- 不引入第二个数据库或 ORM；当前 SQLite 单连接和显式 SQL 足够。
- 不为纯函数机械增加 context。
- 不新增、删除或暴露聊天指令来完成架构迁移。

## 11. 推荐执行顺序

```text
P0 身份/错误契约 + 测试目录 + CI 基线
  → P1 新窄端口 + 显式依赖 + context
  → P2 schema v7 群成员实体化与迁移
  → P3 store/admin/bot 拆文件 + 薄化编排
  → P4 app 装配与生命周期
  → P5 前端拆分 + 完整 CI
  → P6 架构守卫与文档收口
```

如果只能先做三项，应选择 P0、P1、P2：先固定可验证的身份与错误契约，再让端口显式化，最后用 schema v7
把群内用户主键落到数据库。这三项完成后，后续拆文件和前端重构才有稳定基础。
