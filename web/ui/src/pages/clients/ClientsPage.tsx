import { useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import {
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
import { ListFooter, SearchBox, SimpleTable, useListState } from "@/components/data-table"
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
import { copyText, formatBytes, formatRate } from "@/lib/format"

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
              <code
                className="cursor-pointer rounded bg-muted p-2 font-mono text-xs break-all hover:bg-muted/70"
                title={t("word-copy")}
                onClick={() => void copyText(c.cmd)}
              >
                {c.cmd}
              </code>
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

export default function ClientsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const { state, setSearch, prevPage, nextPage } = useListState()
  const [commandFor, setCommandFor] = useState<ClientView | null>(null)
  const [qrFor, setQrFor] = useState<ClientView | null>(null)

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

  return (
    <div className="flex flex-col gap-4">
      {dialog}
      <CommandDialog client={commandFor} onClose={() => setCommandFor(null)} />
      <QrDialog client={qrFor} onClose={() => setQrFor(null)} />

      <div className="flex items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("page-clientlist")}</h1>
        <div className="flex items-center gap-2">
          <SearchBox value={state.search} onChange={setSearch} />
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

      <SimpleTable
        loading={isLoading}
        empty={rows.length === 0}
        headers={[
          "ID",
          t("word-remark"),
          t("word-clientstatus"),
          t("word-address"),
          t("word-version"),
          t("word-speed"),
          t("word-nowconn"),
          t("word-trafficstatistics"),
          t("word-tunnel"),
          t("word-option"),
        ]}
      >
        {rows.map((c) => (
          <TableRow key={c.id}>
            <TableCell className="font-mono">{c.id}</TableCell>
            <TableCell className="max-w-40 truncate">{c.remark}</TableCell>
            <TableCell>
              <div className="flex items-center gap-2">
                <Badge variant={c.isConnect ? "default" : "secondary"}>
                  {c.isConnect ? t("word-online") : t("word-offline")}
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
            <TableCell className="font-mono text-xs">{c.addr}</TableCell>
            <TableCell className="max-w-28 truncate text-xs">{c.version}</TableCell>
            <TableCell className="text-xs">{formatRate(c.nowRate)}</TableCell>
            <TableCell>{c.nowConn}</TableCell>
            <TableCell className="text-xs">
              {formatBytes(c.flow.inletFlow)} / {formatBytes(c.flow.exportFlow)}
            </TableCell>
            <TableCell>
              <Link to={`/tunnels?clientId=${c.id}`} className="text-primary hover:underline">
                {c.tunnelNum}
              </Link>
            </TableCell>
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
                  title={t("word-verifykey")}
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
                  onClick={() =>
                    act.mutate(async () => {
                      const r = await api.clients.ping(c.id)
                      toast.info(`RTT: ${r.rtt} ms`)
                    })
                  }
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
                      onClick={async () => {
                        if (await confirm(t("clear"))) {
                          act.mutate(() => api.clients.clear(c.id, "flow"))
                        }
                      }}
                    >
                      <Eraser className="size-3.5" />
                    </Button>
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
                  </>
                )}
              </div>
            </TableCell>
          </TableRow>
        ))}
      </SimpleTable>

      <ListFooter state={state} total={data?.total ?? 0} onPrev={prevPage} onNext={nextPage} />
    </div>
  )
}
