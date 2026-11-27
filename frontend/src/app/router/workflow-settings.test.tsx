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
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import {
  currentUser,
  jsonResponse,
  problem,
  problemResponse,
  projects,
  teams,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schemas = components['schemas']
const [team] = teams.data
if (!team) throw new Error('Expected a team fixture')
const teamId = team.id
const instanceId = '0193a5b0-aaaa-7000-8000-000000000001'
const date = '2026-11-08T00:00:00Z'
const path = `/settings/process?team_id=${teamId}`
const owner = {
  ...currentUser,
  actions: [...currentUser.actions, 'process.manage'],
  roles: [
    ...currentUser.roles,
    {
      role: 'process_owner',
      team_id: teamId,
      project_id: null,
      service_id: null,
      assigned_at: date,
    },
  ],
} satisfies Schemas['CurrentUser']
const catalog = {
  instances: [
    {
      instance_id: instanceId,
      provider: 'jira_cloud',
      statuses: [
        { native_id: '1', name: 'Intake', source_category: 'active' },
        { native_id: '2', name: 'Working', source_category: 'indeterminate' },
        { native_id: '3', name: 'Mystery', source_category: null },
      ],
      resolutions: [{ native_id: '4', name: 'Fixed' }],
      issue_types: [{ native_id: '5', name: 'Bug' }],
      fields: [
        { native_id: 'points', name: 'Story Points', value_type: 'number' },
      ],
    },
  ],
  unmapped_statuses: [{ instance_id: instanceId, native_id: '3' }],
} satisfies Schemas['WorkflowCatalog']

function statusRule(meaning: string): Schemas['MappingRule'] {
  return {
    instance_id: instanceId,
    rule_kind: 'status',
    native_ref: '1',
    native_value: '',
    meaning,
    rank: null,
  }
}

function version(
  version_no: number,
  meaning: string,
): Schemas['MappingVersion'] {
  return {
    version_no,
    base_version_no: version_no === 1 ? null : version_no - 1,
    timezone: 'UTC',
    calendar_version_no: null,
    rules: [statusRule(meaning), { ...statusRule('active'), native_ref: '2' }],
    confirmed_at: date,
    confirmed_by: currentUser.user.id,
  }
}

function task(index: number): Schemas['MappingPreview']['tasks'][number] {
  return {
    task: {
      id: `0193a5b0-f000-7000-8000-${String(index).padStart(12, '0')}`,
      display_key: `CORE-${String(index)}`,
      title: null,
      source_url: `https://tracker.example/browse/CORE-${String(index)}`,
      kind: 'task',
    },
    started_at: index === 1 ? null : date,
    completed_at: null,
    outcome: 'open',
    blocked_seconds: index === 2 ? 3600 : null,
    task_type: null,
    size_label: null,
    unmapped: index === 3 ? ['Mystery'] : [],
  }
}

const preview = {
  as_of: date,
  period: { from: '2026-11-02T00:00:00Z', to: date },
  tasks: Array.from({ length: 11 }, (_, index) => task(index + 1)),
  control_values: ['T05', 'T08', 'T12'].map((metric_id) => ({
    metric_id,
    value: {
      state: 'ASSESSED',
      value: 4,
      unit: metric_id === 'T08' ? 'seconds' : 'count',
      numerator: null,
      denominator: null,
      n: 10,
      bound: null,
      low_sample: false,
      excluded_count: 0,
      missing_count: 0,
      reason_code: null,
    },
  })),
  unmapped_statuses: catalog.unmapped_statuses,
} satisfies Schemas['MappingPreview']

const disposers: (() => void)[] = []
beforeAll(() => {
  const prototype = HTMLDialogElement.prototype
  if (!Object.hasOwn(prototype, 'showModal')) {
    Object.defineProperty(prototype, 'showModal', {
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
      configurable: true,
    })
  }
  if (!Object.hasOwn(prototype, 'close')) {
    Object.defineProperty(prototype, 'close', {
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
      configurable: true,
    })
  }
})

afterEach(() => {
  disposers.splice(0).forEach((dispose) => {
    dispose()
  })
})

function emptyCalendarVersions() {
  return jsonResponse(
    {
      data: [],
      next_cursor: null,
    } satisfies Schemas['CalendarVersionPage'],
    200,
    { ETag: '"0"' },
  )
}

function mount({
  initial,
  user = owner,
  readCatalog = () => jsonResponse(catalog),
  readCalendarVersions = emptyCalendarVersions,
  createMapping,
  createCalendar,
}: {
  initial?: Schemas['MappingVersion']
  user?: Schemas['CurrentUser']
  readCatalog?: () => Response
  readCalendarVersions?: () => Response
  createMapping?: (match: string | null) => Response
  createCalendar?: (match: string | null) => Response
} = {}) {
  let current = initial
  const request =
    vi.fn<(endpoint: string, headers: Headers, body: unknown) => void>()
  const fetcher: typeof fetch = async (input, init) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    const headers = new Headers(init?.headers)
    const body: unknown =
      typeof init?.body === 'string'
        ? await new Response(init.body).json()
        : undefined
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse(user)
      case '/api/v1/teams':
        return jsonResponse(teams)
      case '/api/v1/projects':
        return jsonResponse(projects)
      case `/api/v1/teams/${teamId}/workflow-catalog`:
        request('catalog', headers, body)
        return readCatalog()
      case `/api/v1/teams/${teamId}/mapping-previews`:
        expect(init?.method).toBe('POST')
        request('preview', headers, body)
        return jsonResponse(preview)
      case `/api/v1/teams/${teamId}/mapping-versions`:
        if (init?.method === 'POST') {
          request('mapping', headers, body)
          const response = createMapping?.(headers.get('If-Match'))
          if (response) {
            if (response.status === 412) current = version(2, 'wait')
            return response
          }
          current = version(1, 'intake')
          return jsonResponse(current, 201, { ETag: '"1"' })
        }
        return jsonResponse(
          {
            data: current ? [current] : [],
            next_cursor: null,
          } satisfies Schemas['MappingVersionPage'],
          200,
          {
            ETag: `"${String(current?.version_no ?? 0)}"`,
          },
        )
      case `/api/v1/teams/${teamId}/calendar-versions`:
        if (init?.method === 'POST') {
          request('calendar', headers, body)
          return (
            createCalendar?.(headers.get('If-Match')) ??
            problemResponse('validation_failed', 422)
          )
        }
        return readCalendarVersions()
      default:
        throw new Error(`Unexpected URL ${url.pathname}`)
    }
  }
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
    runtime.queryClient.clear()
    runtime.session.dispose()
  })
  return { request, router }
}

function intake() {
  return screen.findByRole(
    'combobox',
    { name: 'Смысл статуса Intake (1)' },
    { timeout: 3_000 },
  )
}

async function runPreview() {
  fireEvent.click(screen.getByRole('button', { name: 'Предпросмотр' }))
  await screen.findByRole('table', { name: 'Задачи предпросмотра' })
}

function openConfirmation() {
  const button = screen.getByRole('button', {
    name: 'Подтвердить соответствие',
  })
  button.focus()
  fireEvent.click(button)
  return screen.getByRole('dialog')
}

it('offers source categories as an editable draft and preserves statuses without rules', async () => {
  const { request } = mount()
  expect(await intake()).toHaveValue('active')
  expect(screen.getAllByText('предложено')).toHaveLength(2)
  expect(screen.getByText(/Mystery \(3\) ·/u)).toBeInTheDocument()
  expect(
    screen.getByText('Эти статусы не дают начала и завершения работы.'),
  ).toBeInTheDocument()
  expect(request.mock.calls.map(([name]) => name)).toEqual(['catalog'])
})

it('sends Intake as intake and shows server timestamps, unknown statuses, and stale preview', async () => {
  const { request } = mount()
  const input = await intake()
  fireEvent.change(input, { target: { value: 'intake' } })
  await runPreview()
  const table = screen.getByRole('table', { name: 'Задачи предпросмотра' })
  expect(within(table).getAllByRole('row')).toHaveLength(11)
  expect(
    within(within(table).getByRole('row', { name: /CORE-1 /u })).getByText(
      'Нет начала',
    ),
  ).toBeInTheDocument()
  const active = within(table).getByRole('row', { name: /CORE-2 /u })
  expect(active.querySelector('time')).toHaveAttribute('datetime', date)
  expect(
    within(within(table).getByRole('row', { name: /CORE-3 /u })).getByText(
      'Mystery',
    ),
  ).toBeInTheDocument()
  expect(screen.getByText('Медиана цикла за неделю (p50)')).toBeInTheDocument()
  fireEvent.click(
    screen.getByRole('button', { name: 'Показать оставшиеся задачи' }),
  )
  expect(
    within(table).getByRole('link', { name: 'CORE-11' }),
  ).toBeInTheDocument()
  const sent = request.mock.calls.find(([name]) => name === 'preview')
  expect(sent?.[1].has('If-Match')).toBe(false)
  expect(sent?.[2]).toMatchObject({
    rules: [statusRule('intake'), { ...statusRule('active'), native_ref: '2' }],
  })
  fireEvent.change(input, { target: { value: 'wait' } })
  expect(
    screen.getByRole('status', { name: 'Устаревшие данные' }),
  ).toBeInTheDocument()
  fireEvent.click(
    screen.getByRole('button', { name: 'Подтвердить соответствие' }),
  )
  expect(screen.queryByRole('dialog')).toBeNull()
})

it('only writes after confirmation, passes If-Match and restores the opener focus', async () => {
  const { request } = mount()
  fireEvent.change(await intake(), { target: { value: 'intake' } })
  await runPreview()
  const dialog = openConfirmation()
  const opener = screen.getByRole('button', {
    name: 'Подтвердить соответствие',
  })
  expect(
    within(dialog).getByText('Станет: Входящие (intake)'),
  ).toBeInTheDocument()
  expect(request.mock.calls.some(([name]) => name === 'mapping')).toBe(false)
  fireEvent.keyDown(dialog, { key: 'Tab' })
  expect(within(dialog).getByRole('button', { name: 'Отмена' })).toHaveFocus()
  fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
  const confirm = within(dialog).getByRole('button', {
    name: 'Подтвердить',
  })
  expect(confirm).toHaveFocus()
  // jsdom does not dispatch the native button click for Enter.
  fireEvent.click(confirm)
  await screen.findByText('Соответствие подтверждено')
  await waitFor(() => {
    expect(opener).toHaveFocus()
  })
  const sent = request.mock.calls.find(([name]) => name === 'mapping')
  expect(sent?.[1].get('If-Match')).toBe('"0"')
  expect(sent?.[1].get('Idempotency-Key')).toBeTruthy()
  fireEvent.click(opener)
  expect(screen.getByText('Изменений нет')).toBeInTheDocument()
})

it('reloads the latest version after 412 and cancel leaves it unmodified', async () => {
  const { request } = mount({
    initial: version(1, 'active'),
    createMapping: () => problemResponse('version_conflict', 412),
  })
  fireEvent.change(await intake(), { target: { value: 'intake' } })
  await runPreview()
  const dialog = openConfirmation()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  await within(dialog).findByText(
    'Данные изменились. Просмотрите актуальную версию',
  )
  expect(within(dialog).getByText('"2"')).toBeInTheDocument()
  expect(within(dialog).getByText('Было: Ожидание (wait)')).toBeInTheDocument()
  expect(
    within(dialog).getByText('Станет: Входящие (intake)'),
  ).toBeInTheDocument()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Отмена' }))
  expect(await intake()).toHaveValue('intake')
  expect(screen.getByText('Действующая версия 2')).toBeInTheDocument()
  const saves = request.mock.calls.filter(([name]) => name === 'mapping')
  expect(saves).toHaveLength(1)
  expect(saves[0]?.[1].get('If-Match')).toBe('"1"')
})

it('requires a second confirmation before writing against a refreshed ETag', async () => {
  const { request } = mount({
    initial: version(1, 'active'),
    createMapping: (match) =>
      match === '"1"'
        ? problemResponse('version_conflict', 412)
        : jsonResponse(version(3, 'intake'), 201, { ETag: '"3"' }),
  })
  fireEvent.change(await intake(), { target: { value: 'intake' } })
  await runPreview()
  const dialog = openConfirmation()
  const submit = within(dialog).getByRole('button', { name: 'Подтвердить' })
  fireEvent.click(submit)
  await within(dialog).findByText(
    'Данные изменились. Просмотрите актуальную версию',
  )
  let attempts = request.mock.calls.filter(([name]) => name === 'mapping')
  expect(attempts).toHaveLength(1)
  fireEvent.click(submit)
  await screen.findByText('Действующая версия 3')
  attempts = request.mock.calls.filter(([name]) => name === 'mapping')
  expect(attempts.map(([, headers]) => headers.get('If-Match'))).toEqual([
    '"1"',
    '"2"',
  ])
  expect(attempts[1]?.[1].get('Idempotency-Key')).not.toBe(
    attempts[0]?.[1].get('Idempotency-Key'),
  )
})

it('retries a mapping POST with the original key after rereading the same version', async () => {
  const createMapping = vi
    .fn<() => Response>()
    .mockImplementationOnce(() => {
      throw new TypeError('Network error before sending POST')
    })
    .mockReturnValueOnce(
      jsonResponse(version(2, 'intake'), 201, { ETag: '"2"' }),
    )
  const { request } = mount({
    initial: version(1, 'active'),
    createMapping,
  })
  fireEvent.change(await intake(), { target: { value: 'intake' } })
  await runPreview()
  const dialog = openConfirmation()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  const retry = await within(dialog).findByRole('button', {
    name: 'Повторить сохранение',
  })
  expect(createMapping).toHaveBeenCalledOnce()

  fireEvent.click(retry)
  await screen.findByText('Действующая версия 2')
  const attempts = request.mock.calls.filter(([name]) => name === 'mapping')
  expect(attempts).toHaveLength(2)
  const headers = attempts.map(([, value]) => value)
  expect(headers.map((value) => value.get('If-Match'))).toEqual(['"1"', '"1"'])
  const key = headers[0]?.get('Idempotency-Key')
  expect(key).toBeTruthy()
  expect(headers[1]?.get('Idempotency-Key')).toBe(key)
  expect(attempts[1]?.[2]).toEqual(attempts[0]?.[2])
})

it('returns to the calendar draft after a manual reread recovers the same version', async () => {
  const readCalendarVersions = vi
    .fn(emptyCalendarVersions)
    .mockImplementationOnce(emptyCalendarVersions)
    .mockImplementationOnce(() => {
      throw new TypeError('Network error while rereading versions')
    })
  const createCalendar = vi.fn<() => Response>(() => {
    throw new TypeError('Network error before sending POST')
  })
  mount({ readCalendarVersions, createCalendar })
  const timezone = await screen.findByRole('textbox', {
    name: 'Часовой пояс календаря (IANA)',
  })
  fireEvent.change(timezone, { target: { value: 'Europe/Moscow' } })
  const opener = screen.getByRole('button', { name: 'Подтвердить календарь' })
  opener.focus()
  fireEvent.click(opener)
  const dialog = screen.getByRole('dialog')
  const cancel = within(dialog).getByRole('button', { name: 'Отмена' })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  const reread = await within(dialog).findByRole('button', {
    name: 'Перечитать ресурс',
  })
  expect(cancel).toBeDisabled()

  fireEvent.click(reread)
  await within(dialog).findByRole('button', { name: 'Повторить сохранение' })
  expect(cancel).toBeEnabled()
  fireEvent.click(cancel)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(timezone).toHaveValue('Europe/Moscow')
  await waitFor(() => {
    expect(opener).toHaveFocus()
  })
  expect(createCalendar).toHaveBeenCalledOnce()
})

it('versions the calendar separately and uses the selected version in the preview', async () => {
  const calendar = {
    version_no: 1,
    base_version_no: null,
    confirmed_by: currentUser.user.id,
    confirmed_at: date,
    timezone: 'Europe/Moscow',
    work_start: null,
    work_end: null,
    workdays: ['mon'],
    holidays: [],
  } satisfies Schemas['CalendarVersion']
  const { request } = mount({
    createCalendar: () => jsonResponse(calendar, 201, { ETag: '"1"' }),
  })
  await intake()
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Часовой пояс календаря (IANA)' }),
    { target: { value: 'Europe/Moscow' } },
  )
  fireEvent.click(screen.getByRole('checkbox', { name: 'Пн' }))
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить календарь' }))
  fireEvent.click(
    within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Подтвердить',
    }),
  )
  await screen.findByText('Календарь подтверждён')
  const saved = request.mock.calls.find(([name]) => name === 'calendar')
  expect(saved?.[1].get('If-Match')).toBe('"0"')
  expect(saved?.[2]).toEqual({
    timezone: 'Europe/Moscow',
    work_start: null,
    work_end: null,
    workdays: ['mon'],
    holidays: [],
  })
  fireEvent.change(
    screen.getByRole('combobox', { name: 'Версия рабочего календаря' }),
    { target: { value: '1' } },
  )
  await runPreview()
  expect(
    request.mock.calls.find(([name]) => name === 'preview')?.[2],
  ).toMatchObject({ calendar_version_no: 1, timezone: 'Europe/Moscow' })
})

it('associates a missing size order and invalid IANA name with fields and the summary', async () => {
  mount()
  await intake()
  const timezone = screen.getByRole('textbox', {
    name: 'Часовой пояс отчёта (IANA)',
  })
  fireEvent.change(timezone, { target: { value: 'Not/AZone' } })
  fireEvent.change(screen.getByRole('combobox', { name: 'Поле размера' }), {
    target: { value: 'points' },
  })
  fireEvent.click(
    screen.getByRole('button', { name: 'Добавить значение размера' }),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Предпросмотр' }))
  expect(timezone).toHaveAccessibleDescription(
    'Укажите известный часовой пояс IANA',
  )
  expect(
    screen.getByRole('spinbutton', { name: 'Порядок размера 1' }),
  ).toHaveAccessibleDescription('Укажите порядок размеров')
  const errors = screen.getByRole('alert')
  expect(errors).toHaveFocus()
  expect(within(errors).getAllByRole('link')).toHaveLength(2)
})

it('returns a 422 from UI-17 to the calendar field and summary', async () => {
  const { request } = mount({
    createCalendar: () =>
      jsonResponse(
        {
          ...problem('validation_failed', 422),
          errors: [{ field: '/timezone', code: 'not_allowed_value' }],
        } satisfies Schemas['Problem'],
        422,
        { 'Content-Type': 'application/problem+json' },
      ),
  })
  await intake()
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить календарь' }))
  fireEvent.click(
    within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Подтвердить',
    }),
  )
  const errors = await screen.findByRole('alert')
  expect(
    within(errors).getByRole('link', { name: /Часовой пояс календаря/u }),
  ).toBeInTheDocument()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(
    screen.getByRole('textbox', { name: /Часовой пояс календаря/u }),
  ).toHaveAccessibleDescription('Проверьте часовой пояс IANA')
  expect(
    request.mock.calls
      .find(([name]) => name === 'calendar')?.[1]
      .get('If-Match'),
  ).toBe('"0"')
})

it('asks before discarding edits during navigation', async () => {
  const { router, request } = mount()
  fireEvent.change(await intake(), { target: { value: 'intake' } })
  await act(async () => router.navigate('/'))
  const dialog = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Продолжить редактирование' }),
  )
  expect(await intake()).toHaveValue('intake')
  await act(async () => router.navigate('/'))
  fireEvent.click(
    within(await screen.findByRole('dialog')).getByRole('button', {
      name: 'Отбросить изменения',
    }),
  )
  expect(router.state.location.pathname).toBe('/')
  expect(request.mock.calls.some(([name]) => name === 'mapping')).toBe(false)
})

it('does not read workflow data without process.manage for the team', async () => {
  const { request } = mount({ user: currentUser })
  await screen.findByRole('heading', { name: 'Объект или область недоступны' })
  expect(request).not.toHaveBeenCalled()
})

it('shows the source-not-configured state for an empty catalog', async () => {
  mount({
    readCatalog: () =>
      jsonResponse({
        instances: [],
        unmapped_statuses: [],
      } satisfies Schemas['WorkflowCatalog']),
  })
  expect(
    await screen.findByRole('status', { name: 'Источник не настроен' }),
  ).toBeInTheDocument()
})
