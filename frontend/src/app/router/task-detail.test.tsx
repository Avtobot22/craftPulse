import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import type { RouteObject } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import type { components, OpenCodes } from '@/shared/api'
import { routePaths, taskAnalysisLinks } from '@/shared/config'
import { TaskDetailLink } from '@/shared/ui'
import {
  currentUser,
  jsonResponse,
  lateTimelineEvent,
  problemResponse,
  teams,
} from '../../../test/fixtures'
import { mergeRequest } from '../../../test/task-changes-fixtures'
import {
  mrReference,
  pipelineReference,
  readyAt,
  readyEvent,
  taskDetail,
  taskHistory,
  taskId,
  taskPage,
  taskReference,
  taskTitle,
} from '../../../test/task-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
type Task = OpenCodes<Schema['TaskDetail']>
type Observation = Task['observations'][number]
const detailPath = taskAnalysisLinks.taskDetail(taskId)
const cleanups: (() => void)[] = []
function addSourceOpener(routes: RouteObject[], path: string): void {
  for (const route of routes) {
    if (route.path === path) {
      // The test supplies the source element, so no lazy route module is needed.
      delete route.lazy
      route.element = (
        <TaskDetailLink taskId={taskId}>
          Открыть задачу из источника
        </TaskDetailLink>
      )
    }
    if (route.children) addSourceOpener(route.children, path)
  }
}

const revision = '0193a5b0-7777-7000-8000-000000000013'

function mount({
  path = detailPath,
  sourceRoute,
  detail = taskDetail,
  loadTask = () => jsonResponse(detail),
  loadHistory = () => jsonResponse(taskHistory),
}: {
  path?: string
  sourceRoute?: string
  detail?: Task
  loadTask?: () => Response
  loadHistory?: (url: URL) => Response
} = {}) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(jsonResponse(currentUser))
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case '/api/v1/tasks':
        return Promise.resolve(jsonResponse(taskPage))
      case `/api/v1/tasks/${taskId}`:
        return Promise.resolve(loadTask())
      case `/api/v1/merge-requests/${mrReference.id}`:
        return Promise.resolve(jsonResponse(mergeRequest))
      case `/api/v1/tasks/${taskId}/events`:
        return Promise.resolve(loadHistory(url))
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
  const runtime = createAppRuntime(fetcher)
  const routes = createAppRoutes(runtime)
  if (sourceRoute) addSourceOpener(routes, sourceRoute)
  const router = createMemoryRouter(routes, {
    initialEntries: [path],
  })
  const view = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  cleanups.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { router, fetcher }
}

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

it('shows the ready MR, the original ready event, and the GitLab reviewer action', async () => {
  mount()
  const observations = await screen.findByRole(
    'region',
    { name: 'Наблюдения' },
    { timeout: 3_000 },
  )
  const facts = within(observations)
  expect(
    facts.getByRole('heading', {
      name: 'MR готов к ревью, ревьюер не назначен',
    }),
  ).toBeVisible()
  expect(
    facts.getByRole('link', { name: '!42 · Перенос отчёта · GitLab' }),
  ).toHaveAttribute('href', mrReference.source_url)
  expect(
    await facts.findByRole('link', { name: 'Готов к ревью · GitLab' }),
  ).toHaveAttribute('href', readyEvent.object.source_url)
  expect(
    facts.getByRole('link', { name: 'Назначить ревьюера в GitLab' }),
  ).toHaveAttribute('href', mrReference.source_url)
  expect(screen.getByRole('link', { name: taskTitle })).toHaveAttribute(
    'href',
    taskDetail.source_url,
  )
  expect(screen.getByText('В работе · In Progress')).toBeVisible()
  expect(screen.getByText('Платформа', { selector: 'dd' })).toBeVisible()
})

it('finds ready evidence on a later page without opening more history', async () => {
  const newer = {
    ...lateTimelineEvent,
    event_time: '2026-11-07T12:00:00Z',
    observed_at: '2026-11-07T12:01:00Z',
  }
  const { fetcher } = mount({
    loadHistory: (url) => {
      const cursor = url.searchParams.get('cursor')
      return jsonResponse({
        ...taskHistory,
        ...(cursor === 'older'
          ? { data: [readyEvent], next_cursor: 'unrelated' }
          : { data: [newer], next_cursor: 'older' }),
      } satisfies Schema['TimelinePage'])
    },
  })
  const facts = within(
    await screen.findByRole('region', { name: 'Наблюдения' }),
  )
  expect(
    await facts.findByRole('link', { name: 'Готов к ревью · GitLab' }),
  ).toHaveAttribute('href', readyEvent.object.source_url)
  expect(fetcher).toHaveBeenCalledWith(
    `/api/v1/tasks/${taskId}/events?cursor=older`,
    expect.anything(),
  )
  expect(fetcher).not.toHaveBeenCalledWith(
    `/api/v1/tasks/${taskId}/events?cursor=unrelated`,
    expect.anything(),
  )
})

it('continues looking for the ready event when a reviewer change shares its timestamp', async () => {
  mount({
    loadHistory: (url) =>
      jsonResponse({
        ...taskHistory,
        ...(url.searchParams.get('cursor') === 'ready'
          ? { data: [readyEvent] }
          : {
              data: [
                {
                  ...readyEvent,
                  id: lateTimelineEvent.id,
                  event_kind: 'reviewer_removed',
                },
              ],
              next_cursor: 'ready',
            }),
      } satisfies Schema['TimelinePage']),
  })
  const facts = within(
    await screen.findByRole('region', { name: 'Наблюдения' }),
  )
  expect(
    await facts.findByRole('link', { name: 'Готов к ревью · GitLab' }),
  ).toHaveAttribute('href', readyEvent.object.source_url)
})

it.each([
  [
    'awaiting_first_response',
    'request_review',
    'MR ждёт первого ответа человека',
    mrReference,
  ],
  ['ci_failed', 'check_pipeline', 'Pipeline упал', pipelineReference],
  ['ci_running', 'check_pipeline', 'Pipeline выполняется', pipelineReference],
  [
    'ci_passed_after_retry',
    'check_pipeline',
    'Pipeline успешен после повторных попыток',
    pipelineReference,
  ],
  [
    'missing_report',
    'restore_report',
    'Ожидаемый отчёт проверки не получен',
    pipelineReference,
  ],
  [
    'new_finding',
    'review_finding',
    'Новая находка в связанном MR',
    { ...mrReference, kind: 'finding' },
  ],
  ['blocked', 'resolve_blocker', 'Задача заблокирована', taskReference],
  ['over_target', 'review_target', 'Превышен целевой срок', taskReference],
] as const satisfies readonly (readonly [
  Schema['TaskDetail']['observations'][number]['kind'],
  Schema['TaskDetail']['observations'][number]['action'],
  string,
  Schema['ObjectRef'],
])[])(
  'labels the %s observation using the contract code',
  async (kind, action, label, basis) => {
    mount({
      detail: {
        ...taskDetail,
        observations: [{ kind, action, since: readyAt, basis: [basis] }],
      },
    })
    const observations = within(
      await screen.findByRole('region', { name: 'Наблюдения' }),
    )
    expect(observations.getByRole('heading', { name: label })).toBeVisible()
    expect(
      observations.getByRole('link', { name: new RegExp(basis.display_key) }),
    ).toHaveAttribute('href', basis.source_url)
    if (basis.kind === 'finding')
      expect(observations.getByText('(косвенная связь через MR)')).toBeVisible()
  },
)

it.each([
  [
    { kind: 'unknown', action: 'clarify_reason', basis: [taskReference] },
    'Что задерживает задачу?',
  ],
  [
    {
      kind: 'no_merge_request',
      action: 'link_merge_request',
      basis: [taskReference],
    },
    'Есть ли MR по задаче?',
  ],
  [
    {
      kind: 'unknown',
      action: 'clarify_reason',
      question: 'reviewer_assigned',
      basis: [mrReference],
    },
    'Назначен ли ревьюер в MR !42?',
  ],
] satisfies [Omit<Observation, 'since'>, string][])(
  'shows the missing question %j without diagnosing an unassigned reviewer',
  async (observation, question) => {
    mount({
      detail: {
        ...taskDetail,
        observations: [{ ...observation, since: null }],
      },
    })
    const observations = within(
      await screen.findByRole('region', { name: 'Наблюдения' }),
    )
    expect(observations.getByText(question)).toBeVisible()
    expect(observations.getByText('Проверенные факты')).toBeVisible()
    expect(
      observations.queryByText(/ревьюер не назначен/i),
    ).not.toBeInTheDocument()
  },
)

it('uses the generic template for a new observation code', async () => {
  mount({
    detail: {
      ...taskDetail,
      observations: [
        {
          kind: 'new_observation_code',
          since: null,
          basis: [mrReference],
          action: 'check_new_signal',
        },
      ],
    },
  })
  const observations = within(
    await screen.findByRole('region', { name: 'Наблюдения' }),
  )
  expect(
    observations.getByRole('heading', { name: 'new_observation_code' }),
  ).toBeVisible()
  expect(
    observations.getByRole('link', { name: '!42 · Перенос отчёта · GitLab' }),
  ).toBeVisible()
  expect(observations.getByText('check_new_signal')).toBeVisible()
})

it('shows an open blocking interval with its start event and metric passport', async () => {
  const blockEvent: Schema['TimelineEvent'] = {
    ...lateTimelineEvent,
    field: 'customfield_10020',
    after_value: 'Impediment',
    object: {
      ...taskReference,
      source_url: taskReference.source_url + '#change-23',
    },
  }
  mount({
    path: taskAnalysisLinks.taskDetail(taskId, { revision_id: revision }),
    detail: {
      ...taskDetail,
      blocked_intervals: [{ from: blockEvent.event_time, to: null }],
    },
    loadHistory: (url) =>
      jsonResponse({
        ...taskHistory,
        ...(url.searchParams.get('cursor') === 'block-start'
          ? { data: [blockEvent] }
          : { data: [readyEvent], next_cursor: 'block-start' }),
      } satisfies Schema['TimelinePage']),
  })
  const blocks = within(
    await screen.findByRole('region', { name: 'Блокировки' }),
  )
  expect(blocks.getByText('Открыта')).toBeVisible()
  expect(blocks.getByText(/\d+ д \d+ ч/)).toBeVisible()
  expect(
    await blocks.findByRole('link', {
      name: 'Событие начала блокировки в Jira DC',
    }),
  ).toHaveAttribute('href', blockEvent.object.source_url)
  expect(blocks.getByRole('link', { name: 'Как рассчитано' })).toHaveAttribute(
    'href',
    `/reports/revisions/${revision}/metrics/T15`,
  )
})

it('describes missing start and target without a zero duration', async () => {
  mount({
    detail: {
      ...taskDetail,
      age_seconds: null,
      target: null,
      over_target: null,
    },
  })
  const timing = within(
    await screen.findByRole('region', { name: 'Возраст и срок' }),
  )
  expect(timing.getByText('Начало эпизода неизвестно')).toBeVisible()
  expect(timing.getByText('Целевой срок не задан')).toBeVisible()
  expect(
    timing.queryByRole('link', { name: 'Как рассчитано' }),
  ).not.toBeInTheDocument()
})

it('filters human and bot events across cursor pages while keeping their source links', async () => {
  const botEvent: Schema['TimelineEvent'] = {
    ...lateTimelineEvent,
    id: '0193a5b0-e1e1-7000-8000-000000000039',
    object: {
      ...pipelineReference,
      source_url: pipelineReference.source_url + '#job-23',
    },
    event_kind: 'pipeline_finished',
    field: null,
    before_value: null,
    after_value: null,
    event_time: '2026-10-03T09:00:00Z',
    actor: { account_id: null, kind: 'bot', display_name: 'CI bot' },
  }
  const { fetcher } = mount({
    loadHistory: (url) =>
      jsonResponse({
        ...taskHistory,
        ...(url.searchParams.get('cursor') === 'page-two'
          ? { data: [botEvent], next_cursor: null }
          : { data: [lateTimelineEvent, readyEvent], next_cursor: 'page-two' }),
      } satisfies Schema['TimelinePage']),
  })
  const history = within(
    await screen.findByRole('region', { name: 'История задачи' }),
  )
  const filter = await history.findByRole('combobox', { name: 'Вид участника' })
  expect(history.getByText(/Граница истории/)).toBeVisible()
  expect(
    history.getByRole('link', { name: 'Открыть в Jira DC' }),
  ).toHaveAttribute('href', lateTimelineEvent.object.source_url)
  expect(
    history.getByRole('link', { name: 'Открыть в GitLab' }),
  ).toHaveAttribute('href', readyEvent.object.source_url)
  fireEvent.change(filter, { target: { value: 'bot' } })
  expect(
    history.queryByText('Человек', { selector: 'span' }),
  ).not.toBeInTheDocument()
  fireEvent.click(history.getByRole('button', { name: 'Загрузить ещё' }))
  expect(await history.findByText('Бот', { selector: 'span' })).toBeVisible()
  expect(
    history.getByRole('link', { name: 'Открыть в GitLab CI' }),
  ).toHaveAttribute('href', botEvent.object.source_url)
  expect(fetcher).toHaveBeenCalledWith(
    `/api/v1/tasks/${taskId}/events?cursor=page-two`,
    expect.anything(),
  )
  fireEvent.change(filter, { target: { value: '' } })
  const rows = history.getAllByRole('listitem')
  expect(
    rows.map((row) => within(row).getByRole('link').getAttribute('href')),
  ).toEqual([
    readyEvent.object.source_url,
    botEvent.object.source_url,
    lateTimelineEvent.object.source_url,
  ])
})

it('returns from the card to the same list filters, cursor, and focused opener', async () => {
  const source = '/tasks?sort=-age&status_meaning=active&cursor=page-one'
  const { router } = mount({ path: source })
  const opener = await screen.findByRole('link', { name: taskTitle })
  opener.focus()
  fireEvent.click(opener, { detail: 0 })
  expect(await screen.findByRole('heading', { name: taskTitle })).toBeVisible()
  const back = screen.getByRole('link', { name: 'Назад' })
  back.focus()
  fireEvent.click(back, { detail: 0 })
  const restored = await screen.findByRole('link', { name: taskTitle })
  await waitFor(() => {
    expect(restored).toHaveFocus()
  })
  expect(router.state.location.pathname + router.state.location.search).toBe(
    source,
  )
})

it('redirects a closed task to UI-04 without showing the task title', async () => {
  const { router, fetcher } = mount({
    loadTask: () => problemResponse('not_found', 404),
  })
  const unavailable = await screen.findByRole('heading', {
    name: 'Объект или область недоступны',
  })
  expect(router.state.location.pathname).toBe('/access-denied')
  await waitFor(() => {
    expect(unavailable).toHaveFocus()
  })
  expect(screen.queryByText(taskDetail.title ?? '')).not.toBeInTheDocument()
  expect(fetcher).not.toHaveBeenCalledWith(
    `/api/v1/tasks/${taskId}/events`,
    expect.anything(),
  )
})

it('returns from UI-04 to the originating task link after the API denies access', async () => {
  const source = '/tasks?sort=key'
  const { router } = mount({
    path: source,
    loadTask: () => problemResponse('forbidden', 403),
  })
  fireEvent.click(await screen.findByRole('link', { name: taskTitle }))
  const unavailable = await screen.findByRole('heading', {
    name: 'Объект или область недоступны',
  })
  await waitFor(() => {
    expect(unavailable).toHaveFocus()
  })
  expect(
    screen.queryByRole('heading', { name: taskTitle }),
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('link', { name: 'Назад' }))
  const opener = await screen.findByRole('link', { name: taskTitle })
  await waitFor(() => {
    expect(opener).toHaveFocus()
  })
  expect(router.state.location.pathname + router.state.location.search).toBe(
    source,
  )
})

it.each([
  [routePaths.teamReport, '/reports/team?date=2026-10-01#tasks'],
  [
    routePaths.metricCohort,
    `/reports/revisions/${revision}/metrics/T14/points/2026-10-01?cursor=page-two#row`,
  ],
  [
    routePaths.projectTimeline,
    '/timeline?date=2026-10-01&actor_kind=bot#history',
  ],
])(
  'restores the link from %s with its URL and focus',
  async (sourceRoute, source) => {
    const { router } = mount({ path: source, sourceRoute })
    const label = 'Открыть задачу из источника'
    fireEvent.click(await screen.findByRole('link', { name: label }))
    await screen.findByRole('heading', { name: taskTitle })
    fireEvent.click(screen.getByRole('link', { name: 'Назад' }))
    expect(await screen.findByRole('link', { name: label })).toHaveFocus()
    const { pathname, search, hash } = router.state.location
    expect(pathname + search + hash).toBe(source)
  },
)
