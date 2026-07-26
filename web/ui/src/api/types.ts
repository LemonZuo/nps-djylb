// Mirror of the Go view types in web/api/dto.go and the auth types in
// web/api/auth.go. These are hand-written rather than generated so the two
// sides can be compared in review; when a field changes there, change it here.

export interface Envelope<T> {
  code: number
  message: string
  data: T
  requestId?: string
}

export interface Paged<T> {
  rows: T[]
  total: number
}

// --- auth ---

export interface Captcha {
  id: string
  image: string
}

export interface Challenge {
  nonce: string
  publicKey: string
  powRequired: boolean
  powBits: number
  captchaOpen: boolean
  captcha?: Captcha
  totpLen: number
  registerAllowed: boolean
  userLoginAllowed: boolean
  vkeyLoginAllowed: boolean
  loginDelayMs: number
  serverTime: number
}

export interface Permissions {
  flowLimit: boolean
  rateLimit: boolean
  timeLimit: boolean
  connNumLimit: boolean
  tunnelNumLimit: boolean
  multiIp: boolean
  secretLink: boolean
  systemInfo: boolean
  localProxy: boolean
  changeUsername: boolean
  userLoginAllowed: boolean
  registerAllowed: boolean
}

export interface MeInfo {
  username: string
  isAdmin: boolean
  clientId: number
  version: string
  year: number
  permissions: Permissions
}

export interface LoginResponse {
  token: string
  expiresAt: number
  user: MeInfo
}

// The body of a rejected login: fresh material for the retry.
export interface LoginFailure {
  nonce: string
  bits?: number
  cert?: string
  timestamp?: number
  captcha?: Captcha
}

// --- resources ---

export interface FlowView {
  inletFlow: number
  exportFlow: number
  flowLimit: number
  timeLimit: number
}

export interface TargetView {
  target: string
  proxyProtocol: number
  localProxy: boolean
}

export interface ClientRef {
  id: number
  remark: string
  verifyKey?: string
  isConnect: boolean
}

export interface ClientView {
  id: number
  remark: string
  verifyKey?: string
  addr: string
  localAddr: string
  mode: string
  version: string
  status: boolean
  isConnect: boolean
  rateLimit: number
  nowRate: number
  maxConn: number
  nowConn: number
  maxTunnelNum: number
  configConnAllow: boolean
  webUserName: string
  hasWebPassword: boolean
  hasTotp: boolean
  basicUser: string
  basicPassword: string
  compress: boolean
  crypt: boolean
  flow: FlowView
  blackIpList: string[]
  tunnelNum: number
  createTime: string
  lastOnlineTime: string
}

export interface TunnelView {
  id: number
  port: number
  serverIp: string
  mode: string
  status: boolean
  runStatus: boolean
  client: ClientRef
  remark: string
  password: string
  targetType: string
  target: TargetView
  auth: string
  localPath: string
  stripPre: string
  httpProxy: boolean
  socks5Proxy: boolean
  destAclMode: number
  destAclRules: string
  nowConn: number
  flow: FlowView
}

export interface HostView {
  id: number
  host: string
  scheme: string
  location: string
  pathRewrite: string
  redirectUrl: string
  remark: string
  client: ClientRef
  target: TargetView
  targetIsHttps: boolean
  headerChange: string
  respHeaderChange: string
  hostChange: string
  auth: string
  isClose: boolean
  httpsJustProxy: boolean
  tlsOffload: boolean
  autoSsl: boolean
  autoHttps: boolean
  autoCors: boolean
  compatMode: boolean
  certType: string
  certFile: string
  keyFile?: string
  nowConn: number
  flow: FlowView
}

export interface GlobalView {
  blackIpList: string[]
}

export interface BanEntry {
  key: string
  failTimes: number
  lastTry: string
  banned: boolean
  type: "ip" | "username"
}

// --- meta ---

export interface BridgeEndpoint {
  type: string
  ip: string
  port: string
  addr: string
  path: string
  alpn: string
}

export interface Bootstrap {
  version: string
  minVersion: string
  year: number
  webBaseUrl: string
  headCustomCode: string
  preferred: BridgeEndpoint
  endpoints: BridgeEndpoint[]
  httpProxyPort: string
  httpsProxyPort: string
  p2pAddr: string
  serverIsWindows: boolean
  permissions: Permissions
}

// Dashboard data is the raw map from server.GetDashboardData; only the keys
// the UI actually renders are declared, the rest pass through untyped.
export interface DashboardData {
  upTime?: string
  tcpCount?: number
  cpu?: number
  load?: string
  swap_mem?: number
  virtual_mem?: number
  io_send?: number
  io_recv?: number
  clientCount?: number
  clientOnlineCount?: number
  hostCount?: number
  taskCount?: number
  bridgeType?: string
  bridgePort?: number | string
  version?: string
  [key: string]: unknown
}

// --- request bodies (create/update) ---

export interface ClientRequest {
  remark?: string
  verifyKey?: string
  rateLimit?: number
  maxConn?: number
  maxTunnelNum?: number
  flowLimit?: number
  timeLimit?: string
  configConnAllow?: boolean
  compress?: boolean
  crypt?: boolean
  basicUser?: string
  basicPassword?: string
  webUserName?: string
  webPassword?: string
  webTotpSecret?: string
  blackIpList?: string
  status?: boolean
  flowReset?: boolean
}

export interface TunnelRequest {
  clientId?: number
  mode?: string
  port?: number
  serverIp?: string
  remark?: string
  password?: string
  target?: string
  targetType?: string
  proxyProtocol?: number
  localProxy?: boolean
  auth?: string
  localPath?: string
  stripPre?: string
  httpProxy?: boolean
  socks5Proxy?: boolean
  destAclMode?: number
  destAclRules?: string
  flowLimit?: number
  timeLimit?: string
  flowReset?: boolean
}

export interface HostRequest {
  clientId?: number
  host?: string
  scheme?: string
  location?: string
  pathRewrite?: string
  redirectUrl?: string
  remark?: string
  target?: string
  proxyProtocol?: number
  localProxy?: boolean
  targetIsHttps?: boolean
  headerChange?: string
  respHeaderChange?: string
  hostChange?: string
  auth?: string
  httpsJustProxy?: boolean
  tlsOffload?: boolean
  autoSsl?: boolean
  autoHttps?: boolean
  autoCors?: boolean
  compatMode?: boolean
  certFile?: string
  keyFile?: string
  flowLimit?: number
  timeLimit?: string
  flowReset?: boolean
}

// A type alias (not an interface) so it stays assignable to request()'s
// Record-typed query parameter.
export type ListQuery = {
  offset?: number
  limit?: number
  search?: string
  sort?: string
  order?: "asc" | "desc"
  clientId?: number
  type?: string
}
