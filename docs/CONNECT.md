# 机器人接入清单

> 目的：把 M0 骨架真正连上 QQ，并跑通"群里 @机器人 → 回复 pong"。
> 当前代码：webhook 验签、Op=13 回包、AccessToken、文本收发、指令路由（`/ping`、`/help`）。

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
   - 回调地址：`https://<你的域名>/webhook`（如反代后是 443，则不带端口）
   - 直接暴露端口时必须是 80/443/8080/8443 之一
3. 勾选事件（M0 只需要前两个，后续按需增加）：

| 事件 | 用途 | 引入版本 |
| --- | --- | --- |
| 群 @ 消息 `GROUP_AT_MESSAGE_CREATE` | 群内指令 | M0 |
| 单聊消息 `C2C_MESSAGE_CREATE` | 私聊指令 | M0 |
| 互动事件 `INTERACTION_CREATE` | 按钮回调（自定义按钮为内邀能力） | M4 |
| 群消息接收开关 `GROUP_MSG_RECEIVE` / 单聊 `C2C_MSG_RECEIVE` | 主动推送资格（群管理员在机器人资料页开启） | M4 |
| 全量群消息 `GROUP_MESSAGE_CREATE` | 非 @ 消息处理（需申请） | 可选 |

4. 保存回调配置时平台会立即发 `op=13` 验证请求，**服务必须已经在线**，否则保存失败。
5. 沙箱/正式环境：沙箱用于开发联调；正式发布需平台审核，审核通过后按需配置 IP 白名单。

> 主动推送（定时课表）除了机器人侧开关，还需要**群管理员**在机器人资料页打开"消息接收"；
> M0 不涉及，M5 实现后再处理。

---

## 3. 服务器侧配置

```bash
git clone git@github.com:mico-v/qqbot-course-schedule.git
cd qqbot-course-schedule
go env -w GOPROXY=https://goproxy.cn,direct   # 本机 proxy.golang.org 不可达
cp config.example.json config.json
$EDITOR config.json
go run ./cmd/bot            # 或 go build -o bin/bot ./cmd/bot && ./bin/bot
```

`config.json` 必填项：

| 键 | 值 |
| --- | --- |
| `port` | 与平台回调端口一致（80/443/8080/8443） |
| `appid` | 机器人 AppID |
| `secret` | 机器人 AppSecret（同时用于 webhook 验签） |
| `domain` | 默认 `https://api.bot.qq.com`，不用改 |
| `public_base_url` | 你的公网地址（M2 发图片时用，现在可留空） |

Caddy 反代示例（443 → 本机 8080）：

```
bot.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

防火墙只需放开 HTTPS 入口；服务本身监听 `0.0.0.0:<port>`。

---

## 4. 验证步骤（按顺序）

```bash
# 1. 服务健康检查
curl -s https://<你的域名>/healthz
# 期望：{"ok":true,"version":"dev"}

# 2. 未签名请求必须被拒
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<你的域名>/webhook -d '{}'
# 期望：401
```

3. 平台「回调配置」点保存：
   - 服务日志出现 `webhook 验证请求已应答` → 验签与回包正确；
   - 页面提示配置成功。
4. 群里 @机器人 发送 `/ping`：期望收到 `pong`，日志出现 `执行指令 prefix=/ping`。
5. 发送 `/help`：期望收到指令列表（含 M1–M5 占位指令）。

---

## 5. 需要我配合时提供什么

- **不要**在聊天/issue 里贴 `AppSecret`。两种方式都行：
  1. 你自己在服务器 `config.json` 填好并启动，然后把现象/日志贴给我；
  2. 本地调试时我可以代跑（config.json 已在 `.gitignore`，不会入库）。
- 需要你确认的信息：
  - 部署形态：直接在服务器跑二进制 / Docker / 本地隧道联调？
  - 公网域名与端口
  - 是否有可用的测试群（沙箱机器人需要先进沙箱群）
  - 是否需要我生成 systemd unit / Dockerfile

---

## 6. 常见失败对照

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 平台保存回调失败 | 服务不在线 / 端口不在 80·443·8080·8443 / 回包格式错 | 先本地 `curl /healthz`；看日志有没有 op=13；确认反代未改 body |
| 请求 401 | `secret` 配错；反代做了压缩/重写导致 body 变化 | 核对 AppSecret；关闭反代的 body 修改 |
| 群里 @ 无响应 | 事件没勾、机器人未进群、消息未 @ 机器人 | 检查平台事件订阅；日志是否收到 webhook |
| 回复发送失败 | AccessToken 取不到（AppID/Secret 错）或域名不可达 | 日志看 `获取 AccessToken 失败`；检查出网 |
| 服务起不来 | `config.json` 缺失或 `port` 非法 | 启动日志会给出中文原因 |

---

## 6.1 主动推送与按钮

- 定时推送：群里由管理员发 `/启用推送`；平台侧还需群管理员在机器人资料页打开「消息推送」，
  否则发送会返回 40034105。`push_cron` 默认 `30 7 * * *`（服务器本地时区）。
- 按钮：官方"自定义按钮"目前为**内邀开通**，且按钮只挂在 markdown 消息上。
  开通后把 `config.json` 的 `buttons` 设为 `true`，卡片会以 markdown+图片+按钮发送，
  失败自动回退媒体消息；同时需要在平台勾选「互动事件」。

## 7. 下一步（M1）

M0 连通后进入 M1：SQLite 三表 + ICS 解析 + RRULE 展开 + `/课表` `/今日课表` 文字版，
随后 M2 接卡片渲染与图片上传（依赖 `public_base_url` 生效）。
