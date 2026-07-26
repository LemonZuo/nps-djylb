import { useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Eraser, LinkIcon, Pencil, Play, Plus, Square, Trash2 } from "lucide-react"
import { api } from "@/api/endpoints"
import type { TunnelView } from "@/api/types"
import { useAuth } from "@/auth/AuthContext"
import { useConfirm } from "@/components/confirm-dialog"
import { ListFooter, SearchBox, SimpleTable, useListState } from "@/components/data-table"
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
  // The mixProxy copy string embeds the client's basic auth (user:pass@) like
  // the old UI did; those fields live on the full client record, not the list
  // row's ClientRef.
  const { data: client } = useQuery({
    queryKey: ["client", tunnel?.client.id],
    enabled: !!tunnel && tunnel.mode === "mixProxy",
    queryFn: () => api.clients.getOne(tunnel!.client.id),
  })

  if (!tunnel) return null
  const auth =
    client?.basicUser && client.basicPassword
      ? `${client.basicUser}:${client.basicPassword}@`
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

export default function TunnelsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const { state, setSearch, prevPage, nextPage } = useListState()
  const [searchParams, setSearchParams] = useSearchParams()
  const [addressFor, setAddressFor] = useState<TunnelView | null>(null)

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
                  {t(`scheme-${m}`)}
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
          "ID",
          t("word-remark"),
          t("word-scheme"),
          t("word-client"),
          t("word-port"),
          t("word-accessaddress"),
          t("word-target"),
          t("word-clientstatus"),
          t("word-runstatus"),
          t("word-trafficstatistics"),
          t("word-option"),
        ]}
      >
        {rows.map((row) => {
          const items = accessItems(row, host)
          return (
            <TableRow key={row.id}>
              <TableCell className="font-mono">{row.id}</TableCell>
              <TableCell className="max-w-32 truncate">{row.remark}</TableCell>
              <TableCell>
                <Badge variant="outline">{t(`scheme-${row.mode}`)}</Badge>
                {row.mode === "mixProxy" && (
                  <span className="ml-1 inline-flex gap-1 align-middle">
                    <Switch
                      title={t("word-enablehttpproxy")}
                      checked={row.httpProxy}
                      onCheckedChange={() =>
                        act.mutate(() => api.tunnels.toggle(row.id, "http", "toggle"))
                      }
                    />
                    <Switch
                      title={t("word-enablesocks5proxy")}
                      checked={row.socks5Proxy}
                      onCheckedChange={() =>
                        act.mutate(() => api.tunnels.toggle(row.id, "socks5", "toggle"))
                      }
                    />
                  </span>
                )}
              </TableCell>
              <TableCell>
                <span className="text-xs">
                  {row.client.id} {row.client.remark && `(${row.client.remark})`}
                </span>
              </TableCell>
              <TableCell
                className="cursor-pointer font-mono"
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
                <Badge variant={row.runStatus ? "default" : "destructive"}>
                  {row.runStatus ? t("word-online") : t("word-offline")}
                </Badge>
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
          )
        })}
      </SimpleTable>

      <ListFooter state={state} total={data?.total ?? 0} onPrev={prevPage} onNext={nextPage} />
    </div>
  )
}
