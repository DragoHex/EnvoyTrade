import { Show, type JSX } from 'solid-js'
import { Router, Route, A, useNavigate, useCurrentMatches } from '@solidjs/router'
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
import { LogoutIcon } from './components/icons'
import { EnvoyLogo } from './components/EnvoyLogo'
import './theme.css'
import './order-details.css'

function TopNav(props: { children?: JSX.Element }) {
  const { theme, toggleTheme } = useTheme()
  const { user, isAuthenticated, logout } = useAuth()
  const navigate = useNavigate()
  const matches = useCurrentMatches()

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
          <Route path="*all" component={NotFoundPage} />
        </Router>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
