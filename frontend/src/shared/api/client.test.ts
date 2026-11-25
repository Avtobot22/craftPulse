import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiClient } from './client'
import { ApiError, fieldErrorsByPointer } from './problem'
import { InvalidResponse, readRecord } from './response'
import { readTeamPage } from './organization'
import type { components } from './generated/schema'
import {
  teams,
  structure,
  structureInput,
  problem,
  jsonResponse,
  problemResponse,
  correlationId,
} from '../../../test/fixtures'

function readVersion(
  value: unknown,
): Pick<components['schemas']['OrganizationStructure'], 'version'> {
  const raw = readRecord(value)
  if (typeof raw.version !== 'number') throw new InvalidResponse()
  return { version: raw.version }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ApiClient', () => {
  it('uses the API base, session cookies, no HTTP cache and an abort signal', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(teams))
    const api = new ApiClient(fetcher)
    const controller = new AbortController()
    const result = await api.read('/teams', readTeamPage, {
      signal: controller.signal,
      query: { limit: 10, cursor: 'opaque+/= value' },
    })
    expect(result.data).toEqual(teams)
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/teams?limit=10&cursor=opaque%2B%2F%3D+value',
      expect.objectContaining({
        method: 'GET',
        credentials: 'same-origin',
        cache: 'no-store',
        signal: controller.signal,
      }),
    )
  })

  it.each([
    { code: 'auth_required', status: 401, kind: 'auth-required' },
    { code: 'forbidden', status: 403, kind: 'forbidden' },
    { code: 'not_found', status: 404, kind: 'not-found' },
    { code: 'version_conflict', status: 412, kind: 'version-conflict' },
    { code: 'validation_failed', status: 422, kind: 'validation' },
    {
      code: 'precondition_required',
      status: 428,
      kind: 'precondition-required',
    },
    { code: 'future_error', status: 409, kind: 'api-error' },
  ])(
    'classifies $code and keeps safe diagnostics without detail',
    async ({ code, status, kind }) => {
      const api = new ApiClient(
        vi.fn<typeof fetch>().mockResolvedValue(problemResponse(code, status)),
      )
      const error: unknown = await api
        .read('/teams', readTeamPage)
        .catch((caught: unknown) => caught)
      expect(error).toBeInstanceOf(ApiError)
      if (!(error instanceof ApiError)) throw new Error('Expected ApiError')
      expect(error.kind).toBe(kind)
      expect(error.problem?.correlation_id).toBe(correlationId)
      expect(error.problem).not.toHaveProperty('detail')
      expect(error.message).not.toContain('private-internal')
    },
  )

  it('branches on code rather than using HTTP status as the error kind', async () => {
    const api = new ApiClient(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(problemResponse('future_error', 403)),
    )
    await expect(api.read('/teams', readTeamPage)).rejects.toMatchObject({
      kind: 'api-error',
      problem: { code: 'future_error' },
    })
  })

  it('passes field errors by JSON Pointer without server detail', async () => {
    const body = {
      ...problem('validation_failed', 422),
      errors: [
        {
          field: '/team_projects/0/team_id',
          code: 'unknown_reference',
          detail: 'private value',
        },
        { field: '/team_projects/0/team_id', code: 'future_code' },
      ],
    }
    const api = new ApiClient(
      vi.fn<typeof fetch>().mockResolvedValue(
        jsonResponse(body, 422, {
          'Content-Type': 'application/problem+json',
        }),
      ),
    )
    const error: unknown = await api
      .read('/teams', readTeamPage)
      .catch((caught: unknown) => caught)
    if (!(error instanceof ApiError) || !error.problem)
      throw new Error('Expected problem')
    const fields = fieldErrorsByPointer(error.problem)
    expect(fields.get('/team_projects/0/team_id')).toEqual([
      { field: '/team_projects/0/team_id', code: 'unknown_reference' },
      { field: '/team_projects/0/team_id', code: 'unknown' },
    ])
  })

  it.each(['"7"', '"0"'])(
    'sends the ETag read from the same resource: %s',
    async (etag) => {
      const fetcher = vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(jsonResponse(structure, 200, { ETag: etag }))
        .mockResolvedValueOnce(
          jsonResponse({ ...structure, version: 8 }, 200, { ETag: '"8"' }),
        )
      const api = new ApiClient(fetcher)
      expect(
        (await api.read('/organization/structure', readVersion)).etag,
      ).toBe(etag)
      expect(
        (
          await api.update(
            'PUT',
            '/organization/structure',
            structureInput,
            readVersion,
          )
        ).data.version,
      ).toBe(8)
      const call = fetcher.mock.calls[1]
      if (!call) throw new Error('Missing mutation request')
      expect(new Headers(call[1]?.headers).get('If-Match')).toBe(etag)
      expect(call[1]?.body).toBe(JSON.stringify(structureInput))
    },
  )

  it('never sends a versioned mutation without a previously read ETag', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = new ApiClient(fetcher)
    await expect(
      api.update('PUT', '/organization/structure', structureInput, readVersion),
    ).rejects.toMatchObject({ kind: 'missing-version' })
    expect(fetcher).not.toHaveBeenCalled()
  })

  it.each(['*', 'W/"7"', '"7", "8"'])(
    'rejects an invalid explicit ETag even with a cached version: %s',
    async (ifMatch) => {
      const fetcher = vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(jsonResponse(structure, 200, { ETag: '"7"' }))
      const api = new ApiClient(fetcher)
      await api.read('/organization/structure', readVersion)

      await expect(
        api.update(
          'PUT',
          '/organization/structure',
          structureInput,
          readVersion,
          {
            ifMatch,
          },
        ),
      ).rejects.toMatchObject({ kind: 'missing-version' })
      expect(fetcher).toHaveBeenCalledOnce()
    },
  )

  it('retires old versions when a new read lacks ETag or the previous write fails', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(structure, 200, { ETag: '"7"' }))
      .mockResolvedValueOnce(jsonResponse(structure))
    const api = new ApiClient(fetcher)
    await api.read('/organization/structure', readVersion)
    await api.read('/organization/structure', readVersion)
    await expect(
      api.update('PUT', '/organization/structure', structureInput, readVersion),
    ).rejects.toMatchObject({ kind: 'missing-version' })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('requires a new read after a version conflict', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(structure, 200, { ETag: '"7"' }))
      .mockResolvedValueOnce(problemResponse('version_conflict', 412))
    const api = new ApiClient(fetcher)
    await api.read('/organization/structure', readVersion)
    await expect(
      api.update('PUT', '/organization/structure', structureInput, readVersion),
    ).rejects.toMatchObject({ kind: 'version-conflict' })
    await expect(
      api.update('PUT', '/organization/structure', structureInput, readVersion),
    ).rejects.toMatchObject({ kind: 'missing-version' })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('blocks the session on malformed 401 as well as standard auth_required', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response('private error', { status: 401 }))
    const api = new ApiClient(fetcher)
    const listener = vi.fn()
    api.onAuthRequired(listener)
    await expect(api.read('/teams', readTeamPage)).rejects.toMatchObject({
      kind: 'auth-required',
      problem: null,
    })
    expect(listener).toHaveBeenCalledOnce()
    await expect(api.read('/teams', readTeamPage)).rejects.toMatchObject({
      kind: 'session-ended',
    })
    expect(fetcher).toHaveBeenCalledOnce()
  })

  it.each([
    { data: { ...teams, next_cursor: 123 } },
    { data: { ...teams, data: [{ id: 'x', name: 'x', status: null }] } },
  ])(
    'rejects malformed cursor or status instead of casting the response',
    async ({ data }) => {
      const api = new ApiClient(
        vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(data)),
      )
      await expect(api.read('/teams', readTeamPage)).rejects.toMatchObject({
        kind: 'invalid-response',
      })
    },
  )

  it('distinguishes network failure from a server problem', async () => {
    const api = new ApiClient(
      vi
        .fn<typeof fetch>()
        .mockRejectedValue(new TypeError('private network address')),
    )
    await expect(api.read('/teams', readTeamPage)).rejects.toMatchObject({
      kind: 'network',
      problem: null,
      message: 'Не удалось выполнить запрос',
    })
  })
})
