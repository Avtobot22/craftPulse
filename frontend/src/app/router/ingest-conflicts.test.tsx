import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import {
  currentUser,
  jsonResponse,
  problem,
  problemResponse,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
type Resolution = Schema['IngestConflictResolutionInput']['resolution']

const screenPath = '/settings/ingest-conflicts'
const apiPath = '/api/v1/ingest-conflicts'
const conflict = {
  id: '0193a5b0-5959-7000-8000-000000000001',
  receipt_id: '0193a5b0-5858-7000-8000-000000000003',
  record_no: 4,
  existing_receipt_id: '0193a5b0-5858-7000-8000-000000000001',
  existing_record_no: 7,
  native_key: 'J1/10345/changelog-88213',
  detected_at: '2026-11-08T10:12:00Z',
  status: 'open',
  resolution: null,
} satisfies Schema['IngestConflict']
const postPath = apiPath + '/' + conflict.id + '/resolution'
const writeActions = [
  'session.read',
  'sync.read',
  'connections.manage',
  'receipts.read',
]
const disposers: (() => void)[] = []

function page(
  data: Schema['IngestConflict'][] = [conflict],
  next_cursor: string | null = null,
) {
  return jsonResponse({
    data,
    next_cursor,
  } satisfies Schema['IngestConflictPage'])
}

function resolved(resolution: Resolution): Schema['IngestConflict'] {
  return { ...conflict, status: 'resolved', resolution }
}

function mount({
  at = screenPath,
  actions = writeActions,
  list = () => page(),
  submit = () => jsonResponse(resolved('kept_existing')),
}: {
  at?: string
  actions?: string[]
  list?: (search: URLSearchParams) => Response
  submit?: (init: RequestInit) => Response
} = {}) {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementation(async (input, init) => {
      await Promise.resolve()
      if (typeof input !== 'string') throw new Error('Expected a request URL')
      const url = new URL(input, 'https://craftpulse.test')
      if (url.pathname === '/api/v1/me')
        return jsonResponse({
          ...currentUser,
          actions,
        } satisfies Schema['CurrentUser'])
      if (url.pathname === apiPath) return list(url.searchParams)
      if (url.pathname === postPath && init?.method === 'POST')
        return submit(init)
      throw new Error(`Unexpected request: ${input}`)
    })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [at],
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
  return { fetcher }
}

// jsdom does not implement native modal dialogs.
beforeAll(() => {
  const prototype = HTMLDialogElement.prototype
  if (!Object.hasOwn(prototype, 'showModal')) {
    Object.defineProperties(prototype, {
      showModal: {
        configurable: true,
        value(this: HTMLDialogElement) {
          this.setAttribute('open', '')
        },
      },
      close: {
        configurable: true,
        value(this: HTMLDialogElement) {
          this.removeAttribute('open')
        },
      },
    })
  }
})

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
})

async function choose(resolution: Resolution) {
  const open = await screen.findByRole('button', { name: /^Решить конфликт /u })
  open.focus()
  fireEvent.click(open)
  const select = screen.getByRole('combobox', { name: 'Решение конфликта' })
  expect(select).toHaveFocus()
  const preview = screen.getByRole('button', {
    name: 'Применить решение конфликта',
  })
  expect(preview).toBeDisabled()
  fireEvent.change(select, { target: { value: resolution } })
  preview.focus()
  fireEvent.click(preview)
  return screen.getByRole('dialog', { name: 'Подтверждение решения конфликта' })
}

function postRequests(fetcher: ReturnType<typeof vi.fn<typeof fetch>>) {
  return fetcher.mock.calls.filter(([, init]) => init?.method === 'POST')
}

it('показывает исходные координаты, время и ссылки на обе квитанции', async () => {
  mount()
  const row = await screen.findByRole('row', {
    name: new RegExp(conflict.native_key),
  })
  expect(within(row).getByText('Запись № 4')).toBeVisible()
  expect(within(row).getByText('Запись № 7')).toBeVisible()
  expect(row.querySelector('time')).toHaveAttribute(
    'datetime',
    conflict.detected_at,
  )
  expect(within(row).getByText('Открыт')).toBeVisible()
  for (const id of [conflict.receipt_id, conflict.existing_receipt_id]) {
    expect(within(row).getByRole('link', { name: id })).toHaveAttribute(
      'href',
      '/settings/receipts/' + id,
    )
  }
})

it('меняет статус списка и передаёт непрозрачный курсор без переноса между фильтрами', async () => {
  const { fetcher } = mount({
    list: (search) => {
      if (search.get('status') === 'resolved')
        return page([resolved('source_corrected')])
      if (search.get('cursor') === 'after/first+row') return page([])
      return page([conflict], 'after/first+row')
    },
  })
  await screen.findByText(conflict.native_key)
  fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
  expect(await screen.findByText('Открытых конфликтов нет')).toBeVisible()
  expect(
    fetcher.mock.calls.some(
      ([url]) =>
        typeof url === 'string' &&
        new URL(url, 'https://craftpulse.test').searchParams.get('cursor') ===
          'after/first+row',
    ),
  ).toBe(true)

  fireEvent.change(screen.getByRole('combobox', { name: 'Статус конфликта' }), {
    target: { value: 'resolved' },
  })
  expect(await screen.findByText('Источник исправлен')).toBeVisible()
  expect(fetcher.mock.calls.map(([url]) => url)).toContain(
    apiPath + '?status=resolved&limit=25',
  )
  expect(
    screen.queryByRole('button', { name: /^Решить конфликт /u }),
  ).toBeNull()
})

it.each([
  [
    'kept_existing',
    'Оставить принятую запись',
    'Принятая запись останется без изменений.',
  ],
  [
    'accepted_new',
    'Принять новую запись',
    'Будет создана новая версия знания и пересчитаны затронутые показатели.',
  ],
  ['source_corrected', 'Источник исправлен', 'Источник будет перечитан.'],
] as const)(
  'отправляет %s только после подтверждения и показывает решение в строке',
  async (resolution, label, consequence) => {
    const { fetcher } = mount({
      submit: () => jsonResponse(resolved(resolution)),
    })
    const dialog = await choose(resolution)
    expect(within(dialog).getByRole('heading')).toHaveFocus()
    expect(within(dialog).getByText(consequence)).toBeVisible()
    expect(postRequests(fetcher)).toHaveLength(0)

    const confirm = within(dialog).getByRole('button', { name: 'Подтвердить' })
    fireEvent.keyDown(within(dialog).getByRole('heading'), {
      key: 'Tab',
      shiftKey: true,
    })
    expect(confirm).toHaveFocus()
    fireEvent.click(confirm)
    fireEvent.click(confirm)

    expect(
      await screen.findByText(label, { selector: 'td span' }),
    ).toBeVisible()
    expect(screen.getByText('Решён')).toBeVisible()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(postRequests(fetcher)).toEqual([
      [
        postPath,
        expect.objectContaining({ body: JSON.stringify({ resolution }) }),
      ],
    ])
  },
)

it('отмена подтверждения сохраняет выбор без записи', async () => {
  const { fetcher } = mount()
  const dialog = await choose('accepted_new')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Отмена' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(
    screen.getByRole('combobox', { name: 'Решение конфликта' }),
  ).toHaveValue('accepted_new')
  expect(postRequests(fetcher)).toHaveLength(0)
})

it('после 409 показывает решение другого администратора со следующей страницы без повторного POST', async () => {
  const list = vi.fn((search: URLSearchParams) => {
    if (search.get('status') !== 'resolved') return page()
    return search.get('cursor') === 'resolved-page-two'
      ? page([resolved('source_corrected')])
      : page(
          [
            {
              ...resolved('kept_existing'),
              id: '0193a5b0-5959-7000-8000-000000000002',
            },
          ],
          'resolved-page-two',
        )
  })
  const { fetcher } = mount({
    list,
    submit: () => problemResponse('state_conflict', 409),
  })
  const dialog = await choose('accepted_new')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByText('Источник исправлен', { selector: 'td span' }),
  ).toBeVisible()
  expect(
    screen.getByText('Конфликт уже решён другим администратором.'),
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /^Решить конфликт /u }),
  ).toBeNull()
  expect(postRequests(fetcher)).toHaveLength(1)
  expect(list.mock.calls.at(-1)?.[0].get('cursor')).toBe('resolved-page-two')
})

it('при ошибке перечитывания после 409 повторяет только чтение', async () => {
  let attempts = 0
  const { fetcher } = mount({
    list: (search) => {
      if (search.get('status') !== 'resolved') return page()
      attempts += 1
      return attempts === 1
        ? problemResponse('internal_error', 500)
        : page([resolved('kept_existing')])
    },
    submit: () => problemResponse('state_conflict', 409),
  })
  const dialog = await choose('accepted_new')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  const retry = await screen.findByRole('button', {
    name: 'Перечитать решение',
  })
  expect(
    screen.queryByRole('button', { name: /^Решить конфликт /u }),
  ).toBeNull()
  fireEvent.click(retry)
  expect(await screen.findByText('Оставить принятую запись')).toBeVisible()
  expect(postRequests(fetcher)).toHaveLength(1)
})

it('после 422 выделяет выбор решения и сохраняет введённое значение', async () => {
  mount({
    submit: () =>
      jsonResponse(
        {
          ...problem('validation_failed', 422),
          errors: [{ field: '/resolution', code: 'not_allowed_value' }],
        } satisfies Schema['Problem'],
        422,
        { 'Content-Type': 'application/problem+json' },
      ),
  })
  const dialog = await choose('accepted_new')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  const select = await screen.findByRole('combobox', {
    name: 'Решение конфликта',
  })
  await waitFor(() => {
    expect(select).toHaveFocus()
  })
  expect(select).toHaveValue('accepted_new')
  expect(select).toHaveAccessibleDescription(/Проверьте выбор/u)
  expect(select).toHaveAttribute('aria-invalid', 'true')
  expect(screen.queryByRole('dialog')).toBeNull()
})

it('показывает список и переход из настроек с одним правом чтения', async () => {
  const { fetcher } = mount({
    at: '/settings/receipts',
    actions: ['session.read', 'sync.read'],
  })
  const link = await screen.findByRole('link', {
    name: 'Конфликты приёма организации',
  })
  expect(
    screen.getByRole('link', { name: 'Конфликты приёма' }),
  ).toHaveAttribute('href', screenPath)
  fireEvent.click(link)
  expect(await screen.findByText(conflict.native_key)).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /^Решить конфликт /u }),
  ).toBeNull()
  expect(postRequests(fetcher)).toHaveLength(0)
})
