import { useEffect, useRef, useSyncExternalStore } from 'react'
import { matchPath, Outlet, useLocation, useNavigate } from 'react-router'
import type { Location } from 'react-router'
import { ToasterComponent, useToaster } from '@gravity-ui/uikit'
import {
  createLoginPageUrl,
  isOrganizationSlug,
  safeReturnTo,
} from '@/shared/auth'
import { routePaths } from '@/shared/config'
import type { AppRuntime } from '../initialization/runtime'

export function sessionEndPath(
  state: 'expired' | 'signed-out',
  { pathname, search, hash }: Location,
): string {
  const path = pathname + search + hash
  if (matchPath(routePaths.mergeRequestSummary, pathname)) {
    const orgSlug = new URLSearchParams(search).get('org_slug')
    if (isOrganizationSlug(orgSlug)) return createLoginPageUrl(orgSlug, path)
  }
  if (state === 'signed-out') return routePaths.login
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
  const toaster = useToaster()
  const isPublicAuthRoute = [
    routePaths.login,
    routePaths.login + '/:orgSlug',
    '/o/:orgSlug',
    routePaths.sessionExpired,
  ].some((pattern) => matchPath(pattern, location.pathname) !== null)
  useEffect(() => {
    if (handled.current === state) return
    handled.current = state
    if (isPublicAuthRoute) return
    if (state === 'expired' || state === 'signed-out')
      void navigate(sessionEndPath(state, location), { replace: true })
  }, [state, navigate, location, isPublicAuthRoute])
  useEffect(() => {
    if (isPublicAuthRoute) toaster.removeAll()
  }, [isPublicAuthRoute, toaster])
  return (
    <>
      <Outlet />
      {!isPublicAuthRoute && <ToasterComponent />}
    </>
  )
}
