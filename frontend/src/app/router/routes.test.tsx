import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useQuery, focusManager } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { listTeams, listServices } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  currentUser,
  teams,
  jsonResponse,
  problemResponse,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import type { AppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

const cleanups: (() => void)[] = []
function mount(
  path: string,
  fetcher: typeof fetch = vi
    .fn<typeof fetch>()
    .mockImplementation(() => Promise.resolve(jsonResponse(currentUser))),
) {
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
  return { runtime, router }
}
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  focusManager.setFocused(undefined)
  vi.restoreAllMocks()
})

describe('application routes', () => {
  it.each([
    ['/', 'craftPulse'],
    ['/login', 'Вход'],
    ['/login/example', 'Вход'],
    ['/session-expired', 'Сессия завершилась'],
    ['/access-denied', 'Объект или область недоступны'],
    ['/installation', 'Диагностика установки'],
    ['/welcome', 'Результат первого входа'],
    ['/organization/structure', 'Структура организации'],
    ['/users', 'Пользователи и роли'],
    ['/users/123', 'Пользователи и роли'],
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
      .mockResolvedValueOnce(jsonResponse(currentUser))
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
      .mockResolvedValueOnce(jsonResponse(currentUser))
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
      if (url === '/api/v1/me')
        return Promise.resolve(jsonResponse(currentUser))
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
