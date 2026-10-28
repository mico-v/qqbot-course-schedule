# QQ 群机器人接入清单

本文只覆盖当前产品：QQ 群聊课表机器人。

## 1. 准备

| 项 | 要求 |
| --- | --- |
| QQ 开放平台 | https://q.qq.com/ |
| 机器人凭据 | `AppID`、`AppSecret` |
| 公网入口 | HTTPS 域名或反向代理 |
| QQ 群 | 机器人已加入的测试群 |
| 服务器 | 可运行单二进制并持久化 `data/` |

本地联调可使用：

```bash
cloudflared tunnel --url http://127.0.0.1:18080
```

## 2. 平台事件

至少订阅：

| 事件 | 用途 |
| --- | --- |
| `GROUP_AT_MESSAGE_CREATE` | 群内 @ 指令 |
| `INTERACTION_CREATE` | 卡片按钮互动 |

需要处理普通群消息和成员观察时，再申请/开启：

| 事件 | 用途 |
| --- | --- |
| `GROUP_MESSAGE_CREATE` | 全量群消息，用于 seen 成员 |
| `GROUP_MSG_RECEIVE` | 群消息接收状态 |

单聊事件不需要订阅。即使平台推送 `C2C_MESSAGE_CREATE`，服务也只 ACK 并忽略。

## 3. 回调

回调地址：

```text
https://<域名>/webhook
```

要求：

- 外部由 Caddy/Nginx 终止 TLS。
- Go 服务默认只监听 `127.0.0.1:18080`。
- 反向代理不得改写原始 body。
- 保存回调时平台会发送 `op=13`，服务必须在线。
- 未通过 Ed25519 验签的请求返回 401。

## 4. 本地配置

```bash
git clone git@github.com:mico-v/qqbot-course-schedule.git
cd qqbot-course-schedule
cp config.example.json config.json
$EDITOR config.json
go run ./cmd/bot
```

关键配置：

| 键 | 说明 |
| --- | --- |
| `bind` | 生产建议 `127.0.0.1` |
| `port` | 默认 `18080`；直接对外时必须是 80/443/8080/8443 |
| `appid` / `secret` | 平台凭据；`secret` 同时用于 Webhook 验签 |
| `public_base_url` | 图片和文件对外 URL |
| `database` | SQLite 路径 |
| `data_dir` | 图片、文件、头像与失败 ICS |
| `admin_password` | 管理台密码；空值时仅 loopback 可访问 |
| `buttons` | 是否发送卡片按钮，平台能力可能需内邀 |

WebUI「机器人设置」可切换：

- **隔离模式**：每个群只使用本群课表。
- **全局模式**：当前群没有有效课表时，跨群查找该成员已有课表。

## 5. 验证

```bash
curl -s http://127.0.0.1:18080/healthz
# {"ok":true,"version":"..."}

curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST http://127.0.0.1:18080/webhook -d '{}'
# 401
```

群内验证：

1. `/ping` 返回 `pong`。
2. `/help` 返回指令列表。
3. 发送 `.ics` 文件后收到导入结果。
4. `/课表` 返回群课表图片。
5. `/休假 明天`、`/假期`、`/销假 明天` 返回对应结果。
6. `/上课时长榜` 返回榜单图片。
7. `/修改课程表` 返回 token 限定的公开编辑链接。

管理台：

```text
https://<域名>/login
```

## 6. 数据迁移

首次使用 schema v7 的新版启动时：

1. 检测 v6 数据库。
2. 生成 `.v6.<timestamp>.bak` 备份。
3. 在事务内迁移群数据。
4. 保留旧表用于回滚。
5. 健康检查成功后完成切换。

部署后建议检查：

```bash
sqlite3 /opt/qqbot-course-schedule/data/course_schedule.sqlite3 \
  "SELECT value FROM metadata WHERE key='schema_version';"
```

期望结果：`7`。

## 7. 常见问题

| 现象 | 检查 |
| --- | --- |
| 平台保存回调失败 | 服务在线、域名/端口、op=13、反向代理 body |
| Webhook 401 | `secret`、签名头、原始 body |
| 群内无响应 | 机器人是否进群、是否 @、事件是否订阅 |
| 图片发送失败 | `public_base_url`、`/images/*`、`/files/*` |
| 管理台不可用 | `admin_password`、代理、loopback 限制 |
| 导入失败 | ICS 编码/格式，查看 `data/failed_ics/` 和日志 |
| 启动失败 | `config.json` 和完整启动错误 |

## 8. 部署与日志

```bash
./deploy/deploy.sh
ssh <host> 'journalctl -u qqbot-course-schedule -f'
ssh <host> 'systemctl status qqbot-course-schedule'
```

详细部署参数见 [deploy/README.md](deploy/README.md)。
