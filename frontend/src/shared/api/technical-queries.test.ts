import { QueryClient } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { jsonResponse } from '../../../test/fixtures'
import {
  ApiClient,
  compareTechnicalResults,
  getCheckProfile,
  getCiConfiguration,
  getComponentOverview,
  getException,
  getFinding,
  getMergeRequestSummary,
  listBaselines,
  listDocumentationRoots,
  listExceptions,
  listFindings,
  listServiceDocuments,
  listTestExecutions,
  listUploadTokens,
} from './index'

it('routes the technical queries with their scope, filters and cancellation signal', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementation(() => Promise.resolve(jsonResponse({})))
  const api = new ApiClient(fetcher)
  const client = new QueryClient()
  // An empty object must be rejected by every decoder.
  const queries = [
    () => client.query(getMergeRequestSummary(api, 'mr', { revision_no: 3 })),
    () => client.query(listBaselines(api, 'p')),
    () =>
      client.query(
        compareTechnicalResults(api, {
          component_id: 'c',
          base_sha: 'base',
          head_sha: 'head',
          analysis_kind: 'code',
        }),
      ),
    () => client.query(getComponentOverview(api, 'c')),
    () =>
      client.query(
        listFindings(api, {
          project_id: 'p',
          severity: 'high',
          cursor: 'next+1',
        }),
      ),
    () => client.query(getFinding(api, 'a/b')),
    () => client.query(listExceptions(api, { state: 'pending' })),
    () => client.query(getException(api, 'e')),
    () => client.query(listServiceDocuments(api, 's', { limit: 10 })),
    () => client.query(listTestExecutions(api, 'ci', { outcome: 'failed' })),
    () => client.query(getCheckProfile(api, 'p')),
    () =>
      client.query(getCiConfiguration(api, 'p', { ci_system: 'gitlab_ci' })),
    () => client.query(listUploadTokens(api, { project_id: 'p' })),
    () => client.query(listDocumentationRoots(api, 's')),
  ]

  for (const query of queries) {
    await expect(query()).rejects.toMatchObject({ kind: 'invalid-response' })
  }
  expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
    '/api/v1/merge-requests/mr/summary?revision_no=3',
    '/api/v1/projects/p/baselines',
    '/api/v1/technical-comparisons?component_id=c&base_sha=base&head_sha=head&analysis_kind=code',
    '/api/v1/components/c/overview',
    '/api/v1/findings?project_id=p&severity=high&cursor=next%2B1',
    '/api/v1/findings/a%2Fb',
    '/api/v1/exceptions?state=pending',
    '/api/v1/exceptions/e',
    '/api/v1/services/s/documents?limit=10',
    '/api/v1/pipelines/ci/test-executions?outcome=failed',
    '/api/v1/projects/p/check-profiles',
    '/api/v1/projects/p/ci-configuration?ci_system=gitlab_ci',
    '/api/v1/upload-tokens?project_id=p',
    '/api/v1/services/s/documentation-roots',
  ])
  expect(
    fetcher.mock.calls.every(
      ([, request]) => request?.signal instanceof AbortSignal,
    ),
  ).toBe(true)
})

it('keeps an unknown comparison reason and caches comparisons separately by their SHA', async () => {
  const response = {
    comparable: false,
    incomparable_reasons: ['future_reason'],
    base: { sha: 'base', report_id: 'report-1' },
    head: { sha: 'head', report_id: null },
    factors: [],
    access_limited: true,
    access_notes: [{ code: 'restricted_by_access' }],
  }
  const api = new ApiClient(
    vi
      .fn<typeof fetch>()
      .mockImplementation(() => Promise.resolve(jsonResponse(response))),
  )
  const params = {
    component_id: 'c',
    base_sha: 'base',
    head_sha: 'head',
    analysis_kind: 'code',
  } as const
  const query = compareTechnicalResults(api, params)
  const client = new QueryClient()
  expect(await client.query(query)).toEqual(response)
  expect(
    client.getQueryData(
      compareTechnicalResults(api, { ...params, head_sha: 'other-head' })
        .queryKey,
    ),
  ).toBeUndefined()
})

it('keeps the profile ETag with the decoded version used for edits', async () => {
  const profile = {
    version_no: 3,
    base_version_no: null,
    entries: [],
    uncovered: [],
    confirmed_by: 'user-1',
    confirmed_at: '2026-10-01T00:00:00Z',
  }
  const response = jsonResponse(profile)
  response.headers.set('ETag', '"profile-3"')
  const api = new ApiClient(vi.fn<typeof fetch>().mockResolvedValue(response))
  const client = new QueryClient()
  expect(await client.query(getCheckProfile(api, 'p'))).toEqual({
    data: profile,
    etag: '"profile-3"',
  })
})
