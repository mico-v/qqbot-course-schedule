# 机器人接入清单

> 目的：把机器人连上 QQ 开放平台并跑通全部功能（M0–M5 已实现）。
> 当前能力：webhook 验签、课表图片、ICS 导入导出、休假调休、时长榜、指令面板、自定义菜单、
> 定时推送、卡片按钮框架（内邀）、Web 管理台。

---

## 1. 你需要准备的

| # | 事项 | 说明 |
| --- | --- | --- |
| 1 | QQ 开放平台账号 + 机器人 | https://q.qq.com/ ，个人开发者即可注册 |
| 2 | `AppID` / `AppSecret` | 机器人详情页「开发设置」获取，**不要发到聊天里** |
| 3 | 公网 HTTPS 入口 | 域名 + 证书；回调端口只能是 **80 / 443 / 8080 / 8443** |
| 4 | 测试 QQ 群 | 把机器人拉进群（沙箱环境下可先自测） |
| 5 | 服务器 | 能跑 Go 二进制即可；有静态 IP 更稳（平台可选 IP 白名单） |

本地没有公网时，用 `cloudflared tunnel --url http://localhost:8080` 也能拿到 443 的临时 HTTPS 地址做验证。

---

## 2. 平台侧配置

1. 创建机器人，记录 `AppID` / `AppSecret`。
2. 「开发设置 → 回调配置」：
   - 回调地址：`https://<你的域名>/webhook`（反代到 443 时不用带端口）
   - 直接暴露端口时必须是 80/443/8080/8443 之一
3. 勾选事件：

| 事件 | 用途 |
| --- | --- |
| 群 @ 消息 `GROUP_AT_MESSAGE_CREATE` | 群内指令（必需） |
| 单聊消息 `C2C_MESSAGE_CREATE` | 私聊指令（必需） |
| 互动事件 `INTERACTION_CREATE` | 卡片按钮回调（需 `buttons=true`，自定义按钮为内邀能力） |
| 群消息接收开关 `GROUP_MSG_RECEIVE` / 单聊 `C2C_MSG_RECEIVE` | 主动推送资格（群管理员在机器人资料页开启） |
| 全量群消息 `GROUP_MESSAGE_CREATE` | 非 @ 消息处理（可选，需申请） |

4. 保存回调配置时平台会立即发 `op=13` 验证请求，**服务必须已经在线**，否则保存失败。
5. 沙箱/正式环境：沙箱用于开发联调；正式发布需平台审核，审核通过后按需配置 IP 白名单。

> 主动推送除了机器人侧 `/启用推送`，还需要**群管理员**在机器人资料页打开「消息推送」，
> 否则发送返回 40034105（机器人会自动暂停该订阅并在下次成功后恢复）。

---

## 3. 服务器侧配置

```bash
git clone git@github.com:mico-v/qqbot-course-schedule.git
cd qqbot-course-schedule
go env -w GOPROXY=https://goproxy.cn,direct   # 本机 proxy.golang.org 不可达
cp config.example.json config.json
$EDITOR config.json
go run ./cmd/bot            # 或 ./deploy/deploy.sh 一键部署到远端
```

`config.json` 关键项：

| 键 | 值 |
| --- | --- |
| `bind` / `port` | 生产建议 `127.0.0.1` + 任意端口（如 `18080`），由反代对外；直接对外时 `port` 必须是 80/443/8080/8443 |
| `appid` / `secret` | 机器人凭据（secret 同时用于 webhook 验签） |
| `domain` | 默认 `https://api.bot.qq.com`，不用改 |
| `public_base_url` | 你的公网地址（卡片图片与导出文件用，如 `https://bot.example.com`） |
| `admin_password` | Web 管理台密码；留空时仅服务器本机可访问 |
| `push_cron` | 每日推送（默认 `30 7 * * *`，本地时区） |
| `buttons` | 卡片按钮（markdown+keyboard）；官方内邀能力，默认 `false` |

Caddy 反代要点（模板见 `deploy/qqbot.caddy`）：公网只放行
`/webhook`、`/healthz`、`/images/*`、`/files/*`；`/admin`、`/api/*` 不公开（走内网或 Basic Auth 白名单）。

---

## 4. 验证步骤（按顺序）

```bash
# 1. 健康检查
curl -s https://<你的域名>/healthz          # {"ok":true,"version":"..."}

# 2. 未签名 webhook 必须被拒
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<你的域名>/webhook -d '{}'   # 401
```

3. 平台「回调配置」保存成功，日志出现 `webhook 验证请求已应答`。
4. 群里 @机器人：
   - `/ping` → `pong`；`/help` → 指令列表
   - 发送 `日历.ics` 附件（或 `/导入课表` + 附件）→ `已创建 <昵称> 的课表：N 个课程事件。`
   - `/今日课表`、`/明日课表`、`/课表 9.17` → 卡片图片
   - `/上课时长榜` → 榜单图片
   - `/休假 明天`、`/假期`、`/销假 明天` → 标记与取消
   - `/导出课表` → 收到 `.ics` 文件
   - `/启用推送` + `/推送测试` → 立即收到一次主动推送（群内需管理员）
5. 管理台（可选）：`http://<ZeroTier 或本机 IP>:18081/admin` 或经反代白名单访问。

---

## 5. 常见失败对照

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 平台保存回调失败 | 服务不在线 / 端口不合规 / 回包格式错 | 先 `curl /healthz`；看日志有没有 op=13；确认反代未改 body |
| 请求 401（webhook） | `secret` 配错；反代压缩/重写 body | 核对 AppSecret；关闭反代 body 修改 |
| 群里 @ 无响应 | 事件没勾、机器人未进群、消息未 @ 机器人 | 检查平台事件订阅；看日志是否收到 webhook |
| 发消息失败 `40034005` | 被动回复窗口过期（5 分钟） | 重新发送指令 |
| 主动推送 `40034105` | 群未开启「消息推送」 | 让群管理员在机器人资料页打开；机器人已自动暂停订阅 |
| 主动推送 `40034100` | 主动消息频控 | 稍后重试（群 20qpm、单群 1000/天） |
| 图片/文件发送失败 | `public_base_url` 不可达或 `/images`、`/files` 未放行 | 公网 `curl` 一次卡片 URL 验证 |
| 管理台 403/404 | 未设 `admin_password` 且非本机访问；或公网未放行 `/admin` | 设置密码或走 ZeroTier/隧道入口 |
| 导入提示解析失败 | 文件不是 iCalendar / 编码异常 | 失败原件留存于 `data/failed_ics/`，看日志定位 |
| 服务起不来 | `config.json` 缺失或字段非法 | 启动日志会给出中文原因 |

---

## 6. 本地联调小抄

```bash
# SSH 隧道访问管理台（无需密码时也要求 loopback；设置密码后用 Basic Auth）
ssh -N -L 18080:127.0.0.1:18080 <host>
# 浏览器打开 http://127.0.0.1:18080/admin

# 实时日志
ssh <host> 'journalctl -u qqbot-course-schedule -f'

# 一键部署（本地执行）
./deploy/deploy.sh --skip-tests
```
