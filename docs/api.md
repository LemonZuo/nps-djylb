# NPS Web API 文档

管理后台已重构为前后端分离：所有管理操作走 `/api/v1` 下的 JSON API，认证使用 JWT Bearer Token。旧版 Beego 表单接口（`/index/*`、`/client/*`、`/login/*`、`/global/*`）已移除，文末附新老接口对照表。

## 通用约定

- **前缀**：所有接口位于 `<web_base_url>/api/v1` 下（未配置 `web_base_url` 时即 `/api/v1`）。
- **请求体**：`Content-Type: application/json`。
- **响应体**：统一信封结构：

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... },
  "requestId": "a1b2c3d4e5f6a7b8"
}
```

| code | HTTP 状态 | 含义 |
|------|-----------|------|
| `0` | 200 | 成功 |
| `40000` | 400 | 请求参数错误 |
| `40100` | 401 | 未认证 / 凭证无效，需重新登录 |
| `40101` | 401 | Token 过期（与 40100 区分以便前端提示） |
| `40300` | 403 | 权限不足（需管理员） |
| `40400` | 404 | 资源不存在 |
| `40900` | 409 | 冲突（如端口占用、vkey 重复） |
| `42900` | 429 | 触发限流或登录封禁 |
| `50000` | 500 | 服务端内部错误（可用 `requestId` 关联日志） |

- **列表接口**：均支持 `offset` / `limit` / `search` / `sort` / `order`（`asc` / `desc`）查询参数，返回 `data: { rows: [...], total: n }`。隧道列表额外支持 `clientId`、`type` 过滤。

## 认证方式

### 方式一：JWT Bearer Token（SPA 使用）

登录成功后获得 token，之后每个请求携带：

```
Authorization: Bearer <token>
```

### 方式二：auth_key + timestamp（第三方脚本兼容）

与旧版完全兼容，通过 **查询参数** 附带（不再要求 POST 表单）：

- `timestamp`：当前 Unix 时间戳（秒级），允许偏差 20 秒；
- `auth_key`：`md5(配置文件中的 auth_key + timestamp)`。

该方式始终以管理员身份访问。需在 `nps.conf` 中配置 `auth_key` 并取消注释，留空则此通道关闭。

```bash
ts=$(date +%s)
key=$(printf '%s%s' "your_auth_key_here" "$ts" | md5sum | cut -d' ' -f1)
curl "http://127.0.0.1:8888/api/v1/clients?auth_key=$key&timestamp=$ts&limit=10"
```

## 登录流程

登录不是单次明文提交，包含 RSA 加密、nonce 防重放，并可能要求验证码 / PoW（工作量证明）。

### 1. 获取挑战

`GET /api/v1/auth/challenge` → `data`：

| 字段 | 说明 |
|------|------|
| `nonce` | 一次性随机串，登录时须封入密文 |
| `publicKey` | RSA 公钥（PEM），用于加密密码载荷 |
| `powRequired` / `powBits` | 是否强制 PoW 及难度（前导零比特数） |
| `captchaOpen` / `captcha` | 是否需要验证码；`captcha` 含 `id` 与 base64 `image` |
| `totpLen` | TOTP 动态码位数（默认 6） |
| `registerAllowed` / `userLoginAllowed` / `vkeyLoginAllowed` | 功能开关 |
| `loginDelayMs` | 失败后建议的重试间隔（毫秒） |
| `serverTime` | 服务端毫秒时间戳（用于校正客户端时钟偏差） |

### 2. 构造密文

将 JSON `{"n": "<nonce>", "t": <毫秒时间戳>, "p": "<明文密码>"}` 用 `publicKey` 做 RSA PKCS#1 v1.5 加密，base64 编码，作为 `password` 提交。时间戳偏差超过 `login_max_skew`（默认 5 分钟）会被拒绝，可用 `serverTime` 校正。

若启用 TOTP，6 位动态码追加在验证码内容之后（未开验证码时追加在明文密码之后）。

### 3. PoW（如要求）

当 `powRequired` 为 true（或失败响应中返回 `bits`）时，求 `powX` 使 `sha256(password密文字符串 + powX)` 有至少 `bits` 个前导零比特，随请求提交 `powX` 与 `bits`。

### 4. 登录

`POST /api/v1/auth/login`

```json
{
  "username": "admin",
  "password": "<RSA密文base64>",
  "captchaId": "...",
  "captchaCode": "...",
  "powX": "...",
  "bits": 20
}
```

成功 → `data`：

```json
{
  "token": "<JWT>",
  "expiresAt": 1785058190,
  "user": { "username": "admin", "isAdmin": true, "clientId": 0, "permissions": { ... } }
}
```

失败（401）时 `data` 中携带新的重试材料：`nonce`、可能的 `bits` / `cert` / `timestamp` / `captcha`，可直接用于下一次尝试，无需重新请求 challenge。

### 其他认证接口

| 接口 | 说明 |
|------|------|
| `GET /auth/captcha` | 单独获取新验证码（注册页 / 刷新用；验证码未启用时 404） |
| `POST /auth/register` | 用户注册，`{username, password, captchaId, captchaCode}`，`password` 与登录同构 |
| `GET /auth/me` | 当前登录身份与权限 |
| `POST /auth/logout` | 登出（JWT 无状态，服务端仅记审计日志，客户端应丢弃 token） |

## 元信息与仪表盘

| 接口 | 说明 |
|------|------|
| `GET /meta/bootstrap` | 版本、bridge 端点列表、代理端口、权限开关等前端引导数据 |
| `GET /dashboard` | 仪表盘统计（客户端/隧道/主机数、流量、CPU、内存等），`?force=true` 跳过缓存 |
| `GET /dashboard/history` | 历史采样序列（time / cpu / virtual_mem / tcp / io_recv / io_send） |

## 客户端管理

| 接口 | 权限 | 说明 |
|------|------|------|
| `GET /clients` | 登录 | 列表（普通用户仅见自己） |
| `POST /clients` | 管理员 | 创建，返回生成的 vkey |
| `GET /clients/{id}` | 登录 | 详情 |
| `PUT /clients/{id}` | 登录 | 修改（普通用户仅能改自己，且受权限开关约束） |
| `DELETE /clients/{id}` | 管理员 | 删除（连带其隧道与主机） |
| `POST /clients/{id}/status` | 管理员 | `{"status": true|false}` 启用/停用 |
| `POST /clients/{id}/ping` | 登录 | 延迟检测，返回 `{"rtt": 毫秒}` |
| `GET /clients/{id}/qrcode` | 登录 | TOTP 二维码 PNG（需带认证头） |
| `POST /clients/{id}/clear` | 管理员 | 清理配额/统计 |
| `POST /clients/clear` | 管理员 | 对全部客户端清理 |

创建/修改请求体（字段均可选，只更新提交的字段）：

```json
{
  "remark": "", "verifyKey": "", "rateLimit": 0, "maxConn": 0, "maxTunnelNum": 0,
  "flowLimit": 0, "timeLimit": "", "flowReset": false,
  "configConnAllow": true, "compress": false, "crypt": false,
  "basicUser": "", "basicPassword": "",
  "webUserName": "", "webPassword": "", "webTotpSecret": "",
  "blackIpList": "", "status": true
}
```

`clear` 的 `mode` 取值：`flow` / `flow_limit` / `time_limit` / `rate_limit` / `conn_limit` / `tunnel_limit`。

## 隧道管理

| 接口 | 权限 | 说明 |
|------|------|------|
| `GET /tunnels` | 登录 | 列表，支持 `clientId`、`type`（`tcp`/`udp`/`httpProxy`/`socks5`/`mixProxy`/`secret`/`p2p`/`file`）过滤 |
| `POST /tunnels` | 登录 | 创建，`port` ≤ 0 时自动分配 |
| `GET /tunnels/{id}` / `PUT` / `DELETE` | 登录 | 详情 / 修改 / 删除（用户仅限自己客户端的隧道） |
| `POST /tunnels/{id}/start` / `stop` | 登录 | 启动 / 停止 |
| `POST /tunnels/{id}/toggle` | 登录 | `{"name": "http"|"socks5", "action": "start"|"stop"|"toggle"}`（mixProxy 子开关） |
| `POST /tunnels/{id}/clear` | 管理员 | `mode`: `flow` / `flow_limit` / `time_limit` |

创建/修改请求体（按模式取用）：

```json
{
  "clientId": 1, "mode": "tcp", "port": 10000, "serverIp": "",
  "remark": "", "password": "", "target": "127.0.0.1:80", "targetType": "",
  "proxyProtocol": 0, "localProxy": false, "auth": "user:pass",
  "localPath": "", "stripPre": "", "httpProxy": true, "socks5Proxy": true,
  "destAclMode": 0, "destAclRules": "",
  "flowLimit": 0, "timeLimit": "", "flowReset": false
}
```

## 域名解析（Host）管理

| 接口 | 权限 | 说明 |
|------|------|------|
| `GET /hosts` | 登录 | 列表 |
| `POST /hosts` | 登录 | 创建 |
| `GET /hosts/{id}` / `PUT` / `DELETE` | 登录 | 详情 / 修改 / 删除 |
| `POST /hosts/{id}/start` / `stop` | 登录 | 启动 / 停止 |
| `POST /hosts/{id}/toggle` | 登录 | 属性开关，`name` 取值：`auto_ssl` / `https_just_proxy` / `tls_offload` / `auto_https` / `auto_cors` / `compat_mode` / `target_is_https`；`action` 同隧道 |
| `POST /hosts/{id}/clear` | 管理员 | `mode`: `flow` / `flow_limit` / `time_limit` |

创建/修改请求体主要字段：`clientId`、`host`、`scheme`（`all`/`http`/`https`）、`location`、`pathRewrite`、`redirectUrl`、`remark`、`target`、`targetIsHttps`、`proxyProtocol`、`localProxy`、`headerChange`、`respHeaderChange`、`hostChange`、`auth`、`httpsJustProxy`、`tlsOffload`、`autoSsl`、`autoHttps`、`autoCors`、`compatMode`、`certFile`、`keyFile`、`flowLimit`、`timeLimit`、`flowReset`。

## 全局管理（仅管理员）

| 接口 | 说明 |
|------|------|
| `GET /global` | 读取全局黑名单，`data: {"blackIpList": ["1.2.3.4"]}` |
| `PUT /global` | 保存，`{"blackIpList": [...]}` |
| `GET /auth/bans` | 登录封禁列表（IP 与用户名维度） |
| `DELETE /auth/bans/{key}` | 解除指定封禁 |
| `DELETE /auth/bans` | 解除全部封禁 |

## 新老接口对照

| 旧接口（已移除） | 新接口 |
|------|------|
| `POST /auth/gettime` | 不再需要（challenge 返回 `serverTime`） |
| `POST /auth/getauthkey` | 已移除（直接在服务端配置读取 `auth_key`） |
| `POST /auth/getcert` | `GET /api/v1/auth/challenge`（`publicKey` 字段） |
| `POST /index/stats` | `GET /api/v1/dashboard` |
| `POST /index/gettunnel` | `GET /api/v1/tunnels` |
| `POST /index/add` / `edit` | `POST` / `PUT /api/v1/tunnels` |
| `POST /index/stop` / `start` | `POST /api/v1/tunnels/{id}/stop` / `start` |
| `POST /index/del` | `DELETE /api/v1/tunnels/{id}` |
| `POST /index/cleartunnel` | `POST /api/v1/tunnels/{id}/clear` |
| `POST /index/gethost` | `GET /api/v1/hosts` |
| `POST /index/addhost` / `edithost` | `POST` / `PUT /api/v1/hosts` |
| `POST /index/stophost` / `starthost` | `POST /api/v1/hosts/{id}/stop` / `start` |
| `POST /index/delhost` | `DELETE /api/v1/hosts/{id}` |
| `POST /index/clearhost` | `POST /api/v1/hosts/{id}/clear` 或 `/toggle` |
| `POST /client/list` | `GET /api/v1/clients` |
| `POST /client/add` / `edit` | `POST` / `PUT /api/v1/clients` |
| `POST /client/getclient` | `GET /api/v1/clients/{id}` |
| `POST /client/pingclient` | `POST /api/v1/clients/{id}/ping` |
| `POST /client/changestatus` | `POST /api/v1/clients/{id}/status` |
| `POST /client/clear` | `POST /api/v1/clients/{id}/clear`（全部时用 `/clients/clear`） |
| `POST /client/del` | `DELETE /api/v1/clients/{id}` |
| `GET /client/qr` | `GET /api/v1/clients/{id}/qrcode` |
| `POST /login/verify` | `POST /api/v1/auth/login` |
| `GET /login/out` | `POST /api/v1/auth/logout` |
| `POST /login/register` | `POST /api/v1/auth/register` |
| `POST /global/save` | `PUT /api/v1/global` |
| `POST /global/banlist` | `GET /api/v1/auth/bans` |
| `POST /global/unban` / `unbanall` | `DELETE /api/v1/auth/bans/{key}` / `/api/v1/auth/bans` |

旧接口参数命名为下划线表单字段（`client_id`、`flow_limit`…），新接口一律为 JSON camelCase（`clientId`、`flowLimit`…）；旧布尔 `0`/`1` 改为 JSON `true`/`false`。
