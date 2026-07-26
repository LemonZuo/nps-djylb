import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ShieldOff, Trash2 } from "lucide-react"
import { api } from "@/api/endpoints"
import { useConfirm } from "@/components/confirm-dialog"
import { SimpleTable } from "@/components/data-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { TableCell, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"

// Admin-only page: the global IP blacklist (persisted in global.json) and the
// live login-ban table.
export default function GlobalPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const [blacklist, setBlacklist] = useState("")

  const { data: settings } = useQuery({ queryKey: ["global"], queryFn: api.global.getSettings })
  const { data: bans, isLoading: bansLoading } = useQuery({
    queryKey: ["bans"],
    queryFn: api.auth.bans,
    refetchInterval: 30_000,
  })

  useEffect(() => {
    if (settings) setBlacklist(settings.blackIpList.join("\n"))
  }, [settings])

  const save = useMutation({
    mutationFn: () =>
      api.global.save(
        blacklist
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean),
      ),
    onSuccess: () => {
      toast.success(t("savesuccess"))
      queryClient.invalidateQueries({ queryKey: ["global"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const act = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      toast.success(t("operationsuccess"))
      queryClient.invalidateQueries({ queryKey: ["bans"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const banRows = bans?.rows ?? []

  return (
    <div className="flex flex-col gap-4">
      {dialog}
      <h1 className="text-2xl font-semibold">{t("word-globalparam")}</h1>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("word-globalblackiplist")}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <Textarea
            value={blacklist}
            onChange={(e) => setBlacklist(e.target.value)}
            rows={8}
            className="font-mono"
            placeholder={t("info-suchasblackiplist")}
          />
          <p className="text-xs text-muted-foreground">{t("info-descblackiplist")}</p>
          <div>
            <Button onClick={() => save.mutate()} disabled={save.isPending}>
              {save.isPending ? t("processing") : t("word-save")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="text-base">{t("word-banlist")}</CardTitle>
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
        </CardHeader>
        <CardContent>
          <SimpleTable
            loading={bansLoading}
            empty={banRows.length === 0}
            headers={[
              t("word-bankey"),
              t("word-bantype"),
              t("word-failtimes"),
              t("word-lastfailtime"),
              t("word-banstatus"),
              t("word-option"),
            ]}
          >
            {banRows.map((b) => (
              <TableRow key={b.key}>
                <TableCell className="font-mono">{b.key}</TableCell>
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
