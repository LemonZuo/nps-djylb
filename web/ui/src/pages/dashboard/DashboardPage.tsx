import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import type { ECharts, EChartsOption } from "echarts"
import { useTheme } from "next-themes"
import { Activity, Clock, MonitorSmartphone, Wifi } from "lucide-react"
import { api } from "@/api/endpoints"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { formatBytes, formatRate } from "@/lib/format"

function StatCard({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Clock
  label: string
  value: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
        <Icon className="size-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold">{value}</div>
      </CardContent>
    </Card>
  )
}

// echarts is ~1 MB minified; loading it on demand keeps it out of the main
// chunk that the login page pays for. The instance survives data refreshes
// (setOption only) so the 5-second dashboard poll doesn't flicker the charts.
function EChart({ option, className }: { option: EChartsOption; className?: string }) {
  const { resolvedTheme } = useTheme()
  const ref = useRef<HTMLDivElement>(null)
  const chartRef = useRef<ECharts | null>(null)
  const [ready, setReady] = useState(0)

  useEffect(() => {
    if (!ref.current) return
    let disposed = false
    const el = ref.current
    void import("echarts").then((echarts) => {
      if (disposed) return
      chartRef.current = echarts.init(el, resolvedTheme === "dark" ? "dark" : undefined)
      setReady((n) => n + 1)
    })
    const onResize = () => chartRef.current?.resize()
    window.addEventListener("resize", onResize)
    return () => {
      disposed = true
      window.removeEventListener("resize", onResize)
      chartRef.current?.dispose()
      chartRef.current = null
    }
  }, [resolvedTheme])

  useEffect(() => {
    chartRef.current?.setOption({ backgroundColor: "transparent", ...option }, true)
  }, [option, ready])

  return <div ref={ref} className={className ?? "h-72 w-full"} />
}

function ChartCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b py-1.5 text-xs last:border-b-0">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="truncate text-right font-medium">{value}</span>
    </div>
  )
}

// CPU / memory rows in the system card, with the old template's progress bar.
function MeterRow({ label, percent }: { label: string; percent: number }) {
  const pct = Math.max(0, Math.min(100, percent))
  return (
    <div className="border-b py-1.5 text-xs">
      <div className="flex items-center justify-between gap-4">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium">{pct}%</span>
      </div>
      <div className="mt-1.5 h-1.5 w-full rounded-full bg-muted">
        <div className="h-1.5 rounded-full bg-primary transition-all" style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

// GetDashboardData's "load" is gopsutil AvgStat serialized as JSON — the old
// template did JSON.parse and printed the three values in order.
function formatLoad(load: unknown): string {
  if (typeof load !== "string" || load === "") return "-"
  try {
    const parsed: unknown = JSON.parse(load)
    if (parsed && typeof parsed === "object") {
      return Object.values(parsed)
        .map((v) => (typeof v === "number" ? v.toFixed(2) : String(v)))
        .join("  ")
    }
  } catch {
    // not JSON — show as-is
  }
  return load
}

const LINE_GRID = { left: "3%", right: "4%", top: 32, bottom: "3%", containLabel: true }

function lineOption(
  times: string[],
  series: { name: string; data: number[] }[],
  yFormatter?: (v: number) => string,
): EChartsOption {
  return {
    tooltip: {
      trigger: "axis",
      valueFormatter: yFormatter ? (v) => yFormatter(Number(v)) : undefined,
    },
    legend: series.length > 1 ? { data: series.map((s) => s.name) } : undefined,
    grid: LINE_GRID,
    xAxis: { type: "category", boundaryGap: false, data: times },
    yAxis: {
      type: "value",
      axisLabel: yFormatter ? { formatter: yFormatter } : undefined,
    },
    series: series.map((s) => ({
      name: s.name,
      type: "line",
      smooth: true,
      showSymbol: false,
      data: s.data,
    })),
  }
}

function pieOption(
  name: string,
  data: { name: string; value: number }[],
  valueFormatter?: (v: number) => string,
): EChartsOption {
  return {
    tooltip: {
      trigger: "item",
      valueFormatter: valueFormatter ? (v) => valueFormatter(Number(v)) : undefined,
    },
    legend: { orient: "vertical", left: "left" },
    series: [
      {
        name,
        type: "pie",
        radius: "55%",
        center: ["50%", "56%"],
        data,
      },
    ],
  }
}

export default function DashboardPage() {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ["dashboard"],
    queryFn: () => api.dashboard.data(),
    refetchInterval: 5_000,
  })
  const { data: bootstrap } = useQuery({ queryKey: ["bootstrap"], queryFn: api.meta.bootstrap })
  // History sampling only runs with system_info_display on; an empty list
  // means the line charts should not render at all.
  const { data: history } = useQuery({
    queryKey: ["dashboard-history"],
    queryFn: api.dashboard.history,
    refetchInterval: 60_000,
  })

  const d = data ?? {}
  const n = (v: unknown) => (typeof v === "number" ? v : 0)
  const s = (v: unknown) => (v === undefined || v === null || v === "" ? "-" : String(v))

  // "TCP:8883 KCP:8883 TLS:8884 WS:8885 WSS:8886 Path:/ws", like the old
  // template built from tcp_p/kcp_p/... — here from the bootstrap endpoints.
  const bridgeMode = useMemo(() => {
    if (!bootstrap || bootstrap.endpoints.length === 0) return "-"
    const parts = bootstrap.endpoints.map((e) => `${e.type.toUpperCase()}:${e.port}`)
    const path = bootstrap.endpoints.find((e) => e.path)?.path
    if (path) parts.push(`Path:${path}`)
    return parts.join(" ")
  }, [bootstrap])

  const rows = useMemo(() => history?.rows ?? [], [history])
  const num = (r: Record<string, unknown>, k: string) => {
    const v = r[k]
    return typeof v === "number" ? v : 0
  }
  const times = useMemo(() => rows.map((r) => String(r.time ?? "")), [rows])
  const col = (k: string) => rows.map((r) => num(r, k))
  // tcp/udp CurrEstab come from ProtoCounters, which some platforms (e.g.
  // Darwin) don't provide — hide the connections chart instead of drawing 0.
  const hasConnHistory = rows.some((r) => typeof r.tcp === "number" || typeof r.udp === "number")

  const flowPie = pieOption(
    t("word-trafficstatistics"),
    [
      { name: t("word-inletflow"), value: n(d.inletFlowCount) },
      { name: t("word-exportflow"), value: n(d.exportFlowCount) },
    ],
    formatBytes,
  )
  const typePie = pieOption(t("word-type"), [
    { name: t("scheme-host"), value: n(d.hostCount) },
    { name: t("scheme-tcp"), value: n(d.tcpC) },
    { name: t("scheme-udp"), value: n(d.udpCount) },
    { name: t("scheme-httpproxy"), value: n(d.httpProxyCount) },
    { name: t("scheme-socks5"), value: n(d.socks5Count) },
    { name: t("scheme-secret"), value: n(d.secretCount) },
    { name: t("scheme-p2p"), value: n(d.p2pCount) },
  ])

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold">{t("word-dashboard")}</h1>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard icon={MonitorSmartphone} label={t("word-totalclients")} value={n(d.clientCount)} />
        <StatCard icon={Wifi} label={t("word-onlineclients")} value={n(d.clientOnlineCount)} />
        <StatCard icon={Activity} label={t("word-tcpconnections")} value={n(d.tcpCount)} />
        <StatCard icon={Clock} label={t("word-uptime")} value={s(d.upTime)} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("word-configurationinformation")}</CardTitle>
          </CardHeader>
          <CardContent>
            <InfoRow label={t("word-bridgingmode")} value={bridgeMode} />
            <InfoRow
              label={t("word-httpports")}
              value={`${s(d.httpProxyPort)} / ${s(d.httpsProxyPort)}`}
            />
            <InfoRow
              label={t("word-iprestriction")}
              value={d.ipLimit === "true" ? t("word-true") : t("word-false")}
            />
            <InfoRow label={t("word-trafficdatapersistence")} value={s(d.flowStoreInterval)} />
            <InfoRow label={t("word-loglevel")} value={s(d.logLevel)} />
            <InfoRow label={t("word-p2paddr")} value={s(d.p2pAddr)} />
            <InfoRow
              label={t("word-serverip")}
              value={`${s(d.p2pIp)} | ${s(d.serverIpv4)} | ${s(d.serverIpv6)}`}
            />
            <InfoRow label={t("word-serverversion")} value={`${s(d.version)} (${s(d.minVersion)})`} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("word-systeminformation")}</CardTitle>
          </CardHeader>
          <CardContent>
            <MeterRow label={t("word-cpu")} percent={n(d.cpu)} />
            <MeterRow label={t("word-memory")} percent={n(d.virtual_mem)} />
            <InfoRow label={t("word-load")} value={formatLoad(d.load)} />
            <InfoRow label={t("word-tcpconnections_established")} value={n(d.tcp)} />
            <InfoRow label={t("word-udpconnections_established")} value={n(d.udp)} />
            <InfoRow label={t("word-outbandwidth")} value={formatRate(n(d.io_send))} />
            <InfoRow label={t("word-inbandwidth")} value={formatRate(n(d.io_recv))} />
          </CardContent>
        </Card>
      </div>

      {rows.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-2">
          <ChartCard title={t("word-load")}>
            <EChart
              option={lineOption(times, [
                { name: "load1", data: col("load1") },
                { name: "load5", data: col("load5") },
                { name: "load15", data: col("load15") },
              ])}
            />
          </ChartCard>
          <ChartCard title={t("word-cpu")}>
            <EChart
              option={lineOption(times, [{ name: t("word-cpu"), data: col("cpu") }], (v) => `${v} %`)}
            />
          </ChartCard>
          <ChartCard title={t("word-memory")}>
            <EChart
              option={lineOption(
                times,
                [
                  { name: t("word-memory"), data: col("virtual_mem") },
                  { name: t("word-swapmemory"), data: col("swap_mem") },
                ],
                (v) => `${v} %`,
              )}
            />
          </ChartCard>
          {hasConnHistory && (
            <ChartCard title={t("word-connections_established")}>
              <EChart
                option={lineOption(times, [
                  { name: "TCP", data: col("tcp") },
                  { name: "UDP", data: col("udp") },
                ])}
              />
            </ChartCard>
          )}
          <ChartCard title={t("word-bandwidth")}>
            <EChart
              option={lineOption(
                times,
                [
                  { name: t("word-inbandwidth"), data: col("io_recv") },
                  { name: t("word-outbandwidth"), data: col("io_send") },
                ],
                (v) => formatRate(v),
              )}
            />
          </ChartCard>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <ChartCard title={t("word-trafficstatistics")}>
          <EChart option={flowPie} />
        </ChartCard>
        <ChartCard title={t("word-type")}>
          <EChart option={typePie} />
        </ChartCard>
      </div>
    </div>
  )
}
