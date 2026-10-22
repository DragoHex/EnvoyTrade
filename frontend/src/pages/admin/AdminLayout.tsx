import { type JSX } from 'solid-js'
import { A } from '@solidjs/router'
import './admin.css'

export function AdminLayout(props: { activeTab: 'status' | 'operations' | 'ips' | 'users'; children: JSX.Element }) {
  return (
    <div class="admin-container">
      <div class="admin-header-tabs">
        <div class="admin-title-row">
          <div>
            <h1 class="admin-title">Admin Operations Portal</h1>
          </div>
        </div>
        <div class="admin-tabs-nav">
          <A href="/admin/status" classList={{ active: props.activeTab === 'status' }}>
            System Status & Kill Switch
          </A>
          <A href="/admin/operations" classList={{ active: props.activeTab === 'operations' }}>
            Trade Operations
          </A>
          <A href="/admin/ips" classList={{ active: props.activeTab === 'ips' }}>
            Proxy IP Pool
          </A>
          <A href="/admin/users" classList={{ active: props.activeTab === 'users' }}>
            Platform Users
          </A>
        </div>
      </div>
      <div class="admin-content">{props.children}</div>
    </div>
  )
}
