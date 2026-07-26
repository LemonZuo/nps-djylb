import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Columns3,
  Search,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
  sort?: string
  order?: "asc" | "desc"
}

export function useListState(limit = 10) {
  const [state, setState] = useState<ListState>({ offset: 0, limit, search: "" })
  return {
    state,
    setSearch: (search: string) => setState((s) => ({ ...s, search, offset: 0 })),
    prevPage: () => setState((s) => ({ ...s, offset: Math.max(0, s.offset - s.limit) })),
    nextPage: () => setState((s) => ({ ...s, offset: s.offset + s.limit })),
    setLimit: (l: number) => setState((s) => ({ ...s, limit: l, offset: 0 })),
    // Repeated clicks on one column cycle asc → desc; the backend list
    // functions take the DTO field names (Id, Remark, TotalFlow, ...).
    toggleSort: (field: string) =>
      setState((s) =>
        s.sort === field
          ? { ...s, order: s.order === "asc" ? "desc" : "asc", offset: 0 }
          : { ...s, sort: field, order: "asc", offset: 0 },
      ),
  }
}

// Column visibility, ported from bootstrap-table's showColumns dropdown. Each
// page declares its columns with the old template's default visibility; user
// overrides persist per-table in localStorage.

export interface ColumnDef {
  key: string
  // labelKey is the i18n key for both the header and the picker entry.
  labelKey: string
  defaultVisible: boolean
  // sortField is the backend DTO field for SortHead; omit for unsortable.
  sortField?: string
}

function loadOverrides(storageKey: string): Record<string, boolean> {
  try {
    const raw = localStorage.getItem(`columns-${storageKey}`)
    return raw ? (JSON.parse(raw) as Record<string, boolean>) : {}
  } catch {
    return {}
  }
}

export function useColumns(storageKey: string, defs: ColumnDef[]) {
  const [store, setStore] = useState(() => ({
    key: storageKey,
    overrides: loadOverrides(storageKey),
  }))
  // The tunnel page swaps storage keys per ?type= without remounting; reload
  // the overrides when that happens (render-time derived-state reset).
  if (store.key !== storageKey) {
    setStore({ key: storageKey, overrides: loadOverrides(storageKey) })
  }
  // defs can change between renders (per-mode columns on the tunnel list);
  // capture the latest for the toggle callback without re-reading storage.
  const defsRef = useRef(defs)
  defsRef.current = defs

  const visible = (key: string) => {
    const def = defs.find((d) => d.key === key)
    if (!def) return false
    return store.overrides[key] ?? def.defaultVisible
  }
  const toggle = (key: string) => {
    setStore((prev) => {
      const def = defsRef.current.find((d) => d.key === key)
      const overrides = {
        ...prev.overrides,
        [key]: !(prev.overrides[key] ?? def?.defaultVisible ?? true),
      }
      localStorage.setItem(`columns-${prev.key}`, JSON.stringify(overrides))
      return { ...prev, overrides }
    })
  }
  return { visible, toggle }
}

// ColumnPicker is the "show columns" dropdown button in the list toolbar.
export function ColumnPicker({
  defs,
  visible,
  onToggle,
}: {
  defs: ColumnDef[]
  visible: (key: string) => boolean
  onToggle: (key: string) => void
}) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="icon" title={t("ui-columns")}>
          <Columns3 className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-96 overflow-y-auto">
        {defs.map((d) => (
          <DropdownMenuCheckboxItem
            key={d.key}
            checked={visible(d.key)}
            onCheckedChange={() => onToggle(d.key)}
            onSelect={(e) => e.preventDefault()}
          >
            {t(d.labelKey)}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// SortHead is a clickable column header wired to useListState's toggleSort.
export function SortHead({
  label,
  field,
  state,
  onSort,
}: {
  label: React.ReactNode
  field: string
  state: ListState
  onSort: (field: string) => void
}) {
  const Icon = state.sort !== field ? ArrowUpDown : state.order === "asc" ? ArrowUp : ArrowDown
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 hover:text-foreground"
      onClick={() => onSort(field)}
    >
      {label}
      <Icon className="size-3.5" />
    </button>
  )
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

const PAGE_SIZES = [5, 10, 20, 50, 100]

export function ListFooter({
  state,
  total,
  onPrev,
  onNext,
  onLimit,
}: {
  state: ListState
  total: number
  onPrev: () => void
  onNext: () => void
  onLimit?: (limit: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between pt-3">
      <div className="flex items-center gap-3">
        <span className="text-sm text-muted-foreground">{t("ui-total", { total })}</span>
        {onLimit && (
          <Select value={String(state.limit)} onValueChange={(v) => onLimit(Number(v))}>
            <SelectTrigger size="sm" className="w-18">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PAGE_SIZES.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>
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
