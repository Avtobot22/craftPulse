import { focusManager } from '@tanstack/react-query'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
  type BoundFunctions,
  type queries,
} from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  connectionExample,
  syncStatusExample,
} from '../../../test/admin-fixtures'
import {
  currentUser,
  jsonResponse,
  problemResponse,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const connectionPath = '/api/v1/connections/' + connectionExample.id
const updatedAt = '2026-11-08T13:00:00Z'
const taskStatus = syncStatusExample.data_kinds[0]
if (!taskStatus) throw new Error('Expected the task sync example')

const restoredTask = {
  ...taskStatus,
  last_attempt_at: updatedAt,
  last_outcome: 'success',
  last_error_code: null,
  failed_page_present: false,
  last_success_at: updatedAt,
  watermark: updatedAt,
  lag_seconds: 0,
  completeness: 'complete',
  unknown_fields: [],
  affected_metrics: [],
} satisfies Schema['SyncDataKindStatus']

const restored = {
  ...syncStatusExample,
  data_kinds: [restoredTask],
} satisfies Schema['SyncStatus']

const accepted = {
  id: '0193a5b0-a0a0-7000-8000-000000000001',
  kind: 'sync_run',
  status: 'queued',
  created_at: updatedAt,
  updated_at: updatedAt,
  result_uri: null,
  error: null,
  retry_after_seconds: 1,
} satisfies Schema['BackgroundOperation']

const reconcile = {
  started_at: updatedAt,
  finished_at: updatedAt,
  outcome: 'incomplete',
  expected_count: 3,
  observed_count: 2,
  missing_count: 1,
  changed_count: 0,
} satisfies NonNullable<Schema['SyncStatus']['last_reconcile']>

function mount({
  detail = () => jsonResponse(connectionExample, 200, { ETag: '"v1"' }),
  sync = () => jsonResponse(syncStatusExample),
  writes = vi.fn<(init: RequestInit, path: string) => Response>(),
  operations = vi.fn<() => Response>(),
  actions = ['connections.manage', 'sync.read'],
}: {
  detail?: () => Response
  sync?: () => Response | Promise<Response>
  writes?: (init: RequestInit, path: string) => Response | Promise<Response>
  operations?: () => Response
  actions?: string[]
} = {}) {
  const fetcher: typeof fetch = async (input, init) => {
    if (typeof input !== 'string') throw new Error('Expected URL')
    const url = new URL(input, 'https://craftpulse.example')
    if (init?.method && init.method !== 'GET') return writes(init, url.pathname)
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse({ ...currentUser, actions })
      case connectionPath:
        return detail()
      case connectionPath + '/sync-status':
        return sync()
      case '/api/v1/operations/' + accepted.id:
        return operations()
      case '/api/v1/connections':
        return jsonResponse({ data: [], next_cursor: null })
      default:
        throw new Error('Unexpected request ' + url.pathname)
    }
  }
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [
      routePaths.connections + '/' + connectionExample.id + '/sync',
    ],
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
}

beforeAll(() => {
  // jsdom has no native modal focus or keyboard activation.
  for (const [name, method] of Object.entries({
    showModal(this: HTMLDialogElement) {
      this.setAttribute('open', '')
    },
    close(this: HTMLDialogElement) {
      this.removeAttribute('open')
    },
  })) {
    if (!Object.hasOwn(HTMLDialogElement.prototype, name)) {
      Object.defineProperty(HTMLDialogElement.prototype, name, {
        value: method,
        configurable: true,
      })
    }
  }
})

const cleanups: (() => void)[] = []
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => {
    cleanup()
  })
  focusManager.setFocused(undefined)
  vi.useRealTimers()
})

async function taskCard() {
  return within(await screen.findByRole('article', { name: 'Задачи' }))
}

function timeAt(card: BoundFunctions<typeof queries>, label: string) {
  return card
    .getByText(label, { selector: 'dt' })
    .nextElementSibling?.querySelector('time')
}

it('retains the old success, watermark, history boundary and metric IDs alongside a failed page', async () => {
  mount()
  const card = await taskCard()
  expect(card.getByText('Источник не ответил вовремя')).toBeVisible()
  expect(card.getByText(/повтор продолжит с неё/)).toBeVisible()
  expect(card.getByText('частично')).toBeVisible()
  expect(card.getByText('T08, T12')).toBeVisible()
  expect(card.getByText('customfield_10031').tagName).toBe('CODE')
  expect(timeAt(card, 'Выборка подтверждена сверкой до')).toHaveAttribute(
    'datetime',
    taskStatus.watermark,
  )
  expect(timeAt(card, 'Последний успешный сбор')).toHaveAttribute(
    'datetime',
    taskStatus.last_success_at,
  )
  expect(timeAt(card, 'История известна с')).toHaveAttribute(
    'datetime',
    taskStatus.history_from,
  )
  expect(await screen.findByText(connectionExample.name)).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Журнал приёма сбора' }),
  ).toHaveAttribute(
    'href',
    routePaths.receiptJournal +
      '?connection_id=' +
      connectionExample.id +
      '&channel=sync',
  )
})

it('waits for the source limit and retains last success', async () => {
  const pausedUntil = '2099-11-08T13:00:00Z'
  mount({
    sync: () =>
      jsonResponse({
        ...syncStatusExample,
        paused_until: pausedUntil,
        data_kinds: [{ ...taskStatus, last_error_code: 'rate_limited' }],
      } satisfies Schema['SyncStatus']),
  })
  const card = await taskCard()
  expect(card.getByText('Источник ограничил частоту запросов')).toBeVisible()
  expect(
    screen.getByText(/Лимит источника, следующий запрос/).querySelector('time'),
  ).toHaveAttribute('datetime', pausedUntil)
  expect(timeAt(card, 'Последний успешный сбор')).toHaveAttribute(
    'datetime',
    taskStatus.last_success_at,
  )
  expect(card.getByRole('button', { name: 'Продолжить сбор' })).toBeDisabled()
})

it.each([
  ['unauthorized', 'Источник отклонил учётные данные'],
  ['forbidden', 'Недостаточно прав для чтения'],
] as const)(
  'explains %s and how to restore access before retrying',
  async (code, reason) => {
    const writes = vi.fn()
    mount({
      detail: () =>
        jsonResponse({
          ...connectionExample,
          status: 'access_lost',
          access_lost_at: updatedAt,
        } satisfies Schema['Connection']),
      sync: () =>
        jsonResponse({
          ...syncStatusExample,
          connection_status: 'access_lost',
          data_kinds: [{ ...taskStatus, last_error_code: code }],
        } satisfies Schema['SyncStatus']),
      writes,
    })
    const card = await taskCard()
    expect(card.getByText(reason)).toBeVisible()
    expect(
      timeAt(within(screen.getByRole('main')), 'Дата потери доступа'),
    ).toHaveAttribute('datetime', updatedAt)
    expect(
      timeAt(within(screen.getByRole('main')), 'Последний успешный снимок'),
    ).toHaveAttribute('datetime', connectionExample.last_success_at)
    expect(
      screen.getByText(/текущие результаты этого источника неизвестны/iu),
    ).toBeVisible()
    expect(
      screen.getByRole('link', { name: 'Заменить учётные данные' }),
    ).toHaveAttribute(
      'href',
      routePaths.connections + '/' + connectionExample.id,
    )
    expect(card.getByRole('button', { name: 'Продолжить сбор' })).toBeDisabled()
    expect(
      card.getByRole('button', { name: 'Сверить с источником' }),
    ).toBeDisabled()
    expect(writes).not.toHaveBeenCalled()
  },
)

it('shows a normal backfill and updates it on tab return', async () => {
  const sync = vi.fn(() =>
    jsonResponse({
      ...syncStatusExample,
      data_kinds: [
        {
          ...taskStatus,
          phase: 'backfill',
          last_outcome: 'success',
          last_error_code: null,
          failed_page_present: false,
        },
      ],
      last_reconcile: null,
    } satisfies Schema['SyncStatus']),
  )
  mount({ sync })
  const card = await taskCard()
  expect(card.getByText('Загрузка истории')).toBeVisible()
  expect(card.getByText('Нет ошибок')).toBeVisible()
  sync.mockImplementation(() => jsonResponse(restored))
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  expect(await card.findByText('Полные данные')).toBeVisible()
})

it('polls during backfill without overlapping a pending status read', async () => {
  const sync = vi.fn<() => Response | Promise<Response>>(() =>
    jsonResponse({
      ...restored,
      data_kinds: [{ ...restoredTask, phase: 'backfill' }],
    } satisfies Schema['SyncStatus']),
  )
  mount({ sync })
  const card = await taskCard()
  await card.findByText('Загрузка истории')
  let finish: (response: Response) => void = () => undefined
  sync.mockReturnValueOnce(
    new Promise<Response>((resolve) => {
      finish = resolve
    }),
  )
  await waitFor(
    () => {
      expect(sync).toHaveBeenCalledTimes(2)
    },
    { timeout: 6500 },
  )
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  expect(sync).toHaveBeenCalledTimes(2)
  act(() => {
    finish(jsonResponse(restored))
  })
  expect(await card.findByText('Сбор изменений')).toBeVisible()
}, 8000)

it('labels incomplete reconciliation as a gap and reports the number of objects loaded', async () => {
  mount({
    sync: () =>
      jsonResponse({
        ...syncStatusExample,
        last_reconcile: reconcile,
      } satisfies Schema['SyncStatus']),
  })
  expect(
    await screen.findByText('Пробел в истории: сверка выявила расхождения'),
  ).toBeVisible()
  expect(
    screen.getByText('Найдено в сверке (объектов загружено)')
      .nextElementSibling,
  ).toHaveTextContent('2')
  expect(
    screen.getByText('Отсутствующие объекты').nextElementSibling,
  ).toHaveTextContent('1')
})

it.each([
  ['resume', 'Продолжить сбор'],
  ['reconcile', 'Сверить с источником'],
] as const)(
  'requests %s once and shows the new success only after reconciliation',
  async (kind, label) => {
    const writes = vi.fn(() =>
      jsonResponse(accepted, 202, {
        Location: '/api/v1/operations/' + accepted.id,
      }),
    )
    const sync = vi.fn(() => jsonResponse(syncStatusExample))
    const operations = vi.fn(() => {
      sync.mockImplementation(() =>
        jsonResponse({
          ...restored,
          last_reconcile: {
            ...reconcile,
            outcome: 'complete',
            observed_count: 3,
            missing_count: 0,
          },
        } satisfies Schema['SyncStatus']),
      )
      return jsonResponse({
        ...accepted,
        status: 'succeeded',
        result_uri: connectionPath + '/sync-status',
        retry_after_seconds: null,
      } satisfies Schema['BackgroundOperation'])
    })
    mount({ writes, sync, operations })
    const card = await taskCard()
    const button = card.getByRole('button', { name: label })
    fireEvent.click(button)
    fireEvent.click(button)
    await screen.findByText('Попытка ожидает запуска')
    expect(
      card.getByRole('button', { name: 'Сверить с источником' }),
    ).toBeDisabled()
    expect(timeAt(card, 'Последний успешный сбор')).toHaveAttribute(
      'datetime',
      taskStatus.last_success_at,
    )
    expect(writes).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ kind, data_kind: 'tasks' }),
      }),
      connectionPath + '/sync-runs',
    )
    await waitFor(() => {
      expect(sync).toHaveBeenCalledTimes(2)
    })
    expect(
      await card.findByText('Полные данные', {}, { timeout: 2500 }),
    ).toBeVisible()
    expect(timeAt(card, 'Последний успешный сбор')).toHaveAttribute(
      'datetime',
      updatedAt,
    )
    expect(await screen.findByText('Сверка завершена')).toBeVisible()
  },
)

it('explains 409 state_conflict without sending another request', async () => {
  const writes = vi.fn(() => problemResponse('state_conflict', 409))
  mount({ writes })
  fireEvent.click(
    (await taskCard()).getByRole('button', { name: 'Продолжить сбор' }),
  )
  expect(
    await screen.findByText(
      'Сбор недоступен. Для восстановления доступа замените учётные данные.',
    ),
  ).toBeVisible()
  expect(writes).toHaveBeenCalledTimes(1)
})

it.each([
  [false, 'Остановить загрузку', 'Возобновить сбор'],
  [true, 'Возобновить сбор', 'Остановить загрузку'],
] as const)(
  'requires confirmation for %s paused state',
  async (paused, label, nextLabel) => {
    let connection: Schema['Connection'] = {
      ...connectionExample,
      sync_paused: paused,
    }
    const detail = () => jsonResponse(connection, 200, { ETag: '"v7"' })
    const writes = vi.fn<(init: RequestInit, path: string) => Response>(() => {
      connection = { ...connection, sync_paused: !paused }
      return jsonResponse(connection, 200, { ETag: '"v8"' })
    })
    mount({ detail, writes })
    const trigger = await screen.findByRole('button', { name: label })
    trigger.focus()
    fireEvent.click(trigger)
    const dialog = within(screen.getByRole('dialog'))
    if (!paused)
      expect(
        dialog.getByText(/сохранится как незавершённая загрузка/),
      ).toBeVisible()
    expect(writes).not.toHaveBeenCalled()
    fireEvent.click(dialog.getByRole('button', { name: 'Отмена' }))
    await waitFor(() => {
      expect(trigger).toHaveFocus()
    })
    expect(writes).not.toHaveBeenCalled()
    fireEvent.click(trigger)
    const confirm = within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Подтвердить',
    })
    confirm.focus()
    fireEvent.click(confirm)
    expect(await screen.findByRole('button', { name: nextLabel })).toBeVisible()
    expect(writes).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ sync_paused: !paused }),
      }),
      connectionPath,
    )
    const init: RequestInit | undefined = writes.mock.calls[0]?.[0]
    const headers = new Headers(init?.headers)
    expect(headers.get('If-Match')).toBe('"v7"')
    expect(headers.get('Content-Type')).toBe('application/merge-patch+json')
  },
)

it('focuses the safe disconnect action, cancels without DELETE, and returns to the list after 204', async () => {
  const writes = vi.fn(() => new Response(null, { status: 204 }))
  mount({ writes })
  const trigger = await screen.findByRole('button', {
    name: 'Отключить источник',
  })
  trigger.focus()
  fireEvent.click(trigger)
  const dialog = screen.getByRole('dialog')
  const cancel = within(dialog).getByRole('button', { name: 'Отмена' })
  expect(cancel).toHaveFocus()
  expect(
    within(dialog).getByText(/в том числе в сохранённых срезах/),
  ).toBeVisible()
  fireEvent(dialog, new Event('cancel', { cancelable: true }))
  await waitFor(() => {
    expect(trigger).toHaveFocus()
  })
  expect(writes).not.toHaveBeenCalled()
  fireEvent.click(trigger)
  const confirm = within(screen.getByRole('dialog')).getByRole('button', {
    name: 'Подтвердить',
  })
  expect(
    within(screen.getByRole('dialog')).getByRole('button', { name: 'Отмена' }),
  ).toHaveFocus()
  fireEvent.keyDown(document.activeElement ?? dialog, {
    key: 'Tab',
    shiftKey: true,
  })
  expect(confirm).toHaveFocus()
  fireEvent.click(confirm)
  expect(
    await screen.findByRole('heading', { name: 'Подключения', level: 1 }),
  ).toBeVisible()
  expect(writes).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ method: 'DELETE' }),
    connectionPath,
  )
})

it('links to organization conflicts with only sync.read', async () => {
  mount({ actions: ['sync.read'] })
  const diagnostics = await screen.findByRole('navigation', {
    name: 'Диагностика приёма',
  })
  expect(
    within(diagnostics).getByRole('link', {
      name: 'Конфликты приёма организации',
    }),
  ).toHaveAttribute('href', routePaths.ingestConflicts)
})

it('keeps sync data and its journal private without sync.read', async () => {
  const sync = vi.fn(() => jsonResponse(syncStatusExample))
  mount({ sync, actions: ['connections.manage'] })
  expect(await screen.findByText(connectionExample.name)).toBeVisible()
  expect(
    screen.getByText('Недостаточно прав для чтения состояния сбора'),
  ).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Журнал приёма сбора' }),
  ).not.toBeInTheDocument()
  expect(
    within(
      screen.getByRole('navigation', { name: 'Диагностика приёма' }),
    ).getByRole('link', { name: 'Конфликты приёма организации' }),
  ).toBeVisible()
  expect(sync).not.toHaveBeenCalled()
})

it('rereads the ETag on 412 and waits for a new confirmation', async () => {
  let version = '"v1"'
  const writes = vi.fn<(init: RequestInit, path: string) => Response>(
    (init) => {
      if (new Headers(init.headers).get('If-Match') === '"v1"') {
        version = '"v2"'
        return problemResponse('version_conflict', 412)
      }
      return jsonResponse({ ...connectionExample, sync_paused: true }, 200, {
        ETag: '"v3"',
      })
    },
  )
  mount({
    detail: () => jsonResponse(connectionExample, 200, { ETag: version }),
    writes,
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Остановить загрузку' }),
  )
  const dialog = within(screen.getByRole('dialog'))
  fireEvent.click(dialog.getByRole('button', { name: 'Подтвердить' }))
  expect(await dialog.findByText('"v2"')).toBeVisible()
  expect(writes).toHaveBeenCalledTimes(1)
  fireEvent.click(dialog.getByRole('button', { name: 'Подтвердить' }))
  await waitFor(() => {
    expect(writes).toHaveBeenCalledTimes(2)
  })
  expect(new Headers(writes.mock.lastCall?.[0].headers).get('If-Match')).toBe(
    '"v2"',
  )
})
