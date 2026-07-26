import { useState } from "react"
import { Link, Outlet, useLocation, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useTheme } from "next-themes"
import { useQuery } from "@tanstack/react-query"
import {
  ArrowLeftRight,
  Ban,
  EyeOff,
  FolderOpen,
  Gauge,
  Globe,
  Languages,
  Layers,
  Lightbulb,
  LogOut,
  Menu,
  MonitorSmartphone,
  Moon,
  Repeat,
  Settings2,
  Shuffle,
  Sun,
} from "lucide-react"
import { api } from "@/api/endpoints"
import { useAuth } from "@/auth/AuthContext"
import { setLanguage, LANGUAGES } from "@/i18n"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet"
import { cn } from "@/lib/utils"

interface NavItem {
  to: string
  labelKey: string
  icon: typeof Gauge
  adminOnly?: boolean
  // Tunnel entries share the /tunnels route and differ only in ?type=,
  // so active state is matched on the query value, not the pathname.
  type?: string
  external?: boolean
}

// Mirrors the old Beego layout.html menu: each tunnel mode is its own entry.
const NAV_ITEMS: NavItem[] = [
  { to: "/dashboard", labelKey: "word-dashboard", icon: Gauge },
  { to: "/clients", labelKey: "word-client", icon: MonitorSmartphone },
  { to: "/hosts", labelKey: "scheme-host", icon: Globe },
  { to: "/tunnels?type=tcp", labelKey: "scheme-tcp", icon: Repeat, type: "tcp" },
  { to: "/tunnels?type=udp", labelKey: "scheme-udp", icon: Shuffle, type: "udp" },
  { to: "/tunnels?type=mixProxy", labelKey: "scheme-mixproxy", icon: Layers, type: "mixProxy" },
  { to: "/tunnels?type=secret", labelKey: "scheme-secret", icon: EyeOff, type: "secret" },
  { to: "/tunnels?type=p2p", labelKey: "scheme-p2p", icon: ArrowLeftRight, type: "p2p" },
  { to: "/tunnels?type=file", labelKey: "scheme-file", icon: FolderOpen, type: "file" },
  { to: "/global", labelKey: "word-globalparam", icon: Settings2, adminOnly: true },
  { to: "/bans", labelKey: "word-banlist", icon: Ban, adminOnly: true },
  { to: "https://d-jy.net/docs/nps/", labelKey: "word-help", icon: Lightbulb, external: true },
]

function NavLinks({ collapsed, onNavigate }: { collapsed?: boolean; onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const currentType = searchParams.get("type") ?? ""

  const isActive = (item: NavItem) => {
    if (item.external) return false
    const path = item.to.split("?")[0]
    if (location.pathname !== path && !location.pathname.startsWith(path + "/")) return false
    if (item.type !== undefined) return currentType === item.type
    return true
  }

  return (
    <nav className="flex flex-col gap-1 p-2">
      {NAV_ITEMS.filter((item) => !item.adminOnly || user?.isAdmin).map((item) => {
        const className = cn(
          "flex items-center gap-3 rounded-lg py-2 text-sm transition-colors",
          collapsed ? "justify-center px-0" : "px-3",
          isActive(item)
            ? "bg-primary text-primary-foreground"
            : "text-muted-foreground hover:bg-muted hover:text-foreground",
        )
        const title = collapsed ? t(item.labelKey) : undefined
        if (item.external) {
          return (
            <a
              key={item.to}
              href={item.to}
              target="_blank"
              rel="noreferrer"
              className={className}
              title={title}
            >
              <item.icon className="size-4 shrink-0" />
              {!collapsed && t(item.labelKey)}
            </a>
          )
        }
        return (
          <Link key={item.to} to={item.to} onClick={onNavigate} className={className} title={title}>
            <item.icon className="size-4 shrink-0" />
            {!collapsed && t(item.labelKey)}
          </Link>
        )
      })}
    </nav>
  )
}

function SidebarHeader({ collapsed }: { collapsed?: boolean }) {
  const { data: bootstrap } = useQuery({ queryKey: ["bootstrap"], queryFn: api.meta.bootstrap })
  return (
    <div
      className={cn(
        "flex h-14 items-center gap-2 border-b",
        collapsed ? "justify-center px-0" : "px-4",
      )}
    >
      <img src="./favicon.svg" alt="NPS" className="size-6" />
      {!collapsed && (
        <>
          <span className="text-lg font-semibold">NPS</span>
          {bootstrap && <span className="text-xs text-muted-foreground">v{bootstrap.version}</span>}
        </>
      )}
    </div>
  )
}

export default function AppLayout() {
  const { t, i18n } = useTranslation()
  const { theme, setTheme } = useTheme()
  const { user, logout } = useAuth()
  const [mobileOpen, setMobileOpen] = useState(false)
  // Desktop sidebar collapse, like the old Inspinia navbar-minimalize button.
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem("sidebar-collapsed") === "1",
  )
  const toggleCollapsed = () => {
    setCollapsed((c) => {
      localStorage.setItem("sidebar-collapsed", c ? "0" : "1")
      return !c
    })
  }

  const isDark = theme === "dark"

  return (
    <div className="flex min-h-screen">
      <aside
        className={cn(
          "hidden shrink-0 flex-col border-r bg-background transition-[width] duration-200 md:flex",
          collapsed ? "w-14" : "w-44",
        )}
      >
        <SidebarHeader collapsed={collapsed} />
        <NavLinks collapsed={collapsed} />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center gap-2 border-b px-4">
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon" className="md:hidden">
                <Menu className="size-5" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-64 p-0">
              <SheetTitle className="sr-only">NPS</SheetTitle>
              <SidebarHeader />
              <NavLinks onNavigate={() => setMobileOpen(false)} />
            </SheetContent>
          </Sheet>

          <Button
            variant="ghost"
            size="icon"
            className="hidden md:inline-flex"
            onClick={toggleCollapsed}
          >
            <Menu className="size-5" />
          </Button>

          <div className="flex-1" />

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" title={t("ui-language")}>
                <Languages className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {LANGUAGES.map((lang) => (
                <DropdownMenuItem
                  key={lang.code}
                  onClick={() => setLanguage(lang.code)}
                  className={cn(i18n.language === lang.code && "font-semibold")}
                >
                  {lang.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          <Button
            variant="ghost"
            size="icon"
            title={isDark ? t("ui-light") : t("ui-dark")}
            onClick={() => setTheme(isDark ? "light" : "dark")}
          >
            {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm">
                {user?.username || t("word-user")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>
                {user?.isAdmin ? t("word-admin") : t("word-user")}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => void logout()}>
                <LogOut className="size-4" />
                {t("word-logout")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>

        <main className="flex-1 overflow-x-auto p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
