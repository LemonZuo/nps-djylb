import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { ThemeProvider } from "next-themes"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { AuthProvider } from "@/auth/AuthContext"
import App from "./App"
import "./i18n"
import "./index.css"

// The server rewrites the <base> element when web_base_url mounts the SPA
// under a sub-path; the router has to agree with it.
function routerBasename(): string {
  const href = document.querySelector("base")?.getAttribute("href") ?? "/"
  const path = new URL(href, window.location.origin).pathname
  return path.replace(/\/$/, "")
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, refetchOnWindowFocus: false },
  },
})

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
        <AuthProvider>
          <TooltipProvider delayDuration={500}>
            <BrowserRouter basename={routerBasename()}>
              <App />
            </BrowserRouter>
          </TooltipProvider>
        </AuthProvider>
        <Toaster position="top-center" />
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)
