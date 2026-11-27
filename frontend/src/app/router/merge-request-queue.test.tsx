import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import type { components } from '@/shared/api'
import { routePaths, taskAnalysisLinks } from '@/shared/config'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  projects,
  teams,
} from '../../../test/fixtures'
import { mergeRequest } from '../../../test/task-changes-fixtures'
import {
  taskDetail,
  taskHistory,
  taskId,
  taskTitle,
} from '../../../test/task-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
type QueuePage = Schema['MergeRequestPage']

const teamId = taskDetail.team_id ?? ''
const projectId = taskDetail.project_id ?? ''
const period = {
  from: '2026-10-01T00:00:00Z',
  to: '2026-11-01T00:00:00Z',
}
const queuePath = taskAnalysisLinks.mergeRequestQueue({
  team_id: teamId,
  project_id: projectId,
  ...period,
})
const reportPath = `/api/v1/teams/${teamId}/report`
const withoutReviewer = {
  ...mergeRequest,
  review_wait: { ...mergeRequest.review_wait, kind: 'no_reviewer' },
} satisfies Schema['MergeRequest']
const secondWithoutReviewer = {
  ...withoutReviewer,
  id: '0193a5b0-b0b0-7000-8000-000000000008',
  iid: 43,
  title: 'Проверка очереди',
  source_url: 'https://gitlab.example/core/-/merge_requests/43',
  review_wait: { ...withoutReviewer.review_wait, wait_seconds: 3600 },
} satisfies Schema['MergeRequest']
// The API represents a missing reviewer field through the waiting kind.
const withoutAssignmentField = {
  ...mergeRequest,
  id: '0193a5b0-b0b0-7000-8000-000000000009',
  iid: 44,
  title: 'Источник без поля ревьюеров',
  source_url: 'https://gitlab.example/core/-/merge_requests/44',
  ready_at: null,
  review_wait: {
    ...mergeRequest.review_wait,
    kind: 'awaiting_first_response',
    since: mergeRequest.opened_at,
    fallback_open: true,
  },
} satisfies Schema['MergeRequest']

const page = (
  data: Schema['MergeRequest'][],
  next_cursor: string | null = null,
): QueuePage => ({
  data,
  next_cursor,
  access_limited: false,
  access_notes: [],
})
function readQueue({ searchParams }: URL) {
  if (searchParams.get('review_wait') === 'awaiting_first_response')
    return jsonResponse(page([withoutAssignmentField]))
  if (searchParams.get('cursor') === 'next-reviewers')
    return jsonResponse(page([secondWithoutReviewer]))
  return jsonResponse(page([withoutReviewer], 'next-reviewers'))
}

const pointStart = '2026-10-05T00:00:00Z'
const metric = (
  metric_id: string,
  value: number,
  overrides: Partial<Schema['MetricValue']>,
  estimator?: string,
): Schema['MetricSeries'] => ({
  metric_id,
  definition_version: 1,
  unit: overrides.unit ?? 'count',
  ...(estimator ? { estimator } : {}),
  points: [
    {
      point_start: pointStart,
      value: {
        state: 'ASSESSED',
        value,
        unit: 'count',
        numerator: null,
        denominator: null,
        n: 2,
        bound: null,
        low_sample: true,
        excluded_count: 0,
        missing_count: 0,
        reason_code: null,
        ...overrides,
      },
    },
  ],
})
const report = {
  team_id: teamId,
  period,
  step: 'week',
  revision: {
    id: '0193a5b0-a1a1-7000-8000-000000000002',
    series_id: '0193a5b0-a2a2-7000-8000-000000000001',
    revision_no: 31,
    as_of: period.to,
    published_at: '2026-11-08T03:40:00Z',
    loading: false,
    irreproducible_reason: null,
  },
  grouping: 'current_structure',
  metrics: [
    metric(
      'A07',
      7200,
      { unit: 'seconds', reason_code: 'first_external_response' },
      'p50',
    ),
    metric(
      'A07',
      10800,
      { unit: 'seconds', reason_code: 'first_external_response' },
      'p85',
    ),
    metric('A05', 3, { n: 3 }),
    metric('A10', 0.25, { unit: 'ratio', numerator: 1, denominator: 4, n: 4 }),
  ],
  access_limited: false,
  access_notes: [],
} satisfies Schema['TeamReport']

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

function mount({
  path = queuePath,
  list = readQueue,
  readReport = () => jsonResponse(report),
}: {
  path?: string
  list?: (url: URL) => Response
  readReport?: () => Response
} = {}) {
  const requests: URL[] = []
  function respond(input: RequestInfo | URL) {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    requests.push(url)
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse({
          ...currentUser,
          actions: [...currentUser.actions, 'mr.read'],
        })
      case '/api/v1/teams':
        return jsonResponse(teams)
      case '/api/v1/projects':
        return jsonResponse(projects)
      case '/api/v1/merge-requests':
        return list(url)
      case reportPath:
        return readReport()
      case `/api/v1/tasks/${taskId}`:
        return jsonResponse(taskDetail)
      case `/api/v1/tasks/${taskId}/events`:
        return jsonResponse(taskHistory)
      case `/api/v1/merge-requests/${mergeRequest.id}`:
        return jsonResponse(mergeRequest)
      default:
        throw new Error(`Unexpected URL ${url.pathname}`)
    }
  }
  const runtime = createAppRuntime((input) => Promise.resolve(respond(input)))
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [path],
  })
  const view = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  function dispose() {
    const index = cleanups.indexOf(dispose)
    if (index !== -1) cleanups.splice(index, 1)
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  }
  cleanups.push(dispose)
  return { router, requests, dispose }
}

const noReviewerGroup = () =>
  within(screen.getByRole('region', { name: 'Нет ревьюера' }))
const firstResponseGroup = () =>
  within(screen.getByRole('region', { name: 'Ждёт первого ответа человека' }))

it('requests both open groups for the shell project, longest waits first', async () => {
  const { requests } = mount()
  await screen.findByRole('link', { name: /!44/ })
  expect(
    await noReviewerGroup().findByRole('link', { name: /!42/ }),
  ).toBeVisible()
  expect(
    noReviewerGroup().queryByRole('link', { name: /!44/ }),
  ).not.toBeInTheDocument()
  expect(firstResponseGroup().getByRole('link', { name: /!44/ })).toBeVisible()
  expect(
    firstResponseGroup().queryByRole('link', { name: /!42/ }),
  ).not.toBeInTheDocument()

  const calls = requests.filter(
    (url) => url.pathname === '/api/v1/merge-requests',
  )
  expect(calls.map((url) => Object.fromEntries(url.searchParams))).toEqual([
    {
      project_id: projectId,
      review_wait: 'no_reviewer',
      state: 'open',
      sort: '-wait',
      limit: '25',
    },
    {
      project_id: projectId,
      review_wait: 'awaiting_first_response',
      state: 'open',
      sort: '-wait',
      limit: '25',
    },
  ])
})

it('renders the order returned by the API after switching wait direction', async () => {
  mount({
    list: (url) => {
      if (url.searchParams.get('review_wait') === 'awaiting_first_response')
        return readQueue(url)
      const rows =
        url.searchParams.get('sort') === 'wait'
          ? [secondWithoutReviewer, withoutReviewer]
          : [withoutReviewer, secondWithoutReviewer]
      return jsonResponse(page(rows))
    },
  })
  await screen.findByRole('link', { name: /!42/ })
  const titles = () =>
    noReviewerGroup()
      .getAllByRole('rowheader')
      .map((cell) => cell.textContent)
  expect(titles()).toEqual(['!42 · Перенос отчёта', '!43 · Проверка очереди'])

  fireEvent.change(screen.getByLabelText('Сортировка'), {
    target: { value: 'wait' },
  })
  await waitFor(() => {
    expect(titles()).toEqual(['!43 · Проверка очереди', '!42 · Перенос отчёта'])
  })
})

it('labels fallback age and exposes the contract GitLab link and author kind', async () => {
  mount()
  expect(await screen.findByText('1 д 22 ч (от открытия)')).toBeVisible()
  expect(
    firstResponseGroup().getByText('Дата готовности неизвестна'),
  ).toBeVisible()
  expect(firstResponseGroup().getByText('Build Bot')).toBeVisible()
  expect(firstResponseGroup().getByText('Бот')).toBeVisible()
  expect(
    firstResponseGroup().getByRole('link', { name: 'Открыть в GitLab' }),
  ).toHaveAttribute('href', withoutAssignmentField.source_url)
  expect(
    await noReviewerGroup().findByRole('link', { name: 'Открыть в GitLab' }),
  ).toHaveAttribute('href', withoutReviewer.source_url)
})

it('shows the report percentiles, sample size and ratio breakdown separately', async () => {
  const { requests } = mount()
  const result = within(
    await screen.findByRole('region', { name: 'Полученные ответы на ревью' }),
  )
  expect(await result.findByText('A07 · p50')).toBeVisible()
  expect(result.getByText('2 ч')).toBeVisible()
  expect(result.getByText('A07 · p85')).toBeVisible()
  expect(result.getByText('3 ч')).toBeVisible()
  expect(result.getAllByText('n = 2')).toHaveLength(2)
  expect(result.getByText('Первый внешний ответ')).toBeVisible()
  expect(result.getByText('A05 · Очередь ревью')).toBeVisible()
  expect(result.getByText('A10 · Доля MR без ревьюера')).toBeVisible()
  expect(result.getByText('1', { selector: 'dd' })).toBeVisible()
  expect(result.getByText('4', { selector: 'dd' })).toBeVisible()
  expect(result.getAllByText('Малая выборка')).toHaveLength(4)
  expect(noReviewerGroup().queryByText('A07 · p50')).not.toBeInTheDocument()
  expect(
    requests.find((url) => url.pathname === reportPath)?.searchParams,
  ).toEqual(
    new URLSearchParams({ ...period, step: 'week', project_id: projectId }),
  )
})

it('announces restricted access without exposing a count for the group', async () => {
  mount({
    list: (url) =>
      url.searchParams.get('review_wait') === 'no_reviewer'
        ? jsonResponse({
            ...page([]),
            access_limited: true,
            access_notes: [{ code: 'source_access_lost' }],
          } satisfies QueuePage)
        : jsonResponse(page([])),
  })
  expect(await screen.findByText('Есть закрытые результаты.')).toBeVisible()
  expect(
    noReviewerGroup().getByText('Подключение к источнику потеряло доступ.'),
  ).toBeVisible()
  expect(
    noReviewerGroup().getByText('На странице нет доступных MR.'),
  ).toBeVisible()
  expect(noReviewerGroup().queryByText(/получено \d/i)).not.toBeInTheDocument()
  expect(firstResponseGroup().getByText('Нет MR в ожидании')).toBeVisible()
})

it('preserves independent pagination, focus and size for the two groups', async () => {
  const { router } = mount()
  await screen.findByRole('link', { name: /!42/ })
  expect(
    await firstResponseGroup().findByRole('link', { name: /!44/ }),
  ).toBeVisible()
  fireEvent.click(
    noReviewerGroup().getByRole('button', { name: 'Следующая страница' }),
  )
  expect(
    await noReviewerGroup().findByRole('link', { name: /!43/ }),
  ).toBeVisible()
  expect(
    noReviewerGroup().getByRole('heading', { name: 'Нет ревьюера' }),
  ).toHaveFocus()
  expect(noReviewerGroup().getByText(/Страница 2 · получено 1/)).toBeVisible()
  expect(
    firstResponseGroup().getByText(/Страница 1 · получено 1/),
  ).toBeVisible()
  fireEvent.change(firstResponseGroup().getByLabelText('Строк на странице'), {
    target: { value: '50' },
  })
  const search = new URLSearchParams(router.state.location.search)
  expect(search.get('no_reviewer_cursor')).toBe('next-reviewers')
  expect(search.get('awaiting_first_response_limit')).toBe('50')
  fireEvent.click(
    noReviewerGroup().getByRole('button', { name: 'Предыдущая страница' }),
  )
  expect(
    await noReviewerGroup().findByRole('link', { name: /!42/ }),
  ).toBeVisible()
  expect(firstResponseGroup().getByLabelText('Строк на странице')).toHaveValue(
    '50',
  )
})

it('restores wait kind, sort and cursor from the address in a fresh router', async () => {
  const first = mount()
  await screen.findByRole('link', { name: /!42/ })
  fireEvent.change(screen.getByLabelText('Вид ожидания'), {
    target: { value: 'no_reviewer' },
  })
  fireEvent.change(screen.getByLabelText('Сортировка'), {
    target: { value: 'wait' },
  })
  await noReviewerGroup().findByRole('link', { name: /!42/ })
  fireEvent.click(
    noReviewerGroup().getByRole('button', { name: 'Следующая страница' }),
  )
  await noReviewerGroup().findByRole('link', { name: /!43/ })
  const { pathname, search } = first.router.state.location
  first.dispose()

  const second = mount({ path: pathname + search })
  expect(await screen.findByRole('link', { name: /!43/ })).toBeVisible()
  expect(screen.getByLabelText('Сортировка')).toHaveValue('wait')
  expect(screen.getByLabelText('Вид ожидания')).toHaveValue('no_reviewer')
  expect(
    screen.queryByRole('region', { name: 'Ждёт первого ответа человека' }),
  ).not.toBeInTheDocument()
  const call = second.requests.find(
    (url) => url.pathname === '/api/v1/merge-requests',
  )
  expect(call?.searchParams.get('sort')).toBe('wait')
  expect(call?.searchParams.get('cursor')).toBe('next-reviewers')
})

it('retries a failed group on the same address and keeps the other results available', async () => {
  let attempts = 0
  const { router } = mount({
    list: (url) => {
      if (
        url.searchParams.get('review_wait') === 'no_reviewer' &&
        ++attempts === 1
      )
        return problemResponse('internal_error', 500)
      return readQueue(url)
    },
  })
  const group = within(
    await screen.findByRole('region', { name: 'Нет ревьюера' }),
  )
  const retry = await group.findByRole('button', {
    name: 'Повторить',
  })
  expect(
    await firstResponseGroup().findByRole('link', { name: /!44/ }),
  ).toBeVisible()
  fireEvent.click(retry)
  expect(
    await noReviewerGroup().findByRole('link', { name: /!42/ }),
  ).toBeVisible()
  expect(router.state.location.pathname + router.state.location.search).toBe(
    queuePath,
  )
})

it('keeps the queue available when the team report is forbidden', async () => {
  mount({ readReport: () => problemResponse('forbidden', 403) })
  expect(await screen.findByText('Отчёт команды недоступен')).toBeVisible()
  expect(screen.queryByText('A07 · p50')).not.toBeInTheDocument()
  expect(await screen.findByRole('link', { name: /!42/ })).toBeVisible()
})

it('returns from the task project queue to the original card and restores focus after changing sort', async () => {
  const scrollTo = vi.spyOn(window, 'scrollTo').mockReturnValue(undefined)
  const path = taskAnalysisLinks.taskDetail(taskId, period)
  const { router, requests } = mount({ path })
  const queueLink = await screen.findByRole('link', {
    name: 'Очередь ревью проекта',
  })
  queueLink.focus()
  fireEvent.click(queueLink)
  await screen.findByRole('link', { name: /!42/ })
  expect(router.state.location.pathname).toBe(routePaths.mergeRequestQueue)
  expect(
    requests
      .find((url) => url.pathname === '/api/v1/merge-requests')
      ?.searchParams.get('project_id'),
  ).toBe(projectId)
  fireEvent.change(screen.getByLabelText('Сортировка'), {
    target: { value: 'wait' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Назад' }))
  expect(
    await screen.findByRole('heading', { name: new RegExp(taskTitle) }),
  ).toBeVisible()
  expect(router.state.location.pathname + router.state.location.search).toBe(
    path,
  )
  const restoredLink = await screen.findByRole('link', {
    name: 'Очередь ревью проекта',
  })
  await waitFor(() => {
    expect(restoredLink).toHaveFocus()
  })
  expect(scrollTo).toHaveBeenCalled()
})
