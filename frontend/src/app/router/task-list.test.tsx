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
import type { Location } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import type { components } from '@/shared/api'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  teams,
} from '../../../test/fixtures'
import {
  taskDetail,
  taskHistory,
  taskId,
  taskPage,
  taskTitle,
} from '../../../test/task-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type TaskPage = components['schemas']['TaskPage']
const cleanups: (() => void)[] = []
const teamId = taskDetail.team_id ?? ''
const unknownTask = {
  ...taskDetail,
  id: '0193a5b0-f000-7000-8000-000000000003',
  key: 'CORE-9',
  title: 'Разобрать импорт',
  status: { native_id: '9', name: 'Waiting for input', meaning: 'unknown' },
  age_seconds: null,
  size_label: null,
  over_target: null,
} satisfies components['schemas']['TaskSummary']

function mount(
  path: string | Location = '/tasks',
  load: (query: URLSearchParams) => Response = () => jsonResponse(taskPage),
) {
  const requests: URLSearchParams[] = []
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(jsonResponse(currentUser))
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case `/api/v1/teams/${teamId}/projects`:
        return Promise.resolve(jsonResponse({ data: [], next_cursor: null }))
      case '/api/v1/tasks':
        requests.push(url.searchParams)
        return Promise.resolve(load(url.searchParams))
      case `/api/v1/tasks/${taskId}`:
        return Promise.resolve(jsonResponse(taskDetail))
      case `/api/v1/tasks/${taskId}/events`:
        return Promise.resolve(jsonResponse(taskHistory))
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
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
  return { router, requests, runtime }
}

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

it('applies explicit status and deadline filters and drops the previous cursor in the URL and request', async () => {
  const { router, requests } = mount(
    `/tasks?team_id=${teamId}&cursor=older&sort=-updated_at`,
    (query) =>
      jsonResponse({
        ...taskPage,
        data:
          query.get('over_target') === 'true'
            ? [taskDetail]
            : [taskDetail, unknownTask],
      } satisfies TaskPage),
  )
  await screen.findByRole('link', { name: /CORE-9/ })
  fireEvent.change(screen.getByLabelText('Смысл статуса'), {
    target: { value: 'active' },
  })
  fireEvent.change(screen.getByLabelText('Превышение срока'), {
    target: { value: 'true' },
  })
  expect(requests).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
  await waitFor(() => {
    expect(requests).toHaveLength(2)
  })
  const expected = {
    team_id: teamId,
    sort: '-updated_at',
    status_meaning: 'active',
    over_target: 'true',
  }
  expect(
    Object.fromEntries(new URLSearchParams(router.state.location.search)),
  ).toEqual(expected)
  expect(Object.fromEntries(requests[1] ?? new URLSearchParams())).toEqual({
    ...expected,
    limit: '25',
  })
  expect(await screen.findByRole('link', { name: taskTitle })).toBeVisible()
  expect(screen.queryByRole('link', { name: /CORE-9/ })).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: 'Сбросить' }))
  await waitFor(() => {
    expect(requests).toHaveLength(3)
  })
  expect(
    Object.fromEntries(new URLSearchParams(router.state.location.search)),
  ).toEqual({
    team_id: teamId,
    sort: '-updated_at',
  })
})

it('sends sort changes to listTasks and displays the returned order and direction', async () => {
  const newer = { ...unknownTask, status: taskDetail.status, age_seconds: 3600 }
  const { router, requests } = mount(
    '/tasks?status_meaning=active&sort=key&cursor=more',
    (query) => {
      const oldestFirst = query.get('sort') === '-age'
      return jsonResponse({
        ...taskPage,
        data: oldestFirst ? [taskDetail, newer] : [newer, taskDetail],
      } satisfies TaskPage)
    },
  )
  const table = await screen.findByRole('table', { name: 'Задачи области' })
  fireEvent.click(
    within(table).getByRole('button', { name: 'Возраст эпизода' }),
  )
  const oldest = await screen.findByRole('columnheader', {
    name: /Возраст эпизода/,
  })
  expect(oldest).toHaveAttribute('aria-sort', 'descending')
  expect(screen.getAllByRole('rowheader')[0]).toHaveTextContent('CORE-15')
  expect(new URLSearchParams(router.state.location.search).get('sort')).toBe(
    '-age',
  )
  expect(requests.at(-1)?.has('cursor')).toBe(false)
  fireEvent.click(within(oldest).getByRole('button'))
  expect(await screen.findByText('По возрастанию')).toBeVisible()
  expect(requests.at(-1)?.get('sort')).toBe('age')
  expect(screen.getAllByRole('rowheader')[0]).toHaveTextContent('CORE-9')
  fireEvent.click(screen.getByRole('button', { name: 'Обновлено' }))
  expect(
    await screen.findByRole('columnheader', { name: /Обновлено/ }),
  ).toHaveAttribute('aria-sort', 'descending')
  expect(requests.at(-1)?.get('sort')).toBe('-updated_at')
  fireEvent.click(screen.getByRole('button', { name: 'Ключ и название' }))
  expect(
    await screen.findByRole('columnheader', { name: /Ключ и название/ }),
  ).toHaveAttribute('aria-sort', 'ascending')
  expect(requests.at(-1)?.get('sort')).toBe('key')
})

it('shows the original Jira status and names unknown values explicitly', async () => {
  mount('/tasks', () =>
    jsonResponse({
      ...taskPage,
      data: [
        taskDetail,
        unknownTask,
        {
          ...unknownTask,
          id: '0193a5b0-f000-7000-8000-000000000004',
          key: 'CORE-10',
          target_configured: true,
        },
      ],
    } satisfies TaskPage),
  )
  const task = await screen.findByRole('row', { name: /CORE-9/ })
  const row = within(task)
  for (const text of [
    'Waiting for input',
    'Статус не сопоставлен',
    'Без размера',
    'Начало неизвестно',
    'Срок не задан',
  ]) {
    expect(row.getByText(text)).toBeVisible()
  }
  expect(screen.getByText('Превышение неизвестно')).toBeVisible()
  const knownTask = within(screen.getByRole('row', { name: /CORE-15/ }))
  expect(knownTask.getByText('Срок превышен')).toBeVisible()
  expect(knownTask.getByText(/14:00:00 UTC/)).toHaveAttribute(
    'datetime',
    taskDetail.updated_at,
  )
  expect(
    knownTask.getByRole('link', { name: 'Открыть в Jira' }),
  ).toHaveAttribute('href', taskDetail.source_url)
})

it('moves between cursor pages, keeps the selection on direct load and announces only received rows', async () => {
  const load = (query: URLSearchParams) =>
    jsonResponse({
      ...taskPage,
      data: query.has('cursor') ? [unknownTask] : [taskDetail],
      next_cursor: query.has('cursor') ? null : 'token+/=',
    } satisfies TaskPage)
  const { router, requests } = mount(
    '/tasks?status_meaning=active&sort=-age',
    load,
  )
  await screen.findByRole('link', { name: taskTitle })
  fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
  const pageTwo = '/tasks' + router.state.location.search
  expect(await screen.findByRole('link', { name: /CORE-9/ })).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Задачи' })).toHaveFocus()
  expect(screen.getByText('Страница 2 · получено 1')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Следующая страница' }),
  ).toBeDisabled()
  expect(requests.at(-1)?.get('cursor')).toBe('token+/=')
  fireEvent.click(screen.getByRole('button', { name: 'Предыдущая страница' }))
  expect(await screen.findByRole('link', { name: taskTitle })).toBeVisible()
  expect(new URLSearchParams(router.state.location.search).has('cursor')).toBe(
    false,
  )

  for (const cleanup of cleanups.splice(0)) cleanup()
  mount(pageTwo, load)
  expect(await screen.findByRole('link', { name: /CORE-9/ })).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'К первой странице' }),
  ).toBeEnabled()
})

it.each([
  [undefined, 'Нет задач по фильтрам'],
  ['source_not_configured', 'Источник не настроен'],
  ['history_not_received', 'История ещё не получена'],
] as const)(
  'renders the empty reason %s distinctly',
  async (emptyReason, message) => {
    mount('/tasks', () =>
      jsonResponse({
        ...taskPage,
        data: [],
        ...(emptyReason ? { empty_reason: emptyReason } : {}),
      } satisfies TaskPage),
    )
    expect(await screen.findByRole('status', { name: message })).toBeVisible()
  },
)

it('marks limited access without displaying an inferred count', async () => {
  mount('/tasks', () =>
    jsonResponse({ ...taskPage, access_limited: true } satisfies TaskPage),
  )
  expect(await screen.findByText('Есть закрытые результаты.')).toBeVisible()
  expect(screen.getByRole('link', { name: taskTitle })).toBeVisible()
  expect(
    screen.getByRole('navigation', { name: 'Страницы задач' }),
  ).not.toHaveTextContent(/получено|из \d/u)
})

it('opens the task card from the row outside the title link', async () => {
  mount()
  const row = await screen.findByRole('row', { name: /CORE-15/ })
  fireEvent.click(within(row).getByText('Story'))
  expect(await screen.findByRole('heading', { name: taskTitle })).toBeVisible()
})

it.each(['link', 'history'] as const)(
  'restores cursor, filters and focused task with %s return',
  async (mode) => {
    const { router } = mount(
      '/tasks?status_meaning=active&over_target=true&sort=-age',
      (query) =>
        jsonResponse({
          ...taskPage,
          data: query.has('cursor') ? [taskDetail] : [unknownTask],
          next_cursor: query.has('cursor') ? null : 'second',
        } satisfies TaskPage),
    )
    await screen.findByRole('link', { name: /CORE-9/ })
    fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
    const opener = await screen.findByRole('link', { name: taskTitle })
    const listSearch = router.state.location.search
    opener.focus()
    fireEvent.click(opener, { detail: 0 })
    await screen.findByRole('heading', { name: taskTitle })

    if (mode === 'link') {
      const back = screen.getByRole('link', { name: 'Назад' })
      back.focus()
      fireEvent.click(back, { detail: 0 })
    } else {
      await act(() => router.navigate(-1))
    }
    const restored = await screen.findByRole('link', { name: taskTitle })
    await waitFor(() => {
      expect(restored).toHaveFocus()
    })
    expect(router.state.location.search).toBe(listSearch)
    expect(screen.getByText('Страница 2 · получено 1')).toBeVisible()
  },
)

it('restores focus on a fresh load of the same browser history entry', async () => {
  const path = '/tasks?sort=-age&status_meaning=active&cursor=second'
  const { router } = mount(path)
  fireEvent.click(await screen.findByRole('link', { name: taskTitle }))
  await screen.findByRole('heading', { name: taskTitle })
  await act(() => router.navigate(-1))
  await screen.findByRole('link', { name: taskTitle })

  const entry = router.state.location
  for (const cleanup of cleanups.splice(0)) cleanup()
  const { requests } = mount(entry)
  expect(await screen.findByRole('link', { name: taskTitle })).toHaveFocus()
  expect(requests.at(-1)?.get('cursor')).toBe('second')
})

it('removes list data once a recheck denies access', async () => {
  let allowed = true
  const { runtime } = mount('/tasks', () =>
    allowed ? jsonResponse(taskPage) : problemResponse('forbidden', 403),
  )
  await screen.findByRole('link', { name: taskTitle })
  allowed = false
  await act(() =>
    runtime.queryClient.invalidateQueries({ queryKey: ['tasks', 'list'] }),
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(screen.queryByText(taskTitle)).toBeNull()
})
