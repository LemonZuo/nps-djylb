import { Fragment, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import {
  ChevronDown,
  ChevronRight,
  Eraser,
  KeyRound,
  Pencil,
  Plus,
  QrCode,
  Radio,
  Terminal,
  Trash2,
} from "lucide-react"
import { api } from "@/api/endpoints"
import { apiBasePath, getToken } from "@/api/http"
import type { ClientView } from "@/api/types"
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
import { Switch } from "@/components/ui/switch"
import { TableCell, TableRow } from "@/components/ui/table"
import { Tip } from "@/components/ui/tooltip"
import { copyText, formatBytes, formatRate } from "@/lib/format"

// The old client/list.html column matrix, defaults included. Field names in
// sortField are the backend DTO names server/list.go sorts on.
const CLIENT_COLUMNS: ColumnDef[] = [
  { key: "id", labelKey: "word-id", defaultVisible: true, sortField: "Id" },
  { key: "remark", labelKey: "word-remark", defaultVisible: true, sortField: "Remark" },
  { key: "version", labelKey: "word-version", defaultVisible: true, sortField: "Version" },
  { key: "bridgeMode", labelKey: "word-scheme", defaultVisible: true, sortField: "Mode" },
  { key: "verifyKey", labelKey: "word-verifykey", defaultVisible: true, sortField: "VerifyKey" },
  { key: "addr", labelKey: "word-address", defaultVisible: false, sortField: "Addr" },
  { key: "localAddr", labelKey: "word-localaddress", defaultVisible: false, sortField: "LocalAddr" },
  { key: "inletFlow", labelKey: "word-inletflow", defaultVisible: false, sortField: "InletFlow" },
  { key: "exportFlow", labelKey: "word-exportflow", defaultVisible: false, sortField: "ExportFlow" },
  { key: "totalFlow", labelKey: "word-totalflow", defaultVisible: true, sortField: "TotalFlow" },
  { key: "flowRemain", labelKey: "word-flowremain", defaultVisible: false, sortField: "FlowRemain" },
  { key: "timeRemain", labelKey: "word-timeremain", defaultVisible: false, sortField: "TimeRemain" },
  { key: "flowLimit", labelKey: "word-flowlimit", defaultVisible: false, sortField: "Flow.FlowLimit" },
  { key: "timeLimit", labelKey: "word-timelimit", defaultVisible: false, sortField: "Flow.TimeLimit" },
  { key: "nowConn", labelKey: "word-nowconn", defaultVisible: true, sortField: "NowConn" },
  { key: "speed", labelKey: "word-speed", defaultVisible: true, sortField: "Rate.NowRate" },
  { key: "status", labelKey: "word-status", defaultVisible: true, sortField: "Status" },
  { key: "isConnect", labelKey: "word-connect", defaultVisible: true, sortField: "IsConnect" },
  { key: "option", labelKey: "word-option", defaultVisible: true },
  { key: "show", labelKey: "word-show", defaultVisible: true },
]

// bridgeModeLabel ports the old getBridgeMode: "target,actual" renders as
// "ACTUAL → TARGET" when the negotiated protocol differs from the dialed one.
function bridgeModeLabel(mode: string): string {
  const [first = "", second = ""] = mode.split(",", 2)
  if (!second || first === second) return first.toUpperCase()
  return `${second.toUpperCase()} → ${first.toUpperCase()}`
}

// CommandDialog shows the npc start command for every advertised bridge
// endpoint, built from /meta/bootstrap the way the old client list did.
function CommandDialog({
  client,
  onClose,
}: {
  client: ClientView | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { data: bootstrap } = useQuery({ queryKey: ["bootstrap"], queryFn: api.meta.bootstrap })

  if (!client) return null
  const ext = bootstrap?.serverIsWindows ? ".exe" : ""
  const commands = (bootstrap?.endpoints ?? []).map((e) => ({
    label: t(`word-commandclient-${e.type}`),
    cmd: `./npc${ext} -server="${e.addr}${e.path ?? ""}" -vkey="${client.verifyKey ?? ""}" -type="${e.type}"`,
  }))

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("word-quicklycommand")}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          {commands.map((c) => (
            <div key={c.label} className="flex flex-col gap-1">
              <span className="text-sm font-medium">{c.label}</span>
              <Tip content={t("word-copy")}>
                <code
                  className="cursor-pointer rounded bg-muted p-2 font-mono text-xs break-all hover:bg-muted/70"
                  onClick={() => void copyText(c.cmd)}
                >
                  {c.cmd}
                </code>
              </Tip>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// The QR endpoint needs the bearer token, which an <img src> cannot carry;
// fetch the PNG with it and show a blob URL instead.
function QrDialog({ client, onClose }: { client: ClientView | null; onClose: () => void }) {
  const { t } = useTranslation()
  const { data: url } = useQuery({
    queryKey: ["client-qr", client?.id],
    enabled: !!client,
    queryFn: async () => {
      const res = await fetch(apiBasePath() + api.clients.qrcodeUrl(client!.id), {
        headers: { Authorization: `Bearer ${getToken()}` },
      })
      if (!res.ok) throw new Error(String(res.status))
      return URL.createObjectURL(await res.blob())
    },
  })

  if (!client) return null
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>{t("ui-qrcode")}</DialogTitle>
        </DialogHeader>
        {url ? (
          <img src={url} alt="TOTP QR" className="mx-auto size-64 rounded bg-white p-2" />
        ) : (
          <p className="text-center text-sm text-muted-foreground">{t("ui-loading")}</p>
        )}
      </DialogContent>
    </Dialog>
  )
}

function ClientDetail({
  c,
  isAdmin,
  onClearLimit,
  onShowQr,
}: {
  c: ClientView
  isAdmin: boolean
  onClearLimit: (mode: string) => void
  onShowQr: () => void
}) {
  const { t } = useTranslation()
  const yes = t("word-true")
  const no = t("word-false")
  const clear = (mode: string) => (isAdmin ? () => onClearLimit(mode) : undefined)
  const totalFlow = c.flow.inletFlow + c.flow.exportFlow
  const flowRemain =
    c.flow.flowLimit === 0 ? "∞" : formatBytes(c.flow.flowLimit * 1024 * 1024 - totalFlow)

  return (
    <div className="bg-muted/30 px-6 py-2 text-xs">
      <div>
        <DetailItem
          label={t("word-maxconnections")}
          value={c.maxConn === 0 ? no : c.maxConn}
          onClear={c.maxConn !== 0 ? clear("conn_limit") : undefined}
        />
        <DetailItem label={t("word-curconnections")} value={c.nowConn} />
        <DetailItem
          label={t("word-flowlimit")}
          value={c.flow.flowLimit === 0 ? no : formatBytes(c.flow.flowLimit * 1024 * 1024)}
          onClear={c.flow.flowLimit !== 0 ? clear("flow_limit") : undefined}
        />
        <DetailItem label={t("word-flowremain")} value={flowRemain} />
        <DetailItem
          label={t("word-timelimit")}
          value={formatTimeLimit(c.flow.timeLimit) ?? no}
          onClear={c.flow.timeLimit ? clear("time_limit") : undefined}
        />
        <DetailItem
          label={t("word-ratelimit")}
          value={c.rateLimit === 0 ? no : `${c.rateLimit} KB/s`}
          onClear={c.rateLimit !== 0 ? clear("rate_limit") : undefined}
        />
        <DetailItem
          label={t("word-maxtunnels")}
          value={c.maxTunnelNum === 0 ? no : c.maxTunnelNum}
          onClear={c.maxTunnelNum !== 0 ? clear("tunnel_limit") : undefined}
        />
      </div>
      <div>
        <DetailItem label={t("word-createtime")} value={c.createTime || "-"} />
        <DetailItem label={t("word-lastonlinetime")} value={c.lastOnlineTime || "-"} />
        <DetailItem label={t("word-address")} value={c.addr || "-"} copyable={c.addr} />
        <DetailItem
          label={t("word-localaddress")}
          value={c.localAddr || "-"}
          copyable={c.localAddr}
        />
      </div>
      {(c.webUserName || c.hasTotp) && (
        <div>
          {c.webUserName && (
            <DetailItem label={t("word-webusername")} value={c.webUserName} />
          )}
          {c.hasWebPassword && <DetailItem label={t("word-webpassword")} value="******" />}
          {c.hasTotp && (
            <DetailItem
              label={t("word-webtotpsecret")}
              value={
                <button type="button" className="text-primary hover:underline" onClick={onShowQr}>
                  {t("ui-qrcode")}
                </button>
              }
            />
          )}
        </div>
      )}
      {(c.basicUser || c.basicPassword) && (
        <div>
          <DetailItem
            label={t("word-basicusername")}
            value={c.basicUser || "-"}
            copyable={c.basicUser}
          />
          <DetailItem
            label={t("word-basicpassword")}
            value={c.basicPassword || "-"}
            copyable={c.basicPassword}
          />
        </div>
      )}
      <div>
        <DetailItem label={t("word-crypt")} value={c.crypt ? yes : no} />
        <DetailItem label={t("word-compress")} value={c.compress ? yes : no} />
        <DetailItem label={t("word-connectbyconfig")} value={c.configConnAllow ? yes : no} />
        <DetailItem label={t("word-blackip")} value={c.blackIpList.join(", ") || "-"} />
      </div>
    </div>
  )
}

export default function ClientsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const { state, setSearch, prevPage, nextPage, setLimit, toggleSort } = useListState()
  const [commandFor, setCommandFor] = useState<ClientView | null>(null)
  const [qrFor, setQrFor] = useState<ClientView | null>(null)
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  const { data, isLoading } = useQuery({
    queryKey: ["clients", state],
    queryFn: () => api.clients.list(state),
    refetchInterval: 10_000,
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["clients"] })

  const act = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      toast.success(t("operationsuccess"))
      invalidate()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const rows = data?.rows ?? []
  const isAdmin = !!user?.isAdmin

  const toggleExpand = (id: number) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const pingOne = async (c: ClientView) => {
    const r = await api.clients.ping(c.id)
    toast.info(`#${c.id} RTT: ${r.rtt} ms`)
  }

  const { visible, toggle } = useColumns("clients", CLIENT_COLUMNS)
  const shown = CLIENT_COLUMNS.filter((c) => visible(c.key))
  // Leading "" is the expand-arrow column, which is always present.
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

  // The old admin lists let you click a flow/limit number to clear it.
  const clearCell = async (id: number, mode: string) => {
    if (await confirm(t("clear"))) act.mutate(() => api.clients.clear(id, mode))
  }

  return (
    <div className="flex flex-col gap-4">
      {dialog}
      <CommandDialog client={commandFor} onClose={() => setCommandFor(null)} />
      <QrDialog client={qrFor} onClose={() => setQrFor(null)} />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("page-clientlist")}</h1>
        <div className="flex flex-wrap items-center gap-2">
          <SearchBox value={state.search} onChange={setSearch} />
          <ColumnPicker defs={CLIENT_COLUMNS} visible={visible} onToggle={toggle} />
          {isAdmin && (
            <>
              <Button
                variant="outline"
                onClick={async () => {
                  if (await confirm(t("clear"))) {
                    act.mutate(() => api.clients.clearAll("flow"))
                  }
                }}
              >
                <Eraser className="size-4" />
                {t("word-clearflow")}
              </Button>
              <Button
                variant="outline"
                onClick={() =>
                  act.mutate(async () => {
                    const online = rows.filter((c) => c.isConnect)
                    if (online.length === 0) {
                      toast.info(t("ui-nodata"))
                      return
                    }
                    await Promise.allSettled(online.map((c) => pingOne(c)))
                  })
                }
              >
                <Radio className="size-4" />
                {t("word-ping")}
              </Button>
            </>
          )}
          {isAdmin && (
            <Button asChild>
              <Link to="/clients/new">
                <Plus className="size-4" />
                {t("word-add")}
              </Link>
            </Button>
          )}
        </div>
      </div>

      <SimpleTable loading={isLoading} empty={rows.length === 0} headers={headers}>
        {rows.map((c) => {
          const totalFlow = c.flow.inletFlow + c.flow.exportFlow
          // Numbers the admin can click to clear, like the old formatters.
          const clearable = (
            text: React.ReactNode,
            mode: string,
            enabled = true,
          ): React.ReactNode =>
            isAdmin && enabled ? (
              <Tip content={t("word-clear")}>
                <span
                  className="cursor-pointer hover:underline"
                  onClick={() => void clearCell(c.id, mode)}
                >
                  {text}
                </span>
              </Tip>
            ) : (
              text
            )
          return (
            <Fragment key={c.id}>
              <TableRow>
                <TableCell className="w-8">
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground"
                    onClick={() => toggleExpand(c.id)}
                  >
                    {expanded.has(c.id) ? (
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
                    onClick={() => void copyText(String(c.id))}
                  >
                    {c.id}
                  </TableCell>
                )}
                {visible("remark") && (
                  <TableCell
                    className="max-w-40 cursor-pointer truncate"
                    title={t("word-copy")}
                    onClick={() => c.remark && void copyText(c.remark)}
                  >
                    {c.remark}
                  </TableCell>
                )}
                {visible("version") && (
                  <TableCell className="max-w-28 truncate text-xs">{c.version}</TableCell>
                )}
                {visible("bridgeMode") && (
                  <TableCell className="text-xs">{bridgeModeLabel(c.mode)}</TableCell>
                )}
                {visible("verifyKey") && (
                  <TableCell
                    className="max-w-36 cursor-pointer truncate font-mono text-xs"
                    title={t("word-copy")}
                    onClick={() => void copyText(c.verifyKey ?? "")}
                  >
                    {c.noStore ? t("word-publicvkey") : c.verifyKey}
                  </TableCell>
                )}
                {visible("addr") && (
                  <TableCell className="font-mono text-xs">{c.addr}</TableCell>
                )}
                {visible("localAddr") && (
                  <TableCell className="font-mono text-xs">{c.localAddr}</TableCell>
                )}
                {visible("inletFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(c.flow.inletFlow), "flow")}
                  </TableCell>
                )}
                {visible("exportFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(c.flow.exportFlow), "flow")}
                  </TableCell>
                )}
                {visible("totalFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(totalFlow), "flow")}
                  </TableCell>
                )}
                {visible("flowRemain") && (
                  <TableCell className="text-xs">
                    {c.flow.flowLimit === 0
                      ? "∞"
                      : clearable(
                          formatBytes(c.flow.flowLimit * 1024 * 1024 - totalFlow),
                          "flow_limit",
                        )}
                  </TableCell>
                )}
                {visible("timeRemain") && (
                  <TableCell className="text-xs">
                    {clearable(
                      formatTimeRemain(c.flow.timeLimit),
                      "time_limit",
                      c.flow.timeLimit !== 0,
                    )}
                  </TableCell>
                )}
                {visible("flowLimit") && (
                  <TableCell className="text-xs">
                    {c.flow.flowLimit === 0
                      ? t("word-false")
                      : clearable(formatBytes(c.flow.flowLimit * 1024 * 1024), "flow_limit")}
                  </TableCell>
                )}
                {visible("timeLimit") && (
                  <TableCell className="text-xs">
                    {formatTimeLimit(c.flow.timeLimit) === null
                      ? t("word-false")
                      : clearable(formatTimeLimit(c.flow.timeLimit), "time_limit")}
                  </TableCell>
                )}
                {visible("nowConn") && <TableCell>{c.nowConn}</TableCell>}
                {visible("speed") && (
                  <TableCell className="text-xs">{formatRate(c.nowRate)}</TableCell>
                )}
                {visible("status") && (
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Badge variant={c.status ? "default" : "destructive"}>
                        {c.status ? t("word-open") : t("word-close")}
                      </Badge>
                      {isAdmin && (
                        <Switch
                          checked={c.status}
                          onCheckedChange={(checked) =>
                            act.mutate(() => api.clients.setStatus(c.id, checked))
                          }
                        />
                      )}
                    </div>
                  </TableCell>
                )}
                {visible("isConnect") && (
                  <TableCell>
                    <Badge
                      variant={c.isConnect ? "default" : "secondary"}
                      className={c.isConnect ? "cursor-pointer" : undefined}
                      title={c.isConnect ? t("word-ping") : undefined}
                      onClick={() => {
                        if (c.isConnect) void pingOne(c)
                      }}
                    >
                      {c.isConnect ? t("word-online") : t("word-offline")}
                    </Badge>
                  </TableCell>
                )}
                {visible("option") && (
                  <TableCell>
                    <div className="flex gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        title={t("word-quicklycommand")}
                        onClick={() => setCommandFor(c)}
                      >
                        <Terminal className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        title={c.noStore ? t("word-publicvkey") : t("word-verifykey")}
                        onClick={() => void copyText(c.verifyKey ?? "")}
                      >
                        <KeyRound className="size-3.5" />
                      </Button>
                      {c.hasTotp && (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("ui-qrcode")}
                          onClick={() => setQrFor(c)}
                        >
                          <QrCode className="size-3.5" />
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        title={t("word-ping")}
                        onClick={() => act.mutate(() => pingOne(c))}
                      >
                        <Radio className="size-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon-xs" title={t("word-edit")} asChild>
                        <Link to={`/clients/${c.id}/edit`}>
                          <Pencil className="size-3.5" />
                        </Link>
                      </Button>
                      {isAdmin && (
                        <>
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            title={t("word-clearflow")}
                            onClick={() => void clearCell(c.id, "flow")}
                          >
                            <Eraser className="size-3.5" />
                          </Button>
                          {!c.noStore && (
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              title={t("word-delete")}
                              onClick={async () => {
                                if (await confirm(t("delete"))) {
                                  act.mutate(() => api.clients.remove(c.id))
                                }
                              }}
                            >
                              <Trash2 className="size-3.5 text-destructive" />
                            </Button>
                          )}
                        </>
                      )}
                    </div>
                  </TableCell>
                )}
                {visible("show") && (
                  <TableCell>
                    <div className="flex gap-2 text-xs">
                      <Link
                        to={`/tunnels?clientId=${c.id}`}
                        className="text-primary hover:underline"
                      >
                        {t("word-tunnel")} {c.tunnelNum}
                      </Link>
                      <Link to={`/hosts?clientId=${c.id}`} className="text-primary hover:underline">
                        {t("word-host")}
                      </Link>
                    </div>
                  </TableCell>
                )}
              </TableRow>
              {expanded.has(c.id) && (
                <TableRow>
                  <TableCell colSpan={columnCount} className="p-0">
                    <ClientDetail
                      c={c}
                      isAdmin={isAdmin}
                      onClearLimit={(mode) => void clearCell(c.id, mode)}
                      onShowQr={() => setQrFor(c)}
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
