import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useQuery, focusManager } from '@tanstack/react-query'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { createBrowserRouter, createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { listTeams, listServices } from '@/shared/api'
import type { components } from '@/shared/api'
import { routePaths, taskAnalysisLinks } from '@/shared/config'
import {
  currentUser,
  projects,
  teams,
  jsonResponse,
  problemResponse,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import type { AppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

beforeAll(async () => {
  // Keep the first transform of the lazy route outside DOM assertion timeouts.
  await import('@/pages/team-report')
})

const cleanups: (() => void)[] = []
const [team] = teams.data
const [project] = projects.data
if (!team || !project) throw new Error('Expected one team and project fixture')
const processOwnerRole = {
  role: 'process_owner',
  team_id: team.id,
  project_id: null,
  service_id: null,
  assigned_at: '2026-10-01T09:00:00Z',
} as const
const admin = {
  ...currentUser,
  actions: [
    ...currentUser.actions,
    'org.manage',
    'users.manage',
    'installation.read',
    'process.manage',
  ],
  roles: [...currentUser.roles, processOwnerRole],
}
const adminWithoutAreas = {
  ...admin,
  areas: { teams: [], projects: [], services: [] },
}
const secondTeam = {
  ...team,
  id: '0193a5b0-3333-7000-8000-000000000002',
  name: 'Другая команда',
}
type CurrentUserResponse = components['schemas']['CurrentUser']
const scopedProcessOwner: CurrentUserResponse = {
  ...currentUser,
  actions: [...currentUser.actions, 'process.manage'],
  areas: { ...currentUser.areas, teams: [team, secondTeam] },
  roles: [
    ...currentUser.roles,
    processOwnerRole,
    {
      role: 'dev_lead',
      team_id: secondTeam.id,
      project_id: null,
      service_id: null,
      assigned_at: '2026-10-01T09:00:00Z',
    },
  ],
}

function mockApi(
  user: CurrentUserResponse = currentUser,
  availableTeams = teams.data,
  requestProjects?: () => Promise<Response>,
) {
  return vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(jsonResponse(user))
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse({ ...teams, data: availableTeams }))
      case '/api/v1/projects':
        if (requestProjects) return requestProjects()
        return Promise.resolve(
          jsonResponse(
            url.searchParams.get('team_id') === team?.id
              ? projects
              : { ...projects, data: [] },
          ),
        )
      case '/api/v1/session/logout':
        return Promise.resolve(new Response(null, { status: 204 }))
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
}
function mount(
  path: string,
  fetcher: typeof fetch = mockApi(admin),
  previousPath?: string,
) {
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: previousPath ? [previousPath, path] : [path],
  })
  return renderRouter(router, runtime)
}

function renderRouter(
  router: ReturnType<typeof createMemoryRouter>,
  runtime: AppRuntime,
) {
  const view = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  const dispose = () => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  }
  cleanups.push(dispose)
  return { runtime, router, dispose }
}
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  localStorage.clear()
  focusManager.setFocused(undefined)
  vi.restoreAllMocks()
})

describe('application routes', () => {
  it.each([
    ['/', 'Обзор'],
    ['/login', 'Вход'],
    ['/login/example', 'Вход'],
    ['/session-expired', 'Сессия завершилась'],
    ['/access-denied', 'Объект или область недоступны'],
    ['/installation', 'Диагностика установки'],
    ['/welcome', 'Результат первого входа'],
    ['/organization/structure', 'Структура организации'],
    ['/users', 'Пользователи и роли'],
    ['/users/123', 'Пользователи и роли'],
    [`/settings/process?team_id=${team.id}`, 'Настройки процесса'],
    [`/settings/targets?team_id=${team.id}`, 'Целевые сроки'],
    [`/reports/team?team_id=${team.id}`, 'Отчёт команды'],
    ['/reports/revisions/r1/metrics/T08', 'Паспорт показателя'],
    [
      '/reports/revisions/r1/metrics/T08/points/2026-10-01T00:00:00Z',
      'Когорта точки',
    ],
    ['/reports/saved-views/view-1', 'Сохранённый срез'],
  ])('opens %s directly', async (path, heading) => {
    mount(path)
    expect(
      await screen.findByRole('heading', { name: heading, level: 1 }),
    ).toBeInTheDocument()
  })

  it('opens a CI receipt at its historical MR head and links to the pipeline', async () => {
    const head = 'a'.repeat(40)
    const mr = '0193a5b0-b0b0-7000-8000-000000000001'
    const pipeline = '0193a5b0-c0c0-7000-8000-000000000001'
    const summaryPath = '/merge-requests/' + mr + '/summary?head_sha=' + head
    const count = (value: number): components['schemas']['MetricValue'] => ({
      state: 'ASSESSED',
      value,
      unit: 'count',
      numerator: null,
      denominator: null,
      n: null,
      bound: null,
      low_sample: false,
      excluded_count: 0,
      missing_count: 0,
      reason_code: null,
    })
    const baseApi = mockApi({
      ...currentUser,
      actions: ['session.read', 'mr.read', 'ci.read'],
    })
    const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
      if (input === '/api/v1' + summaryPath) {
        return Promise.resolve(
          jsonResponse({
            id: '0193a5b0-d0d0-7000-8000-000000000001',
            merge_request_id: mr,
            revision_no: 1,
            head_sha: head,
            base_sha: null,
            built_at: null,
            status: 'stale',
            pipeline_state: 'complete',
            stale_reason: 'new_head',
            ci: {
              pipeline_id: pipeline,
              outcome: 'success',
              attempts: 1,
              retried_jobs: 0,
            },
            publication: {
              status: 'failed',
              error_code: 'write_permission_missing',
              published_at: null,
              external_id: null,
            },
            tests: {
              passed: count(12),
              failed: count(0),
              errored: count(0),
              skipped: count(1),
            },
            change: {
              lines_added: count(24),
              lines_removed: count(5),
              files_changed: count(3),
              complexity_increased_functions: count(2),
              new_clone_groups: {
                ...count(0),
                state: 'NOT_EVALUATED',
                value: null,
                reason_code: 'incomparable_base',
              },
            },
            coverage_changed_lines: {
              ...count(0.75),
              state: 'PROVISIONAL',
              unit: 'ratio',
              numerator: 18,
              denominator: 24,
            },
            expected: { missing_checks: count(0), missing_reports: count(1) },
            last_good: {
              head_sha: 'b'.repeat(40),
              revision_no: 2,
              built_at: '2026-11-08T12:35:00Z',
            },
            not_evaluated: [
              { analysis_kind: 'code', reason_code: 'scan_failed' },
            ],
            access_limited: true,
            access_notes: [{ code: 'restricted_by_access' }],
            redactions: ['items'],
            items: [
              {
                item_no: 1,
                item_kind: 'ci_attempts',
                rule_id: null,
                location: null,
                rank: null,
                finding_id: null,
                severity: null,
              },
            ],
          } satisfies components['schemas']['MergeRequestSummary']),
        )
      }
      if (input === '/api/v1/pipelines/' + pipeline) {
        return Promise.resolve(
          jsonResponse({
            ref_name: 'feature',
            sha: head,
            outcome: 'success',
            reports: [],
          }),
        )
      }
      return baseApi(input, init)
    })

    mount(summaryPath, fetcher)
    expect(
      await screen.findByRole('heading', {
        name: 'Сводка MR, ревизия 1',
        level: 1,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTitle(head)).toBeInTheDocument()
    expect(screen.getByTitle('b'.repeat(40))).toBeInTheDocument()
    expect(screen.getByText('Историческая сводка')).toBeInTheDocument()
    expect(
      screen.getByText('У бота нет права записи в MR.'),
    ).toBeInTheDocument()
    expect(screen.getByText('Есть закрытые результаты.')).toBeInTheDocument()

    expect(screen.getByRole('heading', { name: 'Тесты' })).toBeInTheDocument()
    expect(screen.getByText('Прошли').nextElementSibling).toHaveTextContent(
      '12',
    )
    expect(screen.getByText('Упали').nextElementSibling).toHaveTextContent('0')
    expect(
      screen.getByText('Добавленные строки').nextElementSibling,
    ).toHaveTextContent('24')
    expect(
      screen.getByText('Новые группы клонов').nextElementSibling,
    ).toHaveTextContent('Нет сопоставимой базы')
    expect(
      screen.getByRole('heading', { name: 'Покрытие изменённых строк' })
        .nextElementSibling,
    ).toHaveTextContent(/75\s*%.*частично.*18 из 24/)
    expect(
      screen.getByText('Отсутствующие отчёты').nextElementSibling,
    ).toHaveTextContent('1')
    expect(screen.getByText('Анализ завершился с ошибкой')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('link', { name: 'Открыть pipeline' }))
    expect(
      await screen.findByRole('heading', { name: 'Pipeline', level: 1 }),
    ).toBeInTheDocument()
    expect(await screen.findByText('Исход: success')).toBeInTheDocument()
  })

  it('never requests private data on a public route', async () => {
    const fetcher = vi.fn<typeof fetch>()
    mount('/login/example', fetcher)
    expect(
      await screen.findByRole('heading', { name: 'Вход' }),
    ).toBeInTheDocument()
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('redirects a protected deep link without a session and preserves its complete relative URL', async () => {
    const path = '/organization/structure?team=123#form'
    const { router, runtime } = mount(
      path,
      vi
        .fn<typeof fetch>()
        .mockImplementation(() =>
          Promise.resolve(problemResponse('auth_required', 401)),
        ),
    )
    expect(
      await screen.findByRole('heading', {
        name: 'Сессия завершилась',
      }),
    ).toBeInTheDocument()
    expect(router.state.location.pathname).toBe(routePaths.sessionExpired)
    expect(
      new URLSearchParams(router.state.location.search).get('return_to'),
    ).toBe(path)
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it('does not show server detail on a session verification error', async () => {
    mount(
      '/users',
      vi
        .fn<typeof fetch>()
        .mockImplementation(() =>
          Promise.resolve(problemResponse('future_error', 500)),
        ),
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Не удалось проверить сессию',
    )
    expect(document.body.textContent).not.toContain('private-internal')
    expect(document.body.textContent).not.toContain('SQL')
  })

  it('rechecks /me on window focus and handles a session closed in another tab', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(adminWithoutAreas))
      .mockResolvedValueOnce(problemResponse('auth_required', 401))
    const { runtime } = mount('/users', fetcher)
    await screen.findByRole('heading', { name: 'Пользователи и роли' })
    await act(async () => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
      await Promise.resolve()
    })
    await screen.findByRole('heading', {
      name: 'Сессия завершилась',
    })
    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it('navigates to UI-02 after confirmed logout and clears the cache', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse(adminWithoutAreas))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const { runtime, router } = mount('/users', fetcher)
    await screen.findByRole('heading', { name: 'Пользователи и роли' })
    await act(async () => {
      await runtime.session.logout()
    })
    await screen.findByRole('heading', { name: 'Вход', level: 1 })
    expect(router.state.location.pathname).toBe(routePaths.login)
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it('clears visible private data for 401 on another endpoint and does not restore it on Back', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation((url) => {
      if (url === '/api/v1/me') return Promise.resolve(jsonResponse(admin))
      if (url === '/api/v1/teams') return Promise.resolve(jsonResponse(teams))
      return Promise.resolve(problemResponse('auth_required', 401))
    })
    const runtime = createAppRuntime(fetcher)
    function PrivateProbe({ app }: { app: AppRuntime }) {
      const query = useQuery(listTeams(app.api))
      return (
        <section>
          <p>private:{query.data?.data[0]?.name}</p>
          <button
            onClick={() => {
              void app.queryClient
                .query(listServices(app.api))
                .catch(() => undefined)
            }}
          >
            Проверить сервисы
          </button>
        </section>
      )
    }
    const routes = createAppRoutes(runtime)
    const children = routes[0]?.children?.find((route) => !route.path)
      ?.children?.[0]?.children
    if (!children) throw new Error('Missing protected shell routes')
    children.push({
      path: '/private-probe',
      element: <PrivateProbe app={runtime} />,
    })
    const router = createMemoryRouter(routes, {
      initialEntries: ['/users', '/private-probe'],
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
    await screen.findByText('private:Платформа')
    fireEvent.click(screen.getByRole('button', { name: 'Проверить сервисы' }))
    await screen.findByRole('heading', {
      name: 'Сессия завершилась',
    })
    expect(screen.queryByText('private:Платформа')).not.toBeInTheDocument()
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
    await act(async () => {
      await router.navigate(-1)
    })
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(routePaths.sessionExpired)
    })
    expect(
      new URLSearchParams(router.state.location.search).get('return_to'),
    ).toBe('/users')
    expect(screen.queryByText('private:Платформа')).not.toBeInTheDocument()
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
  })
})

describe('UI-01', () => {
  const scopedRoutes: [path: string, heading: string][] = [
    [routePaths.teamReport, 'Отчёт команды'],
    [routePaths.workflowSettings, 'Настройки процесса'],
  ]

  it.each(scopedRoutes)(
    'waits for projects before rejecting an unavailable project on %s',
    async (path, heading) => {
      let respond!: (response: Response) => void
      const pendingProjects = new Promise<Response>((resolve) => {
        respond = resolve
      })
      const requestProjects = vi.fn(() => pendingProjects)
      const foreignProject = '0193a5b0-4444-7000-8000-000000000002'
      mount(
        `${path}?team_id=${team.id}&project_id=${foreignProject}`,
        mockApi(admin, teams.data, requestProjects),
      )

      await waitFor(() => {
        expect(requestProjects).toHaveBeenCalled()
      })
      expect(
        await screen.findByRole('region', { name: 'Загрузка' }),
      ).toBeVisible()
      expect(
        screen.queryByRole('heading', { name: heading, level: 1 }),
      ).toBeNull()
      respond(jsonResponse(projects))

      expect(
        await screen.findByRole('heading', {
          name: 'Объект или область недоступны',
          level: 1,
        }),
      ).toBeVisible()
      expect(
        screen.queryByRole('heading', { name: heading, level: 1 }),
      ).toBeNull()
    },
  )

  it.each(scopedRoutes)(
    'opens %s only after retrying a failed project request',
    async (path, heading) => {
      const requestProjects = vi
        .fn(() => Promise.resolve(jsonResponse(projects)))
        .mockResolvedValueOnce(problemResponse('internal_error', 500))
      mount(
        `${path}?team_id=${team.id}&project_id=${project.id}`,
        mockApi(admin, teams.data, requestProjects),
      )

      await screen.findByText('Не удалось загрузить проекты')
      expect(
        screen.queryByRole('heading', { name: heading, level: 1 }),
      ).toBeNull()
      fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))

      expect(
        await screen.findByRole('heading', { name: heading, level: 1 }),
      ).toBeVisible()
      expect(requestProjects).toHaveBeenCalledTimes(2)
      expect(screen.getByRole('combobox', { name: 'Проект' })).toHaveValue(
        project.id,
      )
    },
  )

  it('shows the team report and hides process settings for report.read alone', async () => {
    mount(`/?team_id=${team.id}`, mockApi())
    expect(
      await screen.findByRole('link', { name: 'Отчёт команды' }),
    ).toHaveAttribute('href', `/reports/team?team_id=${team.id}`)
    expect(
      screen.queryByRole('link', { name: 'Настройки процесса' }),
    ).toBeNull()
    expect(screen.queryByRole('link', { name: 'Целевые сроки' })).toBeNull()
  })

  it('hides process settings after switching to a read-only team', async () => {
    mount(
      `/?team_id=${team.id}`,
      mockApi(scopedProcessOwner, [team, secondTeam]),
    )
    expect(
      await screen.findByRole('link', { name: 'Настройки процесса' }),
    ).toHaveAttribute('href', `/settings/process?team_id=${team.id}`)
    expect(screen.getByRole('link', { name: 'Целевые сроки' })).toHaveAttribute(
      'href',
      `/settings/targets?team_id=${team.id}`,
    )
    fireEvent.change(screen.getByRole('combobox', { name: 'Команда' }), {
      target: { value: secondTeam.id },
    })
    await waitFor(() => {
      expect(
        screen.getByRole('link', { name: 'Отчёт команды' }),
      ).toHaveAttribute('href', `/reports/team?team_id=${secondTeam.id}`)
    })
    expect(
      screen.queryByRole('link', { name: 'Настройки процесса' }),
    ).toBeNull()
    expect(screen.queryByRole('link', { name: 'Целевые сроки' })).toBeNull()
  })

  it.each([
    [routePaths.workflowSettings, 'Настройки процесса'],
    [routePaths.targetSettings, 'Целевые сроки'],
  ])(
    'blocks %s for a team where the user only reads reports',
    async (path, heading) => {
      mount(
        `${path}?team_id=${secondTeam.id}`,
        mockApi(scopedProcessOwner, [team, secondTeam]),
      )
      await screen.findByRole('link', { name: 'Отчёт команды' })
      expect(
        screen.getByRole('heading', { name: 'Объект или область недоступны' }),
      ).toBeInTheDocument()
      expect(screen.queryByRole('heading', { name: heading })).toBeNull()
    },
  )

  it.each([
    ['/settings/process', 'process.manage'],
    ['/settings/targets', 'process.manage'],
    ['/reports/team', 'report.read'],
    ['/reports/revisions/r1/metrics/T08', 'report.read'],
    ['/reports/revisions/r1/metrics/T08/points/2026-10-01', 'report.read'],
    ['/reports/saved-views/view-1', 'report.read'],
  ])('blocks a deep link to %s without %s', async (path, action) => {
    mount(
      `${path}?team_id=${team.id}`,
      mockApi({
        ...admin,
        actions: admin.actions.filter((entry) => entry !== action),
      }),
    )
    expect(
      await screen.findByRole('heading', {
        name: 'Объект или область недоступны',
      }),
    ).toBeInTheDocument()
  })

  it('shows only implemented pages allowed by actions and blocks forbidden deep links', async () => {
    const { router } = mount(
      '/',
      mockApi({
        ...currentUser,
        actions: [...currentUser.actions, 'installation.read', 'org.manage'],
      }),
    )
    const heading = await screen.findByRole('heading', {
      name: 'Обзор',
      level: 1,
    })
    const skip = screen.getByRole('link', { name: 'К содержимому' })
    skip.focus()
    fireEvent.click(skip)
    expect(heading).toHaveFocus()
    expect(screen.getByRole('link', { name: 'Обзор' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(
      screen.getByRole('link', { name: 'Диагностика установки' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Структура организации' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Пользователи и роли' }),
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Задачи и история' }),
    ).toHaveAttribute('href', '/tasks')
    await act(async () => {
      await router.navigate('/users/123')
    })
    expect(
      await screen.findByRole('heading', {
        name: 'Объект или область недоступны',
      }),
    ).toHaveFocus()
  })

  it('restores a team and project from the address and requests projects for that team', async () => {
    const fetcher = mockApi()
    mount(`/?team_id=${team.id}&project_id=${project.id}`, fetcher)
    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Команда' })).toHaveValue(
        team.id,
      )
      expect(screen.getByRole('combobox', { name: 'Проект' })).toHaveValue(
        project.id,
      )
    })
    expect(fetcher).toHaveBeenCalledWith(
      `/api/v1/projects?team_id=${team.id}`,
      expect.objectContaining({ method: 'GET' }),
    )
    expect(screen.getAllByRole('option', { name: team.name })).toHaveLength(1)
    expect(screen.getAllByRole('option', { name: project.name })).toHaveLength(
      1,
    )
  })

  it('preserves view and period but drops the previous project, revision and object on scope change', async () => {
    const { router } = mount(
      `/users/123?team_id=${team.id}&project_id=${project.id}&view=overview&from=2026-10-01&revision_id=r1#form`,
      mockApi(admin, [...teams.data, secondTeam]),
    )
    const teamSelect = await screen.findByRole('combobox', { name: 'Команда' })
    await waitFor(() => {
      expect(teamSelect).toHaveValue(team.id)
    })
    fireEvent.change(teamSelect, { target: { value: secondTeam.id } })
    expect(router.state.location.pathname).toBe('/users')
    expect(router.state.location.hash).toBe('')
    expect(
      Object.fromEntries(new URLSearchParams(router.state.location.search)),
    ).toEqual({
      team_id: secondTeam.id,
      view: 'overview',
      from: '2026-10-01',
    })
    expect(
      await screen.findByText(
        'Несовместимые параметры сброшены при смене области.',
      ),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: project.name }),
    ).not.toBeInTheDocument()
  })

  it('uses the last accessible area on an unqualified start and changes only view', async () => {
    localStorage.setItem(
      `craftpulse.scope.${currentUser.organization.id}.${currentUser.user.id}`,
      `team_id=${team.id}&project_id=${project.id}`,
    )
    const { router } = mount('/', mockApi())
    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Проект' })).toHaveValue(
        project.id,
      )
    })
    fireEvent.change(screen.getByRole('combobox', { name: 'Вид панели' }), {
      target: { value: 'tasks' },
    })
    expect(
      Object.fromEntries(new URLSearchParams(router.state.location.search)),
    ).toEqual({
      team_id: team.id,
      project_id: project.id,
      view: 'tasks',
    })
  })

  it('does not request projects for a team absent from the API response', async () => {
    const fetcher = mockApi()
    mount('/?team_id=unknown&project_id=unknown', fetcher)
    await screen.findByText(
      'Объект или область недоступны. Выберите доступную область.',
    )
    expect(screen.getByRole('combobox', { name: 'Команда' })).toHaveValue('')
    expect(screen.getByRole('combobox', { name: 'Проект' })).toBeDisabled()
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('reloads the area lists when another organization replaces the session', async () => {
    const nextUser = {
      ...currentUser,
      organization: {
        ...currentUser.organization,
        id: '0193a5b0-3333-7000-8000-000000000003',
        name: 'Другая организация',
        slug: 'another-org',
      },
    }
    const initialApi = mockApi()
    const nextApi = mockApi(nextUser, [secondTeam])
    let fetcher = initialApi
    const { runtime } = mount(`/?team_id=${team.id}`, (input, init) =>
      fetcher(input, init),
    )
    await screen.findByRole('option', { name: team.name })
    fetcher = nextApi
    await act(async () => {
      await runtime.queryClient.refetchQueries({ queryKey: ['session'] })
    })
    await screen.findByText(nextUser.organization.name)
    expect(screen.queryByRole('option', { name: team.name })).toBeNull()
    await screen.findByRole('option', { name: secondTeam.name })
    expect(screen.getByRole('combobox', { name: 'Проект' })).toBeDisabled()
    expect(
      nextApi.mock.calls.some(
        ([input]) =>
          typeof input === 'string' && input.startsWith('/api/v1/projects'),
      ),
    ).toBe(false)
  })

  it('opens the organization structure for an administrator without areas', async () => {
    const fetcher = mockApi(adminWithoutAreas)
    mount('/', fetcher)
    expect(
      await screen.findByRole('heading', { name: 'Структура организации' }),
    ).toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('requests only the session for a user without areas', async () => {
    const fetcher = mockApi({
      ...currentUser,
      areas: { teams: [], projects: [], services: [] },
    })
    mount('/', fetcher)
    expect(
      await screen.findByRole('heading', { name: 'Нет доступной области' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Команда' }),
    ).not.toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('clears server data on logout and keeps it hidden on Back', async () => {
    const fetcher = mockApi(admin)
    const { runtime, router } = mount('/', fetcher, '/users')
    await screen.findByRole('heading', { name: 'Обзор', level: 1 })
    fireEvent.click(screen.getByRole('button', { name: 'Выход' }))
    await screen.findByRole('heading', { name: 'Вход', level: 1 })
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/session/logout',
      expect.objectContaining({ method: 'POST' }),
    )
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
    await act(async () => {
      await router.navigate(-1)
    })
    expect(
      await screen.findByRole('heading', { name: 'Вход', level: 1 }),
    ).toBeInTheDocument()
    expect(screen.queryByText(admin.organization.name)).not.toBeInTheDocument()
    expect(runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
  })
})

describe('task analysis routes', () => {
  const taskReader = { ...currentUser, actions: ['session.read', 'tasks.read'] }
  const mrReader = { ...currentUser, actions: ['session.read', 'mr.read'] }
  const scope = { project_id: project.id }

  it.each([
    [
      taskAnalysisLinks.taskList({
        team_id: team.id,
        status_meaning: 'wait',
        sort: '-age',
      }),
      'Задачи',
      taskReader,
    ],
    [
      taskAnalysisLinks.taskList({ ...scope, over_target: false }),
      'Задачи',
      taskReader,
    ],
    [
      taskAnalysisLinks.taskDetail('task-1', scope),
      'Карточка задачи',
      taskReader,
    ],
    [
      taskAnalysisLinks.projectTimeline({
        ...scope,
        date: '2026-10-02',
        actor_kind: 'bot',
        object_kind: 'task',
      }),
      'Таймлайн проекта',
      taskReader,
    ],
    [
      taskAnalysisLinks.projectTimeline({
        ...scope,
        from: '2026-10-02T00:00:00Z',
        to: '2026-10-03T00:00:00Z',
      }),
      'Таймлайн проекта',
      taskReader,
    ],
    [
      taskAnalysisLinks.mergeRequestQueue({
        ...scope,
        review_wait: 'no_reviewer',
      }),
      'Очередь MR',
      mrReader,
    ],
  ] as const)(
    'opens %s directly and with a fresh router at the same browser URL',
    async (path, heading, user) => {
      const original = window.location.href
      window.history.replaceState(null, '', path)
      cleanups.push(() => {
        window.history.replaceState(null, '', original)
      })
      for (let load = 0; load < 2; load += 1) {
        const runtime = createAppRuntime(mockApi(user))
        const router = createBrowserRouter(createAppRoutes(runtime))
        const { dispose } = renderRouter(router, runtime)
        expect(
          await screen.findByRole('heading', { name: heading, level: 1 }),
        ).toBeVisible()
        expect(screen.getByRole('main')).toContainElement(
          screen.getByRole('heading', { name: heading, level: 1 }),
        )
        expect(window.location.pathname + window.location.search).toBe(path)
        cleanups.splice(cleanups.lastIndexOf(dispose), 1)
        dispose()
      }
    },
  )

  it('allows the task section and hides MR and CI for a tasks-only reader', async () => {
    mount(`/tasks?project_id=${project.id}`, mockApi(taskReader))
    expect(
      await screen.findByRole('link', { name: 'Задачи и история' }),
    ).toHaveAttribute('href', `/tasks?project_id=${project.id}`)
    expect(
      screen.getByRole('link', { name: 'Таймлайн проекта' }),
    ).toHaveAttribute('href', `/timeline?project_id=${project.id}`)
    expect(
      screen.queryByRole('link', { name: 'MR и CI' }),
    ).not.toBeInTheDocument()
  })

  it('allows MR and CI and hides the task section for an MR-only reader', async () => {
    mount(`/merge-requests?project_id=${project.id}`, mockApi(mrReader))
    expect(await screen.findByRole('link', { name: 'MR и CI' })).toBeVisible()
    expect(
      screen.queryByRole('link', { name: 'Задачи и история' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Таймлайн проекта' }),
    ).not.toBeInTheDocument()
  })

  it('rejects a direct queue link for a tasks-only reader', async () => {
    mount(`/merge-requests?project_id=${project.id}`, mockApi(taskReader))
    expect(
      await screen.findByRole('heading', {
        name: 'Объект или область недоступны',
      }),
    ).toBeVisible()
  })

  it('preserves the timeline date and drops filters and cursor when switching projects', async () => {
    const second = { ...project, id: 'second-project', name: 'Второй проект' }
    const path = taskAnalysisLinks.projectTimeline({
      ...scope,
      date: '2026-10-02',
      actor_kind: 'bot',
      cursor: 'page-2',
    })
    const { router } = mount(
      path,
      mockApi({
        ...taskReader,
        areas: { ...taskReader.areas, projects: [project, second] },
      }),
    )
    await screen.findByRole('heading', { name: 'Таймлайн проекта' })
    fireEvent.change(screen.getByRole('combobox', { name: 'Проект' }), {
      target: { value: second.id },
    })
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Несовместимые параметры сброшены при смене области',
    )
    expect(router.state.location.pathname).toBe('/timeline')
    expect([...new URLSearchParams(router.state.location.search)]).toEqual([
      ['date', '2026-10-02'],
      ['project_id', second.id],
    ])
  })
})

const summaryMr = '0193a5b0-b0b0-7000-8000-000000000001'
const summaryURL = `/merge-requests/${summaryMr}/summary?revision_no=3`
const summaryFixture = {
  id: 'summary-3',
  merge_request_id: summaryMr,
  revision_no: 3,
  head_sha: 'a'.repeat(40),
  base_sha: null,
  built_at: null,
  status: 'superseded',
  pipeline_state: 'complete',
  stale_reason: null,
  access_limited: true,
  access_notes: [],
  redactions: [],
  publication: {
    status: 'failed',
    error_code: 'write_permission_missing',
    published_at: null,
    external_id: null,
  },
  coverage_changed_lines: {
    state: 'NOT_EVALUATED',
    reason_code: 'report_missing',
    value: null,
    unit: 'ratio',
    numerator: null,
    denominator: null,
    n: null,
    bound: null,
    low_sample: false,
    excluded_count: 0,
    missing_count: 0,
  },
  ci: null,
  items: [
    {
      item_no: 1,
      item_kind: 'accepted',
      finding_id: 'finding-1',
      rule_id: 'rule-1',
      severity: 'high',
      location: null,
      rank: null,
    },
  ],
  not_evaluated: [],
} satisfies components['schemas']['MergeRequestSummary']

function mockTechnicalApi(action: string) {
  const fallback = mockApi({
    ...currentUser,
    actions: ['session.read', action],
  })
  return vi
    .fn<typeof fetch>()
    .mockImplementation((input, init) =>
      input === '/api/v1' + summaryURL
        ? Promise.resolve(jsonResponse(summaryFixture))
        : fallback(input, init),
    )
}

describe('technical screen routes', () => {
  it.each([
    [summaryURL + '&org_slug=example', 'Сводка MR, ревизия 3', 'mr.read'],
    [
      '/components/component-1/comparison?base_sha=aaaa&head_sha=bbbb&analysis_kind=code',
      'Сравнение результатов проверок',
      'findings.read',
    ],
    ['/pipelines/pipeline-1', 'Pipeline', 'ci.read'],
    [
      '/pipelines/pipeline-1/jobs/test%3A%20matrix%2Flinux',
      'Попытки задания CI',
      'ci.read',
    ],
    [
      `/findings?project_id=${project.id}&finding_kind=security&state=open&severity=high&cursor=next%2B1`,
      'Находки кода и безопасности',
      'findings.read',
    ],
    ['/findings/finding-1', 'Карточка находки', 'findings.read'],
    ['/findings/finding-1/exception', 'Исключение', 'exceptions.manage'],
    ['/exceptions/exception-1/extend', 'Исключение', 'exceptions.manage'],
    [
      '/exceptions?state=pending&component_id=component-1',
      'История исключений',
      'findings.read',
    ],
    [
      '/exceptions/exception-1?state=pending&component_id=component-1',
      'История исключений',
      'findings.read',
    ],
    [
      '/components/overview/component-1',
      'Технический обзор компонента',
      'findings.read',
    ],
    ['/checks/profile/project-1', 'Матрица проверок', 'check_profiles.manage'],
    ['/checks/baselines/project-1', 'Базовые линии', 'ci.read'],
    [
      '/checks/setup/project-1?ci_system=gitlab_ci',
      'Подключение шага CI',
      'check_profiles.manage',
    ],
    ['/documentation/roots/service-1', 'Корни документации', 'docs.read'],
    ['/documentation/services/service-1', 'Документация сервиса', 'docs.read'],
    [
      '/documentation/findings/finding-1',
      'Нарушение документации',
      'findings.read',
    ],
  ])(
    'opens %s directly and after loading the URL again',
    async (path, heading, action) => {
      const original = window.location.href
      window.history.replaceState(null, '', path)
      cleanups.push(() => {
        window.history.replaceState(null, '', original)
      })

      for (let load = 0; load < 2; load += 1) {
        const runtime = createAppRuntime(mockTechnicalApi(action))
        const router = createBrowserRouter(createAppRoutes(runtime))
        const { dispose } = renderRouter(router, runtime)
        expect(
          await screen.findByRole('heading', { name: heading, level: 1 }),
        ).toBeVisible()
        expect(window.location.pathname + window.location.search).toBe(path)
        cleanups.splice(cleanups.lastIndexOf(dispose), 1)
        dispose()
      }
    },
  )

  it('opens the exact revision from an MR comment and shows the publication refusal', async () => {
    mount(summaryURL + '&org_slug=example', mockTechnicalApi('mr.read'))
    expect(
      await screen.findByRole('heading', { name: 'Сводка MR, ревизия 3' }),
    ).toBeVisible()
    expect(screen.getByText('У бота нет права записи в MR.')).toBeVisible()
    expect(screen.getByText('Есть закрытые результаты.')).toBeVisible()
    expect(screen.getByText('Принята', { exact: false })).toBeVisible()
    expect(screen.getByText('не проверено')).toBeVisible()
  })

  it('rejects a different head when the requested revision is already cached', async () => {
    const mismatchedURL = summaryURL + '&head_sha=' + 'b'.repeat(40)
    const fallback = mockTechnicalApi('mr.read')
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation((input, init) =>
        input === '/api/v1' + mismatchedURL
          ? Promise.resolve(jsonResponse(summaryFixture))
          : fallback(input, init),
      )
    const { router } = mount(summaryURL, fetcher)
    const heading = { name: 'Сводка MR, ревизия 3' }
    expect(await screen.findByRole('heading', heading)).toBeVisible()

    await act(() => router.navigate(mismatchedURL))
    expect(
      await screen.findByText('Не удалось загрузить сводку MR'),
    ).toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1' + mismatchedURL,
      expect.objectContaining({ method: 'GET' }),
    )
    expect(screen.queryByRole('heading', heading)).not.toBeInTheDocument()
  })

  const links = [
    ['Находки', '/findings', 'Находки кода и безопасности', 'findings.read'],
    [
      'Технический обзор компонента',
      '/components/overview',
      'Технический обзор компонента',
      'findings.read',
    ],
    [
      'Документация сервиса',
      '/documentation/services',
      'Документация сервиса',
      'docs.read',
    ],
    [
      'Матрица проверок',
      '/checks/profile',
      'Матрица проверок',
      'check_profiles.manage',
    ],
    [
      'Подключение шага CI',
      '/checks/setup',
      'Подключение шага CI',
      'check_profiles.manage',
    ],
  ] as const

  it.each(links)(
    'permits navigation to %s with the required action',
    async (label, path, heading, action) => {
      mount('/', mockTechnicalApi(action))
      const link = await screen.findByRole('link', { name: label })
      expect(link).toHaveAttribute('href', path)
      link.focus()
      expect(link).toHaveFocus()
      fireEvent.click(link)
      expect(
        await screen.findByRole('heading', { name: heading }),
      ).toHaveFocus()
      expect(screen.getByRole('link', { name: label })).toHaveAttribute(
        'aria-current',
        'page',
      )
      for (const [otherLabel, , , otherAction] of links) {
        if (otherAction !== action)
          expect(
            screen.queryByRole('link', { name: otherLabel }),
          ).not.toBeInTheDocument()
      }
    },
  )

  it.each(links)(
    'blocks the direct link for %s without the required action',
    async (label, path) => {
      mount(path, mockApi(currentUser))
      expect(
        await screen.findByRole('heading', {
          name: 'Объект или область недоступны',
        }),
      ).toBeVisible()
      expect(
        screen.queryByRole('link', { name: label }),
      ).not.toBeInTheDocument()
    },
  )
})
