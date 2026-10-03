import {
  createContext,
  createSignal,
  useContext,
  onMount,
  onCleanup,
  type JSX,
  type Accessor,
} from 'solid-js'
import * as api from '../api'

export interface AuthContextValue {
  user: Accessor<api.User | null>
  isLoading: Accessor<boolean>
  isAuthenticated: Accessor<boolean>
  login: (req: api.LoginRequest) => Promise<void>
  register: (req: api.RegisterRequest) => Promise<void>
  logout: () => Promise<void>
  refetchUser: () => Promise<void>
  updateProfile: (req: api.UpdateProfileRequest) => Promise<api.User>
  updatePassword: (req: api.UpdatePasswordRequest) => Promise<void>
}

const AuthContext = createContext<AuthContextValue>()

export function AuthProvider(props: { children: JSX.Element }) {
  const [user, setUser] = createSignal<api.User | null>(null)
  const [isLoading, setIsLoading] = createSignal(true)

  const isAuthenticated = () => user() !== null

  const checkAuth = async () => {
    try {
      const res = await api.getMe()
      setUser(res.user)
    } catch {
      setUser(null)
    } finally {
      setIsLoading(false)
    }
  }

  onMount(() => {
    // Automatically reset user on 401 anywhere in the app
    api.setOnUnauthorized(() => {
      setUser(null)
    })
    checkAuth()
  })

  onCleanup(() => {
    api.setOnUnauthorized(null)
  })

  const login = async (req: api.LoginRequest) => {
    const res = await api.login(req)
    setUser(res.user)
  }

  const register = async (req: api.RegisterRequest) => {
    const res = await api.register(req)
    setUser(res.user)
  }

  const logout = async () => {
    try {
      await api.logout()
    } finally {
      setUser(null)
    }
  }

  const updateProfile = async (req: api.UpdateProfileRequest) => {
    const res = await api.updateProfile(req)
    setUser(res.user)
    return res.user
  }

  const updatePassword = async (req: api.UpdatePasswordRequest) => {
    await api.updatePassword(req)
  }

  const value: AuthContextValue = {
    user,
    isLoading,
    isAuthenticated,
    login,
    register,
    logout,
    refetchUser: checkAuth,
    updateProfile,
    updatePassword,
  }

  return (
    <AuthContext.Provider value={value}>
      {props.children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}
