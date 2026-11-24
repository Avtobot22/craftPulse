import { createLoginUrl } from './return-to'

/** The server starts OIDC; this navigation must leave the SPA. */
export function navigateToLogin(orgSlug: string, returnTo: string): void {
  window.location.assign(createLoginUrl(orgSlug, returnTo))
}
