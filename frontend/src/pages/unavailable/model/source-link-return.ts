import type { Location } from 'react-router'
import { safeReturnTo } from '@/shared/auth'
import { routePaths } from '@/shared/config'

const resultParam = 'source_link_result'

export function readSourceLinkResult(search: string) {
  const value = new URLSearchParams(search).get(resultParam)
  return value === 'linked' || value === 'link_failed' ? value : undefined
}

function safeSourceLinkPath(value: unknown): string {
  if (
    typeof value !== 'string' ||
    value.length > 2048 ||
    value.includes('\\') ||
    Array.from(value).some((character) => character.charCodeAt(0) <= 32)
  )
    return routePaths.home
  const [pathname] = value.split(/[?#]/u, 1)
  // Check origin safety on the pathname. Encoded spaces in filters and
  // fragments are valid and should survive the round trip.
  return pathname && safeReturnTo(pathname) === pathname
    ? value
    : routePaths.home
}

export function sourceLinkReturnPath({ pathname, search, hash }: Location) {
  const params = new URLSearchParams(search)
  if (!params.has(resultParam))
    return safeSourceLinkPath(pathname + search + hash)
  params.delete(resultParam)
  const query = params.toString()
  return safeSourceLinkPath(pathname + (query ? '?' + query : '') + hash)
}

function retryKey(identity: string) {
  return 'craftpulse.source-link.retry.' + identity
}

export function rememberSourceLinkRetry(identity: string, deniedFrom: unknown) {
  const path = safeSourceLinkPath(deniedFrom)
  try {
    if (path === routePaths.home) sessionStorage.removeItem(retryKey(identity))
    else sessionStorage.setItem(retryKey(identity), path)
  } catch {
    // Storage can be disabled. UI-04 still offers a safe exit.
  }
}

export function takeSourceLinkRetry(identity: string) {
  try {
    const path = sessionStorage.getItem(retryKey(identity))
    sessionStorage.removeItem(retryKey(identity))
    return safeSourceLinkPath(path)
  } catch {
    return routePaths.home
  }
}
