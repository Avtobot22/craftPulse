import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiClient, listServices } from '@/shared/api'
import {
  currentUser,
  jsonResponse,
  problemResponse,
} from '../../../test/fixtures'
import { SessionController } from './session'
import { createLoginPageUrl } from './return-to'

const sessions: SessionController[] = []
const clients: QueryClient[] = []
function setup(fetcher: typeof fetch) {
  const queries = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const api = new ApiClient(fetcher)
  const session = new SessionController(api, queries)
  sessions.push(session)
  clients.push(queries)
  return { session, queries, api }
}
beforeEach(() => {
  sessionStorage.clear()
})
afterEach(() => {
  sessions.splice(0).forEach((session) => {
    session.dispose()
  })
  clients.splice(0).forEach((client) => {
    client.clear()
  })
  vi.restoreAllMocks()
  sessionStorage.clear()
})

describe('organization context for a repeated login', () => {
  it('retains only a validated route slug after 401 clears cached data', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(currentUser))
      .mockResolvedValueOnce(problemResponse('auth_required', 401))
    const { session, queries, api } = setup(fetcher)
    await queries.query(session.currentUserOptions())
    await expect(queries.query(listServices(api))).rejects.toBeDefined()
    expect(session.getSnapshot()).toBe('expired')
    expect(queries.getQueryCache().getAll()).toHaveLength(0)
    expect(session.getLoginOrganization()).toBe('example')
    expect(Object.keys(sessionStorage)).toEqual([
      'craftpulse.login.organization',
    ])
    expect(sessionStorage.getItem('craftpulse.login.organization')).toBe(
      'example',
    )
  })
  it('does not trust an injected invalid stored slug', () => {
    sessionStorage.setItem(
      'craftpulse.login.organization',
      '//external.example',
    )
    expect(
      setup(vi.fn<typeof fetch>()).session.getLoginOrganization(),
    ).toBeNull()
  })
  it('rejects an invalid organization slug in a successful response', async () => {
    sessionStorage.setItem('craftpulse.login.organization', 'previous')
    const { queries, session } = setup(
      vi.fn<typeof fetch>().mockResolvedValue(
        jsonResponse({
          ...currentUser,
          organization: { ...currentUser.organization, slug: '../other' },
        }),
      ),
    )
    await queries.query(session.currentUserOptions())
    expect(session.getLoginOrganization()).toBeNull()
    expect(sessionStorage.getItem('craftpulse.login.organization')).toBeNull()
  })
  it('keeps the current slug in memory if browser storage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { queries, session } = setup(
      vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(currentUser)),
    )
    await queries.query(session.currentUserOptions())
    expect(session.getLoginOrganization()).toBe('example')
  })
  it('does not overwrite login context with a late response from an ended session', async () => {
    sessionStorage.setItem('craftpulse.login.organization', 'previous')
    let respond: (response: Response) => void = () => {
      throw new Error('Missing response')
    }
    const pending = new Promise<Response>((resolve) => {
      respond = resolve
    })
    const fetcher = vi
      .fn<typeof fetch>()
      .mockReturnValueOnce(pending)
      .mockResolvedValueOnce(problemResponse('auth_required', 401))
    const { queries, session, api } = setup(fetcher)
    const me = queries
      .query(session.currentUserOptions())
      .catch(() => undefined)
    await expect(api.read('/services', (value) => value)).rejects.toBeDefined()
    respond(jsonResponse(currentUser))
    await me
    expect(session.getLoginOrganization()).toBe('previous')
    expect(sessionStorage.getItem('craftpulse.login.organization')).toBe(
      'previous',
    )
  })
  it('builds the server-compatible public landing page with only a safe continuation', () => {
    expect(createLoginPageUrl('example', '/users?team=123#form')).toBe(
      '/o/example?return_to=%2Fusers%3Fteam%3D123%23form',
    )
    expect(createLoginPageUrl(null, '//external.example')).toBe(
      '/login?return_to=%2F',
    )
  })
})
