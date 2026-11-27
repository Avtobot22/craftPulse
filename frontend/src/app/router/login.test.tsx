import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useState } from 'react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { listServices } from '@/shared/api'
import { createLoginUrl, navigateToLogin } from '@/shared/auth'
import { useNotifications } from '@/shared/ui'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  teams,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import type { AppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

vi.mock('@/shared/auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/shared/auth')>()
  return { ...actual, navigateToLogin: vi.fn() }
})

const cleanups: (() => void)[] = []
beforeEach(() => {
  sessionStorage.clear()
  vi.mocked(navigateToLogin).mockClear()
})
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

function mount(
  path: string,
  runtime = createAppRuntime(vi.fn<typeof fetch>()),
) {
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [path],
  })
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

describe('UI-02 and login errors', () => {
  it.each(['/login/example', '/o/example'])(
    'starts login from the organization address %s',
    (path) => {
      const fetcher = vi.fn<typeof fetch>()
      mount(
        path + '?return_to=' + encodeURIComponent('/users/123?tab=roles#form'),
        createAppRuntime(fetcher),
      )
      expect(
        screen.getByRole('heading', { name: 'Вход', level: 1 }),
      ).toHaveFocus()
      fireEvent.click(
        screen.getByRole('button', { name: 'Войти через GitLab' }),
      )
      expect(navigateToLogin).toHaveBeenCalledWith(
        'example',
        '/users/123?tab=roles#form',
      )
      expect(createLoginUrl('example', '/users/123?tab=roles#form')).toBe(
        '/o/example/login?return_to=%2Fusers%2F123%3Ftab%3Droles%23form',
      )
      expect(fetcher).not.toHaveBeenCalled()
      expect(screen.getByRole('status')).toHaveTextContent(
        'Переходим к провайдеру входа',
      )
      fireEvent.click(
        screen.getByRole('button', { name: 'Войти через GitLab' }),
      )
      expect(navigateToLogin).toHaveBeenCalledOnce()
    },
  )

  it.each([
    'https://evil.example/path',
    '//evil.example',
    '/%252f%252fevil.example',
    '/\\evil.example',
  ])('rejects unsafe return_to %s', (path) => {
    mount('/o/example?return_to=' + encodeURIComponent(path))
    fireEvent.click(screen.getByRole('button', { name: 'Войти через GitLab' }))
    expect(navigateToLogin).toHaveBeenCalledWith('example', '/')
  })

  it('does not invent an organization on the first generic visit', () => {
    mount('/login')
    expect(
      screen.getByRole('button', { name: 'Войти через GitLab' }),
    ).toBeDisabled()
    expect(
      screen.getByText(/Откройте ссылку входа, полученную у администратора/),
    ).toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('starts organization login from an MR comment with empty browser storage', async () => {
    const mr = '0193a5b0-b0b0-7000-8000-000000000001'
    const path = `/merge-requests/${mr}/summary?revision_no=3&org_slug=demo-org#finding-${mr}`
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(problemResponse('auth_required', 401))
    const { router } = mount(path, createAppRuntime(fetcher))
    const button = await screen.findByRole('button', {
      name: 'Войти через GitLab',
    })
    expect(router.state.location.pathname).toBe('/o/demo-org')
    expect(button).toBeEnabled()
    fireEvent.click(button)
    expect(navigateToLogin).toHaveBeenCalledWith('demo-org', path)
  })

  it('keeps only the organization slug from the last /me response across a page reload', async () => {
    const first = createAppRuntime(
      vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(currentUser)),
    )
    await first.queryClient.query(first.session.currentUserOptions())
    first.session.dispose()
    first.queryClient.clear()
    const second = createAppRuntime(vi.fn<typeof fetch>())
    mount('/login', second)
    fireEvent.click(screen.getByRole('button', { name: 'Войти через GitLab' }))
    expect(navigateToLogin).toHaveBeenCalledWith('example', '/')
    expect(document.body).not.toHaveTextContent(currentUser.user.display_name)
    expect(document.body).not.toHaveTextContent(currentUser.organization.name)
    expect(JSON.stringify(sessionStorage)).not.toContain(
      currentUser.user.display_name,
    )
  })

  it('uses the organization in the address rather than the previous organization', async () => {
    const runtime = createAppRuntime(
      vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(currentUser)),
    )
    await runtime.queryClient.query(runtime.session.currentUserOptions())
    mount('/o/another-org', runtime)
    fireEvent.click(screen.getByRole('button', { name: 'Войти через GitLab' }))
    expect(navigateToLogin).toHaveBeenCalledWith('another-org', '/')
  })

  it.each([
    ['provider_unavailable', 'GitLab недоступен', 'Повторить вход'],
    ['provider_rejected', 'GitLab отказал во входе', 'Войти снова'],
    [
      'invalid_response',
      'Не удалось проверить ответ GitLab',
      'Начать вход заново',
    ],
    [
      'user_blocked',
      'Учётная запись заблокирована в craftPulse',
      'Вернуться к началу',
    ],
    [
      'organization_not_configured',
      'Организация ещё не настроена',
      'Вернуться к началу',
    ],
    [
      'unknown-private-password',
      'Если ошибка повторяется, обратитесь к администратору craftPulse.',
      'Начать вход заново',
    ],
  ])(
    'explains login_error=%s without provider details',
    async (code, title, action) => {
      const { router } = mount(
        '/o/example?login_error=' + encodeURIComponent(code),
      )
      expect(
        screen.getByRole('heading', { name: 'Не удалось войти' }),
      ).toHaveFocus()
      expect(screen.getByRole('alert')).toHaveTextContent(title)
      expect(document.body).not.toHaveTextContent('unknown-private-password')
      expect(document.body).not.toHaveTextContent(currentUser.user.display_name)
      expect(document.body).not.toHaveTextContent('назначенный администратор')
      fireEvent.click(screen.getByRole('button', { name: action }))
      if (action === 'Вернуться к началу') {
        await screen.findByRole('heading', { name: 'Вход' })
        expect(router.state.location.pathname).toBe('/o/example')
        expect(router.state.location.search).not.toContain('login_error')
        expect(navigateToLogin).not.toHaveBeenCalled()
      } else expect(navigateToLogin).toHaveBeenCalledWith('example', '/')
    },
  )

  it('allows another attempt after the browser returns from the provider', () => {
    mount('/o/example')
    fireEvent.click(screen.getByRole('button', { name: 'Войти через GitLab' }))
    fireEvent(window, new Event('pageshow'))
    expect(
      screen.getByRole('button', { name: 'Войти через GitLab' }),
    ).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Войти через GitLab' }))
    expect(navigateToLogin).toHaveBeenCalledTimes(2)
  })

  it.each(['/LOGIN/example/', '/o/example/', '/SESSION-EXPIRED/'])(
    'keeps the safe continuation on %s when a previous request returns 401',
    async (path) => {
      const runtime = createAppRuntime(
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(problemResponse('auth_required', 401)),
      )
      const search = '?return_to=' + encodeURIComponent('/users?tab=roles#form')
      const { router } = mount(path + search, runtime)
      await act(async () => {
        await expect(
          runtime.api.read('/services', (value) => value),
        ).rejects.toBeDefined()
      })
      expect(runtime.session.getSnapshot()).toBe('expired')
      expect(router.state.location.pathname).toBe(path)
      expect(router.state.location.search).toBe(search)
    },
  )
})

describe('UI-03 after a 401 on an edited form', () => {
  it('hides old content and toasts, keeps the route and never replays a submission after login', async () => {
    const submit = vi.fn()
    const fetcher = vi.fn<typeof fetch>().mockImplementation((url) => {
      if (url === '/api/v1/me')
        return Promise.resolve(jsonResponse(currentUser))
      if (url === '/api/v1/teams') return Promise.resolve(jsonResponse(teams))
      return Promise.resolve(problemResponse('auth_required', 401))
    })
    function EditedForm({ app }: { app: AppRuntime }) {
      const [value, setValue] = useState('')
      const notifications = useNotifications()
      return (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            submit()
          }}
        >
          <h1>Закрытый объект</h1>
          <label htmlFor="draft">Описание изменения</label>
          <input
            id="draft"
            value={value}
            onChange={(event) => {
              setValue(event.target.value)
            }}
          />
          <label htmlFor="secret">Секретное поле</label>
          <input id="secret" type="password" />
          <button
            type="button"
            onClick={() => {
              notifications.success({
                name: 'private',
                message: 'Уведомление закрытого объекта',
              })
            }}
          >
            Показать тост
          </button>
          <button
            type="button"
            onClick={() => {
              void app.queryClient
                .query(listServices(app.api))
                .catch(() => undefined)
            }}
          >
            Проверить сессию
          </button>
          <button type="submit">Сохранить</button>
        </form>
      )
    }
    function renderEditor() {
      const runtime = createAppRuntime(fetcher)
      const routes = createAppRoutes(runtime)
      const children = routes[0]?.children?.find((route) => !route.path)
        ?.children?.[0]?.children
      if (!children) throw new Error('Missing shell routes')
      children.push({ path: '/edited', element: <EditedForm app={runtime} /> })
      const router = createMemoryRouter(routes, {
        initialEntries: ['/edited?tab=settings#form'],
      })
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
    const first = renderEditor()
    await screen.findByRole('heading', { name: 'Закрытый объект' })
    fireEvent.change(screen.getByLabelText('Описание изменения'), {
      target: { value: 'Несохранённый текст' },
    })
    fireEvent.change(screen.getByLabelText('Секретное поле'), {
      target: { value: 'private-password' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Показать тост' }))
    await screen.findByText('Уведомление закрытого объекта')
    fireEvent.click(screen.getByRole('button', { name: 'Проверить сессию' }))
    expect(
      await screen.findByRole('heading', { name: 'Сессия завершилась' }),
    ).toHaveFocus()
    expect(screen.queryByText('Закрытый объект')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Уведомление закрытого объекта'),
    ).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Секретное поле')).not.toBeInTheDocument()
    expect(first.runtime.queryClient.getQueryCache().getAll()).toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: 'Войти снова' }))
    await screen.findByRole('heading', { name: 'Вход' })
    fireEvent.click(screen.getByRole('button', { name: 'Войти через GitLab' }))
    expect(navigateToLogin).toHaveBeenCalledWith(
      'example',
      '/edited?tab=settings#form',
    )
    expect(submit).not.toHaveBeenCalled()
    first.dispose()
    const second = renderEditor()
    await screen.findByRole('heading', { name: 'Закрытый объект' })
    expect(screen.getByLabelText('Описание изменения')).toHaveValue('')
    expect(screen.getByLabelText('Секретное поле')).toHaveValue('')
    expect(submit).not.toHaveBeenCalled()
    expect(
      fetcher.mock.calls.every(([, options]) => options?.method === 'GET'),
    ).toBe(true)
    second.dispose()
  })
})
