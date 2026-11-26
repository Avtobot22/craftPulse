import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import type { components } from '@/shared/api'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  projects,
  teams,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

vi.mock('@gravity-ui/charts', () => ({ Chart: () => null }))

beforeAll(async () => {
  await import('@/pages/team-report')
  const prototype = HTMLDialogElement.prototype
  if (typeof prototype.showModal !== 'function') {
    Object.defineProperty(prototype, 'showModal', {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
    })
  }
  if (typeof prototype.close !== 'function') {
    Object.defineProperty(prototype, 'close', {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
    })
  }
})

type Schemas = components['schemas']
const [team] = teams.data
const [project] = projects.data
if (!team || !project) throw new Error('Expected a team and a project')
const teamId = team.id
const period = { from: '2026-10-01T00:00:00Z', to: '2026-11-01T00:00:00Z' }
const filters = {
  ...period,
  step: 'month',
  project_id: [project.id],
  task_type: 'Story',
  size_label: 'M',
  group_by: 'task_type',
} satisfies Schemas['SavedViewFilters']
const seriesId = '0193a5b0-a2a2-7000-8000-000000000001'
const firstRevision = '0193a5b0-a1a1-7000-8000-000000000001'
const latestRevision = '0193a5b0-a1a1-7000-8000-000000000002'
const savedId = '0193a5b0-a3a3-7000-8000-000000000001'
const otherId = '0193a5b0-a3a3-7000-8000-000000000002'
const savedPath = '/reports/saved-views'
const savedApi = '/api/v1/saved-views'
const reportApi = '/api/v1/teams/' + teamId + '/report'
const reportSearch = new URLSearchParams({
  team_id: teamId,
  ...period,
  step: filters.step,
  report_project_id: project.id,
  task_type: filters.task_type,
  size_label: filters.size_label,
  group_by: filters.group_by,
})

function report(id = firstRevision, limited = false): Schemas['TeamReport'] {
  return {
    team_id: teamId,
    period,
    step: 'month',
    revision: {
      id,
      series_id: seriesId,
      revision_no: id === firstRevision ? 17 : 18,
      as_of: period.to,
      published_at:
        id === firstRevision ? '2026-11-02T14:00:00Z' : '2026-11-03T14:00:00Z',
      loading: false,
      irreproducible_reason: null,
    },
    grouping: 'structure_at_date',
    metrics: limited
      ? [
          {
            metric_id: 'T14',
            definition_version: 1,
            unit: 'seconds',
            points: [
              {
                point_start: period.from,
                value: {
                  state: 'ASSESSED',
                  value: 3600,
                  unit: 'seconds',
                  numerator: null,
                  denominator: null,
                  n: 1,
                  bound: null,
                  low_sample: false,
                  excluded_count: 0,
                  missing_count: 0,
                  reason_code: null,
                },
              },
            ],
          },
        ]
      : [],
    access_limited: limited,
    access_notes: limited ? [{ code: 'restricted_by_access' }] : [],
  }
}

function descriptor(
  id = savedId,
  input: Schemas['SavedViewInput'] = {
    name: 'Отчёт за октябрь',
    series_id: seriesId,
    filters,
    pinned_revision_id: firstRevision,
  },
): Schemas['SavedView'] {
  return { id, created_at: '2026-11-04T10:00:00Z', ...input }
}

function revision(id: string): Schemas['Revision'] {
  return {
    ...report(id).revision,
    status: 'published',
    window: period,
    timezone: 'UTC',
    knowledge_seq: 1,
    superseded_at: null,
    definitions: [],
    inputs: [],
    source_watermarks: [],
  }
}

const disposers: (() => void)[] = []
function mount(
  path: string,
  saved: (url: URL, init: RequestInit | undefined) => Response,
  tasks: () => Schemas['CohortPage']['data'] = () => [],
) {
  const reads = vi.fn(saved)
  function respond(...[input, init]: Parameters<typeof fetch>) {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    if (url.pathname === '/api/v1/me') return jsonResponse(currentUser)
    if (url.pathname === '/api/v1/teams') return jsonResponse(teams)
    if (url.pathname === '/api/v1/projects') return jsonResponse(projects)
    if (url.pathname === reportApi) return jsonResponse(report())
    if (url.pathname.startsWith(savedApi)) return reads(url, init)
    if (url.pathname.endsWith('/cohort')) {
      return jsonResponse({
        data: tasks(),
        next_cursor: null,
        access_limited: true,
        access_notes: [{ code: 'restricted_by_access' }],
      } satisfies Schemas['CohortPage'])
    }
    const id = url.pathname.split('/').at(-1)
    if (id && url.pathname.startsWith('/api/v1/revisions/'))
      return jsonResponse(revision(id))
    throw new Error(`Unexpected request ${url.pathname}`)
  }
  const fetcher = vi.fn<typeof fetch>((input, init) =>
    Promise.resolve(respond(input, init)),
  )
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
  return { fetcher, reads, router }
}

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
  localStorage.clear()
})

it.each([false, true])(
  'сохраняет параметры отчёта и открывает срез после публикации новой ревизии (закрепление: %s)',
  async (pinned) => {
    const name = 'Поставка за октябрь'
    const input = {
      name,
      series_id: seriesId,
      filters,
      pinned_revision_id: pinned ? firstRevision : null,
    } satisfies Schemas['SavedViewInput']
    const saved = descriptor(savedId, input)
    const resolved = report(pinned ? firstRevision : latestRevision)
    const { reads } = mount(
      '/reports/team?' + reportSearch.toString(),
      (url, init) => {
        if (url.pathname === savedApi && init?.method === 'POST')
          return jsonResponse(saved, 201)
        if (url.pathname === savedApi + '/' + savedId) {
          return jsonResponse({
            ...saved,
            report: resolved,
          } satisfies Schemas['SavedView'])
        }
        throw new Error('Unexpected saved view request')
      },
    )

    const opener = await screen.findByRole('button', { name: 'Сохранить срез' })
    opener.focus()
    fireEvent.click(opener)
    const dialog = screen.getByRole('dialog', { name: 'Сохранить срез' })
    const heading = within(dialog).getByRole('heading')
    const textbox = within(dialog).getByRole('textbox', {
      name: 'Название среза',
    })
    const submit = within(dialog).getByRole('button', { name: 'Сохранить' })
    expect(heading).toHaveFocus()
    fireEvent.keyDown(dialog, { key: 'Tab' })
    expect(textbox).toHaveFocus()
    fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
    expect(submit).toHaveFocus()
    fireEvent.click(submit)
    expect(within(dialog).getByRole('alert')).toHaveTextContent(
      'Укажите название.',
    )
    expect(reads).not.toHaveBeenCalled()
    fireEvent.change(textbox, { target: { value: ' ' + name + ' ' } })
    expect(
      within(dialog).getByRole('radio', {
        name: 'Показывать актуальную ревизию',
      }),
    ).toBeChecked()
    if (pinned)
      fireEvent.click(
        within(dialog).getByRole('radio', {
          name: 'Закрепить текущую ревизию',
        }),
      )
    fireEvent.click(submit)

    const open = await screen.findByRole('link', {
      name: 'Открыть «' + name + '»',
    })
    await waitFor(() => {
      expect(opener).toHaveFocus()
    })
    const request = reads.mock.calls[0]?.[1]
    expect(request?.method).toBe('POST')
    expect(new Headers(request?.headers).has('If-Match')).toBe(false)
    if (typeof request?.body !== 'string')
      throw new Error('Expected a JSON body')
    expect(JSON.parse(request.body)).toEqual(input)
    expect(open).toHaveAttribute('href', savedPath + '/' + savedId)
    fireEvent.click(open)
    expect(await screen.findByRole('heading', { name })).toBeVisible()
    const parameters = screen.getByText('Шаг', { selector: 'dt' }).closest('dl')
    await waitFor(() => {
      expect(parameters).toHaveTextContent(project.name)
    })
    expect(parameters).toHaveTextContent('Месяц')
    expect(parameters).toHaveTextContent('Story')
    expect(parameters).toHaveTextContent('M')
    expect(parameters).toHaveTextContent('по типу задачи')
    expect(parameters?.querySelector('time')).toHaveAttribute(
      'datetime',
      period.from,
    )
    const summary = screen.getByRole('region', { name: 'Ревизия отчёта' })
    expect(
      within(summary).getByText(String(resolved.revision.revision_no)),
    ).toBeVisible()
    expect(
      within(summary).getByText(
        pinned ? 'Закреплённая ревизия' : 'Актуальная ревизия',
      ),
    ).toBeVisible()
    const publishedAt = resolved.revision.published_at
    if (!publishedAt) throw new Error('Expected a published revision')
    expect(
      summary.querySelector('time[datetime="' + publishedAt + '"]'),
    ).toBeVisible()
  },
)

it('открывает прямую ссылку в сессии получателя и показывает только разрешённые задачи', async () => {
  const allowed = {
    object: {
      kind: 'task',
      id: '0193a5b0-0100-7000-8000-000000000001',
      title: 'Разрешённая задача',
      display_key: 'CORE-1',
      source_url: null,
    },
    item_no: 1,
    role: 'numerator',
    value: 3600,
    reason_code: null,
    basis_deleted: false,
    task_type: 'Story',
    size_label: 'M',
  } satisfies Schemas['CohortItem']
  const { fetcher } = mount(
    savedPath + '/' + savedId,
    () =>
      jsonResponse({
        ...descriptor(),
        report: report(firstRevision, true),
      } satisfies Schemas['SavedView']),
    () => [allowed],
  )
  expect(await screen.findByText('Доступ к данным ограничен')).toBeVisible()
  const table = await screen.findByRole('table', {
    name: 'Возраст задач в работе',
  })
  expect(within(table).getAllByRole('row')).toHaveLength(2)
  expect(table).toHaveTextContent('CORE-1')
  expect(
    fetcher.mock.calls.some(
      ([url]) => typeof url === 'string' && url.startsWith(reportApi),
    ),
  ).toBe(false)
})

it('перечитывает задачи при изменении прав в том же срезе', async () => {
  const live = { ...descriptor(), pinned_revision_id: null }
  const formerlyVisible = {
    object: {
      kind: 'task',
      id: '0193a5b0-0100-7000-8000-000000000002',
      title: 'Задача из закрытой области',
      display_key: 'CORE-2',
      source_url: null,
    },
    item_no: 1,
    role: 'numerator',
    value: 3600,
    reason_code: null,
    basis_deleted: false,
    task_type: 'Story',
    size_label: 'M',
  } satisfies Schemas['CohortItem']
  let limited = false
  mount(
    savedPath + '/' + savedId,
    () => {
      const snapshot = report(firstRevision, true)
      return jsonResponse({
        ...live,
        report: {
          ...snapshot,
          access_limited: limited,
          access_notes: limited ? snapshot.access_notes : [],
        },
      } satisfies Schemas['SavedView'])
    },
    () => (limited ? [] : [formerlyVisible]),
  )
  expect(await screen.findByText('CORE-2')).toBeVisible()
  limited = true
  fireEvent.click(screen.getByRole('button', { name: 'Перечитать отчёт' }))
  expect(await screen.findByText('Доступ к данным ограничен')).toBeVisible()
  expect(screen.queryByText('CORE-2')).not.toBeInTheDocument()
})

it('показывает невоспроизводимость и ссылку на актуальную ревизию того же ряда', async () => {
  const pinned = report()
  pinned.revision.irreproducible_reason = 'basis_deleted'
  mount(savedPath + '/' + savedId, () =>
    jsonResponse({
      ...descriptor(),
      report: pinned,
    } satisfies Schemas['SavedView']),
  )
  expect(
    await screen.findByText('Ревизия невоспроизводима (основание удалено)'),
  ).toBeVisible()
  const latest = screen.getByRole('link', { name: 'К актуальной ревизии' })
  expect(latest).toHaveAttribute(
    'href',
    '/reports/team?' + reportSearch.toString(),
  )
})

it('открывает следующую страницу, копирует ссылку и удаляет по подтверждению, когда срез уже удалён', async () => {
  const first = descriptor()
  const second = { ...descriptor(otherId), name: 'Второй отчёт' }
  let exists = true
  const { reads } = mount(savedPath, (url, init) => {
    if (init?.method === 'DELETE') {
      expect(exists).toBe(false)
      expect(url.pathname).toBe(savedApi + '/' + savedId)
      return new Response(null, { status: 204 })
    }
    if (url.pathname === savedApi + '/' + savedId)
      return problemResponse('not_found', 404)
    const next = url.searchParams.get('cursor')
    if (next && next !== 'next-page') throw new Error('Wrong cursor')
    return jsonResponse({
      data: next || !exists ? [second] : [first],
      next_cursor: next || !exists ? null : 'next-page',
    } satisfies Schemas['SavedViewPage'])
  })
  const copy = vi
    .fn<(value: string) => Promise<void>>()
    .mockResolvedValue(undefined)
  const clipboard = Object.getOwnPropertyDescriptor(
    window.navigator,
    'clipboard',
  )
  Object.defineProperty(window.navigator, 'clipboard', {
    configurable: true,
    value: { writeText: copy },
  })
  try {
    await screen.findByRole('link', { name: first.name })
    fireEvent.click(screen.getByRole('button', { name: 'Показать ещё' }))
    await screen.findByRole('link', { name: second.name })
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Скопировать ссылку на срез «' + first.name + '»',
      }),
    )
    expect(await screen.findByText('Ссылка скопирована')).toBeVisible()
    expect(copy).toHaveBeenCalledWith(
      window.location.origin + savedPath + '/' + savedId,
    )

    const opener = screen.getByRole('button', {
      name: 'Удалить срез «' + first.name + '»',
    })
    fireEvent.click(opener)
    const dialog = screen.getByRole('dialog', {
      name: 'Удалить сохранённый срез?',
    })
    const cancel = within(dialog).getByRole('button', { name: 'Отмена' })
    expect(cancel).toHaveFocus()
    fireEvent.click(cancel)
    await waitFor(() => {
      expect(opener).toHaveFocus()
    })
    expect(reads.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(
      false,
    )

    exists = false
    fireEvent.click(opener)
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    )
    expect(await screen.findByText('Срез удалён')).toBeVisible()
    expect(
      screen.queryByRole('link', { name: first.name }),
    ).not.toBeInTheDocument()
    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: 'Сохранённые срезы' }),
      ).toHaveFocus()
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  } finally {
    if (clipboard)
      Object.defineProperty(window.navigator, 'clipboard', clipboard)
    else Reflect.deleteProperty(window.navigator, 'clipboard')
  }
})

it('показывает общий отказ для удалённого или недоступного по правам среза', async () => {
  mount(savedPath + '/' + savedId, () => problemResponse('not_found', 404))
  expect(await screen.findByText('Сохранённый срез недоступен')).toBeVisible()
  expect(
    screen.queryByRole('region', { name: 'Ревизия отчёта' }),
  ).not.toBeInTheDocument()
})
