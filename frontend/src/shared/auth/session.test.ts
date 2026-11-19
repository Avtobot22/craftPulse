import { QueryClient } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiClient,
  listTeams,
  listProjects,
  listServices,
  readRecord,
  readString,
} from '@/shared/api'
import { SessionController } from './session'
import { readCurrentUser } from './current-user'
import {
  currentUser,
  teams,
  projects,
  services,
  jsonResponse,
  problemResponse,
} from '../../../test/fixtures'

const clients: QueryClient[] = []
const sessions: SessionController[] = []
function setup(fetcher: typeof fetch) {
  const queries = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const api = new ApiClient(fetcher)
  const session = new SessionController(api, queries)
  clients.push(queries)
  sessions.push(session)
  return { queries, api, session }
}
afterEach(() => {
  for (const session of sessions.splice(0)) session.dispose()
  for (const client of clients.splice(0)) client.clear()
})

describe('session and query infrastructure', () => {
  it('accepts unknown enum strings, validates branch fields and drops unknown fields', () => {
    const firstRole = currentUser.roles[0]
    if (!firstRole) throw new Error('Missing fixture role')
    const user = readCurrentUser({
      ...currentUser,
      private_value: 'hidden',
      user: { ...currentUser.user, status: 'future_user_status' },
      organization: {
        ...currentUser.organization,
        status: 'future_organization_status',
      },
      roles: [{ ...firstRole, role: 'future_role' }],
    })
    expect(user.user.status).toBe('unknown')
    expect(user.organization.status).toBe('unknown')
    expect(user.roles[0]?.role).toBe('unknown')
    expect(user).not.toHaveProperty('private_value')
    expect(() => readCurrentUser({ ...currentUser, actions: [3] })).toThrow()
    expect(() => readCurrentUser({ ...currentUser, roles: [null] })).toThrow()
  })

  it('creates cursor query options for teams, projects and services', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        jsonResponse({ ...teams, next_cursor: 'cursor/+=' }),
      )
      .mockResolvedValueOnce(jsonResponse({ ...teams, next_cursor: null }))
      .mockResolvedValueOnce(jsonResponse(projects))
      .mockResolvedValueOnce(jsonResponse(services))
    const { queries, api } = setup(fetcher)
    const page = await queries.query(listTeams(api, { limit: 1 }))
    if (page.next_cursor === null) throw new Error('Expected cursor')
    await queries.query(listTeams(api, { limit: 1, cursor: page.next_cursor }))
    await queries.query(listProjects(api, { team_id: 'team-id', limit: 20 }))
    await queries.query(listServices(api))
    expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
      '/api/v1/teams?limit=1',
      '/api/v1/teams?limit=1&cursor=cursor%2F%2B%3D',
      '/api/v1/projects?team_id=team-id&limit=20',
      '/api/v1/services',
    ])
    expect(page).not.toHaveProperty('total')
  })

  it('clears all query and mutation data on 401 from any request', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(problemResponse('auth_required', 401))
    const { queries, api, session } = setup(fetcher)
    queries.setQueryData(['old-user', 'report'], { private: true })
    const mutation = queries
      .getMutationCache()
      .build(queries, { mutationFn: () => Promise.resolve({ private: true }) })
    await mutation.execute(undefined)
    await expect(queries.query(listServices(api))).rejects.toBeDefined()
    expect(session.getSnapshot()).toBe('expired')
    expect(queries.getQueryCache().getAll()).toHaveLength(0)
    expect(queries.getMutationCache().getAll()).toHaveLength(0)
  })

  it('ignores a late successful response from the previous session', async () => {
    let respond: (response: Response) => void = () => {
      throw new Error('Response is not ready')
    }
    const pending = new Promise<Response>((resolve) => {
      respond = resolve
    })
    const fetcher = vi
      .fn<typeof fetch>()
      .mockReturnValueOnce(pending)
      .mockResolvedValueOnce(problemResponse('auth_required', 401))
    const { queries, api } = setup(fetcher)
    const request = api
      .read('/teams', (raw) => readString(readRecord(raw).next_cursor))
      .catch((error: unknown) => error)
    queries.setQueryData(['old'], 'private')
    await expect(api.read('/services', () => undefined)).rejects.toBeDefined()
    respond(jsonResponse({ ...teams, next_cursor: 'late' }))
    expect(await request).toMatchObject({ kind: 'session-ended' })
    expect(queries.getQueryCache().getAll()).toHaveLength(0)
  })

  it('logs out through the contract endpoint, then clears state and blocks late requests', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(currentUser))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const { queries, api, session } = setup(fetcher)
    await queries.query(session.currentUserOptions())
    queries.setQueryData(['teams'], teams)
    expect(session.getSnapshot()).toBe('authenticated')
    await session.logout()
    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/session/logout',
      expect.objectContaining({ method: 'POST', credentials: 'same-origin' }),
    )
    expect(session.getSnapshot()).toBe('signed-out')
    expect(queries.getQueryCache().getAll()).toHaveLength(0)
    await expect(api.read('/teams', () => undefined)).rejects.toMatchObject({
      kind: 'session-ended',
    })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('does not announce or clear a session after a failed logout', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(currentUser))
      .mockResolvedValueOnce(problemResponse('forbidden', 403))
    const { queries, session } = setup(fetcher)
    await queries.query(session.currentUserOptions())
    await expect(session.logout()).rejects.toMatchObject({ kind: 'forbidden' })
    expect(session.getSnapshot()).toBe('authenticated')
    expect(
      queries.getQueryData(session.currentUserOptions().queryKey),
    ).toBeDefined()
  })
})
