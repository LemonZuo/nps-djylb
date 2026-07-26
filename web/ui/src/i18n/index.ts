import i18n from "i18next"
import { initReactI18next } from "react-i18next"
import zhCN from "./zh-CN.json"
import enUS from "./en-US.json"

// The keys are carried over verbatim from the old web/static/page/languages.xml
// so both language files stay diffable against the Beego-era catalogue.

const STORAGE_KEY = "nps-lang"

export const LANGUAGES = [
  { code: "zh-CN", label: "简体中文" },
  { code: "en-US", label: "English" },
] as const

function initialLanguage(): string {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved && LANGUAGES.some((l) => l.code === saved)) return saved
  return navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US"
}

i18n.use(initReactI18next).init({
  resources: {
    "zh-CN": { translation: zhCN },
    "en-US": { translation: enUS },
  },
  lng: initialLanguage(),
  fallbackLng: "en-US",
  interpolation: { escapeValue: false },
  // Keys contain dots ("info-...") nowhere, but they do contain dashes; keep
  // flat lookup so "word-save" is a key, not a nested path.
  keySeparator: false,
  nsSeparator: false,
})

export function setLanguage(code: string) {
  localStorage.setItem(STORAGE_KEY, code)
  i18n.changeLanguage(code)
  document.documentElement.lang = code
}

document.documentElement.lang = i18n.language

export default i18n
