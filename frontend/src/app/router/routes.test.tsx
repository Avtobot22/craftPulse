import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useQuery, focusManager } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { listTeams, listServices } from '@/shared/api'
import { routePaths } from '@/shared/config'
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
const scopedProcessOwner = {
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

function mockApi(user = currentUser, availableTeams = teams.data) {
  return vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(jsonResponse(user))
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse({ ...teams, data: availableTeams }))
      case '/api/v1/projects':
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
  return { runtime, router }
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
      screen.queryByRole('link', { name: 'Задачи и история' }),
    ).not.toBeInTheDocument()
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
