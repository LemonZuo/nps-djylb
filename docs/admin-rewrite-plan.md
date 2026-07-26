# NPS 管理后台重构方案（feat_fork_v2）

> 起点：`feat_fork_v2` @ `697ab6a3`（v0.34.8 合并点，旧 Beego 模板后台）
>
> 目标：React + shadcn/ui 单页后台 + REST JSON API + JWT 认证 + `go:embed` 单二进制

## 1. 目标与边界

把 Beego 服务端渲染的管理后台，替换为 **React SPA + REST JSON API + JWT 认证**，
构建产物用 `go:embed` 编入 `nps` 单二进制。

**不动的部分**：`bridge/`、`server/proxy/`、`lib/mux`、`lib/p2p`、`lib/file` 数据模型。
数据平面与业务逻辑零改动，只换控制平面的表现层与认证层。

**要删除的部分**：`web/views/`、`web/static/`、`web/controllers/`、`web/routers/`，
以及 Beego 的 session、CSRF、模板渲染、captcha 依赖。

## 2. 技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| 前端框架 | React 19 + TypeScript + Vite | shadcn/ui 的标准宿主 |
| UI | shadcn/ui + Tailwind CSS 4 | 指定 |
| 数据层 | TanStack Query + TanStack Table | 列表页 20+ 列带排序/分页/搜索，对齐旧 bootstrap-table |
| 表单 | react-hook-form + zod | 隧道/域名表单字段多、校验复杂 |
| 图表 | ECharts | 仪表盘沿用，旧版已在用，迁移成本最低 |
| i18n | react-i18next | 中英双语，迁移 `languages.xml` 的 345 条词条 |
| 路由 | React Router v7 | 需支持 `web_base_url` 子路径部署 |
| Go HTTP | 标准库 `net/http`（Go 1.22+ 路由模式） | 见 2.1 |
| 配置解析 | `beego/config` 子包 | 见 2.2 |
| JWT | 自实现 HS256 | 见 2.3 |

### 2.1 不引入 gin / echo / chi

Go 1.22 起 `http.ServeMux` 原生支持 `POST /api/v1/tunnels/{id}` 式路由。
本项目 API 规模（约 45 个端点）完全够用。多引一个 web 框架，
只会让这个以网络库为主的项目多背一层依赖。

### 2.2 摆脱 beego 主包，保留 beego/config

`beego.AppConfig` 全局散落在 `server/`、`bridge/`、`lib/` 等约 20 处文件。
新增 `lib/appconfig` 薄封装（内部用 `beego/config` 子包），
保持 `String() / DefaultBool() / DefaultInt()` 等同名 API，各处只改 import。

这样 beego 的 web 框架部分（session / router / template / captcha）整体退场，
而 ini 解析行为与现有 `conf/nps.conf` 100% 兼容。
`beego/config` 可独立使用已实测验证。

### 2.3 不引入 golang-jwt 库

只需 HS256 签发/校验固定结构的 claims，标准库约 60 行可实现，且无解析歧义风险。
自实现必须严格做到，三点均有独立单测：

- 算法白名单硬编码，拒绝 `alg: none` 与任何非 HS256；
- `hmac.Equal` 恒定时间比较；
- 拒绝重复 JSON 字段（防 claims 覆盖攻击）。

## 3. 认证设计

登录握手保留 RSA 加密密码、nonce、时间戳、PoW、图形验证码、TOTP、封禁限频
**全链路**，只把终点的 Session 换成 JWT。

### 3.1 无状态化改造

原有 nonce 存 Session。无状态化后改为服务端 nonce 存储
（内存 `sync.Map` + TTL 5 分钟 + 定期清理），由 challenge 接口下发。

```
GET  /api/v1/auth/challenge     → { nonce, rsaPublicKey, powBits, powRequired,
                                     captchaRequired, captchaId, totpLen,
                                     serverTime, loginDelay }
GET  /api/v1/auth/captcha/{id}  → image/png
POST /api/v1/auth/login         → { token, expiresAt, role, clientId, username }
POST /api/v1/auth/register      → 同样走 challenge
GET  /api/v1/auth/me            → 当前身份 + 权限位
POST /api/v1/auth/logout        → 客户端丢弃 token（服务端无状态，仅审计日志）
```

### 3.2 图形验证码自实现

beego captcha 与 beego cache 绑定，必须自实现。
标准库 `image/png` 绘制 4 位数字 + 干扰线，
验证码 ID/答案存内存 TTL 存储，**一次性消费**（验证后立即删除，防重放）。

### 3.3 JWT

claims：

```json
{ "sub": "admin", "role": "admin|user", "cid": 3, "iat": ..., "exp": ..., "jti": "..." }
```

- 有效期 2 小时，**无 refresh token**。管理后台场景，过期重登可接受，
  避免 refresh 轮转的复杂度和被盗用窗口。
- 签名密钥 `api_jwt_key`：读 `nps.conf`；为空则首次启动生成 32 字节随机值
  并写回配置文件，同时日志提示。升级用户零配置，且重启后 token 不失效。
- 前端存储位置：**内存**（React state / TanStack Query cache），不落 localStorage。
  防 XSS 窃取；刷新页面需重登，配合 2 小时有效期可接受。

### 3.4 权限模型

对齐现有 `isAdmin` / `clientId` 语义：

- `admin`：全量；
- `user`（客户端登录）：只能读写自己 `clientId` 名下的隧道/域名；
  禁止改 vkey、流量/带宽/连接数限额；禁止 `bridge://` 目标；
  禁止访问全局设置与封禁管理。

这些约束现在散在 controller 各处（`CheckUserAuth`、各处 `isAdmin` 判断），
收敛成 `web/api/authz.go` 单一权限判定层，杜绝遗漏。

### 3.5 外部 API 兼容

`auth_key + timestamp` MD5 机制保留，作为 JWT 之外的第二种凭证，
在同一鉴权中间件里判定，等价 admin 权限。
旧 `/client/list` 等路径以薄适配层转发到新 handler。
`docs/api.md` 补充新老对照。

## 4. API 设计

统一前缀 `/api/v1`；`web_base_url` 生效时为 `{base}/api/v1`。

统一响应包络：

```json
{ "code": 0, "message": "ok", "data": {}, "requestId": "..." }
```

列表统一 `{ "rows": [], "total": 0 }`。
错误用恰当 HTTP 状态码 + 非零 `code`。

| 组 | 端点 |
|---|---|
| auth | challenge / captcha / login / register / me / logout |
| dashboard | `GET /dashboard`、`GET /dashboard/history` |
| clients | list / get / create / update / delete / toggle / clear / ping / qrcode |
| tunnels | list / get / create / update / delete / start / stop / clear |
| hosts | list / get / create / update / delete / start / stop / clear |
| global | get / update（黑名单）、bans list / unban / unban-all / clean |
| meta | `GET /meta/bootstrap`（版本、bridge 地址、`allow_*` 功能开关） |

**字段命名**：Go 结构体加 JSON tag 转 camelCase，前端 TS 类型一一对应。
`lib/file` 模型加 tag 时注意 `sync.RWMutex`、`Rate` 等字段必须 `json:"-"`，
避免序列化爆炸或泄露。

**敏感字段**：`VerifyKey` 在 user 角色下脱敏；
`WebPassword`、`WebTotpSecret` 任何角色都不回传明文。

## 5. 目录结构

```
web/
  api/              新增 — REST handlers、JWT、captcha、authz、中间件
  ui/               新增 — React 源码（pnpm）
    src/
      api/          fetch 封装、类型定义
      auth/         登录态、challenge、PoW worker、RSA
      components/ui shadcn 组件
      features/     dashboard | clients | tunnels | hosts | global | login
      i18n/         zh-CN.json / en-US.json
  dist/             构建产物（go:embed 目标）
  embed.go          go:embed + SPA handler（deep-link 回退、base 路径重写）
  controllers/      删除
  routers/          删除
  views/            删除
  static/           删除（error.html 迁入 server/proxy/httpproxy 内嵌）
lib/appconfig/      新增 — beego/config 薄封装
```

## 6. 分阶段计划

| 阶段 | 内容 | 可验证产出 | 状态 |
|---|---|---|---|
| **M1 地基** | `lib/appconfig` 替换 `beego.AppConfig`；根 mux 接管监听；SPA embed handler | `go build` 通过，旧 UI 仍可访问 | ✅ 完成 |
| **M2 认证** | JWT 签发/校验、challenge、自研 captcha、authz 权限层、auth_key 兼容 | 单测覆盖签名伪造/过期/越权/重放 | ✅ 完成 |
| **M3 API** | dashboard / clients / tunnels / hosts / global 全部端点 | curl 可完成全部管理操作 | ✅ 完成 |
| **M4 前端** | Vite + shadcn 脚手架、登录页、六大功能页、i18n、暗色模式 | `pnpm dev` 可完整操作，E2E 登录+全接口 200 | ✅ 完成 |
| **M5 收口** | 删除 views/static/controllers/routers；embed 构建链；build.sh / Dockerfile / installer 同步；文档更新 | 单二进制启动即完整后台 | ✅ 完成 |

每阶段结束汇报并等确认，再进入下一阶段。分多次提交。

## 7. 已知风险与注意事项

1. **`.gitignore` 的 `*.json` 规则会吞掉前端配置**
   `package.json`、`tsconfig.json`、`components.json`、i18n 词条 JSON 都会被忽略。
   M4 前必须为 `web/ui/` 加白名单否定规则。

2. **模板重启限制消失**
   这是好事，但 `web/ui` 改动需 `pnpm build` 才反映到二进制；
   开发时用 `pnpm dev` + Vite 代理绕过。

3. **发布包结构变化**
   `linux_amd64_server.tar.gz` 不再含 `web/views`、`web/static`，
   只剩 `conf/nps.conf` + `nps`。升级用户旧目录残留不影响运行，需在 CHANGELOG 说明。

4. **token 存内存**
   刷新页面需重新登录。若更看重免重登体验，可改 sessionStorage（XSS 风险略升）。

5. **fork 专属 UI 定制需重新实现**
   CLAUDE.md 记录的 `client/add.html` 随机密码刷新按钮、
   `index/list.html` 访问地址列与弹窗复制等定制，
   在新 React UI 中重新实现（已列入 M4 范围），
   旧的 grep 校验清单需相应重写。

6. **`error_page` 配置项**
   `server/proxy/httpproxy` 默认读 `web/static/page/error.html`。
   删除 `web/static` 时改为内嵌默认页，保留 `error_page` 配置项覆盖能力。
