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
const date = '2026-10-01T09:00:00Z'
const user = {
  id: '0193a5b0-1111-7000-8000-000000000001',
  display_name: 'Анна Смирнова',
  status: 'active',
  person_id: '0193a5b0-2222-7000-8000-000000000001',
} satisfies Schemas['User']
const secondUser = {
  id: '0193a5b0-1111-7000-8000-000000000002',
  display_name: null,
  status: 'blocked',
  person_id: null,
} satisfies Schemas['User']
const team = {
  id: '0193a5b0-3333-7000-8000-000000000001',
  name: 'Платформа',
  status: 'active',
} satisfies Schemas['Team']
const original = {
  role: 'dev_lead',
  team_id: team.id,
  project_id: null,
  service_id: null,
  assigned_at: date,
} satisfies Schemas['RoleAssignment']
const future = { ...original, role: 'future_role' }
const processOwner = {
  role: 'process_owner',
  team_id: team.id,
  project_id: null,
  service_id: null,
} satisfies Schemas['RoleAssignmentInput']
const basePath = '/users'
const detailPath = basePath + '/' + user.id
const rolesPath = '/api/v1' + detailPath + '/roles'
const me = { ...currentUser, actions: [...currentUser.actions, 'users.manage'] }
const disposals: (() => void)[] = []

beforeAll(() => {
  Object.defineProperties(HTMLDialogElement.prototype, {
    showModal: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
    },
    close: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
    },
  })
})

afterEach(() => {
  for (const dispose of disposals.splice(0)) dispose()
})

function mount(
  path = detailPath,
  override?: (url: URL, init?: RequestInit) => Response | undefined,
) {
  let status: Schemas['User']['status'] = user.status
  function respond(input: Parameters<typeof fetch>[0], init?: RequestInit) {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    const custom = override?.(url, init)
    if (custom) return custom
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse(me)
      case '/api/v1/users': {
        const page = {
          data: [{ ...user, status }],
          next_cursor: null,
        } satisfies Schemas['UserPage']
        return jsonResponse(page)
      }
      case rolesPath:
        if (init?.method === 'GET') {
          const roles = {
            data: [original],
          } satisfies Schemas['RoleAssignmentList']
          return jsonResponse(roles, 200, { ETag: '"1"' })
        }
        break
      case '/api/v1/teams':
        return jsonResponse({ ...teams, data: [team] })
      case '/api/v1/projects':
        return jsonResponse(projects)
      case '/api/v1/services':
        return jsonResponse({
          data: [],
          next_cursor: null,
        } satisfies Schemas['ServicePage'])
      case '/api/v1' + detailPath + '/block':
        status = 'blocked'
        return new Response(null, { status: 204 })
      case '/api/v1' + detailPath + '/unblock':
        status = 'active'
        return new Response(null, { status: 204 })
      case '/api/v1' + detailPath + '/sessions':
        return new Response(null, { status: 204 })
    }
    throw new Error('Unexpected endpoint: ' + url.pathname)
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
  disposals.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { fetcher, router }
}

function change(label: string, value: string) {
  fireEvent.change(screen.getByRole('combobox', { name: label }), {
    target: { value },
  })
}

async function stageProcessOwner() {
  await screen.findByRole('heading', { name: 'Роли и области' })
  change('Роль', 'process_owner')
  const form = screen.getByRole('form', { name: 'Роли и области' })
  const teamField = within(form).getByRole('combobox', { name: 'Команда' })
  await waitFor(() => {
    expect(
      within(teamField).getByRole('option', { name: team.name }),
    ).toBeInTheDocument()
  })
  fireEvent.change(teamField, { target: { value: team.id } })
  const add = screen.getByRole('button', { name: 'Добавить назначение' })
  add.focus()
  fireEvent.submit(form)
  expect(within(form).getByRole('combobox', { name: 'Роль' })).toHaveFocus()
}

function preview() {
  fireEvent.click(
    screen.getByRole('button', { name: 'Просмотреть изменения ролей' }),
  )
  return screen.getByRole('dialog', { name: 'Подтвердите изменение ролей' })
}

function confirm(dialog: HTMLElement) {
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
}

function writes(fetcher: ReturnType<typeof mount>['fetcher']) {
  return fetcher.mock.calls.flatMap(([url, options]) =>
    url === rolesPath && options?.method === 'PUT' ? [options] : [],
  )
}

it('filters and sorts users on the server, navigates cursors and announces received rows', async () => {
  const { fetcher } = mount(basePath, (url) => {
    if (url.pathname !== '/api/v1/users') return
    const blocked = url.searchParams.get('status') === 'blocked'
    return jsonResponse({
      data: blocked ? [secondUser] : [user],
      next_cursor:
        blocked && !url.searchParams.has('cursor') ? 'next users' : null,
    } satisfies Schemas['UserPage'])
  })
  await screen.findByRole('link', { name: user.display_name })
  change('Статус', 'blocked')
  await screen.findByRole('link', { name: 'Имя не получено' })
  change('Сортировка', '-created_at')
  const next = screen.getByRole('button', { name: 'Следующая страница' })
  await waitFor(() => {
    expect(next).toBeEnabled()
  })
  fireEvent.click(next)
  expect(
    screen.getByRole('heading', { name: 'Список пользователей' }),
  ).toHaveFocus()
  expect(await screen.findByText('Страница 2 · получено 1')).toBeVisible()
  expect(next).toBeDisabled()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/users?limit=25&sort=-created_at&status=blocked&cursor=next+users',
    expect.objectContaining({ method: 'GET' }),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Предыдущая страница' }))
  expect(await screen.findByText('Страница 1 · получено 1')).toBeVisible()
})

it('replaces the full set with If-Match, keeps unknown roles and shows the server response', async () => {
  const { fetcher } = mount(detailPath, (url, init) => {
    if (url.pathname !== rolesPath) return
    if (init?.method === 'PUT') {
      return jsonResponse(
        {
          data: [
            future,
            { ...processOwner, assigned_at: '2026-11-08T10:00:00Z' },
          ],
        },
        200,
        { ETag: '"2"' },
      )
    }
    return jsonResponse({ data: [original, future] }, 200, { ETag: '"1"' })
  })
  await stageProcessOwner()
  const remove = screen.getByRole('button', {
    name: /Снять назначение: Руководитель разработки/,
  })
  fireEvent.click(remove)
  expect(
    screen.queryByRole('button', {
      name: /Снять назначение: Неизвестная роль/,
    }),
  ).not.toBeInTheDocument()
  const dialog = preview()
  expect(within(dialog).getByText('Добавить назначение')).toBeVisible()
  expect(within(dialog).getByText('Снять назначение')).toBeVisible()
  confirm(dialog)
  expect(await screen.findByText('Версия набора ролей: "2"')).toBeVisible()
  expect(screen.getByText('Неизвестная роль (future_role)')).toBeVisible()
  const [, savedRow] = within(
    screen.getByRole('table', { name: 'Роли и области' }),
  ).getAllByRole('row', { name: /Неизвестная роль|Владелец процесса/ })
  expect(savedRow).toHaveTextContent('08.11.2026')
  const [request] = writes(fetcher)
  expect(new Headers(request?.headers).get('If-Match')).toBe('"1"')
  expect(request?.body).toBe(
    JSON.stringify({
      data: [
        {
          role: future.role,
          team_id: team.id,
          project_id: null,
          service_id: null,
        },
        processOwner,
      ],
    }),
  )
})

it.each([
  ['administrator', 'Вся организация', false],
  ['process_owner', 'Команда', true],
  ['service_owner', 'Сервис', true],
])('limits %s to the contract area', async (role, label, needsArea) => {
  mount()
  await screen.findByRole('heading', { name: 'Роли и области' })
  change('Роль', role)
  const select = screen.getByRole('combobox', { name: 'Область роли' })
  expect(select).toBeDisabled()
  expect(
    within(select)
      .getAllByRole('option')
      .map((option) => option.textContent),
  ).toEqual([label])
  const add = screen.getByRole('button', { name: 'Добавить назначение' })
  if (needsArea) expect(add).toBeDisabled()
  else {
    fireEvent.click(add)
    expect(
      screen.getByRole('row', { name: /Администратор Вся организация/ }),
    ).toBeVisible()
  }
})

it('shows the latest set on 412 and waits for explicit confirmation of the rebased change', async () => {
  let attempts = 0
  const concurrent = {
    ...original,
    role: 'wiki_access',
  } satisfies Schemas['RoleAssignment']
  const { fetcher } = mount(detailPath, (url, init) => {
    if (url.pathname !== rolesPath) return
    if (init?.method === 'PUT') {
      attempts += 1
      if (attempts === 1) return problemResponse('version_conflict', 412)
      return jsonResponse(
        {
          data: [original, concurrent, { ...processOwner, assigned_at: date }],
        } satisfies Schemas['RoleAssignmentList'],
        200,
        { ETag: '"3"' },
      )
    }
    if (attempts)
      return jsonResponse(
        {
          data: [original, concurrent],
        } satisfies Schemas['RoleAssignmentList'],
        200,
        { ETag: '"2"' },
      )
    return undefined
  })
  await stageProcessOwner()
  const dialog = preview()
  confirm(dialog)
  expect(
    await within(dialog).findByText(
      'Данные изменились. Просмотрите актуальную версию',
    ),
  ).toBeVisible()
  expect(within(dialog).getByText('Актуальный набор ролей')).toBeVisible()
  expect(
    within(dialog).getByText(/Доступ к wiki.*Платформа/su, { selector: 'dd' }),
  ).toBeVisible()
  expect(attempts).toBe(1)
  confirm(dialog)
  expect(await screen.findByText('Версия набора ролей: "3"')).toBeVisible()
  const [, second] = writes(fetcher)
  expect(new Headers(second?.headers).get('If-Match')).toBe('"2"')
  expect(second?.body).toContain('"role":"wiki_access"')
})

it('shows the 422 JSON Pointer error next to the corresponding area and preserves the draft', async () => {
  mount(detailPath, (url, init) => {
    if (url.pathname !== rolesPath || init?.method !== 'PUT') return
    return jsonResponse(
      {
        ...problem('validation_failed', 422),
        errors: [
          {
            field: '/data/1/team_id',
            code: 'unknown_reference',
            detail: 'Untrusted text',
          },
        ],
      } satisfies Schemas['Problem'],
      422,
      { 'Content-Type': 'application/problem+json' },
    )
  })
  await stageProcessOwner()
  confirm(preview())
  const issue = await screen.findByText(
    'Ошибка назначения. Выбранная область больше недоступна.',
  )
  expect(issue.closest('td')).toHaveTextContent(team.name)
  expect(issue.closest('tr')).toHaveTextContent('Владелец процесса')
  expect(screen.getByRole('alert', { name: '' })).toHaveFocus()
  expect(screen.queryByText('Untrusted text')).not.toBeInTheDocument()
})

it('uses UI-18 for a dirty form and returns focus when editing continues', async () => {
  const { router } = mount()
  await stageProcessOwner()
  const close = screen.getByRole('link', { name: 'Закрыть карточку' })
  fireEvent.click(close)
  const dialog = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  const keep = within(dialog).getByRole('button', {
    name: 'Продолжить редактирование',
  })
  expect(keep).toHaveFocus()
  fireEvent.click(keep)
  expect(router.state.location.pathname).toBe(detailPath)
  fireEvent.click(close)
  fireEvent.click(
    within(await screen.findByRole('dialog')).getByRole('button', {
      name: 'Отбросить изменения',
    }),
  )
  await waitFor(() => {
    expect(router.state.location.pathname).toBe(basePath)
  })
})

it('confirms access actions with safe focus and rereads status after each 204', async () => {
  const { fetcher } = mount()
  for (const [label, endpoint, method, status] of [
    ['Заблокировать', 'block', 'POST', 'Заблокирован'],
    ['Разблокировать', 'unblock', 'POST', 'Активен'],
    ['Завершить все сессии', 'sessions', 'DELETE', 'Активен'],
    ['Завершить все сессии', 'sessions', 'DELETE', 'Активен'],
  ] as const) {
    const opener = await screen.findByRole('button', {
      name: label,
    })
    await waitFor(() => {
      expect(opener).toBeEnabled()
    })
    opener.focus()
    fireEvent.click(opener)
    const dialog = screen.getByRole('dialog', { name: label })
    const safe = within(dialog).getByRole('button', { name: 'Отмена' })
    expect(safe).toHaveFocus()
    const destructive = within(dialog).getByRole('button', { name: label })
    fireEvent.keyDown(safe, { key: 'Tab', shiftKey: true })
    expect(destructive).toHaveFocus()
    const count = fetcher.mock.calls.length
    fireEvent.click(destructive)
    await waitFor(() => {
      expect(dialog).not.toBeInTheDocument()
    })
    await waitFor(() => {
      expect(screen.getByText('Статус: ' + status)).toBeVisible()
    })
    const requests = fetcher.mock.calls.slice(count)
    expect(requests[0]?.[0]).toBe('/api/v1' + detailPath + '/' + endpoint)
    expect(requests[0]?.[1]?.method).toBe(method)
    expect(
      requests.some(
        ([url]) =>
          typeof url === 'string' && url.startsWith('/api/v1/users?limit=100'),
      ),
    ).toBe(true)
  }
})

it('opens UI-04 for a 404 from listUserRoles', async () => {
  const { router } = mount(detailPath, (url) =>
    url.pathname === rolesPath ? problemResponse('not_found', 404) : undefined,
  )
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe('/access-denied')
})

it('does not request users when users.manage is missing', async () => {
  const { fetcher, router } = mount(detailPath, (url) =>
    url.pathname === '/api/v1/me'
      ? jsonResponse({
          ...me,
          actions: me.actions.filter((action) => action !== 'users.manage'),
        })
      : undefined,
  )
  await act(async () => {
    await Promise.resolve()
  })
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/access-denied')
  })
  expect(
    fetcher.mock.calls.some(
      ([url]) => typeof url === 'string' && url.startsWith('/api/v1/users'),
    ),
  ).toBe(false)
})
