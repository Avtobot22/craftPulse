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

export function createLoginUrl(orgSlug: string, returnTo = '/'): string {
  if (!/^[a-z0-9-]{2,40}$/u.test(orgSlug))
    throw new Error('Некорректный адрес организации')
  return `/o/${encodeURIComponent(orgSlug)}/login?${new URLSearchParams({ return_to: safeReturnTo(returnTo) }).toString()}`
}
