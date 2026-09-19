# QQ 官方机器人 API v2 × AstrBot WebSocket 适配度研究

> 研究时间：基于 2026-07 前后抓取的 `https://bot.q.qq.com/wiki/develop/api-v2/` 文档快照。
> 文档快照：本目录 `*.md`（191 页，含 `INDEX.md`），由 `sitemap.xml` 全量抓取。
> 代码基线：仓库 `astrbot/core/platform/sources/qqofficial/`（WebSocket 适配器）。

---

## 1. QQ 官方 API v2 功能全景

### 1.1 接入方式

| 方式 | 说明 |
| --- | --- |
| WebSocket | `/gateway` 或 `/gateway/bot` 获取 wss 地址；OpCode 0/1/2/6/7/9/10/11；支持 shard 分片；心跳、Resume |
| Webhook | HTTP 回调，OpCode 12/13；需签名校验 |
| 鉴权 | AppID + AppSecret 换取 AccessToken（`QQBot {AccessToken}`），旧 Token 已废弃 |

### 1.2 消息场景

| 场景 | 发送接口 | 接收事件 |
| --- | --- | --- |
| QQ 单聊 (C2C) | `POST /v2/users/{openid}/messages`、`/stream_messages` | `C2C_MESSAGE_CREATE` |
| QQ 群聊 | `POST /v2/groups/{group_openid}/messages` | `GROUP_AT_MESSAGE_CREATE`、`GROUP_MESSAGE_CREATE`（全量需开关） |
| 频道文字子频道 | `/channels/{channel_id}/messages` | `AT_MESSAGE_CREATE`、`MESSAGE_CREATE` |
| 频道私信 | `/dms/{guild_id}/messages` | `DIRECT_MESSAGE_CREATE` |

### 1.3 发送消息格式

| msg_type | 类型 | 字段 | 支持场景 |
| --- | --- | --- | --- |
| 0 | 文本 | `content` | 单聊 / 群 / 频道 |
| 2 | Markdown | `markdown` | 单聊 / 群（频道需内邀） |
| 7 | 富媒体 | `media.file_info`（先上传） | 单聊 / 群 / 频道 |
| — | Ark 结构化卡片 | `ark` | 接收侧为主 |
| — | Embed | `embed` | 仅频道 |
| — | keyboard 按钮 | `keyboard` | 单聊 / 群 / 频道 |
| — | 引用 `message_reference` | 回复 | 全部 |
| — | 流式（`stream`，state=1/10） | 单聊专属 | 单聊 |

富媒体：图片(`file_type=1`)/视频(2)/语音(3)/文件(4)；支持整文件上传与分片上传（>约 10MB 推荐）；软/硬限制 20MB/200MB（图片、语音）、30MB/200MB（视频）、200MB（文件）。

### 1.4 事件 / Intents 全表

| Intent | 事件 |
| --- | --- |
| `GROUP_MEMBER_EVENT (1<<24)` | `GROUP_MEMBER_ADD`、`GROUP_MEMBER_REMOVE`、`GROUP_JOIN_REQUEST` |
| `GROUP_AND_C2C_EVENT (1<<25)` | `C2C_MESSAGE_CREATE`、`C2C_MSG_RECEIVE`、`C2C_MSG_REJECT`、`GROUP_AT_MESSAGE_CREATE`、`GROUP_MESSAGE_CREATE`、`GROUP_ADD_ROBOT`、`GROUP_DEL_ROBOT`、`GROUP_MSG_RECEIVE`、`GROUP_MSG_REJECT`、`FRIEND_ADD`、`FRIEND_DEL`、`SUBSCRIBE_MESSAGE_STATUS` |
| `INTERACTION (1<<26)` | `INTERACTION_CREATE`（按钮/快捷菜单/消息反馈/清空会话/授权…） |
| `MESSAGE_AUDIT (1<<27)` | 消息审核通过/拒绝 |
| `GUILDS (1<<0)` | `GUILD_*`、`CHANNEL_*` |
| `PUBLIC_GUILD_MESSAGES (1<<30)` | 频道消息 `AT_MESSAGE_CREATE` / `MESSAGE_CREATE` |
| `DIRECT_MESSAGE (1<<12)` | 频道私信 |

### 1.5 服务端管理 / 工具接口

- **群管理**：群信息、机器人群内状态、群成员列表/详情、入群申请列表与审批、入群自动审批策略（含白名单）、批量移除成员、群黑名单、禁言状态查询与设置。
- **消息**：发送/撤回（群、单聊、频道）、流式单聊。
- **富媒体**：预上传→分片 PUT→分片确认→合并（群 / 单聊）。
- **用户**：`/users/@me`、`/users/@me/guilds`。
- **自定义菜单与指令面板**：`/v2/menu`（GET/PUT）、`/v2/panels`（增删改查、关联目标）。
- **生成分享链接**：`/v2/generate_url_link`。
- **频道**：子频道增删改查、成员/角色/权限、禁言、公告、日程、精华、论坛、表情表态、音频、小程序等（私域偏多）。
- **互动**：`PUT /interactions/{interaction_id}` 响应按钮点击。

---

## 2. AstrBot WebSocket 适配器实现清单

代码：`qqofficial_platform_adapter.py`（931 行）、`qqofficial_message_event.py`（976 行）、`login_registration.py`、`qqofficial_chunked_upload.py`（620 行）。基于 `qq-botpy==1.2.1`，并对 botpy 打补丁。

### 2.1 关键实现

- **Intents**：`public_messages`（=GROUP_AND_C2C_EVENT）、`public_guild_messages`、`direct_message`。`enable_group_c2c` 控制是否启用群/单聊。
- **补丁**：自定义 `Patched*Message` 保留 `raw_data` / `message_type` / `msg_elements`；注册 `parse_group_message_create`；`PatchedGroupMessage._User` 补齐 `username`、`user_openid`、`member_openid` 等；修正 aiohttp>=3.12 的 `_FormData` 兼容。
- **连接管理**：`ManagedBotWebSocket` 处理关闭与重连抑制；`shutdown()` 优雅关闭。
- **事件入口**（仅 5 个）：`on_group_at_message_create`、`on_group_message_create`、`on_at_message_create`、`on_direct_message_create`、`on_c2c_message_create`。
- **扫码建号**：`login_registration.py` 通过 QQ 绑定接口获取 AppID/Secret（AES-GCM 解密）。

### 2.2 接收解析

- 文本、图片、语音、视频、文件附件；语音经 `MediaResolver` 转 WAV。
- 引用消息（`message_type=103` + `msg_elements`）解析为 `Reply` 组件。
- `<faceType=...>` 表情解析为 `[表情:文本]`。
- 群 @ 机器人：剥离 `<@id>`，注入 `At` 组件。
- 群名/频道名补充：`get_group()` 调 `/v2/groups/{openid}/info` 与 `get_channel`/`get_guild`。

### 2.3 发送能力

- 文本（`msg_type=0`）、Markdown（`msg_type=2`，带「不允许原生 markdown」自动回退 content 模式）。
- 富媒体（`msg_type=7`）：图片、语音（转 tencent_silk）、视频、文件；群与 C2C 均支持；>10MB 走分片上传。
- 单聊流式（state=1/10，含 `\n` 收尾与异常处理）。
- 主动推送：`support_proactive_message=True`；群/单聊无 `msg_id` 时也可发送；`msg_seq` 随机去重。
- 媒体自动拆链：`_split_message_chain_by_media` 保证每条消息最多一个媒体。
- 重试：tenacity 对 500/504/SequenceNumberError/超时等重试。

---

## 3. 适配度矩阵

### 3.1 事件接收

| 事件 | 支持 | 说明 |
| --- | --- | --- |
| `C2C_MESSAGE_CREATE` | ✅ | 完整解析 |
| `GROUP_AT_MESSAGE_CREATE` | ✅ | 含 @ 处理 |
| `GROUP_MESSAGE_CREATE`（全量） | ✅ | 框架补丁支持 |
| `AT_MESSAGE_CREATE`（频道） | ✅ | |
| `MESSAGE_CREATE`（频道全量） | ❌ | botpy 有 parser，但 AstrBot 未注册 handler |
| `DIRECT_MESSAGE_CREATE`（频道私信） | ✅（可开关） | |
| `INTERACTION_CREATE` | ❌ | Intent 未订阅，按钮/菜单点击无法处理 |
| `FRIEND_ADD` / `FRIEND_DEL` | ❌ | 事件被静默丢弃 |
| `GROUP_ADD_ROBOT` / `GROUP_DEL_ROBOT` | ❌ | 静默丢弃 |
| `GROUP_MSG_RECEIVE` / `GROUP_MSG_REJECT` | ❌ | 静默丢弃 |
| `C2C_MSG_RECEIVE` / `C2C_MSG_REJECT` | ❌ | 静默丢弃 |
| `SUBSCRIBE_MESSAGE_STATUS` | ❌ | 静默丢弃 |
| `GROUP_MEMBER_ADD` / `REMOVE` / `GROUP_JOIN_REQUEST` | ❌ | botpy 1.2.1 无对应 Intent flag（1<<24），当前无法订阅 |
| `MESSAGE_AUDIT` | ❌ | Intent 未订阅 |
| `GUILD_*` / `CHANNEL_*` | ❌ | 未订阅 |
| 消息撤回事件 | ❌ | 无 |

### 3.2 消息内容解析（接收侧）

| 内容 | 支持 | 说明 |
| --- | --- | --- |
| 文本 | ✅ | |
| 图片/语音/视频/文件 | ✅ | 附件按 `content_type` 归类 |
| 引用消息 (103) | ✅ | 转 `Reply` + 引用链 |
| 表情 (`<faceType>`) | ✅ | 解析 base64 ext 文本 |
| 结构化卡片 (message_type=3 / `ark_data`) | ❌ | 未解析 |
| `msg_elements`（并行/聊天记录/嵌套） | ⚠️ | 仅用于引用场景取第一个元素 |
| `mentions`（@ 其他用户/多人） | ⚠️ | 仅识别机器人自身，其他 @ 不转 `At` |
| `message_scene.ext`（引用索引/鉴权令牌） | ❌ | 未透传到上层 |
| `msg_idx` / `auth_token` | ❌ | 未使用 |
| 语音 `asr_refer_text` / `voice_wav_url` | ⚠️ | 走 url 转 WAV，未利用 ASR 文本 |

### 3.3 消息发送（发送侧）

| 能力 | 支持 | 说明 |
| --- | --- | --- |
| 文本 `msg_type=0` | ✅ | |
| Markdown `msg_type=2` | ✅ | 默认开启，失败自动回退文本 |
| 图片/语音/视频/文件 `msg_type=7` | ✅ | 群 + 单聊，分片上传 |
| 频道消息 / 频道私信 | ✅ | 走 botpy `post_message` / `post_dms` |
| 单聊流式 | ✅ | 仅 C2C |
| 频道流式 | ❌ | QQ 也不支持 |
| 群流式 | ❌ | QQ 目前流式仅单聊 |
| 主动消息 | ✅ | 群/单聊，无 msg_id 发送 |
| 互动召回 `is_wakeup` | ❌ | 未支持 |
| 引用/回复 `message_reference` | ❌ | 入参存在但从不赋值；`Reply` 组件被忽略 |
| 按钮 `keyboard` | ❌ | API 签名有参数，消息链无法表达 |
| Ark / Embed | ❌ | 消息链无对应组件 |
| 频道表情表态 | ❌ | |
| 消息撤回 | ❌ | 未提供 |
| @ 发送 | ❌ | `At` 组件在发送侧被忽略 |
| 文本超长自动分段 | ❌ | 仅按媒体拆分 |

### 3.4 管理 / 工具接口

| 接口 | 支持 | 说明 |
| --- | --- | --- |
| 群基本信息 `/v2/groups/{id}/info` | ✅ | 仅 `get_group()` 调用 |
| 机器人群内状态 | ❌ | |
| 群成员列表/详情 | ❌ | |
| 入群申请列表/审批 | ❌ | |
| 自动审批策略 | ❌ | |
| 批量移除成员 | ❌ | |
| 群黑名单 | ❌ | |
| 禁言查询/设置 | ❌ | |
| 消息撤回（群/单聊） | ❌ | |
| 单聊/群富媒体上传 | ✅ | 整文件 + 分片 |
| `/users/@me`、`/users/@me/guilds` | ❌ | botpy 有封装，AstrBot 未用 |
| 自定义菜单 `/v2/menu` | ❌ | |
| 指令面板 `/v2/panels` | ❌ | |
| 生成分享链接 | ❌ | |
| 频道子频道/成员/角色/权限/公告/日程/精华/论坛/音频/小程序 | ❌ | 未使用 |
| 互动响应 `PUT /interactions/{id}` | ❌ | |

### 3.5 基础能力

| 能力 | 支持 | 说明 |
| --- | --- | --- |
| AccessToken 鉴权 | ✅ | botpy 内部处理 |
| WebSocket 心跳/重连/Resume | ✅ | botpy + 自定义关闭处理 |
| Shard 分片 | ✅ | botpy 按 `/gateway/bot` 返回的 `shards` 自动建立多分片会话，但 AstrBot 未暴露相关配置 |
| IP 白名单 | ⚠️ | 文档提示，需用户侧配置 |
| 扫码一键建号 | ✅ | 独有便利功能 |
| 富媒体分片上传 | ✅ | 自实现，含并发/重试/配额处理 |
| 优雅停机 | ✅ | |
| 错误重试 | ✅ | 500/504/超时等 |

---

## 4. 总体评价

**结论：对「QQ 群/单聊 + 频道」的消息收发这一核心链路，适配度较高（约 8/10）；对 QQ 官方平台的完整能力面（管理、互动、事件生态），适配度中等偏低（约 3.5/10）。**

### 4.1 做得好的地方

1. **核心消息链路完整且打磨细致**：文本/Markdown/图片/语音/视频/文件在群、单聊、频道均可用；主动推送、单聊流式、分片上传、Markdown 回退、媒体拆链、报价引用解析、表情解析、断线重连与优雅停机都有实现，且配有针对性的测试。
2. **弥补了 botpy 1.2.1 的不足**：通过补丁保留 `raw_data`/`msg_elements`/`message_type`，并补充 `GROUP_MESSAGE_CREATE` 解析，才能支持全量群消息与引用消息。
3. **扫码建号**显著降低了接入门槛。
4. **配置简单**，默认 `use_markdown`、`enable_group_c2c`、`enable_guild_direct_message` 覆盖主流用法。

### 4.2 主要缺口（按影响排序）

1. **事件覆盖窄**：只处理 5 个消息事件。`INTERACTION_CREATE`（按钮/快捷菜单）、入群/退群、好友增删、成员变动、入群申请、消息审核、机器人被移出群等全部被静默丢弃，插件层拿不到。这直接封死了「按钮交互机器人」「欢迎新人」「入群审批」等常见玩法。
2. **`GROUP_MEMBER_EVENT (1<<24)` 无法订阅**：`qq-botpy==1.2.1` 的 `Intents` 没有该 bit，也没有 `on_group_join_request` 等事件类。要支持需升级/自行扩展 Intents+parser。
3. **管理类接口几乎空白**：群成员、禁言、黑名单、批量移除、入群审批、消息撤回、菜单/指令面板、分享链接均未封装。插件只能自行拿到 `botpy` 客户端后用 `Route` 裸调——可用但无统一抽象。
4. **发送侧表达力受限**：不支持 `keyboard` 按钮、Ark/Embed、引用回复、`is_wakeup` 互动召回、@ 发送、消息撤回；长文本也不自动分段。`Reply`/`At` 组件在发送时被忽略。
5. **接收侧卡片与元数据缺失**：`ark_data`（结构化卡片）、`message_scene.ext`、非机器人 `mentions` 未解析，信息有损。
6. **频道侧仅覆盖消息收发**：频道全量消息 `MESSAGE_CREATE`、子频道管理、成员/角色/权限、表情表态、公告、日程、精华、论坛等均未接入。
7. **分片（Shard）不可配置**：botpy 会按网关返回的 `shards` 自动连接全部分片，但 AstrBot 未暴露分片相关配置（多数部署无需关心）。

### 4.3 与 Webhook 适配器的关系

`qqofficial_webhook` 复用同一套 `QQOfficialMessageEvent` 与 `_parse_from_qqofficial`，因此上述能力/缺口与 WebSocket 版基本一致，差异只在事件投递通道（Webhook 无法用 WebSocket 长连接，且受回调端口 80/443/8080/8443 限制）。

---

## 5. 给插件开发者的兜底建议

- 平台原生对象可从事件拿到：`event.message_obj.raw_message`（botpy 消息对象）、`event.bot`（`botpy.Client`）。
- 未被框架封装的能力（按钮、撤回、群管理、菜单等）可直接调用 `event.bot.api._http.request(Route(...))`，按本文档快照里的接口路径自行实现。
- 若要接收更多事件，需要改框架：在 `botClient` 增加对应 `on_xxx` 方法，并按需扩展 Intents（必要时直接位运算 `intents.value |= 1 << 24`）。

---

## 6. 文档快照位置

- 全部页面：`/tmp/qqbot-api-docs/*.md`（191 页）
- 目录索引：`/tmp/qqbot-api-docs/INDEX.md`
- 抓取脚本：`/tmp/crawl_qqdoc.py`
