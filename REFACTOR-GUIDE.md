# qqbot-course-schedule 重构手册

> 适用对象：本项目全部 Go 代码。
> 配套文档：[PLAN.md](PLAN.md)（功能规格与决策记录）、[DEV-GUIDE.md](DEV-GUIDE.md)（开发手册）、[CLAUDE.md](CLAUDE.md)（速查约定）。
> 本手册记录**架构评估结论**与**分阶段重构方案**，每条结论都标明证据（文件:行号），可直接核对。
> 实施状态（2026-09-27）：阶段一已于 `05004bb` 完成；阶段二已完成，阶段三仍按需排期。

---

## 0. 总评：先说不该动的地方

重构的前提是认清哪些设计是对的。本项目有四件事做得比同类项目好，**重构时必须保持**：

| # | 设计 | 证据 |
| --- | --- | --- |
| G1 | `schedule` 是纯领域内核，对基础设施零依赖 | `go list -deps ./internal/schedule` 无 gin / sqlite / gg / cron；仅 `rrule-go` + `golang.org/x/text` |
| G2 | 依赖倒置：不是领域依赖存储，而是存储实现 `schedule.Storage` | `internal/store/store.go:158` marshal `schedule.Member`；`internal/schedule/types.go:82` 定义接口 |
| G3 | 会话作用域（`group:`/`private:` + OpenID）三入口共用 | 指令、`/api/*`、批量导入共用同一表与校验 |
| G4 | 指令集是单一事实来源，面板/菜单/help 全部派生 | `internal/bot/panel.go:116` `panelItems` 从 `handler.Commands()` 生成 |

DEV-GUIDE §3 声明的「禁止反向依赖」**逐条验证全部成立**，不是文档口号：

```
config        → (无内部依赖)
qqapi         → config
schedule      → (无内部依赖)      ← 内核
store         → schedule          ← 适配器（方向被反转）
render        → schedule
bot           → config qqapi render schedule
webhook       → bot config qqapi
server        → schedule store
```

实测 `bot` 的非测试代码不 import `store`，只依赖 `schedule.PushStore` / `schedule.PanelStore`
两个窄接口；`schedule` 不 import `gin/qqapi/store`、`server` 不 import `webhook`。
**没有任何一个包同时依赖 `gin` 和 `modernc.org/sqlite`** —— HTTP 边界与持久化边界从未相遇。

---

## 1. 评估结论：三处结构性债务

按「改动成本」排序，而不是按「理论丑度」排序。

### D1 `Message` 把入站载体、出站 API、可变状态压成一个类型 ★★★ ✅ 已完成

**现状**（`internal/bot/message.go`，180 行）：

```go
type Message struct {
    Origin, GroupOpenID, UserOpenID, MsgID, EventID   // ← 入站：不可变事实
    Content, Args, Username, MemberRole, IsBot        // ← 入站：不可变事实
    Attachments, Mentions                             // ← 入站：不可变事实
    Client *qqapi.Client                              // ← 出站：依赖
    seqMu  sync.Mutex                                 // ← 出站：可变状态
    seq    int                                        // ← 出站：可变状态
}
func (m *Message) Reply(...) / ReplyImage / ReplyFile / Push / PushImage
```

**为什么是债务**（三条实测证据）：

1. **构造点分散在 4 处且各自手工填字段**：`webhook/dispatch.go:109`（群）、
   `:133`（单聊）、`bot/handler.go:174`（按钮回调）、`bot/push.go:184`（主动推送）。
   每加一个字段就要改 4 处，漏填即为零值静默降级。
2. **测试被迫二选一**：`handler_test.go` 用 `&Message{Content: ...}` 裸构造（`Client` 为 nil），
   所以只能测路由，**发消息路径完全不可单测**；要测发送就必须起
   `httptest.NewServer` 冒充 QQ API（`pipeline_test.go:142`），并把 `config.Domain` 指过去。
3. **已经踩过坑**：git 历史 `f456f42`「群消息原始负载调试日志」—— 排查官方字段时得回头去
   `dispatch.go:108` 打 `string(payload.D)`，因为 `Message` 丢掉了原始载荷。

这次加「耗时统计」就是最直接的证明：为了给消息打时间戳，必须同时动入站边界（`Dispatch`）
和出站边界（`Reply*`），因为两个职责在同一个类型上。

**结论**：这是唯一「每次改动都会付费」的债务，优先级最高。

---

### D2 `schedule` 里混进了 Web 应用层 ★★

**现状**：`internal/schedule/web.go` 535 行，10 个 `Service` 方法，全部是**管理台专用**：

```go
ScopeSummaries / PageSchedule / SavePageSchedule / CreateMemberSchedules
RecordSeenMember / WebMemberICS / WebDayOverrides / SetWebDayOverride
DeleteWebDayOverride / PendingMembers
+ 类型 WebEvent / PageSchedule / WebEventInput / SavePagePayload / WebDayOverride
```

`schedule` 包内 `Service` 方法分布（实测）：

| 文件 | 方法数 | 性质 |
| --- | --- | --- |
| `web.go` | **10** | 管理台应用层 |
| `service.go` | 5 | ICS 导入 + 日卡（机器人侧） |
| `override.go` | 3 | 领域（休假调休） |
| `backup.go` | 2 | 备份（管理台） |
| `member.go` | 2 | 领域（QQ 绑定） |
| `settings.go` | 2 | 全局设置（机器人侧） |
| `rank.go` | 1 | 领域（榜单） |

**为什么是债务**：

- **包名与职责不符**：`web.go` 服务的是 `server`（HTTP 管理台），不是领域。它现在住在
  `schedule` 里，直接后果是 `internal/server/admin.go:19` 必须同时 import `schedule` 和 `store`
  两个包才能注册路由 —— 一个 Web 层被迫了解持久化类型。
- **不是命名洁癖问题**：按职责划分，`web.go` 与 `bot` 同级 —— 都是「某个入口的应用层」。
  现在只有一个入口的应用层被塞进了内核，导致内核的导出面（exported API）被管理台的需求
  撑大。`schedule.WebEvent`、`schedule.SavePageResult` 这类名字出现在领域包里，就是信号。

**结论**：低风险、纯归位问题。但**不要为它新建包**（见 §2 的取舍）。

---

### D3 `Env` 是 12 字段的服务定位器，只有 1 个字段真正需要收窄 ★ ✅ 已完成

**实测每个 `Env` 字段被多少文件使用**：

| 字段 | 使用文件数 | 判断 |
| --- | --- | --- |
| `DataDir` | 4（avatars/env/import/export） | 宽 |
| `Client` | 7（handler/env/push/message/panel/import/menu） | 宽，但合理 |
| `Service` | 6 | 宽，但合理 |
| `Store` | 已移除 | 已拆为 `PushStore` / `PanelStore` 两个窄接口 |
| `PublicBaseURL` | 2 | 合理 |
| `PushCron` | 3 | 合理 |
| `Now` | 4 | 合理（测试注入点） |
| `Renderer` / `ImagesDir` / `FilesDir` / `Buttons` | 各 1～2 | 合理 |

**为什么只有 `Store` 曾是唯一真问题**：**`bot` 通过 `Env.Store` 绕过 `Service` 直接读写了 5 处 KV**
（另有 3 处在 `panel_test.go` 的测试断言里），且全部是裸字符串命名空间：

```go
internal/bot/push.go:90    e.Store.SetKV("global", pushNamespace, scope, sub)
internal/bot/push.go:95    e.Store.DeleteKV("global", pushNamespace, scope)
internal/bot/push.go:100   e.Store.ListKV("global", pushNamespace)
internal/bot/panel.go:160  env.Store.GetKV(panelKVScope, panelNamespace, scope, &state)
internal/bot/panel.go:165  env.Store.SetKV(panelKVScope, panelNamespace, scope, state)
```

这**违反了 DEV-GUIDE §6.3「所有写操作走服务层」的自我约定**，而且是唯一一处违反。
好在这 5 处全部只碰 KV，没有碰课表数据。

**处理结果（2026-09-27）**：`PushSubscription` / `PanelState` 与对应持久化接口已移到
`schedule`，`store` 实现 `schedule.PushStore` / `schedule.PanelStore`；`Env.Store` 已删除，
`panel.go` / `push.go` 不再接触通用 KV。四个 KV 命名空间也集中定义在
`internal/schedule/kv_namespaces.go`。

**注意反例（不要改）**：`Env` 有 12 个宽字段，看起来像「上帝对象」，但实测绝大多数字段是
**不可变配置**（`DataDir`/`PushCron`/`PublicBaseURL`…）。把 12 个字段拆成 12 个窄参数只会
把复杂度从结构体搬到签名上 —— 那是**净损失**。`.

---

### 附：不作为重构项，但值得记录

| 项 | 现状 | 为什么不重构 |
| --- | --- | --- |
| `bot/commands.go` 551 行 / 16 个处理器 | `NewDefaultHandler` 单函数 145 行 | 编排层本就该是聚合点，拆分只增加跳转成本 |
| `qqapi.Client` 无接口，30 个方法 | `bot` 依赖具体类型 | 测试已用 `httptest` 冒充服务器（`pipeline_test.go`），工作良好；抽接口是净增抽象 |
| `schedule` 73% / `store` 66% 覆盖率 | 低于 DEV-GUIDE §11 的 85%/80% 目标 | 属测试补强，不是架构问题 |
| `render` 依赖 `schedule.DayRow` | 渲染层知道领域 DTO | `DayRow` 是扁平展示结构，目前无实际问题 |
| `SendCard` 先 `nextSeq()` 再 `ReplyImage()` | 多消耗一个被动回复配额（`env.go:180` vs `:195`） | 既有行为，改动影响面大于收益；记录备查 |

---

## 2. 重构总原则

**R1 只做「降低未来改动成本」的重构，不做「让结构更漂亮」的重构。**

判断标准：这次改动是否减少了下一次同类改动的触碰面？如果答案是否，就不做。
（例：给 `Env` 拆 12 个窄接口 = 不减少触碰面，只搬复杂度 → 不做。）

**R2 一步一步来，每步独立可发布、可回滚、测试全绿。**

每阶段结束必须满足：`gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿、
`go run ./cmd/cardpreview -o /tmp/card.jpg` 出的卡片与重构前**逐像素一致**。

**R3 用 `go list -deps` 验证边界，不靠 review 时的自觉。**

每条新边界都要能用命令断言（见 §6 的守卫建议）。文档里的规则会腐化，`go list` 不会。

**R4 优先做「有测试保护」的重构。**

`internal/schedule` 有 1645 行测试、`internal/bot` 有 1759 行，这是安全网。
D2 之所以风险低，就是因为 `schedule` 的导出方法几乎全被测试覆盖（含 4 个
`package schedule_test` 外部测试：`settings_/web_/member_qq_/backup_external_test.go`）。

---

## 3. 阶段一：拆分 `Message`（优先级最高）

### 3.1 目标形态

```go
// internal/bot/inbound.go
// Inbound is one received message: immutable facts, no sending ability.
type Inbound struct {
    Origin      Origin
    GroupOpenID string
    UserOpenID  string
    MsgID       string
    EventID     string
    Content     string
    Args        string      // set by Dispatch after prefix matching
    Command     string      // set by Dispatch: matched command prefix
    Username    string
    MemberRole  string
    IsBot       bool
    Attachments []Attachment
    Mentions    []Mention
    ReceivedAt  time.Time   // ← 新字段落地成本 = 1 行
}

func (in *Inbound) IsAdmin() bool

// internal/bot/replier.go
// Replier sends replies for one inbound message. Holds the reply counter.
type Replier struct {
    inbound *Inbound
    client  *qqapi.Client
    seqMu   sync.Mutex
    seq     int
}

func NewReplier(in *Inbound, client *qqapi.Client) *Replier
func (r *Replier) Reply(ctx context.Context, text string) error
func (r *Replier) ReplyImage(ctx context.Context, imageURL string) error
func (r *Replier) ReplyFile(ctx context.Context, fileURL, fileName string) error
func (r *Replier) Push(ctx context.Context, text string) error
func (r *Replier) PushImage(ctx context.Context, imageURL string) error
func (r *Replier) NextSeq() (int, error)   // 导出，供 SendCard 用
```

指令签名从

```go
func (h *Handler) handleXxx(ctx context.Context, msg *Message) error
```

变为

```go
func (h *Handler) handleXxx(ctx context.Context, in *Inbound, r *Replier) error
```

**为什么把 `args` 与 `replier` 作为两个独立参数**：刻意保留签名噪音，防止「顺手把出站能力
塞进入站载体」的历史重演。

### 3.2 迁移步骤（每步独立可编译）

| 步 | 动作 | 验证 |
| --- | --- | --- |
| 1.1 | 新建 `inbound.go` / `replier.go`，把现有 `Message` 的字段与方法搬运过去；**保留 `type Message = Inbound` 别名**与保留原方法名的转发层 | `go build ./...` 不变、`go test ./...` 全绿 |
| 1.2 | 改 4 个构造点：`webhook/dispatch.go:109`/`:133`、`bot/handler.go:174`、`bot/push.go:184` 构造 `Inbound` + `NewReplier` | 全绿；`pipeline_test.go` 不动即通过（证明行为未变） |
| 1.3 | 逐指令改签名。**一次一个指令，一次一个提交**，从最简单的 `handleHelp`（14 行）开始，最后是 `handlePushTime`（42 行） | 每次提交后 `go test ./...` |
| 1.4 | `HandleCallback`（`handler.go:170`）与 `PushScope`（`push.go:180`）改为构造 `Replier`，不再裸构造带 `Client` 的 `Message` | 全绿 |
| 1.5 | 删除 `Message` 类型与转发层 | `go test ./...`；`grep -rn "bot.Message" --include="*.go" .` 应为空 |
| 1.6 | 加统计字段：`Inbound.ReceivedAt`，在 `dispatch.go` 构造时填入 | 新增测试断言 |

### 3.3 验收标准

- **`grep -rn "&Message{\|bot.Message" --include="*.go" .` 无结果。**
- `Inbound` 无 `Client` 字段、无 `sync.Mutex`、无 `seq`。
- 新增一个字段（如本次的 `ReceivedAt`）只需改 **1 个构造点**，而非 4 个。
- `handler_test.go` 现有的裸构造测试改为 `&Inbound{Content: ...}`，
  **仍不需要起 HTTP 服务器**（证明入站边界独立可测）。
- 渲染产物逐像素不变。

### 3.4 风险与回退

| 风险 | 缓解 |
| --- | --- |
| 触碰面大（168 处 `msg.` 引用） | 步 1.1 的类型别名让中间态**始终可编译**，可随时停在第 1.3 步发布 |
| `handleXxx` 签名变更遗漏调用点 | 编译器强制发现，无静默风险 |
| `SendCard` 依赖 `msg.nextSeq()`（`env.go:180`） | 步 1.1 先导出 `NextSeq()`，行为完全不变；**不要顺手修那个多消耗配额的问题**（R1） |

---

## 4. 阶段二：KV 走服务层（小、独立、可单独发布）✅ 已完成

### 4.1 目标

消灭 D3 的 5 处裸 KV 访问，让「所有写入走服务层」成为真约定而非文档口号。

### 4.2 方案

在 `schedule` 包内为现有 4 个命名空间各起一个窄接口（**不是一个大接口**）：

```go
// internal/schedule/pushstore.go
type PushStore interface {
    GetPushSubscription(scope string) (PushSubscription, bool, error)
    SetPushSubscription(scope string, sub PushSubscription) error
    DeletePushSubscription(scope string) error
    ListPushSubscriptions() (map[string]PushSubscription, error)
}
```

`Store` 实现这些接口（复用现有 `GetKV/SetKV/ListKV/DeleteKV`），`Env` 从
`Store schedule.Storage` 收窄为按需的最小接口集合。`panel.go` 的 `panelState` 同理
（`PanelStore`）。

实际实现为 `internal/schedule/pushstore.go`、`internal/schedule/panelstore.go` 和
`internal/store/kv_state.go`。`PushSubscription` 在 `bot` 保留类型别名，指令和测试无需感知
DTO 搬家。

**注意（R1 的适用）**：**只收窄实际被 `bot` 使用的 `Store`**，不要把 `Env` 的另外 11 个
字段也拆成接口。`Service`/`Client` 虽然宽，但它们是「一个入口需要一个应用层/一个 API 客户端」
的自然形态。

### 4.3 验收标准

- `grep -rn "\.Store\.GetKV\|\.Store\.SetKV\|\.Store\.ListKV\|\.Store\.DeleteKV" internal/bot/`
  **无结果**（已加入 `deploy/deploy.sh` 守卫）。
- `Env.Store` 字段已删除，`panel.go` / `push.go` 只依赖两个窄接口。
- 命名空间常量集中定义在 `internal/schedule/kv_namespaces.go`。

---

## 5. 阶段三：`web.go` 归位（谨慎，可选）

### 5.1 三种方案与取舍

| 方案 | 做法 | 代价 |
| --- | --- | --- |
| **A. 不动** | 保持现状 | `schedule` 导出面继续被管理台需求撑大 |
| **B. 移到 `internal/server`** | `web.go` 变 `server/schedule_api.go` | **`server` 会直接 import `store`**，且把领域逻辑（`findOriginalEvent`、ICS 重建）搬到 HTTP 层 —— 违反 G1/G2，**不推荐** |
| **C. 新建 `internal/admin`** | 新包 `admin.Service`，组合 `schedule.Domain` + `schedule.Storage` | 多一层包，但边界最清晰；`server` 只依赖 `admin` |

### 5.2 推荐：方案 C，但**必须与阶段一/二解耦，单独排期**

`admin.Service` 通过**组合**而非继承获得能力：

```go
// internal/admin/service.go
type Service struct {
    domain  *schedule.Service   // 复用领域能力（BuildDayCard / SaveICS / SetDayOverrides…）
    storage schedule.Storage    // 管理台特有的读取（ScopeSummaries / PendingMembers / seen KV）
}
```

搬迁清单（`internal/schedule/web.go` → `internal/admin/`）：

- 10 个 `Service` 方法（§D2 表格）
- 6 个导出类型：`ScopeSummary`、`ScopeMemberSummary`、`WebEvent`、`PageSchedule`、
  `WebEventInput`、`SavePagePayload`、`SavePageResult`、`NewMember`（按需移，`NewMember`
  被 `server` 用于 `POST /api/schedule/create`）
- `backup.go`（151 行，管理台的备份/恢复）一并移入
- **留下不动**：`service.go`（ICS 导入 + 日卡）、`override.go`、`member.go`、`settings.go`、
  `rank.go`、`daycard.go`、`ics.go`、`occurrence.go`、`dayoff.go`、`timerange.go`、`encoding.go`

**关键约束**：`admin` 不得 import `gin`（保持与 `schedule` 同样的纯净度），
路由仍在 `internal/server`，这样边界可用 `go list -deps` 断言。

**为什么不急**：现在**没有实际的改动痛感** —— 管理台需求已经稳定。先做阶段一/二（有明确
收益），阶段三等到下一次需要动管理台时再顺手做。

---

## 6. 防止倒退：可执行的守卫

### 6.1 依赖边界断言（已加入 `deploy/deploy.sh`）

```bash
# R3：用命令断言，而不是靠 review 自觉
set -e
# schedule 必须是纯领域内核
! go list -deps ./internal/schedule | grep -qE 'gin-gonic|modernc.org/sqlite|fogleman/gg|robfig/cron'
# 没有包同时依赖 HTTP 框架与 SQLite 驱动
for p in $(go list ./internal/...); do
  deps=$(go list -deps "$p")
  if echo "$deps" | grep -q 'gin-gonic' && echo "$deps" | grep -q 'modernc.org/sqlite$'; then
    echo "边界违规：$p 同时依赖 gin 与 sqlite"; exit 1
  fi
done
# server 不得依赖 webhook
! go list -deps ./internal/server | grep -q 'internal/webhook'
# bot 的非测试代码不得 import store（测试需要真实 store，见下）
! grep -rl 'internal/store' $(ls internal/bot/*.go | grep -v _test.go)
```

注意最后一条**必须排除 `_test.go`**：`internal/bot` 的测试确实 import `store`
（`panel_test.go`、`settings_test.go`、`pipeline_test.go` 用 `store.Open` 起真库），
但非测试代码零引用 —— 边界看的是后者。

（三条断言**全部通过**，已加入部署前的 `check_architecture`。）

### 6.2 阶段一、二完成后的结构断言（已加入 `deploy/deploy.sh`）

```bash
! grep -rn '&Message{\|bot\.Message' --include='*.go' .
! grep -rn '\.Store\.[A-Za-z]*KV' $(ls internal/bot/*.go | grep -v _test.go)
```

### 6.3 Review 时的自查清单

- 新增 `Env` 字段前，问：这是**不可变配置**还是**新依赖**？后者才需要接口收窄。
- 新增指令：只加 `Command{Ready: true}`，**不要**改 `panelOrder`（`panel.go:116` 会自动包含）。
- 新增持久化：走 `schedule.Service`，带 `expected_revision`，**不要**新增裸 KV 命名空间。
- 新增事件：只改 `webhook/payload.go` + `dispatch.go`（DEV-GUIDE §6.2），
  **不要把官方字段解析漏进 `bot`**。
- 修改卡片版式：先 `go run ./cmd/cardpreview -o /tmp/card.jpg` 看效果。

---

## 7. 排期与顺序

```
阶段一（Message 拆分）   ✅ 已完成（05004bb）
  1.1 类型别名 + 转发层（可编译中间态）
  1.2 改 4 个构造点
  1.3 逐指令改签名（16 个，一次一个提交）
  1.4 HandleCallback / PushScope
  1.5 删 Message
  1.6 加统计字段（Inbound.ReceivedAt）      ← 与「耗时统计」功能合并交付
阶段二（KV 走服务层）    ✅ 已完成（窄接口 + 集中命名空间 + 部署守卫）
阶段三（web.go 归位）    ← 可选，等下次需要动管理台时再做
```

**阶段一与「耗时统计」功能的协同**：统计功能需要「收到消息的时间戳」。在 `Message` 未拆分时，
这需要同时改入站边界（`Dispatch`）与出站边界（`Reply*`）；拆分后只需给 `Inbound` 加一个
`ReceivedAt` 字段 + 1 个构造点。**建议两者合并交付** —— 用新功能验证新结构，一次拿到收益。

---

## 8. 不要做的事（负面清单）

| 反模式 | 为什么 |
| --- | --- |
| 给 `Env` 的 12 个字段都拆窄接口 | 大多数是不可变配置（实测），拆分只把复杂度从结构体搬到签名上（R1） |
| 抽 `qqapi.Client` 接口 | 30 个方法、测试已用 `httptest` 冒充服务器工作良好，净增抽象 |
| 把 `web.go` 搬进 `internal/server` | 会让 HTTP 层直接依赖 `store`，并把领域逻辑搬到边界层，违反 G1/G2 |
| 拆分 `bot/commands.go` | 编排层本就是聚合点，拆分只增加跳转成本 |
| 顺手修 `SendCard` 的 `nextSeq()` 多消耗 | 既有行为，影响面大于收益（R1）；记录在案即可 |
| 为「更好的分层」在 `bot` 与 `schedule` 之间加一层 | 当前只有 3 层且测试健康；加层是净增复杂度 |
| 让 `admin`（若建）依赖 `gin` | 必须保持与 `schedule` 同样的纯净度，否则 §6.1 的断言失效 |

---

## 9. 结论

**核心分离是「内核对框架依赖的分离」+「接口倒置」，执行得很彻底** ——
`schedule` 可以在没有 HTTP、没有 SQLite、没有绘图库的环境里编译和测试，
这是本项目最值钱的架构资产（G1/G2）。

四个「耦合」（作用域、指令注册、revision 写入、KV 命名空间）都是**有意的收敛**，
耦合只存在一次，不是失控蔓延。阶段一已消除 `Message` 把入站/出站/可变状态压成一个
类型的结构性成本；阶段二已删除 `Env.Store`，让机器人入口只能通过
`schedule.PushStore` / `schedule.PanelStore` 访问持久化。

剩余 D2（`web.go` 错位）仍在可维护范围内。它没有实际改动痛感，按 §7 等到下一次需要动
管理台时再做，避免现在为了分层额外引入 `internal/admin`。
