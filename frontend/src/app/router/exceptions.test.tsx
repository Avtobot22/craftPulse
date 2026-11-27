import { focusManager } from '@tanstack/react-query'
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
  approved,
  authorId,
  draft,
  expired,
  finding,
  pending,
  renewal,
  reviewerId,
} from '../../../test/exception-fixtures'
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

type Schema = components['schemas']
const path = '/api/v1/exceptions'
const detailPath = `${path}/${pending.id}`
const findingPage = `/findings/${finding.id}`
const detailPage = `/exceptions/${pending.id}`
const disposers: (() => void)[] = []

function response(detail: Schema['ExceptionDetail'], etag = '"1"') {
  return jsonResponse(detail, 200, { ETag: etag })
}

function requestBody(init: RequestInit | undefined): unknown {
  if (typeof init?.body !== 'string')
    throw new Error('Expected a JSON request body')
  return JSON.parse(init.body)
}

function mount({
  at = findingPage,
  actor = authorId,
  handle,
}: {
  at?: string
  actor?: string
  handle?: (request: URL, init: RequestInit | undefined) => Response | undefined
} = {}) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) =>
    Promise.resolve().then(() => {
      if (typeof input !== 'string') throw new Error('Expected a request URL')
      const url = new URL(input, 'https://craftpulse.test')
      const item = handle?.(url, init)
      if (item) return item
      switch (url.pathname) {
        case '/api/v1/me':
          return jsonResponse({
            ...currentUser,
            user: { ...currentUser.user, id: actor },
            actions: [
              'session.read',
              'findings.read',
              'exceptions.manage',
              'exceptions.review',
            ],
          } satisfies Schema['CurrentUser'])
        case '/api/v1/teams':
          return jsonResponse(teams)
        case '/api/v1/projects':
          return jsonResponse(projects)
        case '/api/v1' + findingPage:
          return jsonResponse(finding)
        case detailPath:
          return response(pending)
        case path:
          if (init?.method === 'POST')
            return jsonResponse(pending, 201, { ETag: '"1"' })
          return jsonResponse({
            data: [pending],
            next_cursor: null,
            access_limited: false,
            access_notes: [],
          } satisfies Schema['ExceptionPage'])
        default:
          throw new Error(`Unexpected request: ${input}`)
      }
    }),
  )
  vi.spyOn(window, 'scrollTo').mockReturnValue(undefined)
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [at],
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
  return { fetcher, router }
}

// jsdom does not implement native modal dialogs.
beforeAll(() => {
  const prototype = HTMLDialogElement.prototype
  if (!Object.hasOwn(prototype, 'showModal')) {
    Object.defineProperties(prototype, {
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
  }
})

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
  focusManager.setFocused(undefined)
})

async function fillCreate() {
  fireEvent.click(
    await screen.findByRole('link', { name: 'Оформить исключение' }),
  )
  await screen.findByLabelText('Тип решения')
  fireEvent.change(screen.getByLabelText('Тип решения'), {
    target: { value: draft.decision_type },
  })
  fireEvent.change(screen.getByLabelText(/^Причина/u), {
    target: { value: draft.reason },
  })
  fireEvent.change(screen.getByLabelText('Начало действия'), {
    target: { value: '2026-01-01T10:00' },
  })
  fireEvent.change(screen.getByLabelText('Срок пересмотра'), {
    target: { value: '2099-12-08T12:00' },
  })
}

function refocusWindow() {
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
}

async function confirmForm() {
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))
  const dialog = await screen.findByRole('dialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  return dialog
}

it('показывает ошибку вместо формы при первой неудачной загрузке находки', async () => {
  mount({
    at: findingPage + '/exception',
    handle: (url) => {
      if (url.pathname === '/api/v1' + findingPage)
        throw new TypeError('Connection lost')
      return undefined
    },
  })

  expect(
    await screen.findByRole('alert', { name: 'Ошибка' }),
  ).toHaveTextContent('Не удалось загрузить данные исключения.')
  expect(screen.queryByLabelText(/^Причина/u)).toBeNull()
})

it('сохраняет черновик при сетевой ошибке обновления находки', async () => {
  let offline = false
  mount({
    handle: (url) => {
      if (url.pathname === '/api/v1' + findingPage && offline)
        throw new TypeError('Connection lost')
      return undefined
    },
  })
  await fillCreate()

  offline = true
  refocusWindow()

  const failure = await screen.findByRole('alert', { name: 'Ошибка' })
  expect(screen.getByLabelText(/^Причина/u)).toHaveValue(draft.reason)
  expect(failure).toHaveTextContent('Не удалось обновить карточку находки.')

  offline = false
  fireEvent.click(within(failure).getByRole('button', { name: 'Повторить' }))
  await waitFor(() => {
    expect(failure).not.toBeInTheDocument()
  })
  expect(screen.getByLabelText(/^Причина/u)).toHaveValue(draft.reason)
})

it('закрывает редактор после отказа в доступе к находке при обновлении', async () => {
  let accessRevoked = false
  mount({
    handle: (url) => {
      if (url.pathname === '/api/v1' + findingPage && accessRevoked)
        return problemResponse('forbidden', 403)
      return undefined
    },
  })
  await fillCreate()

  accessRevoked = true
  refocusWindow()

  expect(
    await screen.findByRole('alert', { name: 'Ошибка' }),
  ).toHaveTextContent('Не удалось загрузить данные исключения.')
  expect(screen.queryByLabelText(/^Причина/u)).toBeNull()
})

it('оформляет решение из UI-46 и показывает ожидание в UI-48', async () => {
  const { fetcher } = mount()
  await fillCreate()
  const dialog = await confirmForm()
  expect(
    within(dialog).getByRole('list', { name: 'Изменения' }),
  ).toHaveTextContent(draft.reason)
  expect(
    await screen.findByRole('heading', {
      name: 'Ждёт подтверждения ревьюером',
    }),
  ).toHaveFocus()
  expect(screen.queryByText('Действует')).toBeNull()
  const write = fetcher.mock.calls.find(
    ([url, init]) => url === path && init?.method === 'POST',
  )?.[1]
  expect(new Headers(write?.headers).get('Idempotency-Key')).toMatch(
    /^[a-f0-9-]{36}$/iu,
  )
  expect(new Headers(write?.headers).has('If-Match')).toBe(false)
  expect(requestBody(write)).toMatchObject({
    ...draft,
    effective_from: new Date('2026-01-01T10:00').toISOString(),
    expires_at: new Date('2099-12-08T12:00').toISOString(),
  })
  fireEvent.click(screen.getByRole('link', { name: 'Открыть историю решения' }))
  expect(
    await screen.findByRole('heading', { name: 'Ожидающая версия 1' }),
  ).toBeVisible()
  expect(screen.queryByRole('option', { name: 'Подтвердить' })).toBeNull()
  fireEvent.click(screen.getByRole('link', { name: 'К исходной находке' }))
  expect(
    await screen.findByRole('heading', { name: 'Карточка находки' }),
  ).toBeVisible()
  expect(screen.queryByText(/Состояние: Принята/u)).toBeNull()
})

it.each(['в открытом диалоге', 'после повторного открытия диалога'])(
  'после обрыва повторяет создание с прежним ключом и телом %s',
  async (retryMode) => {
    const writes: { key: string | null; body: RequestInit['body'] }[] = []
    mount({
      handle: (url, init) => {
        if (url.pathname !== path || init?.method !== 'POST') return
        writes.push({
          key: new Headers(init.headers).get('Idempotency-Key'),
          body: init.body,
        })
        if (writes.length === 1)
          throw new TypeError('Connection closed after write')
        return jsonResponse(pending, 201)
      },
    })
    await fillCreate()
    await confirmForm()
    const retry = await screen.findByRole('button', {
      name: 'Повторить сохранение',
    })
    if (retryMode === 'после повторного открытия диалога') {
      fireEvent.click(
        within(screen.getByRole('dialog')).getByRole('button', {
          name: 'Отмена',
        }),
      )
      await confirmForm()
    } else {
      fireEvent.click(retry)
    }
    expect(
      await screen.findByRole('heading', {
        name: 'Ждёт подтверждения ревьюером',
      }),
    ).toBeVisible()
    expect(writes).toHaveLength(2)
    expect(writes[1]).toEqual(writes[0])
  },
)

it('показывает поле и причину для 422 чужой области', async () => {
  mount({
    handle: (url, init) => {
      if (url.pathname === path && init?.method === 'POST')
        return jsonResponse(
          {
            ...problem('validation_failed', 422),
            errors: [{ field: '/scope/component_id', code: 'outside_scope' }],
          } satisfies Schema['Problem'],
          422,
          { 'Content-Type': 'application/problem+json' },
        )
      return undefined
    },
  })
  await fillCreate()
  await confirmForm()
  const error = await screen.findByRole('alert')
  expect(error).toHaveTextContent(
    'Область действия · Область выходит за ваши права.',
  )
  expect(
    within(error).getByRole('link', { name: 'Область действия' }),
  ).toHaveAttribute('href', '#scope')
  expect(screen.getByLabelText('Область действия')).toHaveAttribute(
    'aria-invalid',
    'true',
  )
})

it('показывает кто может оформить исключение при 403', async () => {
  mount({
    handle: (url, init) => {
      if (url.pathname === path && init?.method === 'POST')
        return problemResponse('forbidden', 403)
      return undefined
    },
  })
  await fillCreate()
  await confirmForm()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Исключение может оформить техлид или разработчик с правом на эту область.',
  )
})

it('проверяет пустую причину и срок до отправки', async () => {
  const { fetcher } = mount()
  await fillCreate()
  fireEvent.change(screen.getByLabelText(/^Причина/u), {
    target: { value: '  ' },
  })
  fireEvent.change(screen.getByLabelText('Срок пересмотра'), {
    target: { value: '2026-01-01T10:00' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))
  const error = screen.getByRole('alert')
  expect(error).toHaveTextContent('Укажите причину.')
  expect(error).toHaveTextContent(
    'Срок пересмотра должен быть позже начала действия.',
  )
  expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(
    false,
  )
})

it('открывает UI-18 при уходе и сохраняет ввод при возврате в форму', async () => {
  mount()
  await fillCreate()
  fireEvent.click(screen.getByRole('link', { name: 'Отмена' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Продолжить редактирование' }),
  )
  expect(screen.getByLabelText(/^Причина/u)).toHaveValue(draft.reason)
  fireEvent.click(screen.getByRole('link', { name: 'Отмена' }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Отбросить изменения' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Карточка находки' }),
  ).toBeVisible()
})

it('при 412 перечитывает версию и требует явного подтверждения с новым If-Match', async () => {
  const tags: (string | null)[] = []
  mount({
    at: detailPage + '/extend',
    handle: (url, init) => {
      if (url.pathname === detailPath)
        return tags.length ? response(renewal, '"2"') : response(approved)
      if (url.pathname === detailPath + '/versions') {
        tags.push(new Headers(init?.headers).get('If-Match'))
        return tags.length === 1
          ? problemResponse('version_conflict', 412)
          : jsonResponse(
              {
                ...renewal,
                current_version: { ...renewal.current_version, version_no: 3 },
              },
              201,
            )
      }
      return undefined
    },
  })
  fireEvent.change(await screen.findByLabelText('Срок пересмотра'), {
    target: { value: '2099-12-09T12:00' },
  })
  const dialog = await confirmForm()
  expect(
    await within(dialog).findByText(
      'Данные изменились. Просмотрите актуальную версию',
    ),
  ).toBeVisible()
  expect(within(dialog).getByText('Было: Новая причина')).toBeVisible()
  expect(tags).toEqual(['"1"'])
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByRole('heading', {
      name: 'Ждёт подтверждения ревьюером',
    }),
  ).toBeVisible()
  expect(tags).toEqual(['"1"', '"2"'])
})

async function openReview(decision: 'approve' | 'reject') {
  fireEvent.change(await screen.findByLabelText('Решение ревьюера'), {
    target: { value: decision },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Проверить решение' }))
  return screen.findByRole('dialog')
}

it('ревьюер подтверждает версию через UI-17 с клавиатурным фокусом', async () => {
  const writes: RequestInit[] = []
  mount({
    at: detailPage,
    actor: reviewerId,
    handle: (url, init) => {
      if (url.pathname === detailPath + '/decisions' && init) {
        writes.push(init)
        return jsonResponse(approved)
      }
      if (url.pathname === detailPath && writes.length)
        return response(approved)
      return undefined
    },
  })
  const dialog = await openReview('approve')
  expect(within(dialog).getByRole('heading')).toHaveFocus()
  fireEvent.keyDown(dialog, { key: 'Tab' })
  expect(within(dialog).getByRole('button', { name: 'Отмена' })).toHaveFocus()
  fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
  const confirm = within(dialog).getByRole('button', { name: 'Подтвердить' })
  expect(confirm).toHaveFocus()
  fireEvent.click(confirm)
  expect(
    await screen.findByRole('heading', { name: 'Действует' }),
  ).toHaveFocus()
  expect(screen.getByText(reviewerId, { exact: false })).toBeVisible()
  expect(requestBody(writes[0])).toEqual({ decision: 'approve' })
  expect(new Headers(writes[0]?.headers).get('If-Match')).toBe('"1"')
})

it('перечитывает уже окончательное решение после 409', async () => {
  let conflict = false
  mount({
    at: detailPage,
    actor: reviewerId,
    handle: (url) => {
      if (url.pathname === detailPath && conflict) return response(approved)
      if (url.pathname === detailPath + '/decisions') {
        conflict = true
        return problemResponse('state_conflict', 409)
      }
      return undefined
    },
  })
  const dialog = await openReview('approve')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Решение уже в конечном состоянии.',
  )
  expect(
    await screen.findByRole('heading', { name: 'Действует' }),
  ).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Проверить решение' })).toBeNull()
})

it.each([
  { actor: authorId, decision: 'withdraw', status: 'withdrawn' },
  { actor: reviewerId, decision: 'reject', status: 'rejected' },
] as const)(
  'сохраняет переход $decision через UI-17',
  async ({ actor, decision, status }) => {
    const writes: RequestInit[] = []
    const result = {
      ...pending,
      state: status,
      current_version: { ...pending.current_version, status },
    } satisfies Schema['ExceptionDetail']
    mount({
      at: detailPage,
      actor,
      handle: (url, init) => {
        if (url.pathname === detailPath + '/decisions' && init) {
          writes.push(init)
          return jsonResponse(result)
        }
        if (url.pathname === detailPath && writes.length)
          return response(result)
        return undefined
      },
    })
    if (decision === 'withdraw') {
      fireEvent.click(await screen.findByRole('button', { name: 'Отозвать' }))
    } else {
      await openReview('reject')
    }
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    expect(
      await screen.findByRole('heading', {
        name: status === 'withdrawn' ? 'Отозвано' : 'Отклонено',
      }),
    ).toBeVisible()
    expect(requestBody(writes[0])).toEqual({ decision })
    expect(new Headers(writes[0]?.headers).get('If-Match')).toBe('"1"')
  },
)

it('показывает в истории ожидающее продление отдельно от действующей версии', async () => {
  mount({
    at: detailPage,
    handle: (url) => {
      if (url.pathname === detailPath) return response(renewal, '"2"')
      return undefined
    },
  })
  expect(
    await screen.findByRole('heading', { name: 'Действующая версия 1' }),
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Ожидающая версия 2' }),
  ).toBeVisible()
  expect(
    screen
      .getAllByText(/^Версия [12] ·/u, { selector: 'summary' })
      .map((item) => item.textContent.slice(0, 8)),
  ).toEqual(['Версия 2', 'Версия 1'])
})

it('оставляет истёкшее исключение в истории с датой и действием продления', async () => {
  mount({
    at: '/exceptions?state=expired',
    handle: (url) => {
      if (url.pathname === path)
        return jsonResponse({
          data: [expired],
          next_cursor: null,
          access_limited: false,
          access_notes: [],
        } satisfies Schema['ExceptionPage'])
      if (url.pathname === detailPath) return response(expired)
      return undefined
    },
  })
  fireEvent.click(
    await screen.findByRole('link', { name: /Ротация запланирована/u }),
  )
  await waitFor(() => {
    expect(
      screen.getByRole('heading', { name: 'Срок действия истёк' }),
    ).toBeVisible()
  })
  expect(
    screen.getAllByText('Срок истёк')[0]?.nextElementSibling?.firstElementChild,
  ).toHaveAttribute('datetime', expired.current_version.expires_at)
  expect(screen.getByRole('link', { name: 'Продлить' })).toHaveAttribute(
    'href',
    detailPage + '/extend?state=expired',
  )
  expect(screen.queryByRole('button', { name: 'Отозвать' })).toBeNull()
})

it('отправляет фильтры состояния, компонента, находки и сортировку по сроку', async () => {
  const { fetcher } = mount({ at: '/exceptions' })
  fireEvent.change(await screen.findByLabelText('Состояние'), {
    target: { value: 'expired' },
  })
  fireEvent.change(screen.getByLabelText('ID компонента'), {
    target: { value: finding.component_id },
  })
  fireEvent.change(screen.getByLabelText('ID находки'), {
    target: { value: finding.id },
  })
  fireEvent.change(screen.getByLabelText('Порядок'), {
    target: { value: '-expires_at' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Применить фильтры' }))
  await waitFor(() => {
    const query = new URLSearchParams({
      state: 'expired',
      sort: '-expires_at',
      component_id: finding.component_id,
      finding_id: finding.id,
    })
    expect(fetcher).toHaveBeenCalledWith(
      path + '?' + query.toString(),
      expect.objectContaining({ method: 'GET' }),
    )
  })
})

it('показывает новую версию вместо отправки решения повторно после 412', async () => {
  let posts = 0
  mount({
    at: detailPage,
    actor: reviewerId,
    handle: (url) => {
      if (url.pathname === detailPath && posts) return response(renewal, '"2"')
      if (url.pathname === detailPath + '/decisions') {
        posts += 1
        return problemResponse('version_conflict', 412)
      }
      return undefined
    },
  })
  const dialog = await openReview('approve')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByRole('heading', { name: 'Ожидающая версия 2' }),
  ).toBeVisible()
  expect(posts).toBe(1)
  expect(screen.queryByRole('dialog')).toBeNull()
})
