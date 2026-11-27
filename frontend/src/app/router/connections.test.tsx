import { focusManager } from '@tanstack/react-query'
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
import { routePaths } from '@/shared/config'
import {
  connectionExample,
  syncStatusExample,
} from '../../../test/admin-fixtures'
import {
  currentUser,
  jsonResponse,
  problem,
  problemResponse,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schemas = components['schemas']
type Connection = Schemas['Connection']
type SyncStatus = Schemas['SyncStatus']

const taskStatus = syncStatusExample.data_kinds[0]
if (!taskStatus) throw new Error('Expected the OpenAPI task sync example')
const healthySync = {
  ...syncStatusExample,
  data_kinds: [
    {
      ...taskStatus,
      last_outcome: 'success',
      last_error_code: null,
      failed_page_present: false,
      completeness: 'complete',
      lag_seconds: 0,
    },
  ],
} satisfies SyncStatus
const draft = {
  ...connectionExample,
  id: '0193a5b0-5555-7000-8000-000000000002',
  name: 'Черновик GitLab',
  provider: 'gitlab',
  status: 'draft',
  last_success_at: null,
} satisfies Connection
const listApiPath = '/api/v1/connections'
const settingsActions = ['connections.manage', 'sync.read', 'imports.create']
const cleanups: (() => void)[] = []

function connectionPage(data: Connection[], next_cursor: string | null = null) {
  return jsonResponse({ data, next_cursor } satisfies Schemas['ConnectionPage'])
}

function mount({
  list = () => connectionPage([connectionExample]),
  sync = () => jsonResponse(healthySync),
  actions = settingsActions,
  path = routePaths.connections,
}: {
  list?: (search: URLSearchParams) => Response
  sync?: (id: string) => Response
  actions?: string[]
  path?: string
} = {}) {
  const fetcher = vi.fn<typeof fetch>((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.example')
    if (url.pathname === '/api/v1/me')
      return Promise.resolve(
        jsonResponse({
          ...currentUser,
          actions,
          areas: { teams: [], projects: [], services: [] },
        }),
      )
    if (url.pathname === listApiPath)
      return Promise.resolve(list(url.searchParams))
    const connection = /^\/api\/v1\/connections\/([^/]+)\/sync-status$/u.exec(
      url.pathname,
    )?.[1]
    if (connection) return Promise.resolve(sync(connection))
    throw new Error('Unexpected request: ' + url.pathname)
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
    runtime.queryClient.clear()
  })
  return { router, fetcher }
}

afterEach(() => {
  for (const dispose of cleanups.splice(0)) dispose()
  focusManager.setFocused(undefined)
})

async function row(name = connectionExample.name) {
  return within(await screen.findByRole('row', { name: new RegExp(name, 'u') }))
}

function field(scope: Pick<typeof screen, 'getByText'>, label: string) {
  const value = scope.getByText(label, { selector: 'dt' }).nextElementSibling
  if (!(value instanceof HTMLElement))
    throw new Error('Missing field value: ' + label)
  return value
}

it('shows an active connection, instance, scope and separate healthy collection values', async () => {
  mount()
  const connection = await row()
  await connection.findByText('Задачи')
  expect(connection.getByText('Активно')).toBeVisible()
  expect(connection.getByText('Jira Cloud')).toBeVisible()
  expect(connection.getByText('https://example.atlassian.net')).toBeVisible()
  expect(connection.getByText('Редакция: cloud')).toBeVisible()
  expect(connection.getByText('Версия: 1001.0.0-SNAPSHOT')).toBeVisible()
  expect(connection.getByText('CORE')).toBeVisible()
  expect(field(connection, 'Фаза')).toHaveTextContent('Сбор изменений')
  expect(field(connection, 'Ошибка')).toHaveTextContent('Нет ошибок')
  expect(field(connection, 'Полнота')).toHaveTextContent('Полные данные')
  expect(field(connection, 'Отставание')).toHaveTextContent('0 ч')
  expect(field(connection, 'Страница с ошибкой')).toHaveTextContent('Нет')
  const content = within(screen.getByRole('main'))
  expect(content.getByRole('link', { name: 'Импорт' })).toHaveAttribute(
    'href',
    routePaths.importPackage,
  )
  expect(content.getByRole('link', { name: 'Журнал приёма' })).toHaveAttribute(
    'href',
    routePaths.receiptJournal,
  )
})

it('shows the timeout, last successful snapshot, partial coverage and failed-page retry', async () => {
  mount({ sync: () => jsonResponse(syncStatusExample) })
  const connection = await row()
  await connection.findByText('Источник не ответил вовремя')
  expect(field(connection, 'Ошибка')).toHaveTextContent('timeout')
  expect(field(connection, 'Полнота')).toHaveTextContent('частично')
  expect(field(connection, 'Отставание')).toHaveTextContent('1 дн. 7 ч')
  expect(
    field(connection, 'Последний успешный сбор').querySelector('time'),
  ).toHaveAttribute('datetime', taskStatus.last_success_at)
  expect(
    connection.getByText(
      'Сбор остановился на странице, повтор продолжит с неё',
    ),
  ).toBeVisible()
})

it('shows the source retry deadline and the separate administrator pause', async () => {
  const pausedUntil = '2026-11-08T04:00:00Z'
  mount({
    list: () => connectionPage([{ ...connectionExample, sync_paused: true }]),
    sync: () =>
      jsonResponse({
        ...syncStatusExample,
        paused_until: pausedUntil,
        data_kinds: [{ ...taskStatus, last_error_code: 'rate_limited' }],
      } satisfies SyncStatus),
  })
  const connection = await row()
  await connection.findByText('Источник ограничил частоту запросов')
  expect(
    connection
      .getByText(/Лимит источника, следующий запрос не раньше/u)
      .querySelector('time'),
  ).toHaveAttribute('datetime', pausedUntil)
  expect(connection.getByText('Сбор остановлен администратором')).toBeVisible()
})

it.each(['unauthorized', 'forbidden'])(
  'explains access loss after %s and how to restore it',
  async (code) => {
    mount({
      list: () =>
        connectionPage([{ ...connectionExample, status: 'access_lost' }]),
      sync: () =>
        jsonResponse({
          ...syncStatusExample,
          connection_status: 'access_lost',
          data_kinds: [{ ...taskStatus, last_error_code: code }],
        } satisfies SyncStatus),
    })
    const connection = await row()
    await connection.findByText(code)
    expect(connection.getByText('Доступ потерян')).toBeVisible()
    expect(
      connection.getByText(
        'Доступ к источнику потерян, нужна замена учётных данных',
      ),
    ).toBeVisible()
  },
)

it('shows a draft without history and opens the draft and active links', async () => {
  const { router } = mount({
    list: () => connectionPage([draft, connectionExample]),
    sync: (id) =>
      jsonResponse(
        id === draft.id
          ? ({
              connection_id: draft.id,
              connection_status: 'draft',
              paused_until: null,
              data_kinds: [],
              last_reconcile: null,
            } satisfies SyncStatus)
          : healthySync,
      ),
  })
  const draftRow = await row(draft.name)
  expect(draftRow.getByText('Черновик')).toBeVisible()
  expect(draftRow.getByText('История ещё не получена')).toBeVisible()
  const draftLink = draftRow.getByRole('link', { name: draft.name })
  expect(draftLink).toHaveAttribute(
    'href',
    routePaths.connections + '/' + draft.id,
  )
  draftLink.focus()
  expect(draftLink).toHaveFocus()
  fireEvent.click(draftLink)
  expect(
    await screen.findByRole('heading', { name: 'Настройки подключения' }),
  ).toBeVisible()
  await act(() => router.navigate(-1))
  const activeLink = (await row()).getByRole('link', {
    name: connectionExample.name,
  })
  expect(activeLink).toHaveAttribute(
    'href',
    routePaths.connections + '/' + connectionExample.id + '/sync',
  )
  activeLink.focus()
  expect(activeLink).toHaveFocus()
  fireEvent.click(activeLink)
  expect(
    await screen.findByRole('heading', { name: 'Состояние сбора', level: 1 }),
  ).toBeVisible()
})

it('offers the first connection when the list is empty', async () => {
  mount({ list: () => connectionPage([]) })
  const empty = await screen.findByRole('status', {
    name: 'Источник не настроен',
  })
  fireEvent.click(
    within(empty).getByRole('button', { name: 'Добавить подключение' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Новое подключение' }),
  ).toBeVisible()
})

it('keeps another row loaded and retries only the failed sync query', async () => {
  const broken = { ...connectionExample, id: draft.id, name: 'Второй источник' }
  const sync = vi.fn((id: string) =>
    id === broken.id
      ? problemResponse('unavailable', 503)
      : jsonResponse(healthySync),
  )
  mount({ list: () => connectionPage([broken, connectionExample]), sync })
  const brokenRow = await row(broken.name)
  const alert = await brokenRow.findByRole('alert')
  expect(alert).toHaveTextContent('Не удалось загрузить состояние сбора')
  const goodRow = await row()
  expect(await goodRow.findByText('Полные данные')).toBeVisible()
  sync.mockImplementation((id) =>
    jsonResponse({ ...healthySync, connection_id: id }),
  )
  fireEvent.click(within(alert).getByRole('button', { name: 'Повторить' }))
  expect(await brokenRow.findByText('Полные данные')).toBeVisible()
  expect(
    sync.mock.calls.filter(([id]) => id === connectionExample.id),
  ).toHaveLength(1)
})

it('shows the list correlation ID and reloads the requested page on retry', async () => {
  const list = vi.fn(() => problemResponse('unavailable', 503))
  mount({ list })
  const alert = await screen.findByRole('alert', { name: 'Ошибка' })
  expect(alert).toHaveTextContent('Не удалось загрузить список подключений')
  expect(alert).toHaveTextContent(problem('unavailable', 503).correlation_id)
  list.mockImplementation(() => connectionPage([connectionExample]))
  fireEvent.click(within(alert).getByRole('button', { name: 'Повторить' }))
  expect(await (await row()).findByText('Полные данные')).toBeVisible()
})

it('paginates at 25, restores the cursor after details and resets it on status change', async () => {
  const list = vi.fn((search: URLSearchParams) => {
    if (search.get('status') === 'draft') return connectionPage([draft])
    if (search.has('cursor')) return connectionPage([connectionExample])
    return connectionPage([connectionExample], 'opaque +/ cursor')
  })
  const { router } = mount({ list })
  const pagination = () =>
    screen.getByRole('navigation', { name: 'Страницы подключений' })
  const next = await screen.findByRole('button', { name: 'Следующая страница' })
  next.focus()
  fireEvent.click(next)
  expect(
    screen.getByRole('heading', { name: 'Подключения', level: 1 }),
  ).toHaveFocus()
  await waitFor(() => {
    expect(within(pagination()).getByRole('status')).toHaveTextContent(
      'Страница 2 · получено 1',
    )
  })
  expect(list.mock.lastCall?.[0].get('cursor')).toBe('opaque +/ cursor')
  fireEvent.click(
    (await row()).getByRole('link', { name: connectionExample.name }),
  )
  await screen.findByRole('heading', { name: 'Состояние сбора', level: 1 })
  await act(() => router.navigate(-1))
  expect(within(pagination()).getByRole('status')).toHaveTextContent(
    'Страница 2',
  )
  fireEvent.click(
    within(pagination()).getByRole('button', { name: 'Предыдущая страница' }),
  )
  await waitFor(() => {
    expect(within(pagination()).getByRole('status')).toHaveTextContent(
      'Страница 1',
    )
  })
  fireEvent.click(
    within(pagination()).getByRole('button', { name: 'Следующая страница' }),
  )
  await waitFor(() => {
    expect(within(pagination()).getByRole('status')).toHaveTextContent(
      'Страница 2',
    )
  })
  const filter = screen.getByRole('combobox', { name: 'Статус подключения' })
  filter.focus()
  fireEvent.change(filter, { target: { value: 'draft' } })
  expect(filter).toHaveFocus()
  expect((await row(draft.name)).getByText('Черновик')).toBeVisible()
  expect(router.state.location.search).toBe('?status=draft')
  expect(list.mock.lastCall?.[0].get('status')).toBe('draft')
  expect(list.mock.lastCall?.[0].has('cursor')).toBe(false)
  expect(
    list.mock.calls.every(([search]) => search.get('limit') === '25'),
  ).toBe(true)
})

it('refreshes list and sync status on tab return and remount', async () => {
  const list = vi.fn(() => connectionPage([connectionExample]))
  const sync = vi.fn(() => jsonResponse(healthySync))
  const { router } = mount({ list, sync })
  const connection = await row()
  await connection.findByText('Полные данные')
  const link = connection.getByRole('link', { name: connectionExample.name })
  link.focus()
  sync.mockImplementation(() => jsonResponse(syncStatusExample))
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  await connection.findByText('Источник не ответил вовремя')
  expect(link).toHaveFocus()
  expect(list).toHaveBeenCalledTimes(2)
  expect(sync).toHaveBeenCalledTimes(2)
  fireEvent.click(link)
  await screen.findByRole('heading', { name: 'Состояние сбора', level: 1 })
  list.mockImplementation(() =>
    connectionPage([{ ...connectionExample, name: 'Новое имя' }]),
  )
  sync.mockImplementation(() => jsonResponse(healthySync))
  await act(() => router.navigate(-1))
  expect(
    await (await row('Новое имя')).findByText('Полные данные'),
  ).toBeVisible()
  expect(list).toHaveBeenCalledTimes(3)
  expect(sync).toHaveBeenCalledTimes(3)
})

it('hides restricted actions and skips sync reads without sync.read', async () => {
  const sync = vi.fn(() => jsonResponse(healthySync))
  mount({ sync, actions: ['connections.manage'] })
  const connection = await row()
  expect(
    connection.getByText('Недостаточно прав для чтения состояния сбора'),
  ).toBeVisible()
  const content = within(screen.getByRole('main'))
  expect(
    content.queryByRole('link', { name: 'Импорт' }),
  ).not.toBeInTheDocument()
  expect(
    content.queryByRole('link', { name: 'Журнал приёма' }),
  ).not.toBeInTheDocument()
  expect(sync).not.toHaveBeenCalled()
})
