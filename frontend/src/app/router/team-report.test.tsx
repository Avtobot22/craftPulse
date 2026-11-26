import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
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
})

type Schemas = components['schemas']
const [team] = teams.data
const [project] = projects.data
if (!team || !project) throw new Error('Expected one team and project fixture')
const teamId = team.id
const otherProject = {
  ...project,
  id: '0193a5b0-4444-7000-8000-000000000002',
  name: 'Delivery',
}
const revisionId = '0193a5b0-a1a1-7000-8000-000000000002'
const oldRevisionId = '0193a5b0-a1a1-7000-8000-000000000001'
const seriesId = '0193a5b0-a2a2-7000-8000-000000000001'
const pointStart = '2026-10-05T00:00:00Z'
const period = { from: '2026-10-01T00:00:00Z', to: '2026-11-01T00:00:00Z' }
const reportUrl = `/reports/team?${new URLSearchParams({ team_id: teamId, ...period, step: 'week' }).toString()}`
const reportApi = `/api/v1/teams/${teamId}/report`
const processOwner = {
  ...currentUser,
  actions: [...currentUser.actions, 'process.manage'],
  roles: [
    ...currentUser.roles,
    {
      role: 'process_owner',
      team_id: teamId,
      project_id: null,
      service_id: null,
      assigned_at: pointStart,
    },
  ],
} satisfies Schemas['CurrentUser']

function metric(
  metricId: string,
  value: number,
  options?: {
    estimator?: string
    value?: Partial<Schemas['MetricValue']>
  },
): Schemas['MetricSeries'] {
  return {
    metric_id: metricId,
    definition_version: 1,
    unit: options?.value?.unit ?? 'count',
    ...(options?.estimator ? { estimator: options.estimator } : {}),
    points: [
      {
        point_start: pointStart,
        value: {
          state: 'ASSESSED',
          value,
          unit: 'count',
          numerator: null,
          denominator: null,
          n: 24,
          bound: null,
          low_sample: false,
          excluded_count: 0,
          missing_count: 0,
          reason_code: null,
          ...options?.value,
        },
      },
    ],
  }
}

function report(
  id = revisionId,
  step: Schemas['TeamReport']['step'] = 'week',
): Schemas['TeamReport'] {
  return {
    team_id: teamId,
    period,
    step,
    grouping: 'current_structure',
    revision: {
      id,
      series_id: seriesId,
      revision_no: id === oldRevisionId ? 30 : 31,
      as_of: period.to,
      published_at: '2026-11-08T03:40:00Z',
      loading: false,
      irreproducible_reason: null,
    },
    metrics: [
      metric('T04', 9),
      metric('T05', 7),
      metric('T07', 2),
      metric('T12', 10),
      metric('T13', 0.3, {
        value: { unit: 'ratio', numerator: 3, denominator: 10, n: 10 },
      }),
      metric('T18', 2),
      metric('T38', 0.5, {
        estimator: 'size',
        value: { unit: 'ratio', denominator: 24, numerator: 12 },
      }),
    ],
    access_limited: false,
    access_notes: [],
  }
}

function revision(id: string, hasMapping = true): Schemas['Revision'] {
  return {
    ...report(id).revision,
    status: 'published',
    timezone: 'Europe/Moscow',
    knowledge_seq: 42,
    window: period,
    superseded_at: null,
    definitions: [],
    inputs: hasMapping
      ? [{ input_kind: 'mapping', input_id: teamId, version: '1' }]
      : [],
    source_watermarks: [],
  }
}

const queued = {
  id: '0193a5b0-a0a0-7000-8000-000000000001',
  kind: 'series_refresh',
  status: 'queued',
  created_at: period.to,
  updated_at: period.to,
  result_uri: null,
  error: null,
  retry_after_seconds: 3,
} satisfies Schemas['BackgroundOperation']

const cohort = {
  data: ['cancelled', 'duplicate'].map(
    (reason_code, item_no): Schemas['CohortItem'] => ({
      object: {
        kind: 'task',
        id: '0193a5b0-0800-7000-8000-00000000000' + String(item_no + 1),
        title: null,
        source_url: null,
        display_key: null,
      },
      item_no,
      role: 'numerator',
      value: 1,
      reason_code,
      basis_deleted: false,
    }),
  ),
  next_cursor: null,
  access_limited: false,
  access_notes: [],
} satisfies Schemas['CohortPage']

const disposers: (() => void)[] = []
function mount({
  path = reportUrl,
  user = currentUser,
  read = (url: URL) =>
    jsonResponse(
      report(
        url.searchParams.get('revision_id') ?? revisionId,
        url.searchParams.get('step') === 'month' ? 'month' : 'week',
      ),
    ),
  refresh = () => jsonResponse(queued, 202),
  readRevision = (id: string) => revision(id),
  readCohort = () => cohort,
  readTargets = () => ({ data: [], next_cursor: null }),
}: {
  path?: string
  user?: Schemas['CurrentUser']
  read?: (url: URL) => Response | Promise<Response>
  refresh?: () => Response | Promise<Response>
  readRevision?: (id: string) => Schemas['Revision']
  readCohort?: (url: URL) => Schemas['CohortPage']
  readTargets?: (url: URL) => Schemas['TargetVersionPage']
} = {}) {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementation(async (input, init) => {
      if (typeof input !== 'string') throw new Error('Expected a request URL')
      const url = new URL(input, 'https://craftpulse.test')
      switch (url.pathname) {
        case '/api/v1/me':
          return jsonResponse(user)
        case '/api/v1/teams':
          return jsonResponse(teams)
        case '/api/v1/projects':
          return jsonResponse({
            ...projects,
            data: [...projects.data, otherProject],
          } satisfies Schemas['ProjectPage'])
        case `/api/v1/teams/${teamId}/target-versions`:
          return jsonResponse(readTargets(url))
        case reportApi:
          return read(url)
        case `/api/v1/series/${seriesId}/refreshes`:
          if (
            init?.method !== 'POST' ||
            new Headers(init.headers).has('If-Match')
          )
            throw new Error('Expected an unversioned POST')
          return refresh()
        default: {
          const segments = url.pathname.split('/')
          if (segments[3] === 'revisions' && segments[4]) {
            if (segments[5] === 'cohort') return jsonResponse(readCohort(url))
            return jsonResponse(readRevision(segments[4]))
          }
          throw new Error(`Unexpected URL ${url.pathname}`)
        }
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
  disposers.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  const currentSearch = () => new URLSearchParams(router.state.location.search)
  return { fetcher, router, currentSearch }
}
afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
  localStorage.clear()
})

const reportReady = () =>
  screen.findByRole('region', { name: 'Ревизия отчёта' })

describe('UI-20', () => {
  it('switches the scale in the URL and sends the new step without applying the draft', async () => {
    const { currentSearch, fetcher } = mount()
    await reportReady()
    fireEvent.change(screen.getByLabelText('Тип задачи'), {
      target: { value: 'bug' },
    })
    fireEvent.click(screen.getByRole('radio', { name: 'Месяц' }))
    await reportReady()
    expect(currentSearch().get('step')).toBe('month')
    expect(currentSearch().has('task_type')).toBe(false)
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining('step=month'),
      expect.anything(),
    )
    expect(screen.getByLabelText('Тип задачи')).toHaveValue('bug')
  })

  it('applies and resets report filters while retaining the team scope', async () => {
    const { currentSearch, fetcher } = mount()
    await reportReady()
    const selector = screen.getByRole<HTMLSelectElement>('listbox', {
      name: 'Проекты отчёта',
    })
    await within(selector).findByRole('option', { name: 'Delivery' })
    for (const option of selector.options) option.selected = true
    fireEvent.change(selector)
    fireEvent.change(screen.getByLabelText('Тип задачи'), {
      target: { value: 'bug' },
    })
    fireEvent.change(screen.getByLabelText('Размер задачи'), {
      target: { value: 'unknown' },
    })
    fireEvent.change(screen.getByLabelText('Разбиение'), {
      target: { value: 'size_label' },
    })
    expect(currentSearch().has('task_type')).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
    await reportReady()
    const chips = screen.getByRole('list', { name: 'Применённые фильтры' })
    expect(chips).toHaveTextContent('Размер: Без размера')
    expect(chips).toHaveTextContent('Проект: Core')
    expect(chips).toHaveTextContent('Проект: Delivery')
    expect(currentSearch().getAll('report_project_id')).toEqual([
      project.id,
      otherProject.id,
    ])
    expect(currentSearch().get('task_type')).toBe('bug')
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining(
        `project_id=${project.id}&project_id=${otherProject.id}&task_type=bug&size_label=unknown&group_by=size_label`,
      ),
      expect.anything(),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Сбросить' }))
    expect(currentSearch().get('team_id')).toBe(teamId)
    expect(currentSearch().has('task_type')).toBe(false)
    expect(currentSearch().has('report_project_id')).toBe(false)
  })

  it('resets filters without clearing the selected project from the shell', async () => {
    const { currentSearch, fetcher } = mount({
      path: `${reportUrl}&project_id=${project.id}&task_type=bug`,
    })
    await reportReady()
    fireEvent.click(screen.getByRole('button', { name: 'Сбросить' }))
    await reportReady()
    expect(currentSearch().get('project_id')).toBe(project.id)
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining(`step=week&project_id=${project.id}`),
      expect.anything(),
    )
  })

  it('restores a bookmarked filter, scale and historical revision on open', async () => {
    const { fetcher } = mount({
      path: `${reportUrl.replace('step=week', 'step=month')}&revision_id=${oldRevisionId}&task_type=feature`,
    })
    expect(await reportReady()).toHaveTextContent('Ревизия30')
    expect(screen.getByText('Закреплённая ревизия')).toBeVisible()
    expect(screen.getByRole('radio', { name: 'Месяц' })).toBeChecked()
    expect(screen.getByLabelText('Тип задачи')).toHaveValue('feature')
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining(`revision_id=${oldRevisionId}`),
      expect.anything(),
    )
    expect(fetcher).toHaveBeenCalledWith(
      `/api/v1/revisions/${oldRevisionId}`,
      expect.anything(),
    )
    expect(screen.getByText('Europe/Moscow')).toBeVisible()
  })

  it.each([false, true])(
    'shows an unconfirmed mapping without a numeric value (process owner=%s)',
    async (canManage) => {
      mount({
        user: canManage ? processOwner : currentUser,
        read: () =>
          jsonResponse({
            ...report(),
            metrics: [
              metric('T41', 0, {
                value: {
                  state: 'NOT_EVALUATED',
                  value: null,
                  n: null,
                  reason_code: 'mapping_missing',
                },
              }),
            ],
          } satisfies Schemas['TeamReport']),
      })
      expect(
        await screen.findByText(/NOT_EVALUATED.*Сопоставление статусов/),
      ).toBeVisible()
      expect(screen.queryByText('0 шт.')).not.toBeInTheDocument()
      const settings = screen.queryByRole('link', {
        name: 'Настроить сопоставление',
      })
      if (canManage)
        expect(settings).toHaveAttribute(
          'href',
          `/settings/process?team_id=${teamId}`,
        )
      else expect(settings).not.toBeInTheDocument()
    },
  )

  it.each(['task_type', 'size_label'])(
    'keeps the missing-mapping explanation after applying %s grouping',
    async (groupBy) => {
      mount({
        user: processOwner,
        readRevision: (id) => revision(id, false),
        read: (url) =>
          jsonResponse({
            ...report(),
            metrics: url.searchParams.has('group_by')
              ? []
              : [
                  metric('T41', 0, {
                    value: {
                      state: 'NOT_EVALUATED',
                      value: null,
                      reason_code: 'mapping_missing',
                    },
                  }),
                ],
          } satisfies Schemas['TeamReport']),
      })
      await screen.findByText(/NOT_EVALUATED.*Сопоставление статусов/)
      fireEvent.change(screen.getByLabelText('Разбиение'), {
        target: { value: groupBy },
      })
      fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
      await reportReady()
      expect(
        screen.getByText(/NOT_EVALUATED.*Сопоставление статусов/),
      ).toBeVisible()
      expect(
        screen.getByRole('link', { name: 'Настроить сопоставление' }),
      ).toHaveAttribute('href', `/settings/process?team_id=${teamId}`)
      expect(screen.queryByText('0 шт.')).not.toBeInTheDocument()
    },
  )

  it('keeps the empty selection for an empty grouping with a confirmed mapping', async () => {
    mount({
      path: `${reportUrl}&group_by=task_type`,
      user: processOwner,
      read: () =>
        jsonResponse({
          ...report(),
          metrics: [],
        } satisfies Schemas['TeamReport']),
    })
    expect(
      await screen.findByText(
        'Нет задач за выбранный период в доступной области.',
      ),
    ).toBeVisible()
    expect(
      screen.queryByRole('link', { name: 'Настроить сопоставление' }),
    ).not.toBeInTheDocument()
  })

  it('labels permission limits and incomplete history without showing access-note identifiers', async () => {
    mount({
      read: () => {
        const response = report()
        return jsonResponse({
          ...response,
          revision: {
            ...response.revision,
            loading: true,
            irreproducible_reason: 'basis_deleted',
          },
          access_limited: true,
          access_notes: [
            { code: 'restricted_by_access', connection_id: project.id },
          ],
        } satisfies Schemas['TeamReport'])
      },
    })
    expect(await screen.findByText('Доступ к данным ограничен')).toBeVisible()
    expect(screen.getByText(/Загрузка истории не завершена/)).toBeVisible()
    expect(screen.getByText(/Ревизия невоспроизводима/)).toBeVisible()
    expect(screen.queryByText(project.id)).not.toBeInTheDocument()
  })

  it('keeps cancellation reasons and blocked WIP separate from completion', async () => {
    const { fetcher } = mount()
    const throughput = await screen.findByRole('region', {
      name: 'Вход и выход задач',
    })
    const completion = within(throughput).getByRole('button', {
      name: /Впервые завершённые задачи, .* 7 шт/,
    })
    expect(completion).toBeVisible()
    expect(
      within(throughput).getByRole('button', {
        name: 'Отмены и дубликаты: 2 шт.',
      }),
    ).toBeVisible()
    fireEvent.click(
      within(throughput).getByRole('button', { name: 'Показать причины' }),
    )
    expect(await screen.findByText('Отменены: 1')).toBeVisible()
    expect(screen.getByText('Признаны дубликатами: 1')).toBeVisible()
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining(`/revisions/${revisionId}/cohort?metric_id=T07`),
      expect.anything(),
    )
    const wip = screen.getByRole('region', {
      name: 'Задачи в работе и блокировки',
    })
    expect(
      within(wip).getByRole('button', {
        name: /Задачи в работе \(WIP\), .* 10 шт/,
      }),
    ).toBeVisible()
    expect(
      within(wip).getByRole('button', {
        name: /Заблокированные задачи внутри WIP, .* 3 шт.*Доля в WIP: 30\s*%/,
      }),
    ).toBeVisible()
  })

  it('marks cancellation reasons as partial when a cohort item has lost its basis', async () => {
    mount({
      readCohort: () => ({
        ...cohort,
        data: cohort.data.map((item) =>
          item.reason_code === 'duplicate'
            ? { ...item, reason_code: null, basis_deleted: true }
            : item,
        ),
      }),
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Показать причины' }),
    )
    expect(
      await screen.findByText('Причины доступны только для части закрытий.'),
    ).toBeVisible()
    expect(screen.getByText('Отменены: 1')).toBeVisible()
  })

  it.each(['report_project_id', 'project_id'])(
    'loads cancellation reasons from the cohort narrowed by %s',
    async (projectParam) => {
      const { fetcher } = mount({
        path: `${reportUrl}&${projectParam}=${project.id}&revision_id=${oldRevisionId}`,
      })
      fireEvent.click(
        await screen.findByRole('button', { name: 'Показать причины' }),
      )
      expect(await screen.findByText('Отменены: 1')).toBeVisible()
      expect(screen.getByText('Признаны дубликатами: 1')).toBeVisible()
      expect(fetcher).toHaveBeenCalledWith(
        `/api/v1/revisions/${oldRevisionId}/cohort?${new URLSearchParams({
          metric_id: 'T07',
          point_start: pointStart,
          role: 'numerator',
          project_id: project.id,
        }).toString()}`,
        expect.anything(),
      )
    },
  )

  it('shows freshness dates from the visible metrics when source watermarks are restricted', async () => {
    const response = report()
    mount({
      read: () =>
        jsonResponse({
          ...response,
          access_limited: true,
          metrics: [
            metric('T40', 3600, {
              estimator: 'lag',
              value: { unit: 'seconds' },
            }),
            metric('T40', 7200, {
              estimator: 'last_success_age',
              value: { unit: 'seconds' },
            }),
          ],
        } satisfies Schemas['TeamReport']),
      readRevision: (id) => ({
        ...revision(id),
        source_watermarks: [
          {
            connection_id: project.id,
            watermark: '2026-10-31T20:00:00Z',
            completeness: 'complete',
          },
        ],
      }),
    })
    const completeness = await screen.findByRole('region', {
      name: 'Полнота данных',
    })
    expect(
      within(completeness)
        .getByText(/Самая ранняя отметка загрузки/)
        .querySelector('time'),
    ).toHaveAttribute('datetime', '2026-10-31T23:00:00.000Z')
    expect(
      within(completeness)
        .getByText(/Самая ранняя из последних успешных/)
        .querySelector('time'),
    ).toHaveAttribute('datetime', '2026-10-31T22:00:00.000Z')
    expect(
      within(completeness).queryByRole('list', {
        name: 'Отметки загрузки источников',
      }),
    ).not.toBeInTheDocument()
  })

  it('shows a zero freshness age as a date boundary', async () => {
    const response = report()
    response.metrics = [
      metric('T40', 0, {
        estimator: 'last_success_age',
        value: { unit: 'seconds' },
      }),
    ]
    mount({ read: () => jsonResponse(response) })
    expect(
      await screen.findByText(/Самая ранняя из последних успешных.*не ранее/),
    ).toBeVisible()
  })

  it('keeps the published revision during refresh and changes it only on a later read', async () => {
    let accept: (value: Response) => void = () => {
      throw new Error('Refresh not started')
    }
    const pending = new Promise<Response>((resolve) => {
      accept = resolve
    })
    let response = report(oldRevisionId)
    const { fetcher } = mount({
      read: () => jsonResponse(response),
      refresh: () => pending,
    })
    await reportReady()
    fireEvent.click(screen.getByRole('button', { name: 'Обновить' }))
    expect(await reportReady()).toHaveTextContent('Ревизия30')
    expect(fetcher).toHaveBeenCalledWith(
      `/api/v1/series/${seriesId}/refreshes`,
      expect.objectContaining({ method: 'POST' }),
    )
    act(() => {
      accept(jsonResponse(queued, 202))
    })
    expect(
      await screen.findByText(/Пересчёт поставлен в очередь/),
    ).toBeVisible()
    expect(await reportReady()).toHaveTextContent('Ревизия30')
    response = report()
    response.revision.published_at = null
    fireEvent.click(screen.getByRole('button', { name: 'Перечитать отчёт' }))
    expect(
      await screen.findByText('Новая ревизия ещё не опубликована.'),
    ).toBeVisible()
    expect(await reportReady()).toHaveTextContent('Ревизия30')
    expect(fetcher).not.toHaveBeenCalledWith(
      `/api/v1/revisions/${revisionId}`,
      expect.anything(),
    )

    response = { ...report(), metrics: [metric('T05', 11)] }
    fireEvent.click(screen.getByRole('button', { name: 'Перечитать отчёт' }))
    await waitFor(() => {
      expect(
        screen.getByRole('region', { name: 'Ревизия отчёта' }),
      ).toHaveTextContent('Ревизия31')
    })
    const flow = screen.getByRole('region', {
      name: 'Начало и завершение работы',
    })
    expect(within(flow).getByRole('button', { name: /11 шт\./ })).toBeVisible()
    expect(within(flow).queryByRole('button', { name: /7 шт\./ })).toBeNull()
  })

  it('opens the passport of a T05 point and restores the report and keyboard focus', async () => {
    const { currentSearch, router } = mount({
      path: `${reportUrl}&task_type=bug`,
    })
    const chart = await screen.findByRole('region', {
      name: 'Начало и завершение работы',
    })
    const target = within(chart).getByRole('button', {
      name: /Впервые завершённые задачи,/,
    })
    target.focus()
    fireEvent.click(target)
    expect(
      await screen.findByRole('heading', { name: 'Паспорт показателя' }),
    ).toBeVisible()
    expect(router.state.location.pathname).toBe(
      `/reports/revisions/${revisionId}/metrics/T05`,
    )
    expect(currentSearch().get('point_start')).toBe(pointStart)
    fireEvent.click(screen.getByRole('button', { name: 'Вернуться к отчёту' }))
    const restored = await screen.findByRole('region', {
      name: 'Начало и завершение работы',
    })
    expect(router.state.location.pathname).toBe('/reports/team')
    expect(currentSearch().get('task_type')).toBe('bug')
    expect(currentSearch().get('from')).toBe(period.from)
    await waitFor(() => {
      expect(
        within(restored).getByRole('button', {
          name: /Впервые завершённые задачи,/,
        }),
      ).toHaveFocus()
    })
  })

  it('preserves the URL after a read error and retries the same report', async () => {
    const read = vi
      .fn(() => jsonResponse(report()))
      .mockReturnValueOnce(problemResponse('internal_error', 500))
    const { currentSearch } = mount({
      path: `${reportUrl}&task_type=bug`,
      read,
    })
    expect(
      await screen.findByText('Не удалось прочитать отчёт команды'),
    ).toBeVisible()
    expect(currentSearch().get('task_type')).toBe('bug')
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(await reportReady()).toHaveTextContent('Ревизия31')
    expect(read).toHaveBeenCalledTimes(2)
  })

  it('hides the previously loaded values when access is revoked on reread', async () => {
    const read = vi
      .fn(() => jsonResponse(report()))
      .mockReturnValueOnce(jsonResponse(report()))
      .mockReturnValueOnce(problemResponse('forbidden', 403))
    mount({ read })
    await reportReady()
    fireEvent.click(screen.getByRole('button', { name: 'Перечитать отчёт' }))
    expect(
      await screen.findByText('Отчёт или ревизия недоступны'),
    ).toBeVisible()
    expect(
      screen.queryByRole('region', { name: 'Ревизия отчёта' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Вход и выход задач' }),
    ).not.toBeInTheDocument()
  })

  it('validates the half-open period before reading it', async () => {
    const { currentSearch, fetcher } = mount()
    await reportReady()
    fireEvent.change(
      screen.getByLabelText('Конец периода (UTC, исключительно)'),
      {
        target: { value: '2026-10-01T00:00' },
      },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Применить' }))
    expect(
      await screen.findByText('Конец периода должен быть позже начала.'),
    ).toBeVisible()
    expect(currentSearch().get('to')).toBe(period.to)
    expect(
      fetcher.mock.calls.filter(
        ([input]) => typeof input === 'string' && input.startsWith(reportApi),
      ),
    ).toHaveLength(1)
  })

  it('labels an empty task cohort and keeps the measured zero points', async () => {
    const response = report()
    response.metrics = [
      metric('T04', 0),
      metric('T39', 0, {
        value: { state: 'NOT_APPLICABLE', value: null, n: 0, unit: 'ratio' },
      }),
    ]
    mount({ read: () => jsonResponse(response) })
    expect(
      await screen.findByRole('status', { name: 'Нет объектов по фильтрам' }),
    ).toBeVisible()
    expect(
      within(
        screen.getByRole('region', { name: 'Начало и завершение работы' }),
      ).getByRole('button', { name: /Начатые задачи, .* 0 шт/ }),
    ).toBeVisible()
  })

  it('does not treat imported tasks with incomplete history as an unconfigured source', async () => {
    const response = report()
    response.metrics = [
      metric('T39', 0, {
        value: { unit: 'ratio', numerator: 0, denominator: 24 },
      }),
      metric('T40', 0, {
        estimator: 'lag',
        value: {
          unit: 'seconds',
          state: 'NOT_APPLICABLE',
          value: null,
          n: 0,
        },
      }),
    ]
    mount({ read: () => jsonResponse(response) })
    expect(await screen.findByText('Полнота истории')).toBeVisible()
    expect(
      screen.queryByRole('status', { name: 'Источник не настроен' }),
    ).not.toBeInTheDocument()
  })

  it.each([
    ['Нет объектов по фильтрам', '', false],
    ['История ещё не получена', '', true],
    ['Источник не настроен', 'source_not_configured', false],
  ])('distinguishes an empty report: %s', async (message, reason, loading) => {
    const response = report()
    mount({
      read: () =>
        jsonResponse({
          ...response,
          revision: { ...response.revision, loading },
          metrics: reason
            ? [
                metric('T40', 0, {
                  value: { value: null, state: 'UNKNOWN', reason_code: reason },
                }),
              ]
            : [],
        } satisfies Schemas['TeamReport']),
    })
    expect(await screen.findByRole('status', { name: message })).toBeVisible()
  })
})

const detailReport = (...metrics: Schemas['MetricSeries'][]) => ({
  ...report(),
  metrics: [...report().metrics, ...metrics],
})

const unknownTarget = {
  state: 'UNKNOWN',
  value: null,
  reason_code: 'target_missing',
  missing_count: 2,
  n: 0,
} satisfies Partial<Schemas['MetricValue']>

const reopening = metric('T23', 0.5, {
  value: {
    unit: 'ratio',
    numerator: 2,
    denominator: 4,
    n: 4,
    excluded_count: 3,
  },
})

function detailTask(
  index: number,
  data: Partial<Schemas['CohortItem']> = {},
): Schemas['CohortItem'] {
  const key = `CORE-${String(index)}`
  return {
    object: {
      kind: 'task',
      id: `0193a5b0-0800-7000-8000-${String(index).padStart(12, '0')}`,
      display_key: key,
      source_url: `https://jira.example/browse/${key}`,
      title: null,
    },
    item_no: 0,
    basis_deleted: false,
    task_type: 'Story',
    size_label: 'S',
    value: 3600,
    role: 'numerator',
    reason_code: null,
    ...data,
  }
}

describe('UI-20: длительности, возраст, целевые сроки и переоткрытия', () => {
  it('shows the paginated T08 sample with its original values and a column without size', async () => {
    const readCohort = vi.fn((url: URL) => ({
      ...cohort,
      data: url.searchParams.has('cursor')
        ? [
            detailTask(2, { size_label: null, value: 10800 }),
            detailTask(3, { role: 'excluded', value: null }),
          ]
        : [detailTask(1), detailTask(4, { value: 7200 })],
      next_cursor: url.searchParams.has('cursor') ? null : 'page-2',
    }))
    mount({
      read: () =>
        jsonResponse(
          detailReport(
            metric('T08', 7200, {
              estimator: 'p50',
              value: { unit: 'seconds', n: 3 },
            }),
            metric('T08', 10800, {
              estimator: 'p85',
              value: { unit: 'seconds', n: 3 },
            }),
          ),
        ),
      readCohort,
    })
    const chart = await screen.findByRole('figure', {
      name: /Длительность отдельных задач/,
    })
    const markers = within(chart).getAllByRole('link')
    expect(markers).toHaveLength(3)
    expect(
      within(chart).getByRole('link', { name: 'CORE-1 · 3600 с' }),
    ).toHaveAttribute('href', 'https://jira.example/browse/CORE-1')
    expect(
      within(chart).getByRole('link', { name: 'CORE-4 · 7200 с' }),
    ).toBeVisible()
    expect(
      within(chart).getByRole('link', { name: 'CORE-2 · 10800 с' }),
    ).toBeVisible()
    expect(within(chart).getByText('Без размера ·')).toBeVisible()
    expect(within(chart).getByRole('button', { name: 'n = 1' })).toBeVisible()
    expect(
      readCohort.mock.calls.map(([url]) => [
        url.pathname,
        url.searchParams.get('metric_id'),
        url.searchParams.get('point_start'),
        url.searchParams.get('cursor'),
      ]),
    ).toEqual([
      [`/api/v1/revisions/${revisionId}/cohort`, 'T08', pointStart, null],
      [`/api/v1/revisions/${revisionId}/cohort`, 'T08', pointStart, 'page-2'],
    ])
    const distribution = screen.getByRole('table', {
      name: 'Длительность по типу и размеру',
    })
    const row = within(distribution).getByRole('row', { name: /Story · S/ })
    expect(within(row).getByRole('button', { name: '1 ч' })).toBeVisible()
    expect(within(row).getByRole('button', { name: '2 ч' })).toBeVisible()
    expect(within(row).getByText(/Малая выборка/)).toBeVisible()
  })

  it('returns from a T08 group passport to the selected cohort step', async () => {
    const duration = metric('T08', 3600, {
      estimator: 'p50',
      value: { unit: 'seconds', n: 1 },
    })
    const [point] = duration.points
    if (!point) throw new Error('Expected a point fixture')
    const older = { ...point, point_start: period.from }
    const { router, currentSearch } = mount({
      read: () =>
        jsonResponse(detailReport({ ...duration, points: [older, point] })),
      readCohort: (url) => ({
        ...cohort,
        data: [
          url.searchParams.get('point_start') === period.from
            ? detailTask(2, { size_label: null })
            : detailTask(1),
        ],
      }),
    })
    const select = await screen.findByLabelText('Шаг длительности')
    fireEvent.change(select, { target: { value: period.from } })
    const chart = await screen.findByRole('figure', {
      name: /Длительность отдельных задач/,
    })
    expect(within(chart).getByText('Без размера ·')).toBeVisible()
    fireEvent.click(within(chart).getByRole('button', { name: 'n = 1' }))
    await screen.findByRole('heading', { name: 'Паспорт показателя' })
    expect(router.state.location.pathname).toBe(
      ['/reports/revisions', revisionId, 'metrics', 'T08'].join('/'),
    )
    expect(currentSearch().get('point_start')).toBe(period.from)
    expect(currentSearch().get('size_label')).toBe('unknown')
    fireEvent.click(screen.getByRole('button', { name: 'Вернуться к отчёту' }))
    const restored = await screen.findByRole('figure', {
      name: /Длительность отдельных задач/,
    })
    expect(screen.getByLabelText('Шаг длительности')).toHaveValue(period.from)
    await waitFor(() => {
      expect(
        within(restored).getByRole('button', { name: 'n = 1' }),
      ).toHaveFocus()
    })
  })

  it.each([
    ['T10', 'Соблюдение целевых сроков', 'ratio'],
    ['T11', 'Возраст текущей работы и блокировки', 'count'],
  ])(
    'shows %s without a target as unknown with a count and without a percentage',
    async (metricId, title, unit) => {
      mount({
        read: () =>
          jsonResponse(
            detailReport(
              metric(metricId, 0, { value: { ...unknownTarget, unit } }),
            ),
          ),
      })
      const region = await screen.findByRole('region', { name: title })
      expect(
        within(region).getByRole('button', {
          name: 'Не рассчитано · Целевой срок не задан',
        }),
      ).toBeVisible()
      expect(within(region).getByRole('button', { name: '2' })).toBeVisible()
      expect(region).not.toHaveTextContent('%')
    },
  )

  it('shows current episode, full age, target ratios and time blocked from separate contributions', async () => {
    mount({
      read: () =>
        jsonResponse(
          detailReport(
            metric('T14', 3600, {
              estimator: 'p50',
              value: { unit: 'seconds' },
            }),
            metric('T11', 1, { value: { numerator: 1, denominator: 1 } }),
            metric('T15', 0.25, {
              value: { unit: 'ratio', numerator: 3600, denominator: 14400 },
            }),
          ),
        ),
      readCohort: (url) => ({
        ...cohort,
        data:
          url.searchParams.get('metric_id') === 'T14'
            ? [
                detailTask(1),
                detailTask(1, {
                  item_no: 1,
                  value: 10800,
                  reason_code: 'full_age',
                }),
                detailTask(2),
              ]
            : url.searchParams.get('metric_id') === 'T11'
              ? [
                  detailTask(1, { role: 'denominator', value: 1.5 }),
                  detailTask(2, {
                    role: 'missing',
                    value: null,
                    reason_code: 'target_missing',
                  }),
                ]
              : [
                  detailTask(1),
                  detailTask(1, {
                    item_no: 1,
                    role: 'denominator',
                    value: 14400,
                  }),
                  detailTask(2, { value: 0 }),
                ],
      }),
    })
    const age = await screen.findByRole('table', {
      name: 'Возраст задач в работе',
    })
    expect(
      within(age).getByRole('row', { name: /CORE-1 1 ч 3 ч 150\s*%/ }),
    ).toBeVisible()
    expect(
      within(age).getByRole('row', {
        name: /CORE-2 .*Не рассчитано · Целевой срок не задан/,
      }),
    ).toBeVisible()
    expect(await screen.findByRole('button', { name: '1 из 1' })).toBeVisible()
    const blocked = await screen.findByRole('table', {
      name: 'Блокировки за шаг',
    })
    expect(
      within(blocked).getByRole('row', { name: /CORE-1 1 ч 25\s*%/ }),
    ).toBeVisible()
    expect(within(blocked).queryByText('CORE-2')).not.toBeInTheDocument()
  })

  it('separates immature tasks from the T23 denominator and reads the observation window from the revision', async () => {
    mount({
      read: () => jsonResponse(detailReport(reopening)),
      readRevision: (id) => ({
        ...revision(id),
        definitions: [
          {
            metric_id: 'T23',
            definition_version: 1,
            name: 'Reopened',
            unit: 'ratio',
            catalog_ref: '#t23',
            parameters: { follow_up_days: 42 },
          },
        ],
      }),
    })
    const region = await screen.findByRole('region', {
      name: 'Переоткрытия workflow',
    })
    expect(
      within(region).getByText('42 дн. после первого завершения'),
    ).toBeVisible()
    expect(
      within(region).getByRole('button', { name: /50\s*% \(2 из 4\)/ }),
    ).toBeVisible()
    expect(
      within(region).getByRole('columnheader', { name: 'Незрелые задачи' }),
    ).toBeVisible()
    expect(within(region).getByRole('button', { name: '3' })).toBeVisible()
    expect(region).not.toHaveTextContent(/дефект|production/iu)
  })

  it.each([
    ['T10', 'Соблюдение целевых сроков'],
    ['T23', 'Переоткрытия workflow'],
  ])(
    'opens the %s passport and restores the older point of the same revision',
    async (metricId, title) => {
      const read = vi.fn((url: URL) => {
        const series = { ...reopening, metric_id: metricId }
        const [point] = reopening.points
        if (!point) throw new Error('Expected a point fixture')
        const older = { ...point, point_start: period.from }
        return jsonResponse({
          ...detailReport({ ...series, points: [...series.points, older] }),
          revision: {
            ...report().revision,
            id: url.searchParams.get('revision_id') ?? revisionId,
          },
        })
      })
      const { router, currentSearch } = mount({ read })
      const region = await screen.findByRole('region', { name: title })
      const selected = within(region).getAllByRole('button', {
        name: /2 из 4/,
      })[1]
      if (!selected) throw new Error('Expected two outcome points')
      fireEvent.click(selected)
      await screen.findByRole('heading', { name: 'Паспорт показателя' })
      expect(router.state.location.pathname).toBe(
        `/reports/revisions/${revisionId}/metrics/${metricId}`,
      )
      expect(currentSearch().get('point_start')).toBe(period.from)
      fireEvent.click(
        screen.getByRole('button', { name: 'Вернуться к отчёту' }),
      )
      const restored = await screen.findByRole('region', { name: title })
      await waitFor(() => {
        expect(
          within(restored).getAllByRole('button', { name: /2 из 4/ })[1],
        ).toHaveFocus()
      })
      expect(read.mock.calls.at(-1)?.[0].searchParams.get('revision_id')).toBe(
        revisionId,
      )
    },
  )

  it('shows the latest target version of each group with its basis n', async () => {
    const target = {
      task_type: 'Story',
      size_label: 'S',
      version_no: 2,
      base_version_no: 1,
      duration_seconds: 7200,
      basis: 'historical_p85',
      basis_revision_id: revisionId,
      basis_value_seconds: 7200,
      basis_n: 11,
      basis_period: period,
      confirmed_by: currentUser.user.id,
      confirmed_at: pointStart,
    } satisfies Schemas['TargetVersion']
    mount({
      user: processOwner,
      read: () =>
        jsonResponse(
          detailReport(
            metric('T10', 1, {
              value: { unit: 'ratio', numerator: 1, denominator: 1 },
            }),
          ),
        ),
      readTargets: () => ({
        data: [
          target,
          { ...target, version_no: 1, base_version_no: null, basis_n: 5 },
        ],
        next_cursor: null,
      }),
    })
    expect(
      await screen.findByRole('row', { name: 'Story · S 2 2 ч 11' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('row', { name: 'Story · S 1 2 ч 5' }),
    ).not.toBeInTheDocument()
  })
})
