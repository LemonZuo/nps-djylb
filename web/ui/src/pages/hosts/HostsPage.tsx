import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Eraser, ExternalLink, Pencil, Play, Plus, Square, Trash2 } from "lucide-react"
import { api } from "@/api/endpoints"
import { useAuth } from "@/auth/AuthContext"
import { useConfirm } from "@/components/confirm-dialog"
import { ListFooter, SearchBox, SimpleTable, useListState } from "@/components/data-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { TableCell, TableRow } from "@/components/ui/table"
import { copyText, formatBytes } from "@/lib/format"

export default function HostsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const { state, setSearch, prevPage, nextPage } = useListState()

  const { data, isLoading } = useQuery({
    queryKey: ["hosts", state],
    queryFn: () => api.hosts.list(state),
    refetchInterval: 10_000,
  })

  const act = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      toast.success(t("operationsuccess"))
      queryClient.invalidateQueries({ queryKey: ["hosts"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const rows = data?.rows ?? []
  const isAdmin = !!user?.isAdmin

  return (
    <div className="flex flex-col gap-4">
      {dialog}

      <div className="flex items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("page-hostlist")}</h1>
        <div className="flex items-center gap-2">
          <SearchBox value={state.search} onChange={setSearch} />
          <Button asChild>
            <Link to="/hosts/new">
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
          t("word-host"),
          t("word-scheme"),
          t("word-client"),
          t("word-target"),
          t("word-remark"),
          t("word-clientstatus"),
          t("word-runstatus"),
          t("word-trafficstatistics"),
          t("word-option"),
        ]}
      >
        {rows.map((row) => (
          <TableRow key={row.id}>
            <TableCell className="font-mono">{row.id}</TableCell>
            <TableCell>
              <span
                className="cursor-pointer font-mono text-xs hover:underline"
                title={t("word-copy")}
                onClick={() => void copyText(row.host)}
              >
                {row.host}
              </span>
              <a
                href={`${row.scheme === "https" ? "https" : "http"}://${row.host}`}
                target="_blank"
                rel="noreferrer"
                className="ml-1 inline-block align-middle text-muted-foreground hover:text-foreground"
              >
                <ExternalLink className="size-3" />
              </a>
              {row.location && row.location !== "/" && (
                <span className="ml-1 text-xs text-muted-foreground">{row.location}</span>
              )}
            </TableCell>
            <TableCell>
              <Badge variant="outline">{row.scheme}</Badge>
            </TableCell>
            <TableCell className="text-xs">
              {row.client.id} {row.client.remark && `(${row.client.remark})`}
            </TableCell>
            <TableCell
              className="max-w-40 cursor-pointer truncate font-mono text-xs"
              onClick={() => void copyText(row.target.target)}
            >
              {row.target.target}
            </TableCell>
            <TableCell className="max-w-32 truncate">{row.remark}</TableCell>
            <TableCell>
              <Badge variant={row.client.isConnect ? "default" : "secondary"}>
                {row.client.isConnect ? t("word-online") : t("word-offline")}
              </Badge>
            </TableCell>
            <TableCell>
              <Badge variant={row.isClose ? "destructive" : "default"}>
                {row.isClose ? t("word-stop") : t("word-start")}
              </Badge>
            </TableCell>
            <TableCell className="text-xs">
              {formatBytes(row.flow.inletFlow)} / {formatBytes(row.flow.exportFlow)}
            </TableCell>
            <TableCell>
              <div className="flex gap-1">
                {row.isClose ? (
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    title={t("word-start")}
                    onClick={async () => {
                      if (await confirm(t("start"))) {
                        act.mutate(() => api.hosts.start(row.id))
                      }
                    }}
                  >
                    <Play className="size-3.5" />
                  </Button>
                ) : (
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    title={t("word-stop")}
                    onClick={async () => {
                      if (await confirm(t("stop"))) {
                        act.mutate(() => api.hosts.stop(row.id))
                      }
                    }}
                  >
                    <Square className="size-3.5" />
                  </Button>
                )}
                <Button variant="ghost" size="icon-xs" title={t("word-edit")} asChild>
                  <Link to={`/hosts/${row.id}/edit`}>
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
                        act.mutate(() => api.hosts.clear(row.id, "flow"))
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
                      act.mutate(() => api.hosts.remove(row.id))
                    }
                  }}
                >
                  <Trash2 className="size-3.5 text-destructive" />
                </Button>
              </div>
            </TableCell>
          </TableRow>
        ))}
      </SimpleTable>

      <ListFooter state={state} total={data?.total ?? 0} onPrev={prevPage} onNext={nextPage} />
    </div>
  )
}
