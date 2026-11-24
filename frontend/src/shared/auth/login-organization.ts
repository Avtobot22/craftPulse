import { isOrganizationSlug } from './return-to'

const key = 'craftpulse.login.organization'

/** Retain only the route slug in this tab, never the CurrentUser response. */
export function readLoginOrganization(): string | null {
  try {
    const value = sessionStorage.getItem(key)
    return isOrganizationSlug(value) ? value : null
  } catch {
    return null
  }
}

export function rememberLoginOrganization(value: string): string | null {
  const slug = isOrganizationSlug(value) ? value : null
  try {
    if (slug) sessionStorage.setItem(key, slug)
    else sessionStorage.removeItem(key)
  } catch {
    // The current session still retains the slug when browser storage is blocked.
  }
  return slug
}
