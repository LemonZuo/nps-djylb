import { useEffect, useRef } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import type { ECharts } from "echarts"
import { useTheme } from "next-themes"
import {
  Activity,
  ArrowDownToLine,
  ArrowUpFromLine,
  Clock,
  Cpu,
  Globe,
  HardDrive,
  MemoryStick,
  MonitorSmartphone,
  Waypoints,
  Wifi,
} from "lucide-react"
import { api } from "@/api/endpoints"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { formatBytes, formatRate } from "@/lib/format"

function StatCard({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: typeof Cpu
  label: string
  value: React.ReactNode
  sub?: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
        <Icon className="size-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold">{value}</div>
        {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
      </CardContent>
    </Card>
  )
}

// The history endpoint returns tool.StatusSnapshot(): one map per sample with
// time, cpu, load1/5/15, swap_mem, virtual_mem, tcp/udp CurrEstab, io_send/recv.
function HistoryChart() {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const ref = useRef<HTMLDivElement>(null)
  const { data } = useQuery({
    queryKey: ["dashboard-history"],
    queryFn: api.dashboard.history,
    refetchInterval: 60_000,
  })

  useEffect(() => {
    if (!ref.current || !data || data.rows.length === 0) return
    // echarts is ~1 MB minified; loading it on demand keeps it out of the
    // main chunk that the login page pays for.
    let chart: ECharts | null = null
    let disposed = false
    const el = ref.current
    const rows = data.rows
    const times = rows.map((r) => String(r.time ?? ""))
    const num = (r: Record<string, unknown>, k: string) => {
      const v = r[k]
      return typeof v === "number" ? v : 0
    }
    void import("echarts").then((echarts) => {
      if (disposed) return
      chart = echarts.init(el, resolvedTheme === "dark" ? "dark" : undefined)
      chart.setOption({
      backgroundColor: "transparent",
      tooltip: { trigger: "axis" },
      legend: {
        data: ["CPU %", t("word-memory") + " %", "TCP", t("word-inbandwidth"), t("word-outbandwidth")],
      },
      grid: { left: 48, right: 48, top: 40, bottom: 24 },
      xAxis: { type: "category", data: times },
      yAxis: [
        { type: "value", max: 100, position: "left" },
        {
          type: "value",
          position: "right",
          axisLabel: { formatter: (v: number) => formatBytes(v) },
        },
      ],
      series: [
        { name: "CPU %", type: "line", smooth: true, showSymbol: false, data: rows.map((r) => num(r, "cpu")) },
        {
          name: t("word-memory") + " %",
          type: "line",
          smooth: true,
          showSymbol: false,
          data: rows.map((r) => num(r, "virtual_mem")),
        },
        { name: "TCP", type: "line", smooth: true, showSymbol: false, data: rows.map((r) => num(r, "tcp")) },
        {
          name: t("word-inbandwidth"),
          type: "line",
          smooth: true,
          showSymbol: false,
          yAxisIndex: 1,
          data: rows.map((r) => num(r, "io_recv")),
        },
        {
          name: t("word-outbandwidth"),
          type: "line",
          smooth: true,
          showSymbol: false,
          yAxisIndex: 1,
          data: rows.map((r) => num(r, "io_send")),
        },
        ],
      })
    })
    const onResize = () => chart?.resize()
    window.addEventListener("resize", onResize)
    return () => {
      disposed = true
      window.removeEventListener("resize", onResize)
      chart?.dispose()
    }
  }, [data, resolvedTheme, t])

  if (!data || data.rows.length === 0) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("ui-flow-history")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div ref={ref} className="h-80 w-full" />
      </CardContent>
    </Card>
  )
}

export default function DashboardPage() {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ["dashboard"],
    queryFn: () => api.dashboard.data(),
    refetchInterval: 5_000,
  })

  const d = data ?? {}
  const n = (v: unknown) => (typeof v === "number" ? v : 0)

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold">{t("word-dashboard")}</h1>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          icon={MonitorSmartphone}
          label={t("word-totalclients")}
          value={n(d.clientCount)}
          sub={`${t("word-onlineclients")}: ${n(d.clientOnlineCount)}`}
        />
        <StatCard
          icon={Waypoints}
          label={t("word-tunnel")}
          value={
            n(d.tcpC) + n(d.udpCount) + n(d.socks5Count) + n(d.httpProxyCount) + n(d.secretCount) + n(d.p2pCount)
          }
          sub={`TCP ${n(d.tcpC)} · UDP ${n(d.udpCount)} · S5 ${n(d.socks5Count)} · HTTP ${n(d.httpProxyCount)} · ${t("scheme-secret")} ${n(d.secretCount)} · P2P ${n(d.p2pCount)}`}
        />
        <StatCard icon={Globe} label={t("page-hostlist")} value={n(d.hostCount)} />
        <StatCard
          icon={Activity}
          label={t("word-curconnections")}
          value={n(d.tcpCount)}
          sub={`${t("word-tcpconnections_established")}: ${n(d.tcp)}`}
        />
        <StatCard icon={Cpu} label={t("word-cpu")} value={`${n(d.cpu)}%`} sub={`${t("word-load")}: ${String(d.load ?? "-")}`} />
        <StatCard
          icon={MemoryStick}
          label={t("word-memory")}
          value={`${n(d.virtual_mem)}%`}
          sub={`${t("word-swapmemory")}: ${n(d.swap_mem)}%`}
        />
        <StatCard
          icon={Wifi}
          label={t("word-bandwidth")}
          value={
            <span className="flex items-center gap-2 text-lg">
              <ArrowDownToLine className="size-4" />
              {formatRate(n(d.io_recv))}
              <ArrowUpFromLine className="size-4" />
              {formatRate(n(d.io_send))}
            </span>
          }
        />
        <StatCard
          icon={HardDrive}
          label={t("word-trafficstatistics")}
          value={
            <span className="flex items-center gap-2 text-lg">
              <ArrowDownToLine className="size-4" />
              {formatBytes(n(d.inletFlowCount))}
              <ArrowUpFromLine className="size-4" />
              {formatBytes(n(d.exportFlowCount))}
            </span>
          }
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-1">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Clock className="size-4" />
              {t("word-systeminformation")}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <InfoRow label={t("word-serverversion")} value={String(d.version ?? "-")} />
            <InfoRow label={t("word-minsupportversion")} value={String(d.minVersion ?? "-")} />
            <InfoRow label={t("word-uptime")} value={String(d.upTime ?? "-")} />
            <InfoRow label={t("word-type")} value={String(d.bridgeType ?? "-")} />
            <InfoRow label={t("word-httpport")} value={String(d.httpProxyPort ?? "-")} />
            <InfoRow label={t("word-httpsport")} value={String(d.httpsProxyPort ?? "-")} />
            <InfoRow label={t("word-p2paddr")} value={String(d.p2pAddr ?? "-")} />
            <InfoRow label={t("word-serveripv4")} value={String(d.serverIpv4 ?? "-")} />
            <InfoRow label={t("word-loglevel")} value={String(d.logLevel ?? "-")} />
          </CardContent>
        </Card>

        <div className="lg:col-span-2">
          <HistoryChart />
        </div>
      </div>
    </div>
  )
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate font-mono text-xs">{value}</span>
    </div>
  )
}
