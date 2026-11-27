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
import { afterEach, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import { routePaths, taskAnalysisLinks } from '@/shared/config'
import {
  currentUser,
  jsonResponse,
  problem,
  problemResponse,
  teams,
} from '../../../test/fixtures'
import { taskId, taskTitle } from '../../../test/task-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

const source = {
  connection_id: '0193a5b0-5555-7000-8000-000000000003',
  provider: 'confluence_dc',
  display_name: 'Confluence',
  state: 'not_linked',
  linked_at: null,
} satisfies components['schemas']['SourceLink']
const listPath = '/api/v1/me/source-links'
const startPath = listPath + '/' + source.connection_id + '/authorization'
const taskApiPath = '/api/v1/tasks/' + taskId
const detailPath = taskAnalysisLinks.taskDetail(taskId)
const unavailableHeading = { name: 'Объект или область недоступны' }
const authorizationAction = { name: 'Разрешить доступ' }
const disposals: (() => void)[] = []

function mount(
  path: string,
  {
    state = 'not_linked',
    denial = 404,
    start = () => Promise.resolve(problemResponse('state_conflict', 409)),
  }: {
    state?: components['schemas']['SourceLink']['state']
    denial?: 403 | 404
    start?: () => Promise<Response>
  } = {},
) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    switch (new URL(input, window.location.origin).pathname) {
      case '/api/v1/me':
        return Promise.resolve(jsonResponse(currentUser))
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case taskApiPath:
        return Promise.resolve(
          jsonResponse(
            {
              ...problem(denial === 403 ? 'forbidden' : 'not_found', denial),
              detail: taskTitle + ' 123456 ' + taskId,
            },
            denial,
            { 'Content-Type': 'application/problem+json' },
          ),
        )
      case listPath:
        return Promise.resolve(jsonResponse({ data: [{ ...source, state }] }))
      case startPath:
        return start()
      default:
        throw new Error('Unexpected request: ' + input)
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
  disposals.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { router, fetcher }
}

function dispose() {
  for (const cleanup of disposals.splice(0)) cleanup()
}

afterEach(() => {
  dispose()
  sessionStorage.clear()
  window.history.replaceState(
    null,
    '',
    window.location.pathname + window.location.search,
  )
})

it.each([403, 404] as const)(
  'shows the same source offer without denied details after %s',
  async (denial) => {
    const { router, fetcher } = mount(detailPath, { denial })
    const offer = await screen.findByRole('region', {
      name: source.display_name,
    })
    expect(offer).toHaveTextContent(
      'Разрешите craftPulse читать документы от вашего имени.',
    )
    expect(within(offer).getByRole('button', authorizationAction)).toBeVisible()
    const content = screen.getByRole('main')
    expect(content.innerHTML).not.toContain(taskTitle)
    expect(content.innerHTML).not.toContain(taskId)
    expect(content.innerHTML).not.toContain('123456')
    expect(router.state.location.pathname).toBe(routePaths.unavailable)
    expect(screen.getByRole('heading', unavailableHeading)).toHaveFocus()
    expect(fetcher).toHaveBeenCalledWith(
      listPath,
      expect.objectContaining({ method: 'GET' }),
    )
  },
)

it('starts only one authorization and retries the original route after success', async () => {
  // jsdom implements same-document navigation and keeps the actual Location method.
  const authorizationUrl = new URL(
    '#source-authorization',
    window.location.href,
  )
  let resolveResponse: (response: Response) => void = () => {
    throw new Error('Expected pending authorization')
  }
  const response = new Promise<Response>((resolve) => {
    resolveResponse = resolve
  })
  const origin = detailPath + '?search=open%20issues#details'
  const { fetcher } = mount(origin, { start: () => response })
  const action = await screen.findByRole('button', authorizationAction)
  act(() => {
    action.click()
    action.click()
  })
  expect(action).toBeDisabled()
  expect(
    fetcher.mock.calls.filter(([, init]) => init?.method === 'POST'),
  ).toEqual([
    [
      startPath + '?return_to=%2Faccess-denied',
      expect.objectContaining({ method: 'POST' }),
    ],
  ])
  resolveResponse(jsonResponse({ authorization_url: authorizationUrl.href }))
  await waitFor(() => {
    expect(window.location.href).toBe(authorizationUrl.href)
  })
  dispose()

  const returned = mount(
    routePaths.unavailable + '?source_link_result=linked',
    {
      state: 'linked',
    },
  )
  expect(await screen.findByText('Доступ к источнику разрешён.')).toBeVisible()
  expect(
    returned.fetcher.mock.calls.some(([input]) => input === taskApiPath),
  ).toBe(false)
  fireEvent.click(screen.getByRole('button', { name: 'Повторить переход' }))
  const { pathname, search, hash } = returned.router.state.location
  expect(pathname + search + hash).toBe(origin)
  await waitFor(() => {
    expect(returned.fetcher).toHaveBeenCalledWith(
      taskApiPath,
      expect.objectContaining({ method: 'GET' }),
    )
  })
})

it.each([
  ['state_conflict', 409, 'Источник сейчас не принимает разрешения.'],
  ['not_found', 404, 'Подключение больше недоступно.'],
  [
    'forbidden',
    403,
    'Не удалось начать разрешение доступа. Попробуйте ещё раз.',
  ],
] as const)(
  'reports an authorization error with HTTP %s (%i)',
  async (code, status, message) => {
    mount(routePaths.unavailable, {
      start: () => Promise.resolve(problemResponse(code, status)),
    })
    const action = await screen.findByRole('button', authorizationAction)
    fireEvent.click(action)
    expect(await screen.findByText(message)).toHaveAttribute('role', 'alert')
    expect(screen.getByRole('heading', unavailableHeading)).toBeVisible()
    expect(action).toBeEnabled()
  },
)

it('uses the standard session-expired route when starting returns 401', async () => {
  const { router } = mount(routePaths.unavailable, {
    start: () => Promise.resolve(problemResponse('auth_required', 401)),
  })
  fireEvent.click(await screen.findByRole('button', authorizationAction))
  expect(
    await screen.findByRole('heading', { name: 'Сессия завершилась' }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe(routePaths.sessionExpired)
})

it('keeps the denied route closed after failed authorization and allows a new attempt', async () => {
  const { fetcher } = mount(
    detailPath + '?search=open%20issues&source_link_result=link_failed#details',
    {
      state: 'revoked',
    },
  )
  expect(
    await screen.findByText('Не удалось разрешить доступ к источнику.'),
  ).toBeVisible()
  const action = await screen.findByRole('button', {
    name: 'Повторить разрешение',
  })
  expect(screen.getByRole('heading', unavailableHeading)).toHaveFocus()
  fireEvent.click(action)
  expect(
    await screen.findByText('Источник сейчас не принимает разрешения.'),
  ).toBeVisible()
  const post = fetcher.mock.calls.find(([, init]) => init?.method === 'POST')
  expect(post?.[0]).toBe(
    startPath +
      '?' +
      new URLSearchParams({
        return_to: detailPath + '?search=open+issues#details',
      }).toString(),
  )
  expect(fetcher.mock.calls.some(([input]) => input === taskApiPath)).toBe(
    false,
  )
})

it('does not request a detail route even when the successful callback points to it', async () => {
  const { fetcher } = mount(detailPath + '?source_link_result=linked', {
    state: 'linked',
  })
  expect(await screen.findByText('Доступ к источнику разрешён.')).toBeVisible()
  await waitFor(() => {
    expect(fetcher).toHaveBeenCalledWith(listPath, expect.anything())
  })
  expect(fetcher.mock.calls.some(([input]) => input === taskApiPath)).toBe(
    false,
  )
})
