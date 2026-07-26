import { useState } from "react"
import { NavLink, Outlet } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { useTheme } from "next-themes"
import { useQuery } from "@tanstack/react-query"
import {
  Gauge,
  Globe,
  Languages,
  LogOut,
  Menu,
  MonitorSmartphone,
  Moon,
  Settings2,
  Sun,
  Waypoints,
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
}

const NAV_ITEMS: NavItem[] = [
  { to: "/dashboard", labelKey: "word-dashboard", icon: Gauge },
  { to: "/clients", labelKey: "page-clientlist", icon: MonitorSmartphone },
  { to: "/tunnels", labelKey: "word-tunnel", icon: Waypoints },
  { to: "/hosts", labelKey: "page-hostlist", icon: Globe },
  { to: "/global", labelKey: "word-globalparam", icon: Settings2, adminOnly: true },
]

function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { user } = useAuth()

  return (
    <nav className="flex flex-col gap-1 p-2">
      {NAV_ITEMS.filter((item) => !item.adminOnly || user?.isAdmin).map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          onClick={onNavigate}
          className={({ isActive }) =>
            cn(
              "flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors",
              isActive
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )
          }
        >
          <item.icon className="size-4" />
          {t(item.labelKey)}
        </NavLink>
      ))}
    </nav>
  )
}

function SidebarHeader() {
  const { data: bootstrap } = useQuery({ queryKey: ["bootstrap"], queryFn: api.meta.bootstrap })
  return (
    <div className="flex h-14 items-center gap-2 border-b px-4">
      <img src="./favicon.svg" alt="NPS" className="size-6" />
      <span className="text-lg font-semibold">NPS</span>
      {bootstrap && <span className="text-xs text-muted-foreground">v{bootstrap.version}</span>}
    </div>
  )
}

export default function AppLayout() {
  const { t, i18n } = useTranslation()
  const { theme, setTheme } = useTheme()
  const { user, logout } = useAuth()
  const [mobileOpen, setMobileOpen] = useState(false)

  const isDark = theme === "dark"

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 shrink-0 flex-col border-r bg-background md:flex">
        <SidebarHeader />
        <NavLinks />
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
