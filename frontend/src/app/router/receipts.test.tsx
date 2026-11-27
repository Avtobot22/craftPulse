import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { afterEach, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import { adminLinks } from '@/shared/config'
import {
  connectionExample,
  receiptExample,
  syncStatusExample,
} from '../../../test/admin-fixtures'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  projects,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const { id } = receiptExample
const receiptPath = adminLinks.receipt(id)
const receiptApi = '/api/v1/receipts/' + id
const journalApi = '/api/v1/receipts'
const recordsApi = receiptApi + '/records'
const accepted = {
  record_no: 1,
  record_kind: 'task',
  native_key: 'J1/10200/10345',
  outcome: 'accepted',
  reason_code: null,
} satisfies Schema['ReceiptRecord']
const rejected = {
  record_no: 2,
  record_kind: 'task',
  native_key: null,
  outcome: 'rejected',
  reason_code: 'missing_source_id',
} satisfies Schema['ReceiptRecord']
const adminActions = [
  'session.read',
  'imports.create',
  'receipts.read',
  'sync.read',
  'connections.manage',
  'org.read',
]
const leadActions = ['session.read', 'imports.create', 'receipts.read']
const disposers: (() => void)[] = []

function recordsPage(
  data: Schema['ReceiptRecord'][] = [],
  next_cursor: string | null = null,
) {
  return jsonResponse({
    data,
    next_cursor,
  } satisfies Schema['ReceiptRecordPage'])
}

function mount({
  path = receiptPath,
  actions = adminActions,
  respond = () => undefined,
}: {
  path?: string
  actions?: string[]
  respond?: (url: URL) => Response | Promise<Response> | undefined
} = {}) {
  const requests: URL[] = []
  const fetcher: typeof fetch = async (input) => {
    if (typeof input !== 'string') throw new Error('Expected request URL')
    const url = new URL(input, 'https://craftpulse.test')
    requests.push(url)
    const { pathname } = url
    if (pathname === '/api/v1/me')
      return jsonResponse({
        ...currentUser,
        actions,
        roles: [
          {
            role: actions.includes('connections.manage')
              ? 'administrator'
              : 'tech_lead',
            project_id: null,
            team_id: null,
            service_id: null,
            assigned_at: '2026-11-01T00:00:00Z',
          },
        ],
      } satisfies Schema['CurrentUser'])
    const override = respond(url)
    if (override) return override
    if (pathname === receiptApi) return jsonResponse(receiptExample)
    if (pathname === recordsApi) return recordsPage([accepted, rejected])
    if (pathname === '/api/v1/projects') return jsonResponse(projects)
    if (pathname === '/api/v1/connections')
      return jsonResponse({
        data: [connectionExample],
        next_cursor: null,
      } satisfies Schema['ConnectionPage'])
    if (pathname === '/api/v1/connections/' + connectionExample.id)
      return jsonResponse(connectionExample, 200, { ETag: '"1"' })
    if (
      pathname ===
      '/api/v1/connections/' + connectionExample.id + '/sync-status'
    )
      return jsonResponse(syncStatusExample)
    throw new Error('Unexpected request ' + pathname)
  }
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const runtime = createAppRuntime(fetcher)
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
  return { requests, router }
}

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
  vi.useRealTimers()
})

it('waits for the server delay before showing the partial receipt and the reason for the rejected record', async () => {
  vi.useFakeTimers()
  let calls = 0
  const { requests } = mount({
    respond: ({ pathname }) => {
      if (pathname !== receiptApi) return undefined
      calls += 1
      return jsonResponse(
        calls === 1
          ? ({
              ...receiptExample,
              status: 'quarantined',
              retry_after_seconds: 3,
              accepted_count: 0,
              rejected_count: 0,
              affected_metrics: [],
              result_url: null,
            } satisfies Schema['Receipt'])
          : receiptExample,
      )
    },
  })
  const advance = async (ms: number) => {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms)
    })
  }
  await advance(20)
  expect(screen.getByText('Пакет проверяется')).toBeVisible()
  expect(requests.some(({ pathname }) => pathname === recordsApi)).toBe(false)
  await advance(2900)
  expect(calls).toBe(1)
  await advance(120)
  expect(screen.getByText('Принят частично')).toBeVisible()
  const table = within(screen.getByRole('table', { name: 'Исходы записей' }))
  expect(table.getByText('Принята')).toBeVisible()
  expect(table.getByText('J1/10200/10345')).toBeVisible()
  expect(table.getByText('Отклонена')).toBeVisible()
  expect(table.getByText('идентичность не определена')).toBeVisible()
  expect(table.getByText('missing_source_id')).toBeVisible()
  expect(screen.getByText(/T04, T05/u)).toBeVisible()
  expect(screen.getByRole('link', { name: 'К результату' })).toHaveAttribute(
    'href',
    receiptExample.result_url,
  )
  await advance(3000)
  expect(calls).toBe(2)
})

it('explains rejection of the entire package and opens the import fields with the known schema', async () => {
  const { router } = mount({
    path: receiptPath + '?outcome=rejected',
    respond: ({ pathname }) =>
      pathname === receiptApi
        ? jsonResponse({
            ...receiptExample,
            status: 'rejected',
            accepted_count: 0,
            reason_code: 'schema_unsupported',
            result_url: null,
          } satisfies Schema['Receipt'])
        : undefined,
  })
  expect(await screen.findByText('schema_unsupported')).toBeVisible()
  expect(
    screen.getByText(/Схема или её версия не поддерживается/u),
  ).toBeVisible()
  expect(
    screen.getByText('Прежний принятый результат не изменён'),
  ).toBeVisible()
  const retry = screen.getByRole('link', { name: 'Исправить и повторить' })
  retry.focus()
  expect(retry).toHaveFocus()
  fireEvent.click(retry)
  const form = within(await screen.findByRole('form', { name: 'Импорт' }))
  expect(screen.getByRole('heading', { name: 'Импорт' })).toHaveFocus()
  expect(form.getByRole('combobox', { name: 'Вид данных' })).toHaveValue(
    'tasks',
  )
  expect(form.getByRole('textbox', { name: 'Версия схемы' })).toHaveValue('1')
  expect(form.getByLabelText('Файл')).toHaveValue('')
  expect(router.state.location.pathname).toBe('/settings/import')
  fireEvent.click(form.getByRole('button', { name: 'Отмена' }))
  expect(await screen.findByText('schema_unsupported')).toBeVisible()
  expect(router.state.location.search).toBe('?outcome=rejected')
})

it('shows a duplicate as a repeat delivery and links to the original receipt', async () => {
  const duplicateOf = '0193a5b0-5858-7000-8000-000000000001'
  const { requests } = mount({
    respond: ({ pathname }) =>
      pathname === receiptApi
        ? jsonResponse({
            ...receiptExample,
            status: 'duplicate',
            accepted_count: 0,
            rejected_count: 0,
            duplicate_of: duplicateOf,
          } satisfies Schema['Receipt'])
        : undefined,
  })
  expect(
    await screen.findByText(
      'Повтор доставки: пакет уже принят, новых записей нет',
    ),
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Исходная квитанция' }),
  ).toHaveAttribute('href', adminLinks.receipt(duplicateOf))
  expect(
    screen.getByText('Принято', { selector: 'dt' }).nextElementSibling,
  ).toHaveTextContent('0')
  expect(
    screen.queryByRole('table', { name: 'Исходы записей' }),
  ).not.toBeInTheDocument()
  expect(requests.some(({ pathname }) => pathname === recordsApi)).toBe(false)
})

it('uses the outcome and cursor on the server, clearing the old cursor when the outcome changes', async () => {
  const { router, requests } = mount({
    respond: ({ pathname, searchParams }) =>
      pathname !== recordsApi
        ? undefined
        : searchParams.has('cursor') ||
            searchParams.get('outcome') === 'rejected'
          ? recordsPage([rejected])
          : recordsPage([accepted], 'record-page-2'),
  })
  const next = await screen.findByRole('button', { name: 'Следующая страница' })
  expect(screen.getByText(/получено 1/u)).toBeVisible()
  fireEvent.click(next)
  expect(await screen.findByText('missing_source_id')).toBeVisible()
  expect(screen.getByText(/Страница 2/u)).toBeVisible()
  expect(
    requests
      .filter(({ pathname }) => pathname === recordsApi)
      .at(-1)
      ?.searchParams.get('cursor'),
  ).toBe('record-page-2')
  const filter = screen.getByRole('combobox', { name: 'Исход записи' })
  filter.focus()
  fireEvent.change(filter, { target: { value: 'rejected' } })
  expect(await screen.findByText('missing_source_id')).toBeVisible()
  expect(filter).toHaveFocus()
  expect(router.state.location.search).toBe('?outcome=rejected')
  expect(
    requests
      .filter(({ pathname }) => pathname === recordsApi)
      .at(-1)
      ?.searchParams.get('outcome'),
  ).toBe('rejected')
  expect(screen.queryByText(accepted.native_key)).not.toBeInTheDocument()
})

it('opens the journal from UI-11 with a connection filter and combines all four filters', async () => {
  const older = {
    ...receiptExample,
    id: '0193a5b0-5858-7000-8000-000000000001',
    received_at: '2026-11-07T08:00:00Z',
  }
  const project = projects.data[0]
  if (!project) throw new Error('Expected the OpenAPI project example')
  const other = {
    ...connectionExample,
    name: 'Jira резервный',
    id: '0193a5b0-5555-7000-8000-000000000002',
  }
  const { router, requests } = mount({
    path: '/settings/connections/' + connectionExample.id + '/sync',
    respond: ({ pathname, searchParams }) => {
      if (pathname === '/api/v1/connections')
        return jsonResponse(
          searchParams.has('cursor')
            ? { data: [other], next_cursor: null }
            : { data: [connectionExample], next_cursor: 'next-connection' },
        )
      if (pathname !== journalApi) return undefined
      return jsonResponse({
        data: searchParams.has('cursor') ? [older] : [receiptExample],
        next_cursor: searchParams.has('cursor') ? null : 'next-receipt',
      } satisfies Schema['ReceiptPage'])
    },
  })
  fireEvent.click(
    within(await screen.findByRole('main')).getByRole('link', {
      name: 'Журнал приёма сбора',
    }),
  )
  const journal = await screen.findByRole('heading', { name: 'Журнал приёма' })
  expect(journal).toBeVisible()
  expect(router.state.location.search).toBe(
    '?connection_id=' + connectionExample.id + '&channel=sync',
  )
  expect(
    await screen.findByRole('option', { name: connectionExample.name }),
  ).toBeInTheDocument()
  const connection = screen.getByRole('combobox', { name: 'Подключение' })
  expect(connection).toHaveValue(connectionExample.id)
  expect(
    (await screen.findByRole('link', { name: id })).getAttribute('href'),
  ).toBe(receiptPath)
  fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
  expect(await screen.findByRole('link', { name: older.id })).toBeVisible()
  fireEvent.change(connection, { target: { value: other.id } })
  fireEvent.change(screen.getByRole('combobox', { name: 'Канал' }), {
    target: { value: 'import' },
  })
  fireEvent.change(screen.getByRole('combobox', { name: 'Статус пакета' }), {
    target: { value: 'partial' },
  })
  expect(
    await screen.findByRole('option', { name: project.name }),
  ).toBeInTheDocument()
  const journalProject = within(screen.getByRole('main')).getByRole(
    'combobox',
    { name: 'Проект' },
  )
  fireEvent.change(journalProject, {
    target: { value: project.id },
  })
  await waitFor(() => {
    const query = requests
      .filter(({ pathname }) => pathname === journalApi)
      .at(-1)?.searchParams
    expect(Object.fromEntries(query ?? [])).toEqual({
      limit: '25',
      channel: 'import',
      status: 'partial',
      connection_id: other.id,
      project_id: project.id,
    })
  })
  fireEvent.click(await screen.findByRole('link', { name: id }))
  expect(
    await screen.findByRole('heading', { name: 'Квитанция приёма' }),
  ).toBeVisible()
})

it.each([true, false])(
  'shows the accepted receipt and gates conflict navigation by connections.manage (%s)',
  async (manage) => {
    mount({
      actions: manage ? adminActions : leadActions,
      respond: ({ pathname }) =>
        (pathname === receiptApi &&
          jsonResponse({
            ...receiptExample,
            status: 'accepted',
            accepted_count: 2,
            rejected_count: 0,
            conflict_count: 1,
          } satisfies Schema['Receipt'])) ||
        undefined,
    })
    expect(await screen.findByText('Принят')).toBeVisible()
    const link = screen.queryByRole('link', {
      name: 'Конфликты приёма организации',
    })
    if (manage)
      expect(link).toHaveAttribute('href', '/settings/ingest-conflicts')
    else expect(link).not.toBeInTheDocument()
  },
)

it('allows direct receipt access for a tech lead and denies the journal without sync.read', async () => {
  const { router, requests } = mount({ actions: leadActions })
  expect(await screen.findByText('Принят частично')).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Журнал приёма' }),
  ).not.toBeInTheDocument()
  await act(async () => {
    await router.navigate(adminLinks.receiptJournal())
  })
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(requests.some(({ pathname }) => pathname === journalApi)).toBe(false)
})

it.each([receiptApi, recordsApi])(
  'opens UI-04 when %s returns 404',
  async (missing) => {
    mount({
      respond: ({ pathname }) =>
        pathname === missing ? problemResponse('not_found', 404) : undefined,
    })
    expect(
      await screen.findByRole('heading', {
        name: 'Объект или область недоступны',
      }),
    ).toBeVisible()
    expect(screen.queryByText('Принят частично')).not.toBeInTheDocument()
  },
)
