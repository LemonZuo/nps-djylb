import { useEffect, useState } from "react"
import { useNavigate, useParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { RefreshCw } from "lucide-react"
import { api } from "@/api/endpoints"
import type { ClientRequest } from "@/api/types"
import { useAuth } from "@/auth/AuthContext"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { generateRandomPassword } from "@/lib/format"

interface FormState {
  remark: string
  verifyKey: string
  basicUser: string
  basicPassword: string
  compress: boolean
  crypt: boolean
  configConnAllow: boolean
  webUserName: string
  webPassword: string
  webTotpSecret: string
  blackIpList: string
  rateLimit: string
  maxConn: string
  maxTunnelNum: string
  flowLimit: string
  timeLimit: string
  flowReset: boolean
}

const EMPTY: FormState = {
  remark: "",
  verifyKey: "",
  basicUser: "",
  basicPassword: "",
  compress: false,
  crypt: false,
  configConnAllow: true,
  webUserName: "",
  webPassword: "",
  webTotpSecret: "",
  blackIpList: "",
  rateLimit: "",
  maxConn: "",
  maxTunnelNum: "",
  flowLimit: "",
  timeLimit: "",
  flowReset: false,
}

// The limit inputs use datetime-local / plain numbers; the backend accepts a
// unix-seconds string for time_limit, so that is what gets submitted.
function unixToLocal(secs: number): string {
  if (!secs) return ""
  const d = new Date(secs * 1000)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function localToUnixStr(v: string): string {
  if (!v) return ""
  const ms = new Date(v).getTime()
  return Number.isNaN(ms) ? "" : String(Math.floor(ms / 1000))
}

// One form serves create and edit; the request DTO uses optional fields, so
// only what the user may change is sent. Admin-only fields are simply not
// rendered for a regular user — the server enforces the same split. The old
// pages additionally hid each limit behind its allow_* config switch; those
// arrive here as user.permissions, and a hidden field is also never submitted
// so an untouched limit stays as-is.
export default function ClientFormPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const navigate = useNavigate()
  const params = useParams()
  const id = params.id ? Number(params.id) : null
  const isAdmin = !!user?.isAdmin
  const perms = user?.permissions

  // The old add page pre-filled a random basic-auth password on load.
  const [form, setForm] = useState<FormState>(() =>
    id === null ? { ...EMPTY, basicPassword: generateRandomPassword() } : EMPTY,
  )
  const [busy, setBusy] = useState(false)

  const { data: existing } = useQuery({
    queryKey: ["client", id],
    enabled: id !== null,
    queryFn: () => api.clients.getOne(id!),
  })

  useEffect(() => {
    if (!existing) return
    setForm({
      remark: existing.remark,
      verifyKey: existing.verifyKey ?? "",
      basicUser: existing.basicUser,
      basicPassword: existing.basicPassword,
      compress: existing.compress,
      crypt: existing.crypt,
      configConnAllow: existing.configConnAllow,
      webUserName: existing.webUserName,
      webPassword: "",
      webTotpSecret: "",
      blackIpList: existing.blackIpList.join("\n"),
      rateLimit: existing.rateLimit ? String(existing.rateLimit) : "",
      maxConn: existing.maxConn ? String(existing.maxConn) : "",
      maxTunnelNum: existing.maxTunnelNum ? String(existing.maxTunnelNum) : "",
      flowLimit: existing.flow.flowLimit ? String(existing.flow.flowLimit) : "",
      timeLimit: unixToLocal(existing.flow.timeLimit),
      flowReset: false,
    })
  }, [existing])

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((f) => ({ ...f, [key]: value }))

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      const req: ClientRequest = {
        remark: form.remark,
        basicUser: form.basicUser,
        basicPassword: form.basicPassword,
        compress: form.compress,
        crypt: form.crypt,
        configConnAllow: form.configConnAllow,
        webUserName: form.webUserName,
        blackIpList: form.blackIpList,
      }
      // Passwords and secrets are write-only in the view; send them only when
      // the user typed something, so an untouched field is left as-is.
      if (form.webPassword !== "") req.webPassword = form.webPassword
      if (form.webTotpSecret !== "") req.webTotpSecret = form.webTotpSecret
      if (isAdmin) {
        if (form.verifyKey !== (existing?.verifyKey ?? "")) req.verifyKey = form.verifyKey
        // A limit hidden by its allow_* switch is not sent at all, so the
        // stored value survives — same as the old form omitting the input.
        if (perms?.rateLimit) req.rateLimit = form.rateLimit === "" ? 0 : Number(form.rateLimit)
        if (perms?.connNumLimit) req.maxConn = form.maxConn === "" ? 0 : Number(form.maxConn)
        if (perms?.tunnelNumLimit)
          req.maxTunnelNum = form.maxTunnelNum === "" ? 0 : Number(form.maxTunnelNum)
        if (perms?.flowLimit) req.flowLimit = form.flowLimit === "" ? 0 : Number(form.flowLimit)
        if (perms?.timeLimit) req.timeLimit = localToUnixStr(form.timeLimit)
        req.flowReset = form.flowReset
      }
      if (id === null) {
        await api.clients.create(req)
        toast.success(t("addsuccess"))
      } else {
        await api.clients.update(id, req)
        toast.success(t("modifiedsuccess"))
      }
      navigate("/clients")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <h1 className="text-2xl font-semibold">
        {id === null ? t("page-clientadd") : t("page-clientedit")}
      </h1>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("word-configurationinformation")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <Field label={t("word-remark")}>
            <Input value={form.remark} onChange={(e) => set("remark", e.target.value)} />
          </Field>
          {isAdmin && (
            <Field label={t("word-identificationkey")} hint={t("info-autogenerated")}>
              <Input
                value={form.verifyKey}
                onChange={(e) => set("verifyKey", e.target.value)}
                className="font-mono"
              />
            </Field>
          )}
          <Field label={t("word-basicusername")}>
            <Input value={form.basicUser} onChange={(e) => set("basicUser", e.target.value)} />
          </Field>
          <Field label={t("word-basicpassword")}>
            <PasswordWithGenerator
              value={form.basicPassword}
              onChange={(v) => set("basicPassword", v)}
            />
          </Field>
          <ToggleField
            label={t("word-compress")}
            checked={form.compress}
            onChange={(v) => set("compress", v)}
          />
          <ToggleField
            label={t("word-crypt")}
            checked={form.crypt}
            onChange={(v) => set("crypt", v)}
          />
          <ToggleField
            label={t("word-connectbyconfig")}
            checked={form.configConnAllow}
            onChange={(v) => set("configConnAllow", v)}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("word-webusername")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          {/* The old page hid the whole login block behind allow_user_login,
              and the username inside it behind allow_user_change_username. */}
          {perms?.userLoginAllowed && (
            <>
              {(isAdmin || perms.changeUsername) && (
                <Field label={t("word-webusername")}>
                  <Input
                    value={form.webUserName}
                    onChange={(e) => set("webUserName", e.target.value)}
                  />
                </Field>
              )}
              <Field label={t("word-webpassword")}>
                <PasswordWithGenerator
                  value={form.webPassword}
                  onChange={(v) => set("webPassword", v)}
                />
              </Field>
              <Field label={t("word-webtotpsecret")}>
                <Input
                  value={form.webTotpSecret}
                  onChange={(e) => set("webTotpSecret", e.target.value)}
                  className="font-mono"
                  placeholder={existing?.hasTotp ? "••••••" : ""}
                />
              </Field>
            </>
          )}
          <Field label={t("word-blackiplist")} hint={t("info-descblackiplist")}>
            <Textarea
              value={form.blackIpList}
              onChange={(e) => set("blackIpList", e.target.value)}
              rows={3}
              className="font-mono"
            />
          </Field>
        </CardContent>
      </Card>

      {isAdmin &&
        (perms?.rateLimit ||
          perms?.connNumLimit ||
          perms?.tunnelNumLimit ||
          perms?.flowLimit ||
          perms?.timeLimit ||
          id !== null) && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("word-admin")}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-2">
            {perms?.rateLimit && (
              <Field label={`${t("word-ratelimit")} (KB/s)`} hint={t("info-unrestricted")}>
                <Input
                  type="number"
                  value={form.rateLimit}
                  onChange={(e) => set("rateLimit", e.target.value)}
                />
              </Field>
            )}
            {perms?.connNumLimit && (
              <Field label={t("word-maxconnections")} hint={t("info-unrestricted")}>
                <Input
                  type="number"
                  value={form.maxConn}
                  onChange={(e) => set("maxConn", e.target.value)}
                />
              </Field>
            )}
            {perms?.tunnelNumLimit && (
              <Field label={t("word-maxtunnels")} hint={t("info-unrestricted")}>
                <Input
                  type="number"
                  value={form.maxTunnelNum}
                  onChange={(e) => set("maxTunnelNum", e.target.value)}
                />
              </Field>
            )}
            {perms?.flowLimit && (
              <Field label={`${t("word-flowlimit")} (MB)`} hint={t("info-unrestricted")}>
                <Input
                  type="number"
                  value={form.flowLimit}
                  onChange={(e) => set("flowLimit", e.target.value)}
                />
              </Field>
            )}
            {perms?.timeLimit && (
              <Field label={t("word-timelimit")} hint={t("info-timelimit")}>
                <Input
                  type="datetime-local"
                  value={form.timeLimit}
                  onChange={(e) => set("timeLimit", e.target.value)}
                />
              </Field>
            )}
            {id !== null && (
              <ToggleField
                label={t("word-flowreset")}
                checked={form.flowReset}
                onChange={(v) => set("flowReset", v)}
              />
            )}
          </CardContent>
        </Card>
      )}

      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? t("processing") : t("word-save")}
        </Button>
        <Button type="button" variant="outline" onClick={() => navigate("/clients")}>
          {t("word-cancel")}
        </Button>
      </div>
    </form>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function ToggleField({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-lg border p-3">
      <Label>{label}</Label>
      <Switch checked={checked} onCheckedChange={onChange} />
    </div>
  )
}

// The fork's random-password button, reborn: fills the field with a fresh
// 32-char alphanumeric and spins the icon for feedback.
export function PasswordWithGenerator({
  value,
  onChange,
}: {
  value: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const [spinning, setSpinning] = useState(false)
  return (
    <div className="flex gap-1">
      <Input value={value} onChange={(e) => onChange(e.target.value)} className="font-mono" />
      <Button
        type="button"
        variant="outline"
        size="icon"
        title={t("word-generatenewpassword")}
        onClick={() => {
          onChange(generateRandomPassword())
          setSpinning(true)
          setTimeout(() => setSpinning(false), 500)
        }}
      >
        <RefreshCw className={spinning ? "size-4 animate-spin" : "size-4"} />
      </Button>
    </div>
  )
}
