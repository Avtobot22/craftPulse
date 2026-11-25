const base = 'https://craftpulse.invalid'

function isSafe(value: string): boolean {
  if (
    !value.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\') ||
    Array.from(value).some((character) => character.charCodeAt(0) <= 32)
  )
    return false
  const url = new URL(value, base)
  return url.origin === base && !url.pathname.startsWith('//')
}

export function safeReturnTo(value: unknown): string {
  if (typeof value !== 'string' || value.length > 2048) return '/'
  let decoded = value
  for (let depth = 0; depth < value.length + 1; depth += 1) {
    if (!isSafe(decoded)) return '/'
    let next: string
    try {
      next = decodeURIComponent(decoded)
    } catch {
      break
    }
    if (next === decoded) break
    decoded = next
  }
  const url = new URL(value, base)
  return url.pathname + url.search + url.hash
}

export function isOrganizationSlug(value: unknown): value is string {
  return typeof value === 'string' && /^[a-z0-9-]{2,40}$/u.test(value)
}

export function createLoginUrl(orgSlug: string, returnTo = '/'): string {
  if (!isOrganizationSlug(orgSlug))
    throw new Error('Некорректный адрес организации')
  return `/o/${encodeURIComponent(orgSlug)}/login?${new URLSearchParams({ return_to: safeReturnTo(returnTo) }).toString()}`
}

export function createLoginPageUrl(
  orgSlug: string | null,
  returnTo = '/',
): string {
  const path = isOrganizationSlug(orgSlug)
    ? `/o/${encodeURIComponent(orgSlug)}`
    : '/login'
  return `${path}?${new URLSearchParams({ return_to: safeReturnTo(returnTo) }).toString()}`
}
