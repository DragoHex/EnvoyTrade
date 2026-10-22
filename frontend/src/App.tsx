import { createSignal, onMount, onCleanup, createEffect, Show, type JSX } from 'solid-js'
import { Router, Route, A, useNavigate, useCurrentMatches, useLocation } from '@solidjs/router'
import { ThemeProvider, useTheme } from './theme/ThemeProvider'
import { ThemeToggle } from './components/ThemeToggle'
import { AuthProvider, useAuth } from './context/AuthContext'
import { ProtectedRoute } from './components/ProtectedRoute'
import { Dashboard } from './pages/Dashboard'
import { AccountsPage } from './pages/AccountsPage'
import { GroupManagePage } from './pages/GroupManagePage'
import { AnalyticsPage } from './pages/AnalyticsPage'
import { SettingsPage } from './pages/SettingsPage'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { AdminStatusPage } from './pages/admin/AdminStatusPage'
import { AdminOperationsPage } from './pages/admin/AdminOperationsPage'
import { AdminIPsPage } from './pages/admin/AdminIPsPage'
import { AdminUsersPage } from './pages/admin/AdminUsersPage'
import { AdminUserAccountsPage } from './pages/admin/AdminUserAccountsPage'
import { getKillSwitchStatus, type KillSwitchStatus } from './api'
import { LogoutIcon } from './components/icons'
import { EnvoyLogo } from './components/EnvoyLogo'
import './theme.css'
import './order-details.css'

function TopNav(props: { children?: JSX.Element }) {
  const { theme, toggleTheme } = useTheme()
  const { user, isAuthenticated, logout } = useAuth()
  const navigate = useNavigate()
  const matches = useCurrentMatches()
  const location = useLocation()

  const [killSwitch, setKillSwitch] = createSignal<KillSwitchStatus | null>(null)
  const isAdminRoute = () => location.pathname.startsWith('/admin')

  const checkKillSwitch = async () => {
    if (!isAuthenticated()) {
      setKillSwitch(null)
      return
    }
    try {
      const status = await getKillSwitchStatus()
      setKillSwitch(status)
    } catch {
      // Ignore network errors
    }
  }

  let intervalId: ReturnType<typeof setInterval> | null = null

  const startPolling = () => {
    if (isAdminRoute()) return
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
    if (intervalId === null) {
      intervalId = setInterval(() => {
        if (!isAdminRoute() && typeof document !== 'undefined' && document.visibilityState === 'visible') {
          checkKillSwitch()
        }
      }, 30_000)
    }
  }

  const stopPolling = () => {
    if (intervalId !== null) {
      clearInterval(intervalId)
      intervalId = null
    }
  }

  onMount(() => {
    checkKillSwitch()

    // Listen for instant kill switch toggle events from AdminStatusPage
    const onKillSwitchChange = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (detail) {
        setKillSwitch(detail)
      }
    }
    if (typeof window !== 'undefined') {
      window.addEventListener('killswitch:change', onKillSwitchChange)
    }

    startPolling()

    const onVisibilityChange = () => {
      if (typeof document !== 'undefined') {
        if (document.visibilityState === 'visible') {
          if (!isAdminRoute()) {
            checkKillSwitch()
            startPolling()
          }
        } else {
          stopPolling()
        }
      }
    }

    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', onVisibilityChange)
    }

    onCleanup(() => {
      stopPolling()
      if (typeof window !== 'undefined') {
        window.removeEventListener('killswitch:change', onKillSwitchChange)
      }
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', onVisibilityChange)
      }
    })
  })

  createEffect(() => {
    if (isAdminRoute()) {
      stopPolling()
    } else if (isAuthenticated() && typeof document !== 'undefined' && document.visibilityState === 'visible') {
      startPolling()
    } else {
      stopPolling()
    }
  })

  createEffect(() => {
    if (isAuthenticated()) {
      checkKillSwitch()
    } else {
      setKillSwitch(null)
    }
  })

  const isNotFound = () =>
    matches().some(
      (m) =>
        m.route.originalPath === '*all' ||
        m.route.pattern?.includes('*') ||
        (m.route as any).path === '*all',
    )

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <>
      <Show when={!isNotFound()}>
        <nav class="app-nav">
          <div class="nav-brand-bar">
            <span class="brand-title">
              <EnvoyLogo size={24} />
              <span>EnvoyTrade</span>
            </span>
          </div>
          <Show when={isAuthenticated()}>
            <div class="nav-links">
              <A href="/" end>Dashboard</A>
              <A href="/accounts">Accounts</A>
              <A href="/analytics">Analytics</A>
              <Show when={user()?.role === 'admin'}>
                <A href="/admin/status" classList={{ active: isAdminRoute() }}>Admin</A>
              </Show>
            </div>
          </Show>
          <div class="nav-actions">
            <Show when={isAuthenticated()}>
              <A
                href="/settings"
                class="user-badge-link"
                data-testid="user-badge"
                aria-label="User Settings"
              >
                {user()?.name || user()?.username || user()?.email}
              </A>
              <button
                type="button"
                onClick={handleLogout}
                data-tooltip="Sign Out"
                data-tooltip-pos="bottom"
                aria-label="Sign Out"
                class="icon-button"
                style={{ color: '#fff' }}
              >
                <LogoutIcon />
              </button>
            </Show>
            <ThemeToggle dark={theme() === 'dark'} onToggle={toggleTheme} />
          </div>
        </nav>
      </Show>
      <Show when={isAuthenticated() && killSwitch()?.engaged}>
        <div class="killswitch-global-banner" role="alert" data-testid="killswitch-banner">
          <div class="killswitch-banner-content">
            <span class="killswitch-pulse" />
            <span>
              <strong>EMERGENCY KILL SWITCH ENGAGED:</strong> Copy-trade engine is currently halted. Follower order placements are paused.
              <Show when={killSwitch()?.reason}>
                {' '}— Reason: <em>{killSwitch()!.reason}</em>
              </Show>
            </span>
          </div>
        </div>
      </Show>
      {props.children}
    </>
  )
}

function Protected(Component: () => JSX.Element) {
  return () => (
    <ProtectedRoute>
      <Component />
    </ProtectedRoute>
  )
}

function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <Router root={TopNav}>
          <Route path="/login" component={LoginPage} />
          <Route path="/" component={Protected(Dashboard)} />
          <Route path="/accounts" component={Protected(AccountsPage)} />
          <Route path="/accounts/groups/:masterId" component={Protected(GroupManagePage)} />
          <Route path="/analytics" component={Protected(AnalyticsPage)} />
          <Route path="/settings" component={Protected(SettingsPage)} />
          <Route path="/admin" component={Protected(AdminStatusPage)} />
          <Route path="/admin/status" component={Protected(AdminStatusPage)} />
          <Route path="/admin/operations" component={Protected(AdminOperationsPage)} />
          <Route path="/admin/ips" component={Protected(AdminIPsPage)} />
          <Route path="/admin/users" component={Protected(AdminUsersPage)} />
          <Route path="/admin/users/:id/accounts" component={Protected(AdminUserAccountsPage)} />
          <Route path="*all" component={NotFoundPage} />
        </Router>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
