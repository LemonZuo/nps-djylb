import { useCallback, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

// useConfirm returns an imperative confirm(message) that resolves to the
// user's choice, plus the dialog element to mount once per page. It replaces
// the old UI's window-level confirm boxes without threading open-state through
// every action button.
export function useConfirm(): {
  confirm: (message: string) => Promise<boolean>
  dialog: React.ReactNode
} {
  const { t } = useTranslation()
  const [message, setMessage] = useState<string | null>(null)
  const resolver = useRef<((ok: boolean) => void) | null>(null)

  const confirm = useCallback((msg: string) => {
    setMessage(msg)
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve
    })
  }, [])

  const close = (ok: boolean) => {
    resolver.current?.(ok)
    resolver.current = null
    setMessage(null)
  }

  const dialog = (
    <Dialog open={message !== null} onOpenChange={(open) => !open && close(false)}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("ui-confirm-title")}</DialogTitle>
          <DialogDescription>{message}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => close(false)}>
            {t("word-cancel")}
          </Button>
          <Button onClick={() => close(true)}>{t("word-alert-confirm")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  return { confirm, dialog }
}
