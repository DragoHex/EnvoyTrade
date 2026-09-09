import type { JSX } from 'solid-js'
import { Router, Route, A } from '@solidjs/router'
import { ThemeProvider, useTheme } from './theme/ThemeProvider'
import { ThemeToggle } from './components/ThemeToggle'
import { Dashboard } from './pages/Dashboard'
import { AccountsPage } from './pages/AccountsPage'
import { GroupManagePage } from './pages/GroupManagePage'
import './theme.css'

function TopNav(props: { children?: JSX.Element }) {
  const { theme, toggleTheme } = useTheme()
  return (
    <>
      <nav>
        <span>EnvoyTrade</span>
        <A href="/">Dashboard</A>
        <A href="/accounts">Accounts</A>
        <A href="/analytics">Analytics</A>
        <ThemeToggle dark={theme() === 'dark'} onToggle={toggleTheme} />
      </nav>
      {props.children}
    </>
  )
}

function Placeholder(props: { name: string }) {
  return <p>{props.name} — not built in this pass.</p>
}

function App() {
  return (
    <ThemeProvider>
      <Router root={TopNav}>
        <Route path="/" component={Dashboard} />
        <Route path="/accounts" component={AccountsPage} />
        <Route path="/accounts/groups/:masterId" component={GroupManagePage} />
        <Route path="/analytics" component={() => <Placeholder name="Analytics" />} />
      </Router>
    </ThemeProvider>
  )
}

export default App
