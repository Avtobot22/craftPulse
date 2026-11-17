import { useEffect, useRef, useSyncExternalStore } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { safeReturnTo } from '@/shared/auth'
import { routePaths } from '@/shared/config'
import type { AppRuntime } from '../initialization/runtime'

export function expiredPath(path: string): string {
  return `${routePaths.sessionExpired}?${new URLSearchParams({ return_to: safeReturnTo(path) }).toString()}`
}

export function SessionNavigation({ runtime }: { runtime: AppRuntime }) {
  const state = useSyncExternalStore(
    runtime.session.subscribe,
    runtime.session.getSnapshot,
  )
  const handled = useRef(state)
  const location = useLocation()
  const navigate = useNavigate()
  const path = location.pathname + location.search + location.hash
  useEffect(() => {
    if (handled.current === state) return
    handled.current = state
    if (state === 'expired') void navigate(expiredPath(path), { replace: true })
    else if (state === 'signed-out')
      void navigate(routePaths.login, { replace: true })
  }, [state, navigate, path])
  return <Outlet />
}
