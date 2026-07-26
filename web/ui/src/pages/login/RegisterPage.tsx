import { useCallback, useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { api } from "@/api/endpoints"
import { ApiError } from "@/api/http"
import type { Captcha, Challenge, LoginFailure } from "@/api/types"
import { encryptLoginPayload } from "@/auth/crypto"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { LanguageThemeBar, LoginFooter } from "./LanguageThemeBar"

// Registration reuses the login envelope: the password travels RSA-encrypted
// with a nonce and timestamp, and the captcha (when enabled) must be answered.
export default function RegisterPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [challenge, setChallenge] = useState<Challenge | null>(null)
  const [captcha, setCaptcha] = useState<Captcha | null>(null)
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [captchaCode, setCaptchaCode] = useState("")
  const [busy, setBusy] = useState(false)
  const retry = useRef<{ nonce?: string; cert?: string; offset?: number }>({})

  useEffect(() => {
    document.title = t("title-register")
  }, [t])

  const loadChallenge = useCallback(async () => {
    try {
      const c = await api.auth.challenge()
      setChallenge(c)
      setCaptcha(c.captcha ?? null)
      setCaptchaCode("")
      retry.current = { offset: c.serverTime - Date.now() }
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
      // keep the old image
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
      await api.auth.register({
        username,
        password: ciphertext,
        captchaId: captcha?.id,
        captchaCode,
      })
      toast.success(t("registersuccess"))
      navigate("/login", { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        const detail = (err.data ?? {}) as LoginFailure
        retry.current.nonce = detail.nonce
        if (detail.cert) retry.current.cert = detail.cert
        if (detail.timestamp) retry.current.offset = detail.timestamp - Date.now()
        if (detail.captcha) {
          setCaptcha(detail.captcha)
          setCaptchaCode("")
        }
        toast.error(t(err.message.replace(/[^a-z]/gi, "").toLowerCase(), err.message))
      } else {
        // Any other rejection (e.g. 409 duplicate username) has still consumed
        // the nonce and captcha server-side, so fetch fresh material.
        toast.error(err instanceof Error ? err.message : String(err))
        void loadChallenge()
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
            {t("title-register")}
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
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
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
              {busy ? t("processing") : t("word-register")}
            </Button>

            <p className="text-center text-sm text-muted-foreground">
              {t("info-haveaccount")}{" "}
              <Link to="/login" className="text-primary hover:underline">
                {t("word-login")}
              </Link>
            </p>
          </form>
        </CardContent>
      </Card>
      <LoginFooter />
    </div>
  )
}
