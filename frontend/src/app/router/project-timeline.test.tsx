import {
  act,
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
import {
  currentUser,
  jsonResponse,
  lateTimelineEvent,
  problemResponse,
  projects,
  teams,
} from '../../../test/fixtures'
import {
  mrReference,
  pipelineReference,
  taskDetail,
  taskHistory,
  taskId,
  taskReference,
  taskTitle,
} from '../../../test/task-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const [project] = projects.data
if (!project) throw new Error('Expected the project fixture')
const projectId = project.id
const projectTimeline = '/api/v1/projects/' + projectId + '/timeline'
const date = '2026-10-02'
const origin = '/timeline?project_id=' + projectId + '&date=' + date
const bounds = {
  connection_id: '0193a5b0-1111-7000-8000-000000000009',
  provider: 'gitlab',
  history_from: '2026-10-01T00:00:00Z',
  completeness: 'complete',
} satisfies NonNullable<Schema['TimelinePage']['sources']>[number]
const taskEvent = { ...lateTimelineEvent, object: taskReference }
const botEvent = {
  ...taskEvent,
  id: '0193a5b0-e1e1-7000-8000-000000000002',
  object: mrReference,
  event_kind: 'ready',
  event_time: '2026-10-02T10:00:00Z',
  observed_at: '2026-10-02T10:01:00Z',
  observed_via: 'webhook',
  late: false,
  actor: { kind: 'bot', display_name: 'Бот ревью', account_id: null },
} satisfies Schema['TimelineEvent']
const systemEvent = {
  ...botEvent,
  id: '0193a5b0-e1e1-7000-8000-000000000003',
  object: pipelineReference,
  event_time: '2026-10-02T08:00:00Z',
  event_kind: 'pipeline_started',
  actor: { kind: 'system', display_name: 'GitLab Runner', account_id: null },
} satisfies Schema['TimelineEvent']
const unknownEvent = {
  ...taskEvent,
  id: '0193a5b0-e1e1-7000-8000-000000000004',
  event_time: '2026-10-02T07:00:00Z',
  event_kind: 'comment',
  actor: { kind: 'unknown', display_name: null, account_id: null },
} satisfies Schema['TimelineEvent']
const events = [botEvent, taskEvent, systemEvent, unknownEvent]
const emptyPage: Schema['TimelinePage'] = {
  data: [],
  history_from: bounds.history_from,
  sources: [bounds],
  next_cursor: null,
  access_limited: false,
  access_notes: [],
}
const revisionId = '0193a5b0-e1e1-7000-8000-000000000009'
const revision: Schema['Revision'] = {
  id: revisionId,
  series_id: '0193a5b0-e1e1-7000-8000-000000000010',
  revision_no: 2,
  knowledge_seq: 4,
  status: 'superseded',
  as_of: '2026-10-03T00:00:00Z',
  timezone: 'UTC',
  published_at: '2026-10-03T00:05:00Z',
  superseded_at: '2026-10-08T00:05:00Z',
  loading: false,
  window: { from: '2026-10-01T00:00:00Z', to: '2026-10-03T00:00:00Z' },
  irreproducible_reason: null,
  definitions: [],
  inputs: [],
  source_watermarks: [],
}

const disposers: (() => void)[] = []
afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
  localStorage.clear()
})

function mount(
  path = origin,
  respond: (query: URLSearchParams) => Response = (query) =>
    jsonResponse({
      ...emptyPage,
      data: events.filter(
        ({ actor, object }) =>
          (!query.has('actor_kind') ||
            query.get('actor_kind') === actor.kind) &&
          (!query.has('object_kind') ||
            query.get('object_kind') === object.kind),
      ),
    } satisfies Schema['TimelinePage']),
) {
  const requests = vi.fn(respond)
  const runtime = createAppRuntime((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(
          jsonResponse({
            ...currentUser,
            actions: [...currentUser.actions, 'tasks.read', 'report.read'],
            areas: { ...currentUser.areas, projects: projects.data },
          }),
        )
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case projectTimeline:
        return Promise.resolve(requests(url.searchParams))
      case '/api/v1/revisions/' + revisionId:
        return Promise.resolve(jsonResponse(revision))
      case '/api/v1/tasks/' + taskId:
        return Promise.resolve(jsonResponse(taskDetail))
      case '/api/v1/tasks/' + taskId + '/events':
        return Promise.resolve(jsonResponse(taskHistory))
      default:
        throw new Error('Unexpected request ' + url.pathname)
    }
  })
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [path],
  })
  const view = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  disposers.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { router, requests, queryClient: runtime.queryClient }
}

it('applies the People and object filters through the URL and HTTP query', async () => {
  const { router, requests } = mount()
  expect(await screen.findByText('Бот ревью')).toBeVisible()
  expect(screen.getByText('Система')).toBeVisible()
  expect(screen.getByText('Вид участника неизвестен')).toBeVisible()

  fireEvent.change(screen.getByLabelText('Вид участника'), {
    target: { value: 'human' },
  })
  fireEvent.change(screen.getByLabelText('Вид объекта'), {
    target: { value: 'task' },
  })
  expect(screen.getByText('Бот ревью')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Применить' }))

  expect(await screen.findByText('Человек')).toBeVisible()
  expect(screen.queryByText('Бот')).not.toBeInTheDocument()
  expect(screen.queryByText('Система')).not.toBeInTheDocument()
  expect(screen.queryByText('Вид участника неизвестен')).not.toBeInTheDocument()
  const search = new URLSearchParams(router.state.location.search)
  expect(search.get('actor_kind')).toBe('human')
  expect(search.get('object_kind')).toBe('task')
  expect(Object.fromEntries(requests.mock.lastCall?.[0] ?? [])).toMatchObject({
    actor_kind: 'human',
    object_kind: 'task',
    from: '2026-10-02T00:00:00.000Z',
    to: '2026-10-03T00:00:00.000Z',
  })
  const filters = within(
    screen.getByRole('group', { name: 'Применённые фильтры' }),
  )
  expect(filters.getByText('Люди')).toBeVisible()
  expect(filters.getByText('Задачи')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Сбросить фильтры' }))
  expect(await screen.findByText('Бот ревью')).toBeVisible()
})

it('keeps late and backfilled events at event time and links each object to its source', async () => {
  mount()
  const list = within(
    await screen.findByRole('list', { name: 'События по времени' }),
  )
  expect(
    list.getAllByRole('button').map((button) => button.textContent),
  ).toEqual([
    'Готов к ревью',
    'Изменение поля',
    'Запуск pipeline',
    'Комментарий',
  ])
  const item = list
    .getByRole('button', { name: 'Изменение поля' })
    .closest('li')
  if (!item) throw new Error('Expected an event list item')
  const row = within(item)
  expect(row.getByText('Получено с опозданием')).toBeVisible()
  expect(row.getByText('Загружено с историей')).toBeVisible()
  expect(row.getByText(/02\.10\.2026.*09:00:00 UTC$/)).toBeVisible()
  expect(row.getByText(/06\.10\.2026.*00:00:00 UTC$/)).toBeVisible()
  expect(row.getByRole('link', { name: 'Открыть в Jira DC' })).toHaveAttribute(
    'href',
    taskReference.source_url,
  )
  expect(list.getByRole('link', { name: 'Открыть в GitLab' })).toHaveAttribute(
    'href',
    mrReference.source_url,
  )
  expect(
    list.getByRole('link', { name: 'Открыть в GitLab CI' }),
  ).toHaveAttribute('href', pipelineReference.source_url)
})

it('names the source and its boundary for an older date, then shows a covered empty day', async () => {
  mount(origin.replace(date, '2026-09-01'), () => jsonResponse(emptyPage))
  expect(
    await screen.findByRole('status', {
      name: 'История за выбранный период не загружена',
    }),
  ).toBeVisible()
  const coverage = within(
    screen.getByRole('region', { name: 'История по источникам' }),
  )
  expect(coverage.getByText('GitLab')).toBeVisible()
  expect(coverage.getByText(/01\.10\.2026.*UTC$/)).toBeVisible()
  expect(
    screen.queryByText('Нет событий за выбранный период'),
  ).not.toBeInTheDocument()

  fireEvent.change(screen.getByLabelText(/^Дата$/u), {
    target: { value: date },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
  expect(
    await screen.findByRole('status', {
      name: 'Нет событий за выбранный период',
    }),
  ).toBeVisible()
})

it('discloses the field, before and after values, actor, clocks and observation method', async () => {
  mount()
  const opener = await screen.findByRole('button', { name: 'Изменение поля' })
  expect(opener).toHaveAttribute('aria-expanded', 'false')
  fireEvent.click(opener)
  expect(opener).toHaveAttribute('aria-expanded', 'true')
  const details = within(
    screen.getByRole('region', { name: 'Подробности события' }),
  )
  for (const value of [
    'status',
    'Open',
    'In Progress',
    'Анна Смирнова',
    'Загрузка истории',
  ]) {
    expect(details.getByText(value)).toBeVisible()
  }
  expect(details.getByText(/02\.10\.2026.*09:00:00 UTC$/)).toBeVisible()
  expect(details.getByText(/06\.10\.2026.*00:00:00 UTC$/)).toBeVisible()
  expect(
    details.getByRole('link', { name: 'Открыть в Jira DC' }),
  ).toHaveAttribute('href', taskReference.source_url)
})

it.each(['link', 'history', 'cold-cache'] as const)(
  'returns by %s to the focused event on an appended page',
  async (way) => {
    const path = origin + '&actor_kind=human&object_kind=task'
    const { router, queryClient } = mount(path, (query) => {
      const cursor = query.get('cursor') ?? ''
      switch (cursor) {
        case 'oldest':
          return jsonResponse({ ...emptyPage, data: [taskEvent] })
        case 'older':
          return jsonResponse({
            ...emptyPage,
            data: [
              {
                ...taskEvent,
                id: systemEvent.id,
                event_kind: 'link_change',
                event_time: '2026-10-02T10:00:00Z',
              },
            ],
            next_cursor: 'oldest',
          })
        default:
          return jsonResponse({
            ...emptyPage,
            data: [
              {
                ...taskEvent,
                id: botEvent.id,
                event_kind: 'comment',
                event_time: '2026-10-02T11:00:00Z',
              },
            ],
            next_cursor: 'older',
          })
      }
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Ещё' }))
    await screen.findByRole('button', { name: 'Изменение связи' })
    fireEvent.click(screen.getByRole('button', { name: 'Ещё' }))
    const opener = await screen.findByRole('button', { name: 'Изменение поля' })
    opener.focus()
    fireEvent.click(opener)
    fireEvent.click(
      screen.getByRole('link', { name: 'Открыть карточку задачи' }),
    )
    expect(
      await screen.findByRole('heading', { name: taskTitle, level: 1 }),
    ).toBeVisible()
    if (way === 'cold-cache') {
      queryClient.removeQueries({ queryKey: ['projects', 'timeline-pages'] })
    }
    if (way === 'link')
      fireEvent.click(screen.getByRole('link', { name: 'Назад' }))
    else await act(() => router.navigate(-1))

    const restored = await screen.findByRole('button', {
      name: 'Изменение поля',
    })
    expect(restored).toHaveFocus()
    expect(restored).toHaveAttribute('aria-expanded', 'true')
    expect(router.state.location.pathname + router.state.location.search).toBe(
      path,
    )
  },
)

it('requests a historical revision and labels the knowledge cut with its date', async () => {
  const { requests } = mount(origin + '&revision_id=' + revisionId, () =>
    jsonResponse({
      ...emptyPage,
      data: [taskEvent],
    } satisfies Schema['TimelinePage']),
  )
  expect(await screen.findByText('Срез знания по ревизии')).toBeVisible()
  expect(await screen.findByText(/03\.10\.2026.*UTC$/)).toBeVisible()
  expect(requests.mock.lastCall?.[0].get('revision_id')).toBe(revisionId)
})

it('keeps visible rows during a cursor retry and marks limited access without a hidden total', async () => {
  let failed = false
  mount(origin, (query) => {
    if (!query.has('cursor'))
      return jsonResponse({
        ...emptyPage,
        data: [botEvent],
        next_cursor: 'older',
        access_limited: true,
      } satisfies Schema['TimelinePage'])
    if (!failed) {
      failed = true
      return problemResponse('internal_error', 500)
    }
    return jsonResponse({
      ...emptyPage,
      data: [taskEvent],
    } satisfies Schema['TimelinePage'])
  })
  fireEvent.click(await screen.findByRole('button', { name: 'Ещё' }))
  expect(
    await screen.findByText('Не удалось загрузить следующие события'),
  ).toBeVisible()
  expect(screen.getByText('Бот ревью')).toBeVisible()
  expect(screen.getByText('Есть закрытые результаты.')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
  expect(await screen.findByText('Получено: 2')).toBeVisible()
})

it('validates the URL date and sends UTC instants for an explicitly chosen interval', async () => {
  const { router, requests } = mount(origin.replace(date, '2026-02-30'), () =>
    jsonResponse(emptyPage),
  )
  expect(await screen.findByText('Укажите существующую дату.')).toBeVisible()
  expect(requests).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('Период'), {
    target: { value: 'interval' },
  })
  fireEvent.change(screen.getByLabelText('С (включительно)'), {
    target: { value: '2026-10-02T09:00' },
  })
  fireEvent.change(screen.getByLabelText('До (исключительно)'), {
    target: { value: '2026-10-02T10:00' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
  await waitFor(() => {
    expect(requests).toHaveBeenCalledTimes(1)
  })
  const search = new URLSearchParams(router.state.location.search)
  expect(search.has('date')).toBe(false)
  expect(search.get('from')).toBe('2026-10-02T09:00:00.000Z')
  expect(requests.mock.lastCall?.[0].get('to')).toBe('2026-10-02T10:00:00.000Z')
})

it('hides the already loaded events when the project access is revoked', async () => {
  mount(origin, (query) =>
    query.has('cursor')
      ? problemResponse('forbidden', 403)
      : jsonResponse({
          ...emptyPage,
          data: [botEvent],
          next_cursor: 'older',
        } satisfies Schema['TimelinePage']),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Ещё' }))
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(screen.queryByText('Бот ревью')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('region', { name: 'История по источникам' }),
  ).not.toBeInTheDocument()
})
