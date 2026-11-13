import { AppHeader } from "./_components/app-header"
import { AuthGate } from "./_components/auth-gate"

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGate>
      <AppHeader />
      {children}
    </AuthGate>
  )
}
