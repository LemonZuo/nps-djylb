import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react"
import type { ReactNode } from "react"
import { api } from "@/api/endpoints"
import { getToken, setToken, setUnauthorizedHandler } from "@/api/http"
import type { MeInfo } from "@/api/types"

// Login state for the whole app. The token itself stays inside api/http; this
// context only tracks who is logged in, so components can render accordingly.

interface AuthState {
  user: MeInfo | null
  login: (token: string, user: MeInfo) => void
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<MeInfo | null>(null)
  // A token restored from sessionStorage needs an /auth/me round trip before
  // the router can decide between the app and the login page.
  const [restoring, setRestoring] = useState(() => getToken() !== null)

  const login = useCallback((token: string, me: MeInfo) => {
    setToken(token)
    setUser(me)
  }, [])

  const logout = useCallback(async () => {
    try {
      await api.auth.logout()
    } catch {
      // Logging out of an already-dead session is still logging out.
    }
    setToken(null)
    setUser(null)
  }, [])

  useEffect(() => {
    // A 401 from any call means the token expired server-side; drop the local
    // state so the router shows the login page.
    setUnauthorizedHandler(() => {
      setToken(null)
      setUser(null)
    })
    return () => setUnauthorizedHandler(null)
  }, [])

  useEffect(() => {
    if (!restoring) return
    api.auth
      .me()
      .then(setUser)
      .catch(() => setToken(null))
      .finally(() => setRestoring(false))
  }, [restoring])

  const value = useMemo(() => ({ user, login, logout }), [user, login, logout])
  // Render nothing during the restore round trip — a flash of the login page
  // followed by an instant redirect looks like a logout.
  if (restoring) return null
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider")
  return ctx
}
