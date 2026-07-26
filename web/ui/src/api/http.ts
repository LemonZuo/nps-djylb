import type { Envelope } from "./types"

// Thin fetch wrapper for the /api/v1 JSON API.
//
// The token lives in module state, not localStorage: an XSS that can read
// storage cannot lift a token that was never written there. The price is a
// fresh login after every full page load, which docs/admin-rewrite-plan.md
// accepts deliberately.

let token: string | null = null
let onUnauthorized: (() => void) | null = null

export function setToken(t: string | null) {
  token = t
}

export function getToken(): string | null {
  return token
}

// setUnauthorizedHandler registers the "kick back to login" action. The auth
// provider owns navigation; this module only reports that a 401 happened.
export function setUnauthorizedHandler(fn: (() => void) | null) {
  onUnauthorized = fn
}

// The SPA may be mounted under web_base_url; the server rewrites the <base>
// element to match, so resolving against it yields the right prefix. Exported
// for the rare caller that fetches a binary endpoint outside request().
export function apiBasePath(): string {
  const base = document.querySelector("base")?.getAttribute("href") ?? "/"
  return base.replace(/\/$/, "") + "/api/v1"
}

export class ApiError extends Error {
  readonly status: number
  readonly code: number
  readonly data: unknown

  constructor(status: number, code: number, message: string, data: unknown) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
    this.data = data
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
  query?: Record<string, string | number | boolean | undefined>
  // skipUnauthorizedHandler stops a 401 from bouncing to the login page; the
  // login flow itself needs to inspect 401 bodies.
  skipUnauthorizedHandler?: boolean
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const url = new URL(apiBasePath() + path, window.location.origin)
  for (const [k, v] of Object.entries(opts.query ?? {})) {
    if (v !== undefined && v !== "") url.searchParams.set(k, String(v))
  }

  const headers: Record<string, string> = {}
  if (token) headers["Authorization"] = `Bearer ${token}`
  if (opts.body !== undefined) headers["Content-Type"] = "application/json"

  const res = await fetch(url, {
    method: opts.method ?? "GET",
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  })

  // Binary endpoints (the TOTP QR code) are fetched directly, not through
  // here, so everything that arrives is an envelope.
  const env = (await res.json().catch(() => null)) as Envelope<T> | null

  if (!res.ok || !env || env.code !== 0) {
    if (res.status === 401 && !opts.skipUnauthorizedHandler) {
      onUnauthorized?.()
    }
    throw new ApiError(res.status, env?.code ?? -1, env?.message ?? res.statusText, env?.data)
  }
  return env.data
}

export const get = <T>(path: string, query?: RequestOptions["query"]) =>
  request<T>(path, { query })

export const post = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: "POST", body })

export const put = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: "PUT", body })

export const del = <T>(path: string) => request<T>(path, { method: "DELETE" })
