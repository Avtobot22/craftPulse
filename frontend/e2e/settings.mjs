import { realpathSync, statSync } from 'node:fs'
import { isAbsolute, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

export const profiles = {
  'chromium-1280': {
    browserName: 'chromium',
    viewport: { width: 1280, height: 800 },
  },
  'chromium-1920': {
    browserName: 'chromium',
    viewport: { width: 1920, height: 1080 },
  },
  'firefox-1280': {
    browserName: 'firefox',
    viewport: { width: 1280, height: 800 },
  },
  'firefox-1920': {
    browserName: 'firefox',
    viewport: { width: 1920, height: 1080 },
  },
}

export const accounts = {
  adminA: 'cp87-admin-a',
  adminB: 'cp87-admin-b',
  member: 'cp87-member',
  outsider: 'cp87-no-repo',
}

const passwordVariables = {
  adminA: 'AC01_ADMIN_A_PASSWORD',
  adminB: 'AC01_ADMIN_B_PASSWORD',
  member: 'AC01_MEMBER_PASSWORD',
  outsider: 'AC01_OUTSIDER_PASSWORD',
}

function requireEnv(name) {
  const value = process.env[name]
  if (!value) throw new Error(`Set ${name} before running browser acceptance`)
  return value
}

function origin(name) {
  const value = requireEnv(name)
  const url = URL.canParse(value) ? new URL(value) : null
  if (!url || url.href !== url.origin + '/' || url.protocol !== 'https:') {
    throw new Error(`${name} must be an HTTPS origin`)
  }
  return url.origin
}

export function settings() {
  const profile = requireEnv('AC01_PROFILE')
  if (!Object.hasOwn(profiles, profile)) throw new Error('Unknown AC01_PROFILE')
  const location = requireEnv('AC01_EVIDENCE_DIR')
  const output = realpathSync(location)
  const repo = fileURLToPath(new URL('../../', import.meta.url))
  if (
    !isAbsolute(location) ||
    !relative(realpathSync(repo), output).startsWith('../') ||
    !statSync(output).isDirectory()
  ) {
    throw new Error(
      'AC01_EVIDENCE_DIR must be an absolute path outside the checkout',
    )
  }
  for (const name of Object.values(passwordVariables)) requireEnv(name)
  const sha = requireEnv('AC01_RELEASE_SHA')
  if (!/^[a-f0-9]{40}$/.test(sha)) throw new Error('Invalid AC01_RELEASE_SHA')
  const browserVersion = requireEnv('AC01_BROWSER_VERSION')
  if (!/^\d+(\.\d+){1,3}$/.test(browserVersion)) {
    throw new Error('Invalid AC01_BROWSER_VERSION')
  }
  const site = origin('AC01_SITE')
  const gitlab = origin('AC01_GITLAB')
  if (site === gitlab)
    throw new Error('Use distinct craftPulse and GitLab origins')
  return { output, profile, sha, browserVersion, site, gitlab }
}

export function password(account) {
  return requireEnv(passwordVariables[account])
}
