import { useTranslation } from "react-i18next"
import { useTheme } from "next-themes"
import { Languages, Moon, Sun } from "lucide-react"
import { LANGUAGES, setLanguage } from "@/i18n"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

// Shared footer for the unauthenticated pages, mirroring the old template's
// copyright line and upstream link.
export function LoginFooter() {
  const { t } = useTranslation()
  return (
    <div className="mt-6 flex w-full max-w-sm items-center justify-between text-xs text-muted-foreground">
      <span>
        {t("word-copyright")} NPS &copy; 2018-{new Date().getFullYear()}
      </span>
      <span>
        {t("word-readmore")}{" "}
        <a
          href="https://github.com/djylb/nps"
          target="_blank"
          rel="noreferrer"
          className="text-primary hover:underline"
        >
          {t("word-go")}
        </a>
      </span>
    </div>
  )
}

// Shared corner controls for the unauthenticated pages.
export function LanguageThemeBar() {
  const { t, i18n } = useTranslation()
  const { theme, setTheme } = useTheme()
  const isDark = theme === "dark"

  return (
    <div className="absolute top-4 right-4 flex gap-1">
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
    </div>
  )
}
