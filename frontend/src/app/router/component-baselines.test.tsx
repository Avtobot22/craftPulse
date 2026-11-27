import {
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
import {
  currentUser,
  jsonResponse,
  structure,
  teams,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const [component] = structure.components
const [project] = structure.projects
const [serviceFixture] = structure.services
if (!component || !project || !serviceFixture)
  throw new Error('Missing structure fixtures')
const projectId = project.id
const service = serviceFixture
const componentPath = `/components/overview/${component.id}`
const baselinesPath = `/checks/baselines/${projectId}`
const overviewRequest = `/api/v1/components/${component.id}/overview`
const baselineRequest = `/api/v1/projects/${projectId}/baselines`
const reportId = '0193a5b0-d4d4-7000-8000-000000000001'
const sha = '1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d'
const mr = '0193a5b0-b0b0-7000-8000-000000000001'
const head = '9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e'
const established = '2026-10-10T13:00:00Z'
const establishedContext = { from: '2026-10-01T00:00:00Z', to: established }
const pending = {
  component_id: component.id,
  language: '',
  analysis_kind: 'sca',
  state: 'pending',
  report_id: null,
  branch: 'main',
  sha: null,
  established_at: null,
  reason_code: 'full_run_missing',
} satisfies Schema['BaselineList']['data'][number]

const baselines = {
  data: [
    pending,
    {
      ...pending,
      analysis_kind: 'sast',
      language: 'go',
      state: 'failed',
      reason_code: 'scan_failed',
    },
    {
      ...pending,
      analysis_kind: 'code',
      language: 'go',
      state: 'ready',
      report_id: reportId,
      sha,
      established_at: established,
      reason_code: null,
    },
  ],
} satisfies Schema['BaselineList']

const metric = {
  state: 'ASSESSED',
  value: 7,
  unit: 'count',
  numerator: null,
  denominator: null,
  n: null,
  bound: null,
  low_sample: false,
  excluded_count: 0,
  missing_count: 0,
  reason_code: null,
} satisfies Schema['MetricValue']

const empty = {
  component: { ...component, status: 'confirmed' },
  findings_by_state: [],
  facts: [],
  hotspots: [],
  access_limited: false,
  access_notes: [],
  redactions: [],
} satisfies Schema['ComponentOverview']

const overview = {
  ...empty,
  findings_by_state: [
    { finding_kind: 'code', state: 'open', count: metric },
    {
      finding_kind: 'code',
      state: 'accepted_risk',
      count: { ...metric, value: 1 },
    },
    {
      finding_kind: 'security',
      state: 'false_positive',
      count: { ...metric, value: 2 },
    },
  ],
  facts: [
    {
      metric_id: 'C05',
      source: 'builtin',
      window: establishedContext,
      value: {
        ...metric,
        value: 0.07,
        unit: 'ratio',
        numerator: 7,
        denominator: 100,
      },
    },
    {
      metric_id: 'C05',
      source: 'import',
      window: { ...establishedContext, from: '2026-10-02T00:00:00Z' },
      value: {
        ...metric,
        value: 0.26,
        unit: 'ratio',
        numerator: 26,
        denominator: 100,
      },
    },
    {
      metric_id: 'C09',
      source: 'builtin',
      window: establishedContext,
      value: {
        ...metric,
        state: 'NOT_EVALUATED',
        value: null,
        missing_count: 1,
        reason_code: 'report_missing',
      },
    },
  ],
  hotspots: [
    {
      path: 'future-feature.go',
      complexity: 55,
      churn: 12,
      authors_share: 0.5,
    },
  ],
  access_notes: [{ code: 'restricted_by_access' }],
} satisfies Schema['ComponentOverview']

const cleanups: (() => void)[] = []
let nextHistoryKey = 0

function mount(
  path: string,
  {
    componentOverview = overview,
    loadBaselines = () => baselines,
  }: {
    componentOverview?: Schema['ComponentOverview']
    loadBaselines?: () => Schema['BaselineList']
  } = {},
) {
  const requests: URL[] = []
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    requests.push(url)
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(
          jsonResponse({
            ...currentUser,
            areas: { ...currentUser.areas, services: [service] },
            actions: [
              'session.read',
              'findings.read',
              'ci.read',
              'check_profiles.manage',
              'mr.read',
            ],
          } satisfies Schema['CurrentUser']),
        )
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case overviewRequest:
        return Promise.resolve(jsonResponse(componentOverview))
      case baselineRequest:
        return Promise.resolve(jsonResponse(loadBaselines()))
      case '/api/v1/findings':
        return Promise.resolve(
          jsonResponse({
            data: [],
            next_cursor: null,
            access_limited: false,
            access_notes: [],
          } satisfies Schema['FindingPage']),
        )
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
  vi.spyOn(window, 'scrollTo').mockReturnValue(undefined)
  const runtime = createAppRuntime(fetcher)
  const entry = new URL(path, 'https://craftpulse.test')
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [
      {
        pathname: entry.pathname,
        search: entry.search,
        key: `technical-test-${String(++nextHistoryKey)}`,
      },
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
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { router, requests }
}

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  localStorage.clear()
})

it('показывает факты каждого источника со своим окном и состоянием доступа', async () => {
  mount(componentPath)
  const facts = await screen.findByRole('region', { name: 'Структура кода' })
  const [builtin, imported, missing] = within(facts).getAllByRole('listitem')
  if (!builtin || !imported || !missing) throw new Error('Missing code facts')

  for (const [row, origin, value, from] of [
    [builtin, 'builtin', '7', establishedContext.from],
    [imported, 'import', '26', '2026-10-02T00:00:00Z'],
  ] as const) {
    expect(within(row).getByRole('heading', { name: /C05/u })).toBeVisible()
    expect(
      within(row).getByText(new RegExp(`\\(${origin}\\)`, 'u')),
    ).toBeVisible()
    expect(
      within(row).getByText(new RegExp(`^${value}\\s*%$`, 'u')),
    ).toBeVisible()
    expect(row.querySelector('time')).toHaveAttribute('datetime', from)
  }
  expect(within(missing).getByText('не проверено')).toBeVisible()
  expect(within(missing).getByText('Отчёт проверки отсутствует')).toBeVisible()
  expect(screen.getByText('Есть закрытые результаты.')).toBeVisible()
  expect(screen.getByText(component.id)).toBeVisible()
  expect(screen.getByText(service.name)).toBeVisible()
  expect(screen.getByText(component.repository_area_id)).toBeVisible()
  expect(screen.getByText(component.path_prefix)).toBeVisible()
  expect(screen.queryByText(/future-feature|горячие точки/u)).toBeNull()
})

it('показывает отсутствие отчётов без нулевых значений и отмечает redactions', async () => {
  mount(componentPath, {
    componentOverview: { ...empty, redactions: ['restricted_field'] },
  })
  const findings = await screen.findByRole('region', {
    name: 'Находки по видам и состояниям',
  })
  const facts = screen.getByRole('region', { name: 'Структура кода' })
  expect(within(findings).getByText('не проверено')).toBeVisible()
  expect(within(facts).getByText('не проверено')).toBeVisible()
  expect(within(findings).queryByRole('link')).toBeNull()
  expect(within(facts).queryByText(/^0/u)).toBeNull()
  expect(screen.getByText('Есть закрытые результаты.')).toBeVisible()
})

it('сохраняет фильтры числа в UI-45 и восстанавливает фокус перед переходом в UI-52', async () => {
  const { router, requests } = mount(`${componentPath}?project_id=${projectId}`)
  const count = await screen.findByRole('link', {
    name: /7 шт.*Код · Открыта/u,
  })
  for (const [label, kind, state] of [
    ['Код · Открыта', 'code', 'open'],
    ['Код · Принята (принят риск)', 'code', 'accepted_risk'],
    [
      'Безопасность · Принята (ложное срабатывание)',
      'security',
      'false_positive',
    ],
  ]) {
    const links = within(
      screen.getByRole('region', { name: 'Находки по видам и состояниям' }),
    ).getAllByRole('link')
    const link = links.find((item) => item.textContent.includes(label ?? ''))
    if (!link) throw new Error(`Missing count link: ${label ?? ''}`)
    const target = new URL(
      link.getAttribute('href') ?? '',
      'https://craftpulse.test',
    )
    expect(target.pathname).toBe('/findings')
    expect(Object.fromEntries(target.searchParams)).toEqual({
      component_id: component.id,
      finding_kind: kind,
      state,
    })
  }
  expect(screen.getByRole('heading', { name: /C11/u })).toBeVisible()
  expect(screen.getByRole('heading', { name: /C26/u })).toBeVisible()
  expect(screen.getByRole('heading', { name: /S35/u })).toBeVisible()

  count.focus()
  expect(count).toHaveFocus()
  fireEvent.click(count)
  await screen.findByText('По фильтру находок нет')
  const findings = requests.find(
    ({ pathname }) => pathname === '/api/v1/findings',
  )
  expect(findings?.searchParams.get('component_id')).toBe(component.id)
  expect(findings?.searchParams.get('finding_kind')).toBe('code')
  expect(findings?.searchParams.get('state')).toBe('open')
  fireEvent.click(screen.getByRole('button', { name: 'Назад' }))
  const returnedCount = await screen.findByRole('link', {
    name: /7 шт.*Код · Открыта/u,
  })
  await waitFor(() => {
    expect(returnedCount).toHaveFocus()
  })
  const main = screen.getByRole('main')
  expect(
    within(main).getByRole('link', { name: 'Матрица проверок' }),
  ).toHaveAttribute(
    'href',
    expect.stringContaining(`/checks/profile/${projectId}`),
  )
  expect(
    within(main).getByRole('link', { name: 'Причины изменения результата' }),
  ).toHaveAttribute(
    'href',
    expect.stringContaining(`/components/${component.id}/comparison`),
  )
  fireEvent.click(within(main).getByRole('link', { name: 'Базовые линии' }))
  expect(
    await screen.findByRole('table', { name: 'Матрица базовых линий' }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe(baselinesPath)
})

it('отдельно показывает pending и failed с причинами и открывает сведения о готовом отчёте', async () => {
  mount(`${baselinesPath}?mr_id=${mr}&head_sha=${head}&revision_no=2`)
  const table = await screen.findByRole('table', {
    name: 'Матрица базовых линий',
  })
  const [header, pendingRow, failedRow, readyRow] =
    within(table).getAllByRole('row')
  if (!header || !pendingRow || !failedRow || !readyRow)
    throw new Error('Missing baseline rows')
  expect(within(pendingRow).getByText('Ожидает отчёта')).toBeVisible()
  expect(
    within(pendingRow).getByText('Нет полного прогона основной ветки'),
  ).toBeVisible()
  expect(within(failedRow).getByText('Не удалось подготовить')).toBeVisible()
  expect(
    within(failedRow).getByText('Проверка завершилась ошибкой'),
  ).toBeVisible()
  for (const row of [pendingRow, failedRow]) {
    expect(within(row).queryByText('Готова')).toBeNull()
    expect(within(row).getByText('Нет отчёта')).toBeVisible()
    expect(
      within(row).queryByRole('link', { name: /Открыть отчёт/u }),
    ).toBeNull()
  }
  expect(within(readyRow).getByText('Готова')).toBeVisible()
  expect(within(readyRow).getByText('main')).toBeVisible()
  expect(within(readyRow).getByLabelText(sha)).toBeVisible()
  expect(readyRow.querySelector('time')).toHaveAttribute(
    'datetime',
    established,
  )
  const main = within(screen.getByRole('main'))
  expect(main.getByRole('link', { name: 'Открыть сводку MR' })).toHaveAttribute(
    'href',
    `/merge-requests/${mr}/summary?project_id=${projectId}&revision_no=2&head_sha=${head}`,
  )
  expect(main.getByRole('link', { name: 'Матрица проверок' })).toHaveAttribute(
    'href',
    expect.stringContaining(`/checks/profile/${projectId}`),
  )
  fireEvent.click(
    within(readyRow).getByRole('link', { name: /Открыть отчёт/u }),
  )
  expect(await screen.findByText(reportId)).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Отчёт базовой линии' }),
  ).toBeVisible()
})

it('перечитывает состояние базовой линии по запросу пользователя', async () => {
  let loads = 0
  mount(baselinesPath, {
    loadBaselines: () => (++loads === 1 ? { data: [pending] } : baselines),
  })
  expect(await screen.findByText('Ожидает отчёта')).toBeVisible()
  const refresh = screen.getByRole('button', { name: 'Обновить' })
  refresh.focus()
  fireEvent.click(refresh)
  expect(await screen.findByText('Готова')).toBeVisible()
  expect(refresh).toHaveFocus()
  expect(loads).toBe(2)
})
