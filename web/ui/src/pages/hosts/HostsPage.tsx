import { Fragment, useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import {
  ChevronDown,
  ChevronRight,
  Eraser,
  Pencil,
  Play,
  Plus,
  Square,
  Trash2,
} from "lucide-react"
import { api } from "@/api/endpoints"
import type { HostView } from "@/api/types"
import { useAuth } from "@/auth/AuthContext"
import { useConfirm } from "@/components/confirm-dialog"
import {
  ColumnPicker,
  ListFooter,
  SearchBox,
  SimpleTable,
  SortHead,
  useColumns,
  useListState,
  type ColumnDef,
} from "@/components/data-table"
import { DetailItem, formatTimeLimit, formatTimeRemain } from "@/components/detail-item"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { TableCell, TableRow } from "@/components/ui/table"
import { copyText, formatBytes } from "@/lib/format"

// The old index/hlist.html column matrix, defaults included. The boolean
// feature columns are hidden by default and toggle on click like the old
// starthost/stophost formatters.
const HOST_COLUMNS: ColumnDef[] = [
  { key: "id", labelKey: "word-id", defaultVisible: true, sortField: "Id" },
  { key: "clientId", labelKey: "word-clientid", defaultVisible: true, sortField: "Client.Id" },
  { key: "remark", labelKey: "word-remark", defaultVisible: true, sortField: "Remark" },
  {
    key: "clientVerifyKey",
    labelKey: "word-verifykey",
    defaultVisible: false,
    sortField: "Client.VerifyKey",
  },
  { key: "host", labelKey: "word-host", defaultVisible: true, sortField: "Host" },
  { key: "scheme", labelKey: "word-scheme", defaultVisible: true, sortField: "Scheme" },
  {
    key: "targetIsHttps",
    labelKey: "word-targettype",
    defaultVisible: false,
    sortField: "TargetIsHttps",
  },
  {
    key: "target",
    labelKey: "word-target",
    defaultVisible: true,
    sortField: "Target.TargetStr",
  },
  { key: "location", labelKey: "word-path", defaultVisible: true, sortField: "Location" },
  {
    key: "pathRewrite",
    labelKey: "word-pathrewrite",
    defaultVisible: false,
    sortField: "PathRewrite",
  },
  {
    key: "httpsJustProxy",
    labelKey: "word-httpsjustproxytitle",
    defaultVisible: false,
    sortField: "HttpsJustProxy",
  },
  {
    key: "tlsOffload",
    labelKey: "word-tlsoffloadtitle",
    defaultVisible: false,
    sortField: "TlsOffload",
  },
  { key: "autoSsl", labelKey: "word-autossl", defaultVisible: false, sortField: "AutoSSL" },
  {
    key: "autoHttps",
    labelKey: "word-autohttpstitle",
    defaultVisible: false,
    sortField: "AutoHttps",
  },
  {
    key: "autoCors",
    labelKey: "word-autocorstitle",
    defaultVisible: false,
    sortField: "AutoCORS",
  },
  {
    key: "compatMode",
    labelKey: "word-compatmode",
    defaultVisible: false,
    sortField: "CompatMode",
  },
  { key: "inletFlow", labelKey: "word-inletflow", defaultVisible: false, sortField: "InletFlow" },
  {
    key: "exportFlow",
    labelKey: "word-exportflow",
    defaultVisible: false,
    sortField: "ExportFlow",
  },
  { key: "totalFlow", labelKey: "word-totalflow", defaultVisible: true, sortField: "TotalFlow" },
  {
    key: "flowRemain",
    labelKey: "word-flowremain",
    defaultVisible: false,
    sortField: "FlowRemain",
  },
  {
    key: "timeRemain",
    labelKey: "word-timeremain",
    defaultVisible: false,
    sortField: "TimeRemain",
  },
  {
    key: "flowLimit",
    labelKey: "word-flowlimit",
    defaultVisible: false,
    sortField: "Flow.FlowLimit",
  },
  {
    key: "timeLimit",
    labelKey: "word-timelimit",
    defaultVisible: false,
    sortField: "Flow.TimeLimit",
  },
  { key: "nowConn", labelKey: "word-nowconn", defaultVisible: true, sortField: "NowConn" },
  { key: "runStatus", labelKey: "word-runstatus", defaultVisible: true, sortField: "IsClose" },
  {
    key: "clientStatus",
    labelKey: "word-clientstatus",
    defaultVisible: true,
    sortField: "Client.IsConnect",
  },
  { key: "option", labelKey: "word-option", defaultVisible: true },
]

// hostUrl builds the clickable public URL like the old list: scheme + host +
// the matching proxy port from bootstrap + location.
function hostUrl(row: HostView, httpPort?: string, httpsPort?: string): string {
  const https = row.scheme !== "http"
  const port = https ? httpsPort : httpPort
  const portPart = port && port !== (https ? "443" : "80") ? `:${port}` : ""
  return `${https ? "https" : "http"}://${row.host}${portPart}${row.location ?? ""}`
}

// The old bootstrap-table detailView for a host row: flow with click-to-clear,
// the boolean feature spans with click-to-toggle, request rewrites, and the
// TLS cert material when one is configured inline.
function HostDetail({
  h,
  isAdmin,
  onClear,
  onToggle,
}: {
  h: HostView
  isAdmin: boolean
  onClear: (mode: string) => void
  onToggle: (name: string) => void
}) {
  const { t } = useTranslation()
  const yes = t("word-true")
  const no = t("word-false")
  const clear = (mode: string) => (isAdmin ? () => onClear(mode) : undefined)
  const bool = (v: boolean) => (v ? yes : no)
  const showCert =
    h.scheme !== "http" && !h.httpsJustProxy && !h.autoSsl && (h.certFile || h.keyFile)

  return (
    <div className="bg-muted/30 px-6 py-2 text-xs">
      <div>
        <DetailItem
          label={t("word-inletflow")}
          value={formatBytes(h.flow.inletFlow)}
          onClear={clear("flow")}
        />
        <DetailItem
          label={t("word-exportflow")}
          value={formatBytes(h.flow.exportFlow)}
          onClear={clear("flow")}
        />
        <DetailItem label={t("word-nowconn")} value={h.nowConn} />
        <DetailItem label={t("word-crypt")} value={bool(h.client.crypt)} />
        <DetailItem label={t("word-compress")} value={bool(h.client.compress)} />
        <DetailItem
          label={t("word-flowlimit")}
          value={h.flow.flowLimit === 0 ? no : formatBytes(h.flow.flowLimit * 1024 * 1024)}
          onClear={h.flow.flowLimit !== 0 ? clear("flow_limit") : undefined}
        />
        <DetailItem
          label={t("word-timelimit")}
          value={formatTimeLimit(h.flow.timeLimit) ?? no}
          onClear={h.flow.timeLimit ? clear("time_limit") : undefined}
        />
      </div>
      <div>
        <DetailItem
          label={t("word-autohttps")}
          value={bool(h.autoHttps)}
          onToggle={() => onToggle("auto_https")}
        />
        <DetailItem
          label={t("word-autocors")}
          value={bool(h.autoCors)}
          onToggle={() => onToggle("auto_cors")}
        />
        <DetailItem
          label={t("word-compatmode")}
          value={bool(h.compatMode)}
          onToggle={() => onToggle("compat_mode")}
        />
        <DetailItem
          label={t("word-proxyprotocoltitle")}
          value={t(`word-proxyprotocolv${h.target.proxyProtocol}`)}
        />
        <DetailItem
          label={t("word-httpsjustproxytitle")}
          value={bool(h.httpsJustProxy)}
          onToggle={() => onToggle("https_just_proxy")}
        />
        <DetailItem
          label={t("word-tlsoffloadtitle")}
          value={bool(h.tlsOffload)}
          onToggle={() => onToggle("tls_offload")}
        />
        <DetailItem
          label={t("word-targettype")}
          value={t(`word-ishttps${h.targetIsHttps}`)}
          onToggle={() => onToggle("target_is_https")}
        />
        <DetailItem
          label={t("word-autossl")}
          value={bool(h.autoSsl)}
          onToggle={() => onToggle("auto_ssl")}
        />
      </div>
      {(h.client.basicUser || h.client.basicPassword) && (
        <div>
          <DetailItem
            label={t("word-basicusername")}
            value={h.client.basicUser || "-"}
            copyable={h.client.basicUser}
          />
          <DetailItem
            label={t("word-basicpassword")}
            value={h.client.basicPassword || "-"}
            copyable={h.client.basicPassword}
          />
        </div>
      )}
      {h.auth && (
        <div>
          <DetailItem label={t("word-auth")} value={h.auth} />
        </div>
      )}
      {h.pathRewrite && (
        <div>
          <DetailItem
            label={t("word-pathrewrite")}
            value={h.pathRewrite}
            copyable={h.pathRewrite}
          />
        </div>
      )}
      {h.hostChange && (
        <div>
          <DetailItem
            label={t("word-requesthost")}
            value={h.hostChange}
            copyable={h.hostChange}
          />
        </div>
      )}
      {h.headerChange && (
        <div>
          <DetailItem label={t("word-requestheader")} value={h.headerChange} />
        </div>
      )}
      {h.respHeaderChange && (
        <div>
          <DetailItem label={t("word-responseheader")} value={h.respHeaderChange} />
        </div>
      )}
      {h.redirectUrl && (
        <div>
          <DetailItem
            label={t("word-redirecturl")}
            value={h.redirectUrl}
            copyable={h.redirectUrl}
          />
        </div>
      )}
      {showCert && (
        <div className="mt-2 flex flex-col gap-2">
          <div>
            <b>{t("word-httpscert")}</b>:
            <pre
              className="mt-1 max-h-16 cursor-pointer overflow-auto rounded border bg-background p-1.5 font-mono text-xs whitespace-nowrap"
              title={t("word-copy")}
              onClick={() => void copyText(h.certFile)}
            >
              {h.certFile}
            </pre>
          </div>
          {h.keyFile !== undefined && (
            <div>
              <b>{t("word-httpskey")}</b>:
              <pre
                className="mt-1 max-h-16 cursor-pointer overflow-auto rounded border bg-background p-1.5 font-mono text-xs whitespace-nowrap"
                title={t("word-copy")}
                onClick={() => void copyText(h.keyFile ?? "")}
              >
                {h.keyFile}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function HostsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { confirm, dialog } = useConfirm()
  const { state, setSearch, prevPage, nextPage, setLimit, toggleSort } = useListState()
  const [searchParams] = useSearchParams()
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  const clientId = searchParams.get("clientId")

  const query = useMemo(
    () => ({ ...state, clientId: clientId ? Number(clientId) : undefined }),
    [state, clientId],
  )

  const { data, isLoading } = useQuery({
    queryKey: ["hosts", query],
    queryFn: () => api.hosts.list(query),
    refetchInterval: 10_000,
  })

  const { data: bootstrap } = useQuery({ queryKey: ["bootstrap"], queryFn: api.meta.bootstrap })

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

  const toggleExpand = (id: number) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const { visible, toggle } = useColumns("hosts", HOST_COLUMNS)
  const shown = HOST_COLUMNS.filter((c) => visible(c.key))
  const headers: React.ReactNode[] = [
    "",
    ...shown.map((c) =>
      c.sortField ? (
        <SortHead
          key={c.key}
          label={t(c.labelKey)}
          field={c.sortField}
          state={state}
          onSort={toggleSort}
        />
      ) : (
        t(c.labelKey)
      ),
    ),
  ]
  const columnCount = headers.length

  const clearCell = async (id: number, mode: string) => {
    if (await confirm(t("clear"))) act.mutate(() => api.hosts.clear(id, mode))
  }
  const toggleFeature = (id: number, name: string) =>
    act.mutate(() => api.hosts.toggle(id, name, "toggle"))

  return (
    <div className="flex flex-col gap-4">
      {dialog}

      <div className="flex items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">
          {t("page-hostlist")}
          {clientId && (
            <span className="ml-2 text-base font-normal text-muted-foreground">
              {t("page-listclientid")}
              {clientId}
            </span>
          )}
        </h1>
        <div className="flex items-center gap-2">
          <SearchBox value={state.search} onChange={setSearch} />
          <ColumnPicker defs={HOST_COLUMNS} visible={visible} onToggle={toggle} />
          <Button asChild>
            <Link to={`/hosts/new${clientId ? `?clientId=${clientId}` : ""}`}>
              <Plus className="size-4" />
              {t("word-add")}
            </Link>
          </Button>
        </div>
      </div>

      <SimpleTable loading={isLoading} empty={rows.length === 0} headers={headers}>
        {rows.map((row) => {
          const totalFlow = row.flow.inletFlow + row.flow.exportFlow
          const clearable = (
            text: React.ReactNode,
            mode: string,
            enabled = true,
          ): React.ReactNode =>
            isAdmin && enabled ? (
              <span
                className="cursor-pointer hover:underline"
                title={t("word-clear")}
                onClick={() => void clearCell(row.id, mode)}
              >
                {text}
              </span>
            ) : (
              text
            )
          // The old boolean feature columns flip the flag on click.
          const featureCell = (value: boolean, name: string) => (
            <span
              className="cursor-pointer text-primary hover:underline"
              onClick={() => toggleFeature(row.id, name)}
            >
              {value ? t("word-true") : t("word-false")}
            </span>
          )
          return (
            <Fragment key={row.id}>
              <TableRow>
                <TableCell className="w-8">
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground"
                    onClick={() => toggleExpand(row.id)}
                  >
                    {expanded.has(row.id) ? (
                      <ChevronDown className="size-4" />
                    ) : (
                      <ChevronRight className="size-4" />
                    )}
                  </button>
                </TableCell>
                {visible("id") && (
                  <TableCell
                    className="cursor-pointer font-mono"
                    title={t("word-copy")}
                    onClick={() => void copyText(String(row.id))}
                  >
                    {row.id}
                  </TableCell>
                )}
                {visible("clientId") && (
                  <TableCell
                    className={row.client.verifyKey ? "cursor-pointer text-xs" : "text-xs"}
                    title={row.client.verifyKey ? t("word-copy") : undefined}
                    onClick={() => row.client.verifyKey && void copyText(row.client.verifyKey)}
                  >
                    {row.client.id} {row.client.remark && `(${row.client.remark})`}
                  </TableCell>
                )}
                {visible("remark") && (
                  <TableCell
                    className="max-w-32 cursor-pointer truncate"
                    title={t("word-copy")}
                    onClick={() => row.remark && void copyText(row.remark)}
                  >
                    {row.remark}
                  </TableCell>
                )}
                {visible("clientVerifyKey") && (
                  <TableCell
                    className="max-w-36 cursor-pointer truncate font-mono text-xs"
                    title={t("word-copy")}
                    onClick={() => row.client.verifyKey && void copyText(row.client.verifyKey)}
                  >
                    {row.client.verifyKey}
                  </TableCell>
                )}
                {visible("host") && (
                  <TableCell>
                    <a
                      href={hostUrl(row, bootstrap?.httpProxyPort, bootstrap?.httpsProxyPort)}
                      target="_blank"
                      rel="noreferrer"
                      className="font-mono text-xs hover:underline"
                    >
                      {row.host}
                    </a>
                  </TableCell>
                )}
                {visible("scheme") && (
                  <TableCell>
                    <Badge variant="outline">{row.scheme}</Badge>
                  </TableCell>
                )}
                {visible("targetIsHttps") && (
                  <TableCell className="text-xs">
                    <span
                      className="cursor-pointer text-primary hover:underline"
                      onClick={() => toggleFeature(row.id, "target_is_https")}
                    >
                      {t(`word-ishttps${row.targetIsHttps}`)}
                    </span>
                  </TableCell>
                )}
                {visible("target") && (
                  <TableCell
                    className="max-w-40 cursor-pointer truncate font-mono text-xs"
                    title={t("word-copy")}
                    onClick={() => void copyText(row.target.target)}
                  >
                    {row.target.target}
                  </TableCell>
                )}
                {visible("location") && (
                  <TableCell className="font-mono text-xs">{row.location}</TableCell>
                )}
                {visible("pathRewrite") && (
                  <TableCell className="font-mono text-xs">{row.pathRewrite}</TableCell>
                )}
                {visible("httpsJustProxy") && (
                  <TableCell className="text-xs">
                    {featureCell(row.httpsJustProxy, "https_just_proxy")}
                  </TableCell>
                )}
                {visible("tlsOffload") && (
                  <TableCell className="text-xs">
                    {featureCell(row.tlsOffload, "tls_offload")}
                  </TableCell>
                )}
                {visible("autoSsl") && (
                  <TableCell className="text-xs">{featureCell(row.autoSsl, "auto_ssl")}</TableCell>
                )}
                {visible("autoHttps") && (
                  <TableCell className="text-xs">
                    {featureCell(row.autoHttps, "auto_https")}
                  </TableCell>
                )}
                {visible("autoCors") && (
                  <TableCell className="text-xs">
                    {featureCell(row.autoCors, "auto_cors")}
                  </TableCell>
                )}
                {visible("compatMode") && (
                  <TableCell className="text-xs">
                    {featureCell(row.compatMode, "compat_mode")}
                  </TableCell>
                )}
                {visible("inletFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(row.flow.inletFlow), "flow")}
                  </TableCell>
                )}
                {visible("exportFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(row.flow.exportFlow), "flow")}
                  </TableCell>
                )}
                {visible("totalFlow") && (
                  <TableCell className="text-xs">
                    {clearable(formatBytes(totalFlow), "flow")}
                  </TableCell>
                )}
                {visible("flowRemain") && (
                  <TableCell className="text-xs">
                    {row.flow.flowLimit === 0
                      ? "∞"
                      : clearable(
                          formatBytes(row.flow.flowLimit * 1024 * 1024 - totalFlow),
                          "flow_limit",
                        )}
                  </TableCell>
                )}
                {visible("timeRemain") && (
                  <TableCell className="text-xs">
                    {clearable(
                      formatTimeRemain(row.flow.timeLimit),
                      "time_limit",
                      row.flow.timeLimit !== 0,
                    )}
                  </TableCell>
                )}
                {visible("flowLimit") && (
                  <TableCell className="text-xs">
                    {row.flow.flowLimit === 0
                      ? t("word-false")
                      : clearable(formatBytes(row.flow.flowLimit * 1024 * 1024), "flow_limit")}
                  </TableCell>
                )}
                {visible("timeLimit") && (
                  <TableCell className="text-xs">
                    {formatTimeLimit(row.flow.timeLimit) === null
                      ? t("word-false")
                      : clearable(formatTimeLimit(row.flow.timeLimit), "time_limit")}
                  </TableCell>
                )}
                {visible("nowConn") && <TableCell>{row.nowConn}</TableCell>}
                {visible("runStatus") && (
                  <TableCell>
                    <Badge variant={row.isClose ? "destructive" : "default"}>
                      {row.isClose ? t("word-close") : t("word-open")}
                    </Badge>
                  </TableCell>
                )}
                {visible("clientStatus") && (
                  <TableCell>
                    {row.client.isConnect ? (
                      <a
                        href={hostUrl(row, bootstrap?.httpProxyPort, bootstrap?.httpsProxyPort)}
                        target="_blank"
                        rel="noreferrer"
                      >
                        <Badge variant="default">{t("word-online")}</Badge>
                      </a>
                    ) : (
                      <Badge variant="secondary">{t("word-offline")}</Badge>
                    )}
                  </TableCell>
                )}
                {visible("option") && (
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
                          onClick={() => void clearCell(row.id, "flow")}
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
                )}
              </TableRow>
              {expanded.has(row.id) && (
                <TableRow>
                  <TableCell colSpan={columnCount} className="p-0">
                    <HostDetail
                      h={row}
                      isAdmin={isAdmin}
                      onClear={(mode) => void clearCell(row.id, mode)}
                      onToggle={(name) => toggleFeature(row.id, name)}
                    />
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          )
        })}
      </SimpleTable>

      <ListFooter
        state={state}
        total={data?.total ?? 0}
        onPrev={prevPage}
        onNext={nextPage}
        onLimit={setLimit}
      />
    </div>
  )
}
