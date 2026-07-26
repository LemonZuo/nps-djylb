import { useCallback, useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { api } from "@/api/endpoints"
import { ApiError } from "@/api/http"
import type { Captcha, Challenge, LoginFailure } from "@/api/types"
import { encryptLoginPayload, solvePoW } from "@/auth/crypto"
import { useAuth } from "@/auth/AuthContext"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { LanguageThemeBar, LoginFooter } from "./LanguageThemeBar"

// The login chain, mirroring web/api/auth.go:
//   1. GET /auth/challenge → nonce, RSA public key, PoW difficulty, captcha,
//      server clock.
//   2. Optionally solve PoW over the ciphertext.
//   3. RSA-encrypt {nonce, timestamp, password}; a skewed local clock is
//      corrected with the serverTime offset from step 1.
//   4. POST /auth/login. A 401 carries fresh retry material (nonce, captcha,
//      new difficulty, cert, server time) which replaces our copy.
export default function LoginPage() {
  const { t } = useTranslation()
  const { login } = useAuth()
  const navigate = useNavigate()

  const [challenge, setChallenge] = useState<Challenge | null>(null)
  const [captcha, setCaptcha] = useState<Captcha | null>(null)
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [captchaCode, setCaptchaCode] = useState("")
  const [busy, setBusy] = useState(false)
  const [solving, setSolving] = useState(false)
  // Retry material from a failed attempt overrides the initial challenge.
  const retry = useRef<{ nonce?: string; bits?: number; cert?: string; offset?: number }>({})
  // The server rejects attempts closer together than loginDelayMs, so the form
  // waits out the remainder client-side instead of burning a failure.
  const lastAttempt = useRef(Date.now())

  useEffect(() => {
    document.title = t("title-login")
  }, [t])

  const loadChallenge = useCallback(async () => {
    try {
      const c = await api.auth.challenge()
      setChallenge(c)
      setCaptcha(c.captcha ?? null)
      setCaptchaCode("")
      retry.current = { offset: c.serverTime - Date.now() }
      lastAttempt.current = Date.now()
    } catch {
      toast.error(t("ui-loading"))
    }
  }, [t])

  useEffect(() => {
    void loadChallenge()
  }, [loadChallenge])

  const refreshCaptcha = useCallback(async () => {
    try {
      const c = await api.auth.captcha()
      if (c) setCaptcha(c)
      setCaptchaCode("")
    } catch {
      // A failed refresh leaves the old image; the user can try again.
    }
  }, [])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!challenge || busy) return
    setBusy(true)
    try {
      const nonce = retry.current.nonce ?? challenge.nonce
      const cert = retry.current.cert ?? challenge.publicKey
      const offset = retry.current.offset ?? 0
      const ciphertext = encryptLoginPayload(cert, nonce, password, offset)

      // PoW binds to the ciphertext (req.Password server-side). Solve whenever
      // the server has ever demanded it — up front, after a failure, or when
      // the captcha field holds a bare TOTP (which the server treats as a
      // failed captcha rescued by 2FA, and that path requires PoW).
      let powX = ""
      let bits = 0
      const totpOnly =
        challenge.captchaOpen && challenge.totpLen > 0 && captchaCode.length === challenge.totpLen
      const wantBits =
        retry.current.bits ??
        (challenge.powRequired || totpOnly ? challenge.powBits : 0)
      if (wantBits > 0) {
        setSolving(true)
        powX = await solvePoW(wantBits, ciphertext)
        bits = wantBits
        setSolving(false)
      }

      const wait = challenge.loginDelayMs - (Date.now() - lastAttempt.current)
      if (wait > 0) {
        await new Promise((resolve) => setTimeout(resolve, wait))
      }
      lastAttempt.current = Date.now()

      const resp = await api.auth.login({
        username,
        password: ciphertext,
        captchaId: captcha?.id,
        captchaCode,
        powX,
        bits,
      })
      login(resp.token, resp.user)
      toast.success(t("loginsuccess"))
      navigate("/dashboard", { replace: true })
    } catch (err) {
      setSolving(false)
      if (err instanceof ApiError && err.status === 401) {
        const detail = (err.data ?? {}) as LoginFailure
        retry.current.nonce = detail.nonce
        if (detail.bits) retry.current.bits = detail.bits
        if (detail.cert) retry.current.cert = detail.cert
        if (detail.timestamp) retry.current.offset = detail.timestamp - Date.now()
        if (detail.captcha) {
          setCaptcha(detail.captcha)
          setCaptchaCode("")
        }
        toast.error(t(err.message.replace(/[^a-z]/gi, "").toLowerCase(), err.message))
      } else {
        toast.error(err instanceof Error ? err.message : String(err))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-muted/40 p-4">
      <LanguageThemeBar />
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <img src="./favicon.svg" alt="" className="size-6" />
            {t("title-login")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="username">{t("word-username")}</Label>
              <Input
                id="username"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="password">{t("word-password")}</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
              <p className="text-xs text-muted-foreground">{t("ui-totp-hint")}</p>
            </div>

            {challenge?.captchaOpen && captcha && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="captcha">{t("word-captcha")}</Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="captcha"
                    value={captchaCode}
                    onChange={(e) => setCaptchaCode(e.target.value)}
                    autoComplete="off"
                    required
                  />
                  <img
                    src={captcha.image}
                    alt="captcha"
                    className="h-9 cursor-pointer rounded border"
                    title={t("word-refresh")}
                    onClick={() => void refreshCaptcha()}
                  />
                </div>
              </div>
            )}

            <Button type="submit" disabled={busy || !challenge}>
              {solving ? t("ui-solving-pow") : busy ? t("processing") : t("word-login")}
            </Button>

            {challenge?.vkeyLoginAllowed && (
              <p className="text-center text-xs text-muted-foreground">{t("ui-vkey-login-hint")}</p>
            )}
            {challenge?.registerAllowed && (
              <p className="text-center text-sm text-muted-foreground">
                {t("info-noaccount")}{" "}
                <Link to="/register" className="text-primary hover:underline">
                  {t("word-register")}
                </Link>
              </p>
            )}
          </form>
        </CardContent>
      </Card>
      <LoginFooter />
    </div>
  )
}
