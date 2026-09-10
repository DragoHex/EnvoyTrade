import type { JSX } from 'solid-js'
import { Router, Route, A } from '@solidjs/router'
import { ThemeProvider, useTheme } from './theme/ThemeProvider'
import { ThemeToggle } from './components/ThemeToggle'
import { Dashboard } from './pages/Dashboard'
import { AccountsPage } from './pages/AccountsPage'
import { GroupManagePage } from './pages/GroupManagePage'
import { AnalyticsPage } from './pages/AnalyticsPage'
import './theme.css'

function TopNav(props: { children?: JSX.Element }) {
  const { theme, toggleTheme } = useTheme()
  return (
    <>
      <nav>
        <span>EnvoyTrade</span>
        <A href="/" end>Dashboard</A>
        <A href="/accounts">Accounts</A>
        <A href="/analytics">Analytics</A>
        <ThemeToggle dark={theme() === 'dark'} onToggle={toggleTheme} />
      </nav>
      {props.children}
    </>
  )
}

function App() {
  return (
    <ThemeProvider>
      <Router root={TopNav}>
        <Route path="/" component={Dashboard} />
        <Route path="/accounts" component={AccountsPage} />
        <Route path="/accounts/groups/:masterId" component={GroupManagePage} />
        <Route path="/analytics" component={AnalyticsPage} />
      </Router>
    </ThemeProvider>
  )
}

export default App
