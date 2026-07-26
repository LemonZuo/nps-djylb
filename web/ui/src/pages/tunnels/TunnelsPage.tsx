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
  ListFooter,
  SearchBox,
  SimpleTable,
  SortHead,
  useListState,
} from "@/components/data-table"
import { DetailItem, formatTimeLimit } from "@/components/detail-item"
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
              <code
                className="mt-1 block cursor-pointer rounded bg-background p-1.5 font-mono text-xs break-all hover:bg-muted"
                title={t("word-copy")}
                onClick={() => void copyText(c.cmd)}
              >
                {c.cmd}
              </code>
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

  // The old list only showed the HTTP/SOCKS5 start-stop columns on the
  // dedicated mixProxy page, not in the mixed all-modes view.
  const mixColumns = mode === "mixProxy"
  const columnCount = mixColumns ? 14 : 12

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
          <Button asChild>
            <Link to={`/tunnels/new${clientId ? `?clientId=${clientId}` : ""}`}>
              <Plus className="size-4" />
              {t("word-add")}
            </Link>
          </Button>
        </div>
      </div>

      <SimpleTable
        loading={isLoading}
        empty={rows.length === 0}
        headers={[
          "",
          <SortHead key="id" label="ID" field="Id" state={state} onSort={toggleSort} />,
          <SortHead
            key="remark"
            label={t("word-remark")}
            field="Remark"
            state={state}
            onSort={toggleSort}
          />,
          <SortHead
            key="mode"
            label={t("word-scheme")}
            field="Mode"
            state={state}
            onSort={toggleSort}
          />,
          ...(mixColumns
            ? [
                <SortHead
                  key="http"
                  label={t("word-httpproxy")}
                  field="HttpProxy"
                  state={state}
                  onSort={toggleSort}
                />,
                <SortHead
                  key="socks5"
                  label={t("word-socks5proxy")}
                  field="Socks5Proxy"
                  state={state}
                  onSort={toggleSort}
                />,
              ]
            : []),
          <SortHead
            key="client"
            label={t("word-client")}
            field="Client.Id"
            state={state}
            onSort={toggleSort}
          />,
          <SortHead
            key="port"
            label={t("word-port")}
            field="Port"
            state={state}
            onSort={toggleSort}
          />,
          t("word-accessaddress"),
          <SortHead
            key="target"
            label={t("word-target")}
            field="Target.TargetStr"
            state={state}
            onSort={toggleSort}
          />,
          t("word-clientstatus"),
          <SortHead
            key="run"
            label={t("word-runstatus")}
            field="RunStatus"
            state={state}
            onSort={toggleSort}
          />,
          <SortHead
            key="flow"
            label={t("word-trafficstatistics")}
            field="TotalFlow"
            state={state}
            onSort={toggleSort}
          />,
          t("word-option"),
        ]}
      >
        {rows.map((row) => {
          const items = accessItems(row, host)
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
                <TableCell
                  className="cursor-pointer font-mono"
                  title={t("word-copy")}
                  onClick={() => void copyText(String(row.id))}
                >
                  {row.id}
                </TableCell>
                <TableCell
                  className="max-w-32 cursor-pointer truncate"
                  title={t("word-copy")}
                  onClick={() => row.remark && void copyText(row.remark)}
                >
                  {row.remark}
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{t(`scheme-${row.mode.toLowerCase()}`)}</Badge>
                </TableCell>
                {mixColumns && (
                  <>
                    <TableCell>
                      <Switch
                        title={t("word-enablehttpproxy")}
                        checked={row.httpProxy}
                        onCheckedChange={() =>
                          act.mutate(() => api.tunnels.toggle(row.id, "http", "toggle"))
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <Switch
                        title={t("word-enablesocks5proxy")}
                        checked={row.socks5Proxy}
                        onCheckedChange={() =>
                          act.mutate(() => api.tunnels.toggle(row.id, "socks5", "toggle"))
                        }
                      />
                    </TableCell>
                  </>
                )}
                <TableCell
                  className={row.client.verifyKey ? "cursor-pointer text-xs" : "text-xs"}
                  title={row.client.verifyKey ? t("word-copy") : undefined}
                  onClick={() => row.client.verifyKey && void copyText(row.client.verifyKey)}
                >
                  {row.client.id} {row.client.remark && `(${row.client.remark})`}
                </TableCell>
                <TableCell
                  className="cursor-pointer font-mono"
                  title={t("word-copy")}
                  onClick={() => void copyText(String(row.port))}
                >
                  {row.port}
                </TableCell>
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
                <TableCell
                  className="max-w-40 cursor-pointer truncate font-mono text-xs"
                  title={t("word-copy")}
                  onClick={() => void copyText(row.target.target)}
                >
                  {row.target.target}
                </TableCell>
                <TableCell>
                  <Badge variant={row.client.isConnect ? "default" : "secondary"}>
                    {row.client.isConnect ? t("word-online") : t("word-offline")}
                  </Badge>
                </TableCell>
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
                <TableCell className="text-xs">
                  {formatBytes(row.flow.inletFlow)} / {formatBytes(row.flow.exportFlow)}
                </TableCell>
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
                        onClick={async () => {
                          if (await confirm(t("clear"))) {
                            act.mutate(() => api.tunnels.clear(row.id, "flow"))
                          }
                        }}
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
              </TableRow>
              {expanded.has(row.id) && (
                <TableRow>
                  <TableCell colSpan={columnCount} className="p-0">
                    <TunnelDetail
                      row={row}
                      isAdmin={isAdmin}
                      bootstrap={bootstrap}
                      secretLink={secretLink}
                      onClear={async (m) => {
                        if (await confirm(t("clear"))) {
                          act.mutate(() => api.tunnels.clear(row.id, m))
                        }
                      }}
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
