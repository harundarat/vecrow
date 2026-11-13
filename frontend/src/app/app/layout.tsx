import { AppHeader } from "./_components/app-header"
import { AuthGate } from "./_components/auth-gate"
import { RecoveryBanner } from "./_components/recovery-banner"

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGate>
      <AppHeader />
      <RecoveryBanner />
      {children}
    </AuthGate>
  )
}
