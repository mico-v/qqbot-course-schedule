# qqbot-course-schedule 功能规格与决策

> 状态：当前产品规格。
> 架构实现见 [ARCHITECTURE.md](ARCHITECTURE.md)，开发流程见 [DEV-GUIDE.md](DEV-GUIDE.md)。
> 变更流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 1. 定位

该项目是一个只服务 QQ 群聊的独立课表机器人：

- 接收 QQ 官方 Webhook。
- 使用 SQLite 保存群成员课表、标记、设置和统计。
- 使用纯 Go 渲染课表卡片和榜单。
- 通过 QQ 官方 API v2 发送图片、文件、文本和 Markdown。
- 提供一个 Vue 管理台和 token 限定的公开编辑页。
- 保持单二进制部署，不依赖 AstrBot、OneBot、NapCat 或外部浏览器。

## 2. 边界

### 2.1 目标范围

- QQ 群聊。
- 群成员课表隔离。
- Webhook、被动回复和公开编辑链接。
- ICS 与 WakeUp 分享导入。
- 日卡、榜单、休假/调休。
- Web 管理台、批量导入导出、备份恢复。

### 2.2 非目标

- 单聊业务。
- 频道/Guild。
- 未开启全局模式时跨群共享个人课表。
- 主动推送、定时推送。
- AI/LLM 工具接口。
- 用户自定义 QQ 号作为身份。
- 多机器人实例和分布式调度。

### 2.3 兼容数据

`private:*`、`global-user:*`、`schedule_members`、`course_events` 和 `kv_data`
只用于 v6 迁移、回滚或只读导出。当前产品不会从这些兼容入口创建新业务数据。

## 3. 身份与数据

### 3.1 群成员

```go
GroupMemberKey{
	GroupOpenID:  "<group_openid>",
	MemberOpenID: "<member_openid>",
}
```

- `member_openid` 是平台标识，不是 QQ 号、昵称或人工命名。
- 同名字面值出现在不同群时必须隔离。
- `display_name`、`qq`、`union_openid` 只用于展示或附加能力。
- `union_openid` 可以缺失，不影响查询、导入、渲染或编辑。

### 3.2 revision

| 输入 | 行为 |
| --- | --- |
| `expectedRevision=0` | 只创建；已存在返回冲突 |
| `expectedRevision>0` | revision 精确匹配才更新 |
| 0 行受影响 | 区分 not-found 和 conflict |
| 成功 | revision +1 |

### 3.3 上限

| 常量 | 值 |
| --- | ---: |
| ICS 文件 | 2 MiB |
| 单成员事件 | 120 |
| 单次批量创建成员 | 200 |
| 单群标记 | 1000 |
| 单次标记范围 | 180 天 |
| 课程名/地点/成员名 | 200 字符 |
| 备注 | 2000 字符 |
| RRULE | 500 字符 |
| 榜单范围 | 366 天 |

## 4. 功能规格

### 4.1 指令

| 指令 | 说明 |
| --- | --- |
| `/ping` | 连通性测试，返回 `pong` |
| `/help`、`/帮助` | 指令列表 |
| `/课表 [日期/时间]` | 群课表卡片 |
| `/上课时长榜 [范围]` | 群内榜单；别名 `/上课排行`、`/本周上课排行`、`/学习时长榜` |
| `/休假`、`/放假` | 标记休假 |
| `/调休`、`/补课`、`/调课` | 用来源日期课程覆盖目标日期 |
| `/销假`、`/取消休假`、`/取消调休` | 取消标记 |
| `/假期`、`/假期列表`、`/调休列表`、`/休假列表` | 列出标记 |
| `/wakeup` | 通过 WakeUp 分享链接或口令导入 |
| `/修改课程表` | 生成仅能编辑自己课表的临时链接 |
| `/绑定QQ`、`/解绑QQ` | 可选头像绑定 |
| `/nickname`、`/昵称`、`/nikname` | 设置自己的群内展示名 |
| `/同步面板` | 管理员同步群指令面板，不进入公开面板 |

### 4.2 课表存储模式

| 模式 | 行为 |
| --- | --- |
| 隔离模式（`group`） | 每个群只读取本群课表 |
| 全局模式（`global`） | 当前群没有可用课表时，跨群查找该成员课表并投影到当前群 |

全局查找规则：

1. 当前群已有事件的课表优先。
2. 有 `union_openid` 时先精确匹配同一 union 的课表。
3. 没有 union 时，按 `member_openid` 查找最近更新的跨群课表。
4. 空课表不遮蔽全局课表。
5. 全局模式命中其他群课表时，WebUI 编辑和删除作用于实际来源课表。

附件行为：

- 群内发送 `.ics` 文件自动导入当前发送者。
- WakeUp 分享文本自动识别并导入。
- 文件解析失败时保留到 `data/failed_ics/`。

### 4.2 课表查询

- 日期支持绝对日期、相对日期、中文日期和学期解析。
- 不带年份时按 8 月 1 日学期边界解析。
- `/课表` 默认今天，可带日期和查看时刻。
- 卡片包含成员状态、课程时间、地点、调休提示和未上课成员收纳区。
- 无课表时返回明确文本提示。

### 4.3 日卡状态

| 状态 | 条件 |
| --- | --- |
| `holiday` | 当天休假 |
| `active` | 正在上课 |
| `upcoming` | 下一节即将开始 |
| `finished` | 当天课程已结束 |
| `none` | 当天无课 |

排序：状态优先级 → 时间 → 名称 → OpenID。

### 4.4 榜单

- 默认本周，支持今天/本周/上周/本月/上月/自定义范围。
- 重叠课程按时间区间的并集计数。
- 跨天 occurrence 裁剪到统计窗口。
- 全天事件不参与时长榜。
- 最多 366 天，默认展示前 20 名。

### 4.5 休假与调休

- 默认只作用于发送者本人。
- 群管理员显式指定成员或“全体成员”才能操作他人。
- 群管理员不自动等于可操作所有人。
- 成员标记覆盖同日群级标记。
- 调休来源日期和目标日期必须在同一学期。
- 过去日期可标记，但只影响查询和统计。
- 重新导入 ICS 不清除标记。

### 4.6 ICS 与 WakeUp

- 支持 UTF-8、UTF-16、GBK。
- 支持 RRULE、RDATE、EXDATE、时区和嵌套组件。
- 保留 `RAW_ICAL` 和未知 iCalendar 属性。
- 事件按开始时间排序后写入。
- WakeUp 支持分享链接、分享码和分享文案。

### 4.7 头像

- 机器人头像来自 `/users/@me`，内存和磁盘缓存。
- 群成员官方接口不提供头像。
- 成员可自愿绑定 QQ 号，用 qlogo 获取头像。
- QQ 号只用于头像，不是身份主键。
- 未绑定或获取失败时回退为名称首字和稳定底色。

## 5. Web 管理台

### 5.1 鉴权

- `/login` 登录页。
- HttpOnly、SameSite=Lax 会话 Cookie。
- 密码来自 `admin_password`。
- 未设置密码时仅允许 loopback。
- 登录失败限流。

### 5.2 API

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/scopes` | 群列表和成员摘要 |
| GET | `/api/schedule` | 成员课表 |
| POST | `/api/schedule/save` | revision CAS 保存 |
| POST | `/api/schedule/delete` | 删除当前模式命中的成员课表 |
| GET | `/api/members` | 待创建成员 |
| POST | `/api/schedule/create` | 批量创建空课表 |
| GET/POST | `/api/settings` | 机器人设置 |
| GET | `/api/overrides` | 休假/调休列表 |
| POST | `/api/overrides/set` | 设置标记 |
| POST | `/api/overrides/delete` | 删除标记 |
| GET | `/api/export` | ICS ZIP 或原始备份 |
| POST | `/api/import` | ZIP、ICS 或 JSON 导入 |
| GET/POST | `/api/public/schedule*` | token 限定公开编辑 |

HTTP 错误统一包含 `error` 和 `code`；revision 冲突返回 409，失效 token 返回
404/410。

### 5.3 设置

- 总开关。
- 无斜杠、斜杠、@ 机器人三种回复策略。
- 图片或 Markdown 发送格式。
- 机器人昵称。
- 公开回调地址。
- 编辑链接 TTL。
- 管理员 OpenID 名单。
- 隔离/全局存储模式切换。

## 6. QQ 平台能力

| 能力 | 当前用法 |
| --- | --- |
| 群 @ 消息 | `GROUP_AT_MESSAGE_CREATE` |
| 全量群消息 | `GROUP_MESSAGE_CREATE` |
| 互动事件 | `INTERACTION_CREATE`，先 ACK |
| 单聊消息 | 接收后忽略 |
| 文本/Markdown | 群消息 `msg_type=0/2` |
| 图片/文件 | URL 上传后 `msg_type=7` |
| 被动回复 | `msg_id` 或 `event_id`，5 分钟窗口 |
| 指令面板 | 仅 `scope=group` |
| 自定义菜单 | 不在当前产品范围 |
| Webhook 验签 | Ed25519 + `op=13` |

平台限制：

- 群成员列表和按钮能力可能需要内邀。
- 群成员官方接口不返回头像。
- 被动回复窗口和次数由平台限制。
- 不支持彩色 emoji 渲染。

## 7. 验收

### 7.1 身份

- 两群相同 `member_openid` 产生两条成员、两份课表和两套事件。
- 全局模式可在当前群无有效课表时命中其他群课表。
- 隔离模式不读取其他群课表。
- union 缺失时完整可用。
- display_name 变化不影响身份。
- 删除成员级联删除课表、事件和编辑链接。

### 7.2 存储

- revision 0 只创建。
- stale revision 返回 conflict。
- 迁移前生成备份。
- 坏数据导致事务回滚。
- 迁移后 `foreign_key_check` 通过。

### 7.3 功能

- ICS 导入后 `/课表` 可见。
- 休假/调休影响卡片和榜单。
- WebUI 409 提示有效。
- WebUI 删除成员课表有二次确认，删除后成员回到待创建列表。
- 公开链接只能编辑绑定成员。
- ZIP、ICS、JSON 导入导出闭环。

### 7.4 CI

```bash
COVERAGE_STAGE=p2 bash scripts/test-all.sh
RUN_WEB=1 COVERAGE_STAGE=p2 bash scripts/test-all.sh
```

Go 必须通过格式、架构、vet、race、shuffle、覆盖率、构建和卡片 smoke test。
Web 必须通过 lint、Vitest、typecheck、build 和 `web/dist` 一致性检查。

## 8. 决策记录

| # | 决策 |
| --- | --- |
| D1 | 只做群聊业务 |
| D2 | 身份使用 `(group_openid, member_openid)` |
| D3 | 形象字段不参与身份 |
| D4 | 新业务不跨群合并课表 |
| D5 | SQLite 单连接、显式 SQL、单实例 |
| D6 | schema v7 为当前模型，v6 只用于迁移/回滚 |
| D7 | 纯 Go 图片渲染，不做浏览器渲染 |
| D8 | 图片/文件通过公网 URL 上传 |
| D9 | 管理员名单优先于平台群角色 |
| D10 | 管理台和公开编辑共用 revision 语义 |
| D11 | 不恢复主动推送 |
| D12 | 不用文档记录已完成任务；历史由 Git 保存 |
