import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ArrowDown, ArrowUp, ArrowUpDown, RefreshCw, ShieldOff, Trash2 } from "lucide-react"
import { api } from "@/api/endpoints"
import type { BanEntry } from "@/api/types"
import { useConfirm } from "@/components/confirm-dialog"
import { SearchBox, SimpleTable } from "@/components/data-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { TableCell, TableRow } from "@/components/ui/table"
import { copyText } from "@/lib/format"

type SortKey = "key" | "type" | "failTimes" | "lastTry" | "banned"

// Admin-only page: the live login-ban table (failed-login throttling state).
export default function BansPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const [search, setSearch] = useState("")
  const [sortKey, setSortKey] = useState<SortKey | null>(null)
  const [sortAsc, setSortAsc] = useState(true)

  const { data: bans, isLoading: bansLoading } = useQuery({
    queryKey: ["bans"],
    queryFn: api.auth.bans,
    refetchInterval: 30_000,
  })

  const act = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      toast.success(t("operationsuccess"))
      queryClient.invalidateQueries({ queryKey: ["bans"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  // The old UI's refresh button hit /global/banclean first so expired records
  // vanish immediately instead of waiting for the minute-interval sweeper.
  const refresh = useMutation({
    mutationFn: () => api.auth.cleanBans(),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["bans"] }),
  })

  const banRows = useMemo(() => {
    let rows = bans?.rows ?? []
    if (search) {
      const q = search.toLowerCase()
      rows = rows.filter((b) => b.key.toLowerCase().includes(q))
    }
    if (sortKey) {
      const dir = sortAsc ? 1 : -1
      rows = [...rows].sort((a, b) => {
        const av = a[sortKey]
        const bv = b[sortKey]
        if (typeof av === "number" && typeof bv === "number") return (av - bv) * dir
        return String(av).localeCompare(String(bv)) * dir
      })
    }
    return rows
  }, [bans, search, sortKey, sortAsc])

  const toggleSort = (k: SortKey) => {
    if (sortKey === k) {
      setSortAsc((v) => !v)
    } else {
      setSortKey(k)
      setSortAsc(true)
    }
  }

  const sortHeader = (label: string, k: SortKey) => {
    const Icon = sortKey !== k ? ArrowUpDown : sortAsc ? ArrowUp : ArrowDown
    return (
      <button
        type="button"
        className="inline-flex items-center gap-1 hover:text-foreground"
        onClick={() => toggleSort(k)}
      >
        {label}
        <Icon className="size-3.5" />
      </button>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      {dialog}
      <h1 className="text-2xl font-semibold">{t("word-banlist")}</h1>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="text-base">{t("word-banlist")}</CardTitle>
          <div className="flex items-center gap-2">
            <SearchBox value={search} onChange={setSearch} />
            <Button
              variant="outline"
              size="sm"
              onClick={() => refresh.mutate()}
              disabled={refresh.isPending}
            >
              <RefreshCw className={refresh.isPending ? "size-4 animate-spin" : "size-4"} />
              {t("word-refresh")}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={async () => {
                if (await confirm(t("unbanall"))) {
                  act.mutate(() => api.auth.unbanAll())
                }
              }}
            >
              <Trash2 className="size-4" />
              {t("word-unbanall")}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <SimpleTable
            loading={bansLoading}
            empty={banRows.length === 0}
            headers={[
              sortHeader(t("word-bankey"), "key"),
              sortHeader(t("word-bantype"), "type"),
              sortHeader(t("word-failtimes"), "failTimes"),
              sortHeader(t("word-lastfailtime"), "lastTry"),
              sortHeader(t("word-banstatus"), "banned"),
              t("word-option"),
            ]}
          >
            {banRows.map((b: BanEntry) => (
              <TableRow key={b.key}>
                <TableCell
                  className="cursor-pointer font-mono"
                  title={t("word-copy")}
                  onClick={() => void copyText(b.key)}
                >
                  {b.key}
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{b.type === "ip" ? "IP" : t("word-username")}</Badge>
                </TableCell>
                <TableCell>{b.failTimes}</TableCell>
                <TableCell className="text-xs">{b.lastTry}</TableCell>
                <TableCell>
                  <Badge variant={b.banned ? "destructive" : "secondary"}>
                    {b.banned ? t("word-banned") : t("word-normal")}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="xs"
                    onClick={async () => {
                      if (await confirm(t("unban"))) {
                        act.mutate(() => api.auth.unban(b.key))
                      }
                    }}
                  >
                    <ShieldOff className="size-3.5" />
                    {t("word-unban")}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </SimpleTable>
        </CardContent>
      </Card>
    </div>
  )
}
