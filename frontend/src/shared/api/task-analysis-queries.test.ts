import { QueryClient } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { jsonResponse, lateTimelineEvent } from '../../../test/fixtures'
import { contractLabel, contractLabels } from '@/shared/config'
import { ApiClient } from './client'
import { getMergeRequest, listMergeRequests } from './merge-request-queries'
import { getPipeline, listPipelineAttempts } from './pipeline-queries'
import { getTask, listTasks } from './task-queries'
import { listProjectTimeline, listTaskEvents } from './timeline-queries'

const from = '2026-10-02T00:00:00Z'
const to = '2026-10-03T00:00:00Z'

it('keeps the new event code in a decoded timeline page and keys it by project and query', async () => {
  const event = { ...lateTimelineEvent, event_kind: 'future_kind' }
  const fetcher = vi.fn<typeof fetch>().mockImplementation(() =>
    Promise.resolve(
      jsonResponse({
        data: [event],
        next_cursor: null,
        history_from: null,
        access_limited: false,
        access_notes: [],
      }),
    ),
  )
  const api = new ApiClient(fetcher)
  const client = new QueryClient()
  const options = listProjectTimeline(api, 'project-1', {
    from,
    to,
    actor_kind: 'human',
  })

  const page = await client.query(options)
  expect(page.data).toEqual([event])
  expect(
    page.data.map(({ event_kind }) =>
      contractLabel(contractLabels.eventKind, event_kind),
    ),
  ).toEqual(['Неизвестное значение (future_kind)'])
  const [url, request] = fetcher.mock.calls[0] ?? []
  expect(url).toBe(
    '/api/v1/projects/project-1/timeline?from=2026-10-02T00%3A00%3A00Z&to=2026-10-03T00%3A00%3A00Z&actor_kind=human',
  )
  expect(request?.signal).toBeInstanceOf(AbortSignal)
  expect(
    listProjectTimeline(api, 'project-2', { from, to, actor_kind: 'human' })
      .queryKey,
  ).not.toEqual(options.queryKey)
  expect(
    listProjectTimeline(api, 'project-1', { from, to, actor_kind: 'bot' })
      .queryKey,
  ).not.toEqual(options.queryKey)
})

it('routes the remaining queries through the client with their IDs and filters', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementation(() => Promise.resolve(jsonResponse({})))
  const api = new ApiClient(fetcher)
  const client = new QueryClient()
  // Empty objects are deliberately invalid to exercise the response boundary.
  const queries = [
    () =>
      client.query(
        listTasks(api, { team_id: 't', over_target: false, sort: '-age' }),
      ),
    () => client.query(getTask(api, 'a/b')),
    () => client.query(listTaskEvents(api, 'a/b', { cursor: 'next page' })),
    () =>
      client.query(
        listMergeRequests(api, { project_id: 'p', review_wait: 'no_reviewer' }),
      ),
    () => client.query(getMergeRequest(api, 'mr')),
    () => client.query(getPipeline(api, 'ci')),
    () => client.query(listPipelineAttempts(api, 'ci', { outcome: 'failed' })),
  ]
  for (const query of queries) {
    await expect(query()).rejects.toMatchObject({ kind: 'invalid-response' })
  }
  expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
    '/api/v1/tasks?team_id=t&over_target=false&sort=-age',
    '/api/v1/tasks/a%2Fb',
    '/api/v1/tasks/a%2Fb/events?cursor=next+page',
    '/api/v1/merge-requests?project_id=p&review_wait=no_reviewer',
    '/api/v1/merge-requests/mr',
    '/api/v1/pipelines/ci',
    '/api/v1/pipelines/ci/attempts?outcome=failed',
  ])
})
