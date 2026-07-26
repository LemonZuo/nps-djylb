import { useEffect, useState } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { api } from "@/api/endpoints"
import type { TunnelRequest } from "@/api/types"
import { useAuth } from "@/auth/AuthContext"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
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
import { ClientPicker } from "./ClientPicker"
import { TUNNEL_MODES } from "./TunnelsPage"

// Field visibility per mode, carried over from the old add.html matrix.
const MODE_FIELDS: Record<string, string[]> = {
  tcp: ["port", "target", "proxy_protocol", "local_proxy", "client_id", "server_ip"],
  udp: ["port", "target", "proxy_protocol", "local_proxy", "client_id", "server_ip"],
  httpProxy: ["auth", "port", "client_id", "server_ip"],
  socks5: ["auth", "port", "client_id", "server_ip"],
  mixProxy: ["auth", "port", "client_id", "server_ip", "mix_proxy", "dest_acl"],
  secret: ["target_type", "target", "password", "client_id", "server_ip", "local_proxy"],
  p2p: ["target_type", "target", "password", "client_id", "server_ip", "local_proxy"],
  file: ["port", "local_path", "strip_pre", "client_id", "server_ip"],
}

interface FormState {
  mode: string
  clientId: number | null
  remark: string
  serverIp: string
  port: string
  target: string
  targetType: string
  password: string
  auth: string
  localPath: string
  stripPre: string
  proxyProtocol: string
  localProxy: boolean
  httpProxy: boolean
  socks5Proxy: boolean
  destAclMode: string
  destAclRules: string
  flowLimit: string
  timeLimit: string
  flowReset: boolean
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

const EMPTY: FormState = {
  mode: "tcp",
  clientId: null,
  remark: "",
  serverIp: "0.0.0.0",
  port: "",
  target: "",
  targetType: "all",
  password: "",
  auth: "",
  localPath: "",
  stripPre: "",
  proxyProtocol: "0",
  localProxy: false,
  httpProxy: true,
  socks5Proxy: true,
  destAclMode: "0",
  destAclRules: "",
  flowLimit: "",
  timeLimit: "",
  flowReset: false,
}

export default function TunnelFormPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const navigate = useNavigate()
  const params = useParams()
  const [searchParams] = useSearchParams()
  const id = params.id ? Number(params.id) : null
  const isAdmin = !!user?.isAdmin
  const perms = user?.permissions

  // The sidebar menu is per-mode, so adding from a typed list pins the mode
  // (?type=). Editing and the untyped "all" list keep it selectable, like the
  // old add/edit pages.
  const typeParam = searchParams.get("type")
  const lockedMode =
    id === null && typeParam && (TUNNEL_MODES as readonly string[]).includes(typeParam)
      ? typeParam
      : null

  const [form, setForm] = useState<FormState>(() => ({
    ...EMPTY,
    mode: lockedMode ?? EMPTY.mode,
    clientId: searchParams.get("clientId") ? Number(searchParams.get("clientId")) : null,
  }))
  const [busy, setBusy] = useState(false)

  const { data: existing } = useQuery({
    queryKey: ["tunnel", id],
    enabled: id !== null,
    queryFn: () => api.tunnels.getOne(id!),
  })

  useEffect(() => {
    if (!existing) return
    setForm({
      mode: existing.mode,
      clientId: existing.client.id,
      remark: existing.remark,
      serverIp: existing.serverIp,
      port: existing.port ? String(existing.port) : "",
      target: existing.target.target,
      targetType: existing.targetType || "all",
      password: existing.password,
      auth: existing.auth,
      localPath: existing.localPath,
      stripPre: existing.stripPre,
      proxyProtocol: String(existing.target.proxyProtocol),
      localProxy: existing.target.localProxy,
      httpProxy: existing.httpProxy,
      socks5Proxy: existing.socks5Proxy,
      destAclMode: String(existing.destAclMode),
      destAclRules: existing.destAclRules,
      flowLimit: existing.flow.flowLimit ? String(existing.flow.flowLimit) : "",
      timeLimit: unixToLocal(existing.flow.timeLimit),
      flowReset: false,
    })
  }, [existing])

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((f) => ({ ...f, [key]: value }))

  const fields = MODE_FIELDS[form.mode] ?? []
  const has = (f: string) => fields.includes(f)

  // saveAsNew is the old edit page's extra "add" button: POST the current
  // form as a brand-new tunnel instead of updating in place.
  const submit = async (e: React.FormEvent, saveAsNew = false) => {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      const req: TunnelRequest = {
        mode: form.mode,
        remark: form.remark,
        serverIp: form.serverIp,
        port: form.port === "" ? 0 : Number(form.port),
        target: form.target,
        targetType: form.targetType,
        password: form.password,
        auth: form.auth,
        localPath: form.localPath,
        stripPre: form.stripPre,
        proxyProtocol: Number(form.proxyProtocol),
        localProxy: form.localProxy,
        httpProxy: form.httpProxy,
        socks5Proxy: form.socks5Proxy,
        destAclMode: Number(form.destAclMode),
        destAclRules: form.destAclRules,
      }
      if (form.clientId !== null) req.clientId = form.clientId
      if (isAdmin || perms?.flowLimit) {
        req.flowLimit = form.flowLimit === "" ? 0 : Number(form.flowLimit)
      }
      if (isAdmin || perms?.timeLimit) req.timeLimit = localToUnixStr(form.timeLimit)
      if (isAdmin) req.flowReset = form.flowReset

      if (id === null || saveAsNew) {
        await api.tunnels.create(req)
        toast.success(t("addsuccess"))
      } else {
        await api.tunnels.update(id, req)
        toast.success(t("modifiedsuccess"))
      }
      navigate("/tunnels")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <h1 className="text-2xl font-semibold">{id === null ? t("page-add") : t("page-edit")}</h1>

      <Card>
        <CardContent className="grid gap-4 pt-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label>{t("word-scheme")}</Label>
            <Select
              value={form.mode}
              onValueChange={(v) => set("mode", v)}
              disabled={lockedMode !== null}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TUNNEL_MODES.map((m) => (
                  <SelectItem key={m} value={m}>
                    {t(`scheme-${m.toLowerCase()}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t(`info-case${form.mode === "file" ? "file" : form.mode.toLowerCase()}`)}
            </p>
          </div>

          {has("client_id") && isAdmin && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-clientid")}</Label>
              <ClientPicker value={form.clientId} onChange={(v) => set("clientId", v)} />
            </div>
          )}

          <div className="flex flex-col gap-1.5">
            <Label>{t("word-remark")}</Label>
            <Input value={form.remark} onChange={(e) => set("remark", e.target.value)} />
          </div>

          {has("server_ip") && (isAdmin || perms?.multiIp) && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-serverip")}</Label>
              <Input
                value={form.serverIp}
                onChange={(e) => set("serverIp", e.target.value)}
                placeholder={t("info-suchasip")}
              />
            </div>
          )}

          {has("port") && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-serverport")}</Label>
              <Input
                type="number"
                value={form.port}
                onChange={(e) => set("port", e.target.value)}
                placeholder={t("info-suchasport")}
              />
            </div>
          )}

          {has("target_type") && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-targettype")}</Label>
              <Select value={form.targetType} onValueChange={(v) => set("targetType", v)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">ALL</SelectItem>
                  <SelectItem value="tcp">TCP</SelectItem>
                  <SelectItem value="udp">UDP</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          {has("target") && (
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <Label>{t("word-target")}</Label>
              <Textarea
                value={form.target}
                onChange={(e) => set("target", e.target.value)}
                rows={3}
                className="font-mono"
                placeholder={t("info-suchasiplist")}
              />
              <p className="text-xs text-muted-foreground">{t("info-targettunnel")}</p>
            </div>
          )}

          {has("password") && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-identificationkey")}</Label>
              <Input
                value={form.password}
                onChange={(e) => set("password", e.target.value)}
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">{t("info-identificationkey")}</p>
            </div>
          )}

          {has("auth") && (
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <Label>{t("word-auth")}</Label>
              <Textarea
                value={form.auth}
                onChange={(e) => set("auth", e.target.value)}
                rows={3}
                className="font-mono"
                placeholder={t("info-suchasauth")}
              />
              <p className="text-xs text-muted-foreground">{t("info-targetauth")}</p>
            </div>
          )}

          {has("local_path") && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-localpath")}</Label>
              <Input
                value={form.localPath}
                onChange={(e) => set("localPath", e.target.value)}
                placeholder={t("info-suchaslocalpath")}
              />
            </div>
          )}

          {has("strip_pre") && (
            <div className="flex flex-col gap-1.5">
              <Label>{t("word-stripprefix")}</Label>
              <Input
                value={form.stripPre}
                onChange={(e) => set("stripPre", e.target.value)}
                placeholder={t("info-suchasstripprefix")}
              />
            </div>
          )}

          {has("mix_proxy") && (
            <div className="flex flex-col gap-2 rounded-lg border p-3">
              <Label>{t("scheme-mixproxy")}</Label>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={form.httpProxy}
                  onCheckedChange={(v) => set("httpProxy", v === true)}
                />
                {t("word-enablehttpproxy")}
              </label>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={form.socks5Proxy}
                  onCheckedChange={(v) => set("socks5Proxy", v === true)}
                />
                {t("word-enablesocks5proxy")}
              </label>
            </div>
          )}

          {has("dest_acl") && (
            <>
              <div className="flex flex-col gap-1.5">
                <Label>{t("word-destacl")}</Label>
                <Select value={form.destAclMode} onValueChange={(v) => set("destAclMode", v)}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="0">{t("word-disable")}</SelectItem>
                    <SelectItem value="1">{t("word-whitelist")}</SelectItem>
                    <SelectItem value="2">{t("word-blacklist")}</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t("info-destacl")}</p>
              </div>
              {form.destAclMode !== "0" && (
                <div className="flex flex-col gap-1.5">
                  <Label>{t("word-destaclrules")}</Label>
                  <Textarea
                    value={form.destAclRules}
                    onChange={(e) => set("destAclRules", e.target.value)}
                    rows={3}
                    className="font-mono"
                  />
                  <p className="text-xs text-muted-foreground">{t("info-destaclrules")}</p>
                </div>
              )}
            </>
          )}

          {has("proxy_protocol") && (
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
          )}

          {has("local_proxy") && (isAdmin || perms?.localProxy) && (
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

          {(isAdmin || perms?.flowLimit) && (
            <div className="flex flex-col gap-1.5">
              <Label>
                {t("word-flowlimit")} (MB)
              </Label>
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
        <Button type="button" variant="outline" onClick={() => navigate("/tunnels")}>
          {t("word-cancel")}
        </Button>
      </div>
    </form>
  )
}
