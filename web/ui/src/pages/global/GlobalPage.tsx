import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { api } from "@/api/endpoints"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Textarea } from "@/components/ui/textarea"

// Admin-only page: the global IP blacklist (persisted in global.json).
// The login-ban table lives on its own page (BansPage), matching the old
// UI's separate 封禁管理 menu entry.
export default function GlobalPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [blacklist, setBlacklist] = useState("")

  const { data: settings } = useQuery({ queryKey: ["global"], queryFn: api.global.getSettings })

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

  return (
    <div className="flex flex-col gap-4">
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
    </div>
  )
}
