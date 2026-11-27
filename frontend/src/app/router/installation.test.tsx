import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import type { components } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  projects,
  structure,
  teams,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

const admin: components['schemas']['CurrentUser'] = {
  ...currentUser,
  roles: [
    {
      role: 'administrator',
      team_id: null,
      project_id: null,
      service_id: null,
      assigned_at: '2026-10-01T09:00:00Z',
    },
  ],
  actions: ['session.read', 'org.manage', 'installation.read', 'report.read'],
  areas: { teams: [], projects: [], services: [] },
}

const unconfirmedStructure: components['schemas']['OrganizationStructure'] = {
  version: 0,
  teams: [],
  projects: [],
  services: [],
  components: [],
  team_projects: [],
  project_areas: [],
  team_members: [],
}

// Example from getLatestInstallationProtocol in OpenAPI.
const protocol = {
  id: '0193a5b0-eeee-7000-8000-000000000001',
  release_sha: '3f9c2a7b1d4e5f60718293a4b5c6d7e8f9012345',
  started_at: '2026-11-08T08:00:00Z',
  finished_at: '2026-11-08T08:01:10Z',
  outcome: 'passed',
  checks: [
    { check_kind: 'identity_provider', outcome: 'passed', reason_code: null },
    { check_kind: 'update_source', outcome: 'passed', reason_code: null },
  ],
} satisfies components['schemas']['InstallationProtocol']

const structurePath = '/api/v1/organization/structure'
const initialStructure = () =>
  jsonResponse(unconfirmedStructure, 200, { ETag: '"0"' })
const protocolPath = '/api/v1/installation/protocols/latest'
const cleanups: (() => void)[] = []

function mount(
  path: string,
  {
    user = admin,
    readStructure = initialStructure,
    readProtocol = () => jsonResponse(protocol),
  }: {
    user?: components['schemas']['CurrentUser']
    readStructure?: () => Response | Promise<Response>
    readProtocol?: () => Response
  } = {},
) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse(user)
      case '/api/v1/teams':
        return jsonResponse(teams)
      case '/api/v1/projects':
        return jsonResponse(projects)
      case '/api/v1/areas':
      case '/api/v1/persons':
        return jsonResponse({ data: [], next_cursor: null })
      case structurePath:
        return readStructure()
      case protocolPath:
        return readProtocol()
      default:
        throw new Error(`Unexpected request: ${input}`)
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
  cleanups.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { fetcher, router }
}

afterEach(() => {
  for (const dispose of cleanups.splice(0)) dispose()
  localStorage.clear()
})

describe('administrator start', () => {
  it('opens UI-06 and links to UI-07 when the structure version is zero', async () => {
    const { router } = mount(routePaths.home)
    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: 'Результат первого входа' }),
      ).toHaveFocus()
    })
    expect(router.state.location.pathname).toBe(routePaths.welcome)
    expect(screen.getByText('Организация: Пример')).toBeVisible()
    expect(screen.getByText('Ваша роль: Администратор')).toBeVisible()
    const next = screen.getByRole('link', {
      name: 'Описать структуру организации',
    })
    expect(next).toHaveAttribute('href', routePaths.organization)
    next.focus()
    expect(next).toHaveFocus()
    fireEvent.click(next)
    await screen.findByRole('heading', {
      name: 'Структура организации',
      level: 1,
    })
    expect(router.state.location.pathname).toBe(routePaths.organization)
  })

  it('uses the normal start after confirmation, even without assigned areas', async () => {
    const { fetcher, router } = mount(routePaths.home, {
      readStructure: () => jsonResponse(structure),
    })
    await screen.findByRole('heading', { name: 'Нет доступной области' })
    expect(fetcher.mock.calls.map(([path]) => path)).toContain(structurePath)
    expect(router.state.location.pathname).toBe(routePaths.home)
    expect(
      screen.queryByRole('heading', { name: 'Результат первого входа' }),
    ).not.toBeInTheDocument()
  })

  it('restores the last allowed team once the structure is confirmed', async () => {
    const [team] = teams.data
    if (!team) throw new Error('Expected a team fixture')
    const teamId = team.id
    const key = `craftpulse.scope.${admin.organization.id}.${admin.user.id}`
    localStorage.setItem(key, `team_id=${teamId}`)
    const { router } = mount(routePaths.home, {
      user: { ...admin, areas: { ...admin.areas, teams: teams.data } },
      readStructure: () => jsonResponse(structure),
    })
    await waitFor(() => {
      expect(router.state.location.search).toBe(`?team_id=${teamId}`)
    })
    expect(
      screen.queryByRole('heading', { name: 'Результат первого входа' }),
    ).not.toBeInTheDocument()
  })

  it('rechecks a cached zero version before reopening UI-06', async () => {
    let finishRequest: () => void = () => {
      throw new Error('Expected a pending structure request')
    }
    const pendingStructure = new Promise<Response>((resolve) => {
      finishRequest = () => {
        resolve(jsonResponse(structure))
      }
    })
    const readStructure = vi
      .fn<() => Response | Promise<Response>>()
      .mockImplementationOnce(initialStructure)
      .mockImplementationOnce(initialStructure)
      .mockReturnValueOnce(pendingStructure)
    const { router } = mount(routePaths.home, { readStructure })
    await screen.findByRole('heading', { name: 'Результат первого входа' })
    expect(readStructure).toHaveBeenCalledTimes(1)
    await act(() => router.navigate(routePaths.organization))
    await screen.findByText('Сохранённая версия структуры: 0')
    expect(readStructure).toHaveBeenCalledTimes(2)
    await act(() => router.navigate(routePaths.welcome))
    expect(readStructure).toHaveBeenCalledTimes(3)
    expect(
      screen.queryByRole('heading', { name: 'Результат первого входа' }),
    ).not.toBeInTheDocument()
    act(() => {
      finishRequest()
    })
    await screen.findByRole('heading', { name: 'Нет доступной области' })
    expect(router.state.location.pathname).toBe(routePaths.home)
  })

  it('does not read the structure for a regular user', async () => {
    const { fetcher } = mount(routePaths.home, { user: currentUser })
    await screen.findByRole('heading', { name: 'Обзор' })
    expect(fetcher.mock.calls.map(([path]) => path)).not.toContain(
      structurePath,
    )
  })

  it('offers a retry without showing the assignment when the structure request fails', async () => {
    const readStructure = vi
      .fn()
      .mockReturnValueOnce(problemResponse('unavailable', 503))
      .mockImplementation(() => jsonResponse(unconfirmedStructure))
    mount(routePaths.home, { readStructure })
    expect(
      await screen.findByRole('alert', { name: 'Ошибка' }),
    ).toHaveTextContent('Не удалось проверить структуру организации')
    expect(
      screen.queryByRole('heading', { name: 'Результат первого входа' }),
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await screen.findByRole('heading', { name: 'Результат первого входа' })
  })
})

describe('installation protocol', () => {
  it('shows the OpenAPI example and copies the full release SHA', async () => {
    const writeText = vi
      .fn<(value: string) => Promise<void>>()
      .mockResolvedValue(undefined)
    const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    cleanups.push(() => {
      if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard)
      else Reflect.deleteProperty(navigator, 'clipboard')
    })

    mount(routePaths.installation)
    const heading = await screen.findByRole('heading', {
      name: 'Диагностика установки',
      level: 1,
    })
    const table = await screen.findByRole('table', { name: 'Проверки' })
    expect(heading).toHaveFocus()
    expect(screen.getByText(protocol.release_sha)).toBeVisible()
    expect(screen.getByText(/08:00:00 UTC/)).toHaveAttribute(
      'datetime',
      protocol.started_at,
    )
    expect(screen.getByText(/08:01:10 UTC/)).toHaveAttribute(
      'datetime',
      protocol.finished_at,
    )
    expect(
      within(table).getByRole('row', { name: 'Провайдер входа Пройдено Нет' }),
    ).toBeVisible()
    expect(
      within(table).getByRole('row', {
        name: 'Источник обновлений Пройдено Нет',
      }),
    ).toBeVisible()
    expect(screen.getAllByText('Пройдено')).toHaveLength(3)

    const copy = screen.getByRole('button', {
      name: 'Скопировать: SHA выпуска',
    })
    copy.focus()
    expect(copy).toHaveFocus()
    fireEvent.click(copy)
    expect(writeText).toHaveBeenCalledWith(protocol.release_sha)
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Скопировано')
    })
  })

  it('distinguishes failed and skipped checks and shows their reason codes', async () => {
    mount(routePaths.installation, {
      readProtocol: () =>
        jsonResponse({
          ...protocol,
          outcome: 'failed',
          checks: [
            protocol.checks[0],
            {
              check_kind: 'database',
              outcome: 'failed',
              reason_code: 'schema_outdated',
            },
            {
              check_kind: 'adapter',
              outcome: 'skipped',
              reason_code: 'not_configured',
            },
          ],
        }),
    })
    expect(
      await screen.findByRole('row', {
        name: 'База данных Не пройдено schema_outdated',
      }),
    ).toBeVisible()
    expect(
      screen.getByRole('row', { name: 'Адаптер Пропущено not_configured' }),
    ).toBeVisible()
    expect(screen.getAllByText('Не пройдено')).toHaveLength(2)
  })

  it.each([
    ['running', 'Выполняется'],
    ['future_protocol_outcome', 'Неизвестный исход'],
  ])(
    'renders %s with no finish time and tolerates new check codes',
    async (outcome, label) => {
      mount(routePaths.installation, {
        readProtocol: () =>
          jsonResponse({
            ...protocol,
            outcome,
            finished_at: null,
            checks: [
              {
                check_kind: 'future_check',
                outcome: 'future_outcome',
                reason_code: null,
              },
            ],
          }),
      })
      const row = await screen.findByRole('row', {
        name: 'Неизвестная проверка Неизвестный исход Нет',
      })
      expect(row).toBeVisible()
      expect(screen.getByText('Нет данных')).toBeVisible()
      expect(screen.getAllByText(label)).toHaveLength(
        outcome === 'running' ? 1 : 2,
      )
    },
  )

  it('shows an empty state for a missing protocol', async () => {
    mount(routePaths.installation, {
      readProtocol: () => problemResponse('not_found', 404),
    })
    expect(
      await screen.findByRole('status', {
        name: 'Проверка установки ещё не запускалась',
      }),
    ).toHaveTextContent('Запустите проверку вне сайта')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('opens UI-04 if the protocol request is denied by the server', async () => {
    const { router } = mount(routePaths.installation, {
      readProtocol: () => problemResponse('forbidden', 403),
    })
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    })
    expect(router.state.location.pathname).toBe(routePaths.unavailable)
    expect(
      screen.getByText('Это действие недоступно вашей роли.'),
    ).toBeVisible()
  })

  it('hides the settings link and opens UI-04 for a user without installation.read', async () => {
    const { fetcher, router } = mount(routePaths.home, {
      user: { ...currentUser, actions: [...currentUser.actions, 'org.manage'] },
    })
    const settings = await screen.findByRole('group', { name: 'Настройки' })
    expect(
      within(settings).queryByRole('link', { name: 'Диагностика установки' }),
    ).not.toBeInTheDocument()
    await act(() => router.navigate(routePaths.installation))
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    })
    expect(router.state.location.pathname).toBe(routePaths.unavailable)
    expect(
      screen.getByText('Это действие недоступно вашей роли.'),
    ).toBeVisible()
    expect(fetcher.mock.calls.map(([path]) => path)).not.toContain(protocolPath)
  })
})
