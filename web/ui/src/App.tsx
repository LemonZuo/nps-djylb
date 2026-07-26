import { Navigate, Route, Routes } from "react-router-dom"
import { useAuth } from "@/auth/AuthContext"
import LoginPage from "@/pages/login/LoginPage"
import RegisterPage from "@/pages/login/RegisterPage"
import AppLayout from "@/layout/AppLayout"
import DashboardPage from "@/pages/dashboard/DashboardPage"
import ClientsPage from "@/pages/clients/ClientsPage"
import ClientFormPage from "@/pages/clients/ClientFormPage"
import TunnelsPage from "@/pages/tunnels/TunnelsPage"
import TunnelFormPage from "@/pages/tunnels/TunnelFormPage"
import HostsPage from "@/pages/hosts/HostsPage"
import HostFormPage from "@/pages/hosts/HostFormPage"
import GlobalPage from "@/pages/global/GlobalPage"

export default function App() {
  const { user } = useAuth()

  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/clients" element={<ClientsPage />} />
        <Route path="/clients/new" element={<ClientFormPage />} />
        <Route path="/clients/:id/edit" element={<ClientFormPage />} />
        <Route path="/tunnels" element={<TunnelsPage />} />
        <Route path="/tunnels/new" element={<TunnelFormPage />} />
        <Route path="/tunnels/:id/edit" element={<TunnelFormPage />} />
        <Route path="/hosts" element={<HostsPage />} />
        <Route path="/hosts/new" element={<HostFormPage />} />
        <Route path="/hosts/:id/edit" element={<HostFormPage />} />
        <Route path="/global" element={<GlobalPage />} />
        <Route path="*" element={<Navigate to="/dashboard" replace />} />
      </Route>
    </Routes>
  )
}
