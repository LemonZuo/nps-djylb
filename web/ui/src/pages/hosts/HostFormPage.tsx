import { useEffect, useState } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { api } from "@/api/endpoints"
import type { HostRequest } from "@/api/types"
import { useAuth } from "@/auth/AuthContext"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { ClientPicker } from "../tunnels/ClientPicker"

interface FormState {
  clientId: number | null
  host: string
  scheme: string
  location: string
  pathRewrite: string
  redirectUrl: string
  remark: string
  target: string
  targetIsHttps: boolean
  proxyProtocol: string
  localProxy: boolean
  headerChange: string
  respHeaderChange: string
  hostChange: string
  auth: string
  httpsJustProxy: boolean
  tlsOffload: boolean
  autoSsl: boolean
  autoHttps: boolean
  autoCors: boolean
  compatMode: boolean
  certFile: string
  keyFile: string
  flowLimit: string
  timeLimit: string
  flowReset: boolean
}

const EMPTY: FormState = {
  clientId: null,
  host: "",
  scheme: "all",
  location: "",
  pathRewrite: "",
  redirectUrl: "",
  remark: "",
  target: "",
  targetIsHttps: false,
  proxyProtocol: "0",
  localProxy: false,
  headerChange: "",
  respHeaderChange: "",
  hostChange: "",
  auth: "",
  httpsJustProxy: false,
  tlsOffload: false,
  autoSsl: false,
  autoHttps: false,
  autoCors: false,
  compatMode: false,
  certFile: "",
  keyFile: "",
  flowLimit: "",
  timeLimit: "",
  flowReset: false,
}

// unixToLocal / localToUnixStr convert between the API's unix-seconds
// time_limit and the datetime-local input value.
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

// readDroppedPem lets a .pem/.crt/.key file be dragged onto the textarea, as
// the old form allowed.
function pemDropHandler(setValue: (v: string) => void) {
  return (e: React.DragEvent) => {
    e.preventDefault()
    const file = e.dataTransfer.files[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => setValue(String(reader.result ?? ""))
    reader.readAsText(file)
  }
}

export default function HostFormPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const navigate = useNavigate()
  const params = useParams()
  const [searchParams] = useSearchParams()
  const id = params.id ? Number(params.id) : null
  const isAdmin = !!user?.isAdmin
  const perms = user?.permissions

  // /hosts/new?clientId=N (from the client list's jump link) preselects the
  // owner, like the old addhost?client_id= did.
  const presetClientId = searchParams.get("clientId")
  const [form, setForm] = useState<FormState>(() =>
    id === null && presetClientId ? { ...EMPTY, clientId: Number(presetClientId) } : EMPTY,
  )
  const [busy, setBusy] = useState(false)

  const { data: existing } = useQuery({
    queryKey: ["host", id],
    enabled: id !== null,
    queryFn: () => api.hosts.getOne(id!),
  })

  useEffect(() => {
    if (!existing) return
    setForm({
      clientId: existing.client.id,
      host: existing.host,
      scheme: existing.scheme,
      location: existing.location,
      pathRewrite: existing.pathRewrite,
      redirectUrl: existing.redirectUrl,
      remark: existing.remark,
      target: existing.target.target,
      targetIsHttps: existing.targetIsHttps,
      proxyProtocol: String(existing.target.proxyProtocol),
      localProxy: existing.target.localProxy,
      headerChange: existing.headerChange,
      respHeaderChange: existing.respHeaderChange,
      hostChange: existing.hostChange,
      auth: existing.auth,
      httpsJustProxy: existing.httpsJustProxy,
      tlsOffload: existing.tlsOffload,
      autoSsl: existing.autoSsl,
      autoHttps: existing.autoHttps,
      autoCors: existing.autoCors,
      compatMode: existing.compatMode,
      certFile: existing.certFile,
      keyFile: existing.keyFile ?? "",
      flowLimit: existing.flow.flowLimit ? String(existing.flow.flowLimit) : "",
      timeLimit: unixToLocal(existing.flow.timeLimit),
      flowReset: false,
    })
  }, [existing])

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((f) => ({ ...f, [key]: value }))

  // The old edit page's third button: POST the form to addhost, cloning the
  // record as a new host.
  const submit = async (e: React.FormEvent, saveAsNew = false) => {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      const req: HostRequest = {
        host: form.host,
        scheme: form.scheme,
        location: form.location,
        pathRewrite: form.pathRewrite,
        redirectUrl: form.redirectUrl,
        remark: form.remark,
        target: form.target,
        targetIsHttps: form.targetIsHttps,
        proxyProtocol: Number(form.proxyProtocol),
        localProxy: form.localProxy,
        headerChange: form.headerChange,
        respHeaderChange: form.respHeaderChange,
        hostChange: form.hostChange,
        auth: form.auth,
        httpsJustProxy: form.httpsJustProxy,
        tlsOffload: form.tlsOffload,
        autoSsl: form.autoSsl,
        autoHttps: form.autoHttps,
        autoCors: form.autoCors,
        compatMode: form.compatMode,
        certFile: form.certFile,
      }
      // An untouched key field means "keep the stored key" — the view redacts
      // it, so writing the placeholder back would destroy it.
      if (form.keyFile !== "" || isAdmin) req.keyFile = form.keyFile
      if (form.clientId !== null) req.clientId = form.clientId
      if (isAdmin || perms?.flowLimit) {
        req.flowLimit = form.flowLimit === "" ? 0 : Number(form.flowLimit)
      }
      if (isAdmin || perms?.timeLimit) req.timeLimit = localToUnixStr(form.timeLimit)
      if (isAdmin) req.flowReset = form.flowReset

      if (id === null || saveAsNew) {
        await api.hosts.create(req)
        toast.success(t("addsuccess"))
      } else {
        await api.hosts.update(id, req)
        toast.success(t("modifiedsuccess"))
      }
      navigate("/hosts")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const httpsEnabled = form.scheme !== "http"
  const showTls = httpsEnabled && !form.httpsJustProxy

  return (
    <form onSubmit={submit} className="flex w-full flex-col gap-4">
      <h1 className="text-2xl font-semibold">
        {id === null ? t("page-hostadd") : t("page-hostedit")}
      </h1>

      <Card>
        <CardContent className="grid gap-4 pt-4 sm:grid-cols-2">
          {isAdmin && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-clientid")}</Label>
              <ClientPicker value={form.clientId} onChange={(v) => set("clientId", v)} />
            </div>
          )}
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-remark")}</Label>
            <Input value={form.remark} onChange={(e) => set("remark", e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-host")}</Label>
            <Input
              value={form.host}
              onChange={(e) => set("host", e.target.value)}
              placeholder={t("info-suchashost")}
              required
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-scheme")}</Label>
            <Select value={form.scheme} onValueChange={(v) => set("scheme", v)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("word-all")}</SelectItem>
                <SelectItem value="http">HTTP</SelectItem>
                <SelectItem value="https">HTTPS</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-urlroute")}</Label>
            <Input
              value={form.location}
              onChange={(e) => set("location", e.target.value)}
              placeholder="/"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-pathrewrite")}</Label>
            <Input
              value={form.pathRewrite}
              onChange={(e) => set("pathRewrite", e.target.value)}
              placeholder={t("info-urlrewrite")}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-redirecturl")}</Label>
            <Input
              value={form.redirectUrl}
              onChange={(e) => set("redirectUrl", e.target.value)}
              placeholder={t("info-redirecturl")}
            />
          </div>
          <div className="flex flex-col gap-1.5 sm:col-span-2">
            <Label>{t("word-target")}</Label>
            <Textarea
              value={form.target}
              onChange={(e) => set("target", e.target.value)}
              rows={3}
              className="font-mono"
              placeholder={t("info-suchasiplist")}
            />
            <p className="text-xs text-muted-foreground">{t("info-targethost")}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-targetishttps")}</Label>
            <Select
              value={form.targetIsHttps ? "1" : "0"}
              onValueChange={(v) => set("targetIsHttps", v === "1")}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">{t("word-ishttpsfalse")}</SelectItem>
                <SelectItem value="1">{t("word-ishttpstrue")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-requesthost")}</Label>
            <Input value={form.hostChange} onChange={(e) => set("hostChange", e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-requestheader")}</Label>
            <Textarea
              value={form.headerChange}
              onChange={(e) => set("headerChange", e.target.value)}
              rows={2}
              placeholder={t("info-header")}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-responseheader")}</Label>
            <Textarea
              value={form.respHeaderChange}
              onChange={(e) => set("respHeaderChange", e.target.value)}
              rows={2}
              placeholder={t("info-header")}
            />
          </div>
          <div className="flex flex-col gap-1.5 sm:col-span-2">
            <Label>{t("word-auth")}</Label>
            <Textarea
              value={form.auth}
              onChange={(e) => set("auth", e.target.value)}
              rows={2}
              className="font-mono"
              placeholder={t("info-suchasauth")}
            />
            <p className="text-xs text-muted-foreground">{t("info-targetauth")}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-proxyprotocol")}</Label>
            <Select value={form.proxyProtocol} onValueChange={(v) => set("proxyProtocol", v)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">{t("word-proxyprotocolv0")}</SelectItem>
                <SelectItem value="1">{t("word-proxyprotocolv1")}</SelectItem>
                <SelectItem value="2">{t("word-proxyprotocolv2")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {(isAdmin || perms?.localProxy) && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-proxytolocal")}</Label>
              <Select
                value={form.localProxy ? "1" : "0"}
                onValueChange={(v) => set("localProxy", v === "1")}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">{t("word-no")}</SelectItem>
                  <SelectItem value="1">{t("word-yes")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">HTTPS</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          {/* The old form hid the HTTPS controls for scheme=http, and the TLS
              material additionally when https_just_proxy is on. */}
          {httpsEnabled && (
            <ToggleRow
              label={t("word-httpsjustproxytitle")}
              hint={t("info-httpsjustproxy")}
              checked={form.httpsJustProxy}
              onChange={(v) => set("httpsJustProxy", v)}
            />
          )}
          {showTls && (
            <ToggleRow
              label={t("word-tlsoffloadtitle")}
              hint={t("info-tlsoffload")}
              checked={form.tlsOffload}
              onChange={(v) => set("tlsOffload", v)}
            />
          )}
          {showTls && (
            <ToggleRow
              label={t("word-autossl")}
              checked={form.autoSsl}
              onChange={(v) => set("autoSsl", v)}
            />
          )}
          {httpsEnabled && (
            <ToggleRow
              label={t("word-autohttpstitle")}
              checked={form.autoHttps}
              onChange={(v) => set("autoHttps", v)}
            />
          )}
          <ToggleRow
            label={t("word-autocorstitle")}
            checked={form.autoCors}
            onChange={(v) => set("autoCors", v)}
          />
          <ToggleRow
            label={t("word-compatmode")}
            checked={form.compatMode}
            onChange={(v) => set("compatMode", v)}
          />
          {showTls && (
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <Label>{t("word-httpscert")}</Label>
              <Textarea
                value={form.certFile}
                onChange={(e) => set("certFile", e.target.value)}
                onDrop={pemDropHandler((v) => set("certFile", v))}
                onDragOver={(e) => e.preventDefault()}
                rows={3}
                className="font-mono text-xs"
                placeholder={t("info-pemtext")}
              />
            </div>
          )}
          {showTls && (
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <Label>{t("word-httpskey")}</Label>
              <Textarea
                value={form.keyFile}
                onChange={(e) => set("keyFile", e.target.value)}
                onDrop={pemDropHandler((v) => set("keyFile", v))}
                onDragOver={(e) => e.preventDefault()}
                rows={3}
                className="font-mono text-xs"
                placeholder={existing && !isAdmin ? "••••••" : t("info-pemkey")}
              />
            </div>
          )}
        </CardContent>
      </Card>

      {(isAdmin || perms?.flowLimit || perms?.timeLimit) && (
        <Card>
          <CardContent className="grid gap-4 pt-4 sm:grid-cols-2">
            {(isAdmin || perms?.flowLimit) && (
              <div className="flex flex-col gap-1.5">
                <Label>{t("word-flowlimit")} (MB)</Label>
                <Input
                  type="number"
                  value={form.flowLimit}
                  onChange={(e) => set("flowLimit", e.target.value)}
                  placeholder={t("info-unrestricted")}
                />
              </div>
            )}
            {(isAdmin || perms?.timeLimit) && (
              <div className="flex flex-col gap-1.5">
                <Label>{t("word-timelimit")}</Label>
                <Input
                  type="datetime-local"
                  value={form.timeLimit}
                  onChange={(e) => set("timeLimit", e.target.value)}
                />
                <p className="text-xs text-muted-foreground">{t("info-timelimit")}</p>
              </div>
            )}
            {isAdmin && id !== null && (
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={form.flowReset}
                  onCheckedChange={(v) => set("flowReset", v === true)}
                />
                {t("word-flowreset")}
              </label>
            )}
          </CardContent>
        </Card>
      )}

      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? t("processing") : t("word-save")}
        </Button>
        {id !== null && (
          <Button
            type="button"
            variant="secondary"
            disabled={busy}
            onClick={(e) => void submit(e, true)}
          >
            {t("word-add")}
          </Button>
        )}
        <Button type="button" variant="outline" onClick={() => navigate("/hosts")}>
          {t("word-cancel")}
        </Button>
      </div>
    </form>
  )
}

function ToggleRow({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <label className="flex items-start gap-2 rounded-lg border p-3">
      <Checkbox checked={checked} onCheckedChange={(v) => onChange(v === true)} className="mt-0.5" />
      <span className="flex flex-col gap-0.5 text-sm">
        {label}
        {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
      </span>
    </label>
  )
}
