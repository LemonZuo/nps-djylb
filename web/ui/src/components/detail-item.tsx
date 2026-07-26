import { useTranslation } from "react-i18next"
import { copyText } from "@/lib/format"

// One key: value pair in an expanded detail row, matching the old
// bootstrap-table detailView. onClear (admin) mirrors the old "click the
// number to clear the limit" behaviour; onToggle is the old click-to-flip
// boolean feature spans on host rows.
export function DetailItem({
  label,
  value,
  copyable,
  onClear,
  onToggle,
}: {
  label: string
  value: React.ReactNode
  copyable?: string
  onClear?: () => void
  onToggle?: () => void
}) {
  const { t } = useTranslation()
  return (
    <span className="mr-6 inline-flex items-baseline gap-1 text-xs leading-6">
      <b>{label}</b>:
      {copyable !== undefined ? (
        <span
          className="cursor-pointer font-mono hover:underline"
          title={t("word-copy")}
          onClick={() => void copyText(copyable)}
        >
          {value}
        </span>
      ) : onClear ? (
        <span
          className="cursor-pointer hover:underline"
          title={t("word-clear")}
          onClick={onClear}
        >
          {value}
        </span>
      ) : onToggle ? (
        <span className="cursor-pointer text-primary hover:underline" onClick={onToggle}>
          {value}
        </span>
      ) : (
        <span>{value}</span>
      )}
    </span>
  )
}

// formatTimeLimit renders the unix-seconds time limit the old UI showed as a
// datetime, with 0 meaning no limit.
export function formatTimeLimit(unixSecs: number): string | null {
  if (!unixSecs) return null
  return new Date(unixSecs * 1000).toLocaleString()
}

// formatTimeRemain matches the old getRemainingTime helper: countdown to the
// time limit as the two most significant of d/h/m/s, ∞ for no limit.
export function formatTimeRemain(unixSecs: number): string {
  if (!unixSecs) return "∞"
  const diff = unixSecs * 1000 - Date.now()
  if (diff <= 0) return "0"
  const s = Math.floor(diff / 1000)
  const days = Math.floor(s / 86400)
  const hours = Math.floor((s % 86400) / 3600)
  const minutes = Math.floor((s % 3600) / 60)
  const parts = [
    days ? `${days}d` : null,
    hours ? `${hours}h` : null,
    minutes ? `${minutes}m` : null,
    `${s % 60}s`,
  ].filter(Boolean)
  return parts.slice(0, 2).join(" ")
}
