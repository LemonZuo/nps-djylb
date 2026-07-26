import { useState } from "react"
import { useTranslation } from "react-i18next"
import { ChevronLeft, ChevronRight, Search } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"

// The server pages with offset/limit; this state hook and the toolbar/footer
// below are what every list page shares. Column rendering stays per-page —
// the three resources have little column overlap, so a generic column model
// would only add indirection.

export interface ListState {
  offset: number
  limit: number
  search: string
}

export function useListState(limit = 10) {
  const [state, setState] = useState<ListState>({ offset: 0, limit, search: "" })
  return {
    state,
    setSearch: (search: string) => setState((s) => ({ ...s, search, offset: 0 })),
    prevPage: () => setState((s) => ({ ...s, offset: Math.max(0, s.offset - s.limit) })),
    nextPage: () => setState((s) => ({ ...s, offset: s.offset + s.limit })),
  }
}

export function SearchBox({
  value,
  onChange,
}: {
  value: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  return (
    <form
      className="relative"
      onSubmit={(e) => {
        e.preventDefault()
        onChange(draft)
      }}
    >
      <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        className="w-48 pl-8 md:w-64"
        placeholder={t("ui-search")}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => onChange(draft)}
      />
    </form>
  )
}

export function ListFooter({
  state,
  total,
  onPrev,
  onNext,
}: {
  state: ListState
  total: number
  onPrev: () => void
  onNext: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between pt-3">
      <span className="text-sm text-muted-foreground">{t("ui-total", { total })}</span>
      <div className="flex gap-1">
        <Button variant="outline" size="sm" disabled={state.offset === 0} onClick={onPrev}>
          <ChevronLeft className="size-4" />
          {t("ui-page-prev")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={state.offset + state.limit >= total}
          onClick={onNext}
        >
          {t("ui-page-next")}
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  )
}

// SimpleTable renders headers + rows with loading and empty states handled.
export function SimpleTable({
  headers,
  loading,
  empty,
  children,
}: {
  headers: React.ReactNode[]
  loading: boolean
  empty: boolean
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            {headers.map((h, i) => (
              <TableHead key={i}>{h}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            <TableRow>
              <TableCell colSpan={headers.length}>
                <Skeleton className="h-8 w-full" />
              </TableCell>
            </TableRow>
          ) : empty ? (
            <TableRow>
              <TableCell colSpan={headers.length} className="text-center text-muted-foreground">
                {t("ui-nodata")}
              </TableCell>
            </TableRow>
          ) : (
            children
          )}
        </TableBody>
      </Table>
    </div>
  )
}
