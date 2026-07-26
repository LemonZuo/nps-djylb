import { del, get, post, put, request } from "./http"
import type {
  BanEntry,
  Bootstrap,
  Challenge,
  ClientRequest,
  ClientView,
  DashboardData,
  GlobalView,
  HostRequest,
  HostView,
  ListQuery,
  LoginResponse,
  MeInfo,
  Paged,
  TunnelRequest,
  TunnelView,
} from "./types"

// One function per API route, so call sites read as intent and the URL strings
// live in exactly one place.

export const api = {
  auth: {
    challenge: () => get<Challenge>("/auth/challenge"),
    captcha: () => get<Challenge["captcha"]>("/auth/captcha"),
    login: (body: {
      username: string
      password: string
      captchaId?: string
      captchaCode?: string
      powX?: string
      bits?: number
    }) => request<LoginResponse>("/auth/login", { method: "POST", body, skipUnauthorizedHandler: true }),
    register: (body: {
      username: string
      password: string
      captchaId?: string
      captchaCode?: string
    }) => request<unknown>("/auth/register", { method: "POST", body, skipUnauthorizedHandler: true }),
    me: () => get<MeInfo>("/auth/me"),
    logout: () => post<null>("/auth/logout"),
    bans: () => get<Paged<BanEntry>>("/auth/bans"),
    unban: (key: string) => del<null>(`/auth/bans/${encodeURIComponent(key)}`),
    unbanAll: () => del<null>("/auth/bans"),
  },

  meta: {
    bootstrap: () => get<Bootstrap>("/meta/bootstrap"),
  },

  dashboard: {
    data: (force = false) => get<DashboardData>("/dashboard", force ? { force: "true" } : undefined),
    history: () => get<Paged<Record<string, unknown>>>("/dashboard/history"),
  },

  clients: {
    list: (q: ListQuery = {}) => get<Paged<ClientView>>("/clients", q),
    getOne: (id: number) => get<ClientView>(`/clients/${id}`),
    create: (body: ClientRequest) => post<ClientView>("/clients", body),
    update: (id: number, body: ClientRequest) => put<ClientView>(`/clients/${id}`, body),
    remove: (id: number) => del<null>(`/clients/${id}`),
    setStatus: (id: number, status: boolean) => post<null>(`/clients/${id}/status`, { status }),
    clear: (id: number, mode: string) => post<null>(`/clients/${id}/clear`, { mode }),
    clearAll: (mode: string) => post<null>("/clients/clear", { mode }),
    ping: (id: number) => post<{ rtt: number }>(`/clients/${id}/ping`),
    qrcodeUrl: (id: number) => `/clients/${id}/qrcode`,
  },

  tunnels: {
    list: (q: ListQuery = {}) => get<Paged<TunnelView>>("/tunnels", q),
    getOne: (id: number) => get<TunnelView>(`/tunnels/${id}`),
    create: (body: TunnelRequest) => post<TunnelView>("/tunnels", body),
    update: (id: number, body: TunnelRequest) => put<TunnelView>(`/tunnels/${id}`, body),
    remove: (id: number) => del<null>(`/tunnels/${id}`),
    start: (id: number) => post<null>(`/tunnels/${id}/start`),
    stop: (id: number) => post<null>(`/tunnels/${id}/stop`),
    toggle: (id: number, name: "http" | "socks5", action: string) =>
      post<null>(`/tunnels/${id}/toggle`, { name, action }),
    clear: (id: number, mode: string) => post<null>(`/tunnels/${id}/clear`, { mode }),
  },

  hosts: {
    list: (q: ListQuery = {}) => get<Paged<HostView>>("/hosts", q),
    getOne: (id: number) => get<HostView>(`/hosts/${id}`),
    create: (body: HostRequest) => post<HostView>("/hosts", body),
    update: (id: number, body: HostRequest) => put<HostView>(`/hosts/${id}`, body),
    remove: (id: number) => del<null>(`/hosts/${id}`),
    start: (id: number) => post<null>(`/hosts/${id}/start`),
    stop: (id: number) => post<null>(`/hosts/${id}/stop`),
    toggle: (id: number, name: string, action: string) =>
      post<null>(`/hosts/${id}/toggle`, { name, action }),
    clear: (id: number, mode: string) => post<null>(`/hosts/${id}/clear`, { mode }),
  },

  global: {
    getSettings: () => get<GlobalView>("/global"),
    save: (blackIpList: string[]) => put<GlobalView>("/global", { blackIpList }),
  },
}
