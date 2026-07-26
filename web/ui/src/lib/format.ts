import { toast } from "sonner"
import i18n from "@/i18n"

// formatBytes renders a byte count the way the old UI did: binary units, one
// decimal, starting at B.
export function formatBytes(n: number | undefined | null): string {
  const v = typeof n === "number" && Number.isFinite(n) ? n : 0
  if (v < 1024) return `${v} B`
  const units = ["KB", "MB", "GB", "TB", "PB"]
  let value = v
  let i = -1
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i++
  }
  return `${value.toFixed(1)} ${units[i]}`
}

// formatRate is formatBytes per second, for bandwidth figures.
export function formatRate(n: number | undefined | null): string {
  return `${formatBytes(n)}/s`
}

// copyText mirrors the old UI's helper: Clipboard API when the context allows
// it, an off-screen textarea + execCommand otherwise (plain-HTTP deployments
// are common for this tool).
export async function copyText(text: string): Promise<void> {
  const ok = await writeClipboard(text)
  if (ok) {
    toast.success(i18n.t("copied"))
  } else {
    toast.error(i18n.t("copied"))
  }
}

async function writeClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // fall through to the legacy path
    }
  }
  const textarea = document.createElement("textarea")
  textarea.value = text
  textarea.setAttribute("readonly", "")
  textarea.style.cssText =
    "position:fixed;left:0;top:0;width:1px;height:1px;opacity:0;pointer-events:none;"
  document.body.appendChild(textarea)
  const prevFocus = document.activeElement as HTMLElement | null
  textarea.focus()
  textarea.select()
  let ok = false
  try {
    ok = document.execCommand("copy")
  } catch {
    ok = false
  }
  document.body.removeChild(textarea)
  prevFocus?.focus?.()
  return ok
}

// generateRandomPassword matches the old UI's charset and default length, but
// draws from the CSPRNG instead of Math.random.
export function generateRandomPassword(length = 32): string {
  const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  let out = ""
  for (const b of bytes) out += charset[b % charset.length]
  return out
}
