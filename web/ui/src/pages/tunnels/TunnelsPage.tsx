import { Fragment, useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import {
  ChevronDown,
  ChevronRight,
  Eraser,
  LinkIcon,
  Pencil,
  Play,
  Plus,
  Square,
  Trash2,
} from "lucide-react"
import { api } from "@/api/endpoints"
import type { Bootstrap, TunnelView } from "@/api/types"
import { useAuth } from "@/auth/AuthContext"
import { useConfirm } from "@/components/confirm-dialog"
import {
  ColumnPicker,
  ListFooter,
  SearchBox,
  SimpleTable,
  SortHead,
  useColumns,
  useListState,
  type ColumnDef,
} from "@/components/data-table"
import { DetailItem, formatTimeLimit, formatTimeRemain } from "@/components/detail-item"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { TableCell, TableRow } from "@/components/ui/table"
import { Tip } from "@/components/ui/tooltip"
import { copyText, formatBytes } from "@/lib/format"

export const TUNNEL_MODES = [
  "tcp",
  "udp",
  "httpProxy",
  "socks5",
  "mixProxy",
  "secret",
  "p2p",
  "file",
] as const

// The old index/list.html column matrix; which columns exist at all also
// depends on the ?type= filter, so the defs are built per mode below.
function tunnelColumns(mode: string): ColumnDef[] {
  const all = !mode
  const cols: ColumnDef[] = [
    { key: "id", labelKey: "word-id", defaultVisible: true, sortField: "Id" },
    { key: "clientId", labelKey: "word-clientid", defaultVisible: true, sortField: "Client.Id" },
    { key: "remark", labelKey: "word-remark", defaultVisible: false, sortField: "Remark" },
    {
      key: "clientVerifyKey",
      labelKey: "word-verifykey",
      defaultVisible: false,
      sortField: "Client.VerifyKey",
    },
  ]
  if (all) {
    cols.push({ key: "mode", labelKey: "word-scheme", defaultVisible: true, sortField: "Mode" })
  }
  cols.push({ key: "port", labelKey: "word-port", defaultVisible: true, sortField: "Port" })
  if (all || mode === "tcp" || mode === "udp" || mode === "mixProxy") {
    cols.push({ key: "accessAddress", labelKey: "word-accessaddress", defaultVisible: true })
  }
  if (all || mode === "tcp" || mode === "udp" || mode === "secret" || mode === "p2p") {
    cols.push({
      key: "target",
      labelKey: "word-target",
      defaultVisible: true,
      sortField: "Target.TargetStr",
    })
  }
  if (all || mode === "secret" || mode === "p2p") {
    cols.push(
      {
        key: "targetType",
        labelKey: "word-targettype",
        defaultVisible: true,
        sortField: "TargetType",
      },
      {
        key: "password",
        labelKey: "word-identificationkey",
        defaultVisible: true,
        sortField: "Password",
      },
    )
  }
  if (mode === "file") {
    cols.push(
      { key: "localPath", labelKey: "word-localpath", defaultVisible: true },
      { key: "stripPre", labelKey: "word-stripprefix", defaultVisible: true },
    )
  }
  if (mode === "mixProxy") {
    cols.push(
      {
        key: "httpProxy",
        labelKey: "word-httpproxy",
        defaultVisible: true,
        sortField: "HttpProxy",
      },
      {
        key: "socks5Proxy",
        labelKey: "word-socks5proxy",
        defaultVisible: true,
        sortField: "Socks5Proxy",
      },
    )
  }
  cols.push(
    { key: "inletFlow", labelKey: "word-inletflow", defaultVisible: false, sortField: "InletFlow" },
    {
      key: "exportFlow",
      labelKey: "word-exportflow",
      defaultVisible: false,
      sortField: "ExportFlow",
    },
    { key: "totalFlow", labelKey: "word-totalflow", defaultVisible: true, sortField: "TotalFlow" },
    {
      key: "flowRemain",
      labelKey: "word-flowremain",
      defaultVisible: false,
      sortField: "FlowRemain",
    },
    {
      key: "timeRemain",
      labelKey: "word-timeremain",
      defaultVisible: false,
      sortField: "TimeRemain",
    },
    {
      key: "flowLimit",
      labelKey: "word-flowlimit",
      defaultVisible: false,
      sortField: "Flow.FlowLimit",
    },
    {
      key: "timeLimit",
      labelKey: "word-timelimit",
      defaultVisible: false,
      sortField: "Flow.TimeLimit",
    },
    { key: "nowConn", labelKey: "word-nowconn", defaultVisible: true, sortField: "NowConn" },
    { key: "status", labelKey: "word-status", defaultVisible: false, sortField: "Status" },
    { key: "runStatus", labelKey: "word-runstatus", defaultVisible: true, sortField: "RunStatus" },
    {
      key: "clientStatus",
      labelKey: "word-clientstatus",
      defaultVisible: true,
      sortField: "Client.IsConnect",
    },
    { key: "option", labelKey: "word-option", defaultVisible: true },
  )
  return cols
}

interface AccessItem {
  label: string
  display: string
  copy: string
}

// The fork's AccessAddress column: tcp/udp expose ip:port, mixProxy exposes
// scheme URLs with the client's basic-auth baked into the copy text. The
// display host is the browser's view of the server, as with the old
// template's {{.ip}}.
function accessItems(t: TunnelView, host: string, auth = ""): AccessItem[] {
  if (t.mode === "tcp" || t.mode === "udp") {
    const addr = `${host}:${t.port}`
    return [{ label: t.mode.toUpperCase(), display: addr, copy: addr }]
  }
  if (t.mode === "mixProxy") {
    const items: AccessItem[] = []
    if (t.httpProxy) {
      items.push({
        label: "HTTP",
        display: `http://${host}:${t.port}`,
        copy: `http://${auth}${host}:${t.port}`,
      })
    }
    if (t.socks5Proxy) {
      items.push({
        label: "SOCKS5",
        display: `socks5://${host}:${t.port}`,
        copy: `socks5://${auth}${host}:${t.port}`,
      })
    }
    return items
  }
  return []
}

function AddressDialog({
  tunnel,
  host,
  onClose,
}: {
  tunnel: TunnelView | null
  host: string
  onClose: () => void
}) {
  const { t } = useTranslation()

  if (!tunnel) return null
  const auth =
    tunnel.client.basicUser && tunnel.client.basicPassword
      ? `${tunnel.client.basicUser}:${tunnel.client.basicPassword}@`
      : ""
  const items = accessItems(tunnel, host, auth)

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("word-accessaddress")}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          {items.map((item) => (
            <div key={item.label + item.display} className="flex items-center gap-2">
              <Badge variant="secondary" className="w-16 justify-center">
                {item.label}
              </Badge>
              <code className="flex-1 truncate rounded bg-muted px-2 py-1 font-mono text-xs">
                {item.display}
              </code>
              <Button size="sm" variant="outline" onClick={() => void copyText(item.copy)}>
                {t("word-copy")}
              </Button>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// npcCommands builds the visitor-side npc invocations the old detail rows
// printed for secret and p2p tunnels, from the preferred bridge endpoint.
function npcCommands(
  row: TunnelView,
  bootstrap: Bootstrap | undefined,
  secretLink: boolean,
): { key: string; cmd: string }[] {
  const ext = bootstrap?.serverIsWindows ? ".exe" : ""
  const server = bootstrap?.preferred?.addr ?? ""
  const type = bootstrap?.preferred?.type ?? "tls"
  const base = `./npc${ext} -server="${server}" -vkey="${row.client.verifyKey ?? ""}" -type="${type}" -password="${row.password}"`
  if (row.mode === "secret") {
    return [
      {
        key: "word-commandaccess",
        cmd: `${base} -target_type="${row.targetType}" -local_type="secret"`,
      },
    ]
  }
  if (row.mode === "p2p") {
    const fb = ` -fallback_secret="${secretLink}"`
    return [
      {
        key: "word-commandaccessp2p",
        cmd: `${base} -target="${row.target.target}" -target_type="${row.targetType}"`,
      },
      { key: "word-commandaccessp2ps", cmd: `${base} -local_type="p2ps"${fb}` },
      { key: "word-commandaccessp2pt", cmd: `${base} -local_type="p2pt"${fb}` },
    ]
  }
  return []
}

// The old bootstrap-table detailView for a tunnel row: flow with admin
// click-to-clear, client crypt/compress, per-mode extras and the visitor
// commands for secret/p2p.
function TunnelDetail({
  row,
  isAdmin,
  bootstrap,
  secretLink,
  onClear,
}: {
  row: TunnelView
  isAdmin: boolean
  bootstrap: Bootstrap | undefined
  secretLink: boolean
  onClear: (mode: string) => void
}) {
  const { t } = useTranslation()
  const yes = t("word-true")
  const no = t("word-false")
  const clear = (mode: string) => (isAdmin ? () => onClear(mode) : undefined)
  const commands = npcCommands(row, bootstrap, secretLink)
  const destMode = row.destAclMode

  return (
    <div className="bg-muted/30 px-6 py-2 text-xs">
      <div>
        <DetailItem
          label={t("word-inletflow")}
          value={formatBytes(row.flow.inletFlow)}
          onClear={clear("flow")}
        />
        <DetailItem
          label={t("word-exportflow")}
          value={formatBytes(row.flow.exportFlow)}
          onClear={clear("flow")}
        />
        <DetailItem label={t("word-nowconn")} value={row.nowConn} />
        <DetailItem label={t("word-crypt")} value={row.client.crypt ? yes : no} />
        <DetailItem label={t("word-compress")} value={row.client.compress ? yes : no} />
        {(row.mode === "tcp" || row.mode === "udp") && (
          <DetailItem
            label={t("word-proxyprotocoltitle")}
            value={t(`word-proxyprotocolv${row.target.proxyProtocol}`)}
          />
        )}
        <DetailItem label={t("word-ishttp")} value={row.isHttp ? yes : no} />
        <DetailItem
          label={t("word-flowlimit")}
          value={row.flow.flowLimit === 0 ? no : formatBytes(row.flow.flowLimit * 1024 * 1024)}
          onClear={row.flow.flowLimit !== 0 ? clear("flow_limit") : undefined}
        />
        <DetailItem
          label={t("word-timelimit")}
          value={formatTimeLimit(row.flow.timeLimit) ?? no}
          onClear={row.flow.timeLimit ? clear("time_limit") : undefined}
        />
      </div>
      {(row.mode === "secret" || row.mode === "p2p") && (
        <div>
          <DetailItem
            label={t("word-identificationkey")}
            value={row.password || "-"}
            copyable={row.password}
          />
          <DetailItem
            label={t("word-targettype")}
            value={(row.targetType || "all").toUpperCase()}
          />
        </div>
      )}
      {row.mode === "mixProxy" && (
        <>
          {(row.client.basicUser || row.client.basicPassword) && (
            <div>
              <DetailItem
                label={t("word-basicusername")}
                value={row.client.basicUser || "-"}
                copyable={row.client.basicUser}
              />
              <DetailItem
                label={t("word-basicpassword")}
                value={row.client.basicPassword || "-"}
                copyable={row.client.basicPassword}
              />
            </div>
          )}
          {row.auth && (
            <div>
              <DetailItem label={t("word-auth")} value={row.auth} />
            </div>
          )}
          <div>
            <DetailItem
              label={t("word-destacl")}
              value={
                destMode === 1
                  ? t("word-whitelist")
                  : destMode === 2
                    ? t("word-blacklist")
                    : t("word-disable")
              }
            />
            {destMode !== 0 && (
              <DetailItem
                label={t("word-destaclrules")}
                value={row.destAclRules || t("word-empty")}
              />
            )}
          </div>
        </>
      )}
      {(row.mode === "httpProxy" || row.mode === "socks5") && row.auth && (
        <div>
          <DetailItem label={t("word-auth")} value={row.auth} />
        </div>
      )}
      {row.mode === "file" && (
        <div>
          <DetailItem label={t("word-localpath")} value={row.localPath || "-"} />
          <DetailItem label={t("word-stripprefix")} value={row.stripPre || "-"} />
          <DetailItem
            label={t("word-target")}
            value={row.target.target || "-"}
            copyable={row.target.target}
          />
        </div>
      )}
      {commands.length > 0 && (
        <div className="mt-2 flex flex-col gap-2">
          {commands.map((c) => (
            <div key={c.key}>
              <b>{t(c.key)}</b>:
              <Tip content={t("word-copy")}>
                <code
                  className="mt-1 block cursor-pointer rounded bg-background p-1.5 font-mono text-xs break-all hover:bg-muted"
                  onClick={() => void copyText(c.cmd)}
                >
                  {c.cmd}
                </code>
              </Tip>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default function TunnelsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const { state, setSearch, prevPage, nextPage, setLimit, toggleSort } = useListState()
  const [searchParams, setSearchParams] = useSearchParams()
  const [addressFor, setAddressFor] = useState<TunnelView | null>(null)
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  const mode = searchParams.get("type") ?? ""
  const clientId = searchParams.get("clientId")

  const query = useMemo(
    () => ({
      ...state,
      type: mode || undefined,
      clientId: clientId ? Number(clientId) : undefined,
    }),
    [state, mode, clientId],
  )

  const { data, isLoading } = useQuery({
    queryKey: ["tunnels", query],
    queryFn: () => api.tunnels.list(query),
    refetchInterval: 10_000,
  })

  const { data: bootstrap } = useQuery({ queryKey: ["bootstrap"], queryFn: api.meta.bootstrap })

  const act = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      toast.success(t("operationsuccess"))
      queryClient.invalidateQueries({ queryKey: ["tunnels"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const rows = data?.rows ?? []
  const host = window.location.hostname
  const isAdmin = !!user?.isAdmin
  const secretLink = !!user?.permissions.secretLink

  const toggleExpand = (id: number) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  // The column set depends on the ?type= filter (e.g. HTTP/SOCKS5 switches
  // only on the mixProxy page), so overrides persist per mode.
  const defs = useMemo(() => tunnelColumns(mode), [mode])
  const { visible, toggle } = useColumns(`tunnels-${mode || "all"}`, defs)
  const shown = defs.filter((c) => visible(c.key))
  const headers: React.ReactNode[] = [
    "",
    ...shown.map((c) =>
      c.sortField ? (
        <SortHead
          key={c.key}
          label={t(c.labelKey)}
          field={c.sortField}
          state={state}
          onSort={toggleSort}
        />
      ) : (
        t(c.labelKey)
      ),
    ),
  ]
  const columnCount = headers.length

  const clearCell = async (id: number, m: string) => {
    if (await confirm(t("clear"))) act.mutate(() => api.tunnels.clear(id, m))
  }

  return (
    <div className="flex flex-col gap-4">
      {dialog}
      <AddressDialog tunnel={addressFor} host={host} onClose={() => setAddressFor(null)} />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">
          {mode ? t(`page-list${mode === "file" ? "fileserver" : mode.toLowerCase()}`) : t("word-tunnel")}
          {clientId && (
            <span className="ml-2 text-base font-normal text-muted-foreground">
              {t("page-listclientid")}
              {clientId}
            </span>
          )}
        </h1>
        <div className="flex items-center gap-2">
          <Select
            value={mode || "all"}
            onValueChange={(v) => {
              const next = new URLSearchParams(searchParams)
              if (v === "all") next.delete("type")
              else next.set("type", v)
              setSearchParams(next)
            }}
          >
            <SelectTrigger className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("word-all")}</SelectItem>
              {TUNNEL_MODES.map((m) => (
                <SelectItem key={m} value={m}>
                  {t(`scheme-${m.toLowerCase()}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <SearchBox value={state.search} onChange={setSearch} />
          <ColumnPicker defs={defs} visible={visible} onToggle={toggle} />
          <Button asChild>
            <Link
              to={`/tunnels/new?${new URLSearchParams({
                ...(mode ? { type: mode } : {}),
                ...(clientId ? { clientId } : {}),
              }).toString()}`}
            >
              <Plus className="size-4" />
              {t("word-add")}
            </Link>
          </Button>
        </div>
      </div>

      <SimpleTable loading={isLoading} empty={rows.length === 0} headers={headers}>
        {rows.map((row) => {
          const items = accessItems(row, host)
          const totalFlow = row.flow.inletFlow + row.flow.exportFlow
          const clearable = (
            text: React.ReactNode,
            m: string,
            enabled = true,
          ): React.ReactNode =>
            isAdmin && enabled ? (
              <Tip content={t("word-clear")}>
                <span
                  className="cursor-pointer hover:underline"
                  onClick={() => void clearCell(row.id, m)}
                >
                  {text}
                </span>
              </Tip>
            ) : (
              text
            )
          return (
            <Fragment key={row.id}>
              <TableRow>
                <TableCell className="w-8">
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground"
                    onClick={() => toggleExpand(row.id)}
                  >
                    {expanded.has(row.id) ? (
                      <ChevronDown className="size-4" />
                    ) : (
                      <ChevronRight className="size-4" />
                    )}
                  </button>
                </TableCell>
                {visible("id") && (
                  <TableCell
                    className="cursor-pointer font-mono"
                    title={t("word-copy")}
                    onClick={() => void copyText(String(row.id))}
                  >
                    {row.id}
                  </TableCell>
                )}
                {visible("clientId") && (
                  <TableCell
                    className={row.client.verifyKey ? "cursor-pointer text-xs" : "text-xs"}
                    title={row.client.verifyKey ? t("word-copy") : undefined}
                    onClick={() => row.client.verifyKey && void copyText(row.client.verifyKey)}
                  >
                    {row.client.id} {row.client.remark && `(${row.client.remark})`}
                  </TableCell>
                )}
                {visible("remark") && (
                  <TableCell
                    className="max-w-32 cursor-pointer truncate"
                    title={t("word-copy")}
                    onClick={() => row.remark && void copyText(row.remark)}
                  >
                    {row.remark}
                  </TableCell>
                )}
                {visible("clientVerifyKey") && (
                  <TableCell
                    className="max-w-36 cursor-pointer truncate font-mono text-xs"
                    title={t("word-copy")}
                    onClick={() => row.client.verifyKey && void copyText(row.client.verifyKey)}
                  >
                    {row.client.verifyKey}
                  </TableCell>
                )}
                {visible("mode") && (
                  <TableCell>
                    <Badge variant="outline">{t(`scheme-${row.mode.toLowerCase()}`)}</Badge>
                  </TableCell>
                )}
                {visible("port") && (
                  <TableCell
                    className="cursor-pointer font-mono"
                    title={t("word-copy")}
                    onClick={() => void copyText(String(row.port))}
                  >
                    {row.port}
                  </TableCell>
                )}
                {visible("accessAddress") && (
                  <TableCell>
                    {items.length > 0 && (
                      <Button
                        variant="outline"
                        size="xs"
                        title={t("word-viewaddresses")}
                        onClick={() => setAddressFor(row)}
                      >
                        <LinkIcon className="size-3" />
                        {items.length}
                      </Button>
                    )}
                  </TableCell>
                )}
                {visible("target") && (
                  <TableCell
                    className="max-w-40 cursor-pointer truncate font-mono text-xs"
                    title={t("word-copy")}
                    onClick={() => void copyText(row.target.target)}
                  >
                    {row.target.target}
                  </TableCell>
                )}
                {visible("targetType") && (
                  <TableCell className="text-xs">
                    {row.mode === "secret" || row.mode === "p2p"
                      ? (row.targetType || "all").toUpperCase()
                      : ""}
                  </TableCell>
                )}
                {visible("password") && (
                  <TableCell
                    className="max-w-32 cursor-pointer truncate font-mono text-xs"
                    title={t("word-copy")}
                    onClick={() => row.password && void copyText(row.password)}
                  >
                    {row.mode === "secret" || row.mode === "p2p" ? row.password : ""}
                  </TableCell>
                )}
                {visible("localPath") && (
                  <TableCell className="max-w-40 truncate font-mono text-xs">
                    {row.localPath}
                  </TableCell>
                )}
                {visible("stripPre") && (
                  <TableCell className="font-mono text-xs">{row.stripPre}</TableCell>
                )}
                {visible("httpProxy") && (
                  <TableCell>
                    <Switch
                      title={t("word-enablehttpproxy")}
                      checked={row.httpProxy}
                      onCheckedChange={() =>
                        act.mutate(() => api.tunnels.toggle(row.id, "http", "toggle"))
                      }
                    />
                  </TableCell>
                )}
                {visible("socks5Proxy") && (
                  <TableCell>
                    <Switch
                      title={t("word-enablesocks5proxy")}
                      checked={row.socks5Proxy}
                      onCheckedChange={() =>
                        act.mutate(() => api.tunnels.toggle(row.id, "socks5", "toggle"))
                      }
                    />
                  </TableCell>
                )}
                {visible("inletFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(row.flow.inletFlow), "flow")}
                  </TableCell>
                )}
                {visible("exportFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(row.flow.exportFlow), "flow")}
                  </TableCell>
                )}
                {visible("totalFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(totalFlow), "flow")}
                  </TableCell>
                )}
                {visible("flowRemain") && (
                  <TableCell className="text-xs">
                    {row.flow.flowLimit === 0
                      ? "∞"
                      : clearable(
                          formatBytes(row.flow.flowLimit * 1024 * 1024 - totalFlow),
                          "flow_limit",
                        )}
                  </TableCell>
                )}
                {visible("timeRemain") && (
                  <TableCell className="text-xs">
                    {clearable(
                      formatTimeRemain(row.flow.timeLimit),
                      "time_limit",
                      row.flow.timeLimit !== 0,
                    )}
                  </TableCell>
                )}
                {visible("flowLimit") && (
                  <TableCell className="text-xs">
                    {row.flow.flowLimit === 0
                      ? t("word-false")
                      : clearable(formatBytes(row.flow.flowLimit * 1024 * 1024), "flow_limit")}
                  </TableCell>
                )}
                {visible("timeLimit") && (
                  <TableCell className="text-xs">
                    {formatTimeLimit(row.flow.timeLimit) === null
                      ? t("word-false")
                      : clearable(formatTimeLimit(row.flow.timeLimit), "time_limit")}
                  </TableCell>
                )}
                {visible("nowConn") && <TableCell>{row.nowConn}</TableCell>}
                {visible("status") && (
                  <TableCell>
                    <Badge variant={row.status ? "default" : "destructive"}>
                      {row.status ? t("word-open") : t("word-close")}
                    </Badge>
                  </TableCell>
                )}
                {visible("runStatus") && (
                  <TableCell>
                    {/* The old list linked a running tcp tunnel that probes as
                        HTTP straight to its public URL. */}
                    {row.runStatus && row.mode === "tcp" && row.isHttp ? (
                      <a href={`http://${host}:${row.port}`} target="_blank" rel="noreferrer">
                        <Badge variant="default">{t("word-open")}</Badge>
                      </a>
                    ) : (
                      <Badge variant={row.runStatus ? "default" : "destructive"}>
                        {row.runStatus ? t("word-open") : t("word-close")}
                      </Badge>
                    )}
                  </TableCell>
                )}
                {visible("clientStatus") && (
                  <TableCell>
                    <Badge variant={row.client.isConnect ? "default" : "secondary"}>
                      {row.client.isConnect ? t("word-online") : t("word-offline")}
                    </Badge>
                  </TableCell>
                )}
                {visible("option") && (
                  <TableCell>
                    <div className="flex gap-1">
                      {row.status ? (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("word-stop")}
                          onClick={async () => {
                            if (await confirm(t("stop"))) {
                              act.mutate(() => api.tunnels.stop(row.id))
                            }
                          }}
                        >
                          <Square className="size-3.5" />
                        </Button>
                      ) : (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("word-start")}
                          onClick={async () => {
                            if (await confirm(t("start"))) {
                              act.mutate(() => api.tunnels.start(row.id))
                            }
                          }}
                        >
                          <Play className="size-3.5" />
                        </Button>
                      )}
                      <Button variant="ghost" size="icon-xs" title={t("word-edit")} asChild>
                        <Link to={`/tunnels/${row.id}/edit`}>
                          <Pencil className="size-3.5" />
                        </Link>
                      </Button>
                      {isAdmin && (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("word-clearflow")}
                          onClick={() => void clearCell(row.id, "flow")}
                        >
                          <Eraser className="size-3.5" />
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        title={t("word-delete")}
                        onClick={async () => {
                          if (await confirm(t("delete"))) {
                            act.mutate(() => api.tunnels.remove(row.id))
                          }
                        }}
                      >
                        <Trash2 className="size-3.5 text-destructive" />
                      </Button>
                    </div>
                  </TableCell>
                )}
              </TableRow>
              {expanded.has(row.id) && (
                <TableRow>
                  <TableCell colSpan={columnCount} className="p-0">
                    <TunnelDetail
                      row={row}
                      isAdmin={isAdmin}
                      bootstrap={bootstrap}
                      secretLink={secretLink}
                      onClear={(m) => void clearCell(row.id, m)}
                    />
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          )
        })}
      </SimpleTable>

      <ListFooter
        state={state}
        total={data?.total ?? 0}
        onPrev={prevPage}
        onNext={nextPage}
        onLimit={setLimit}
      />
    </div>
  )
}
