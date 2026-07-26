import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { api } from "@/api/endpoints"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

// ClientPicker lists every client for the admin to pick a tunnel/host owner.
// The 1000-row page matches maxPageSize server-side; installs beyond that are
// out of scope for a dropdown anyway.
export function ClientPicker({
  value,
  onChange,
}: {
  value: number | null
  onChange: (id: number) => void
}) {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ["clients-all"],
    queryFn: () => api.clients.list({ limit: 1000 }),
  })

  return (
    <Select
      value={value === null ? undefined : String(value)}
      onValueChange={(v) => onChange(Number(v))}
    >
      <SelectTrigger>
        <SelectValue placeholder={t("word-belongclient")} />
      </SelectTrigger>
      <SelectContent>
        {(data?.rows ?? []).map((c) => (
          <SelectItem key={c.id} value={String(c.id)}>
            {c.id} {c.remark && `- ${c.remark}`}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
