import {
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
import { routePaths } from '@/shared/config'
import { connectionExample } from '../../../test/admin-fixtures'
import {
  currentUser,
  jsonResponse,
  problemResponse,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schemas = components['schemas']
type Account = Schemas['SourceAccount']
type Item = Schemas['AccountAssignmentBatchResult']['items'][number]

const anna = {
  id: '0193a5b0-2222-7000-8000-000000000001',
  display_name: 'Анна Смирнова',
  kind: 'human',
} satisfies Schemas['Person']
const anotherPerson = {
  id: '0193a5b0-2222-7000-8000-000000000002',
  display_name: 'Иван Петров',
  kind: 'human',
} satisfies Schemas['Person']
const anotherConnection = {
  ...connectionExample,
  id: '0193a5b0-5555-7000-8000-000000000002',
  name: 'Jira поддержка',
} satisfies Schemas['Connection']
const candidate = {
  id: '0193a5b0-9999-7000-8000-000000000001',
  instance_id: connectionExample.instance.id,
  provider: 'jira_cloud',
  native_account_id: 'JIRAUSER10021',
  login: 'asmirnova',
  display_name: 'Анна Смирнова',
  proposed_kind: 'human',
  assignment: null,
  proposals: [{ person_id: anna.id, basis: 'login_match' }],
} satisfies Account
const bot = {
  ...candidate,
  id: '0193a5b0-9999-7000-8000-000000000002',
  native_account_id: 'JIRAUSER10022',
  login: 'ci-bot',
  display_name: 'CI',
  proposals: [],
  proposed_kind: 'bot',
} satisfies Account
const separate: Account = {
  ...candidate,
  id: '0193a5b0-9999-7000-8000-000000000003',
  native_account_id: 'JIRAUSER10023',
  login: 'another',
  proposals: [],
}
const confirmedAt = '2026-11-15T09:30:00Z'
const savedId = '0193a5b0-bbbb-7000-8000-000000000001'
const updatedId = '0193a5b0-bbbb-7000-8000-000000000002'

function receipt(...items: Item[]) {
  return jsonResponse({
    items,
  } satisfies Schemas['AccountAssignmentBatchResult'])
}

function mount(
  initial: Account[],
  write: (
    init: RequestInit,
    replace: (accounts: Account[]) => void,
  ) => Response | Promise<Response>,
  path: string = routePaths.sourceAccounts,
  failRead?: (query: URLSearchParams) => boolean,
) {
  let accounts = initial
  const reads: URLSearchParams[] = []
  const requests: RequestInit[] = []
  const fetcher: typeof fetch = async (input, init) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.example')
    const query = url.searchParams
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse({
          ...currentUser,
          actions: ['accounts.manage', 'connections.manage'],
          areas: { teams: [], projects: [], services: [] },
        })
      case '/api/v1/persons':
        return jsonResponse({
          data: query.has('cursor') ? [anotherPerson] : [anna],
          next_cursor: query.has('cursor') ? null : 'people-2',
        } satisfies Schemas['PersonPage'])
      case '/api/v1/connections':
        return jsonResponse({
          data: query.has('cursor') ? [anotherConnection] : [connectionExample],
          next_cursor: query.has('cursor') ? null : 'connections-2',
        } satisfies Schemas['ConnectionPage'])
      case '/api/v1/source-accounts': {
        reads.push(query)
        if (failRead?.(query)) return problemResponse('internal_error', 500)
        const data = accounts.filter(
          (account) =>
            (!query.has('instance_id') ||
              account.instance_id === query.get('instance_id')) &&
            (!query.has('assignment') ||
              query.get('assignment') ===
                (account.assignment ? 'assigned' : 'unassigned')),
        )
        const count = Number(query.get('limit'))
        const start = query.has('cursor') ? count : 0
        return jsonResponse({
          data: data.slice(start, start + count),
          next_cursor: data.length > start + count ? 'accounts-2' : null,
        } satisfies Schemas['SourceAccountPage'])
      }
      case '/api/v1/account-assignments':
        if (init?.method !== 'POST') throw new Error('Expected a POST')
        requests.push(init)
        return write(init, (next) => {
          accounts = next
        })
      default:
        throw new Error('Unexpected request: ' + url.pathname)
    }
  }
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
    runtime.queryClient.clear()
  })
  return { reads, requests, router }
}

const disposals: (() => void)[] = []

// jsdom doesn't implement the modal behavior of <dialog>.
beforeAll(() => {
  const prototype = HTMLDialogElement.prototype
  for (const [name, open] of [
    ['showModal', true],
    ['close', false],
  ] as const) {
    if (!Object.hasOwn(prototype, name))
      Object.defineProperty(prototype, name, {
        value(this: HTMLDialogElement) {
          this.toggleAttribute('open', open)
        },
        configurable: true,
      })
  }
})

afterEach(() => {
  for (const dispose of disposals.splice(0)) dispose()
})

function row(account: Account) {
  return screen.findByRole('row', {
    name: new RegExp(account.native_account_id, 'u'),
  })
}

async function selectCandidate() {
  const option = await screen.findByRole('checkbox', { name: /совпал логин/u })
  await waitFor(() => {
    expect(option).toBeEnabled()
  })
  fireEvent.click(option)
}

async function markBot() {
  fireEvent.click(
    within(await row(bot)).getByRole('button', { name: /Изменить привязку/u }),
  )
  fireEvent.click(
    within(await row(bot)).getByRole('button', { name: 'Пометить ботом' }),
  )
}

function review() {
  fireEvent.click(screen.getByRole('button', { name: 'Просмотреть выбранные' }))
  return screen.getByRole('dialog', {
    name: 'Подтвердите решения по учётным записям',
  })
}

function confirm(dialog = review()) {
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
}

function body(init: RequestInit | undefined): unknown {
  if (typeof init?.body !== 'string') throw new Error('Expected a JSON body')
  return JSON.parse(init.body)
}

const botAssignment = {
  id: savedId,
  person_id: null,
  actor_kind: 'bot',
  confirmed_at: confirmedAt,
} satisfies Account['assignment']

it('отправляет только отмеченное совпадение логина после явного подтверждения', async () => {
  const otherCandidate: Account = {
    ...separate,
    proposals: [{ person_id: anna.id, basis: 'email_match' }],
  }
  const human: Account = {
    ...candidate,
    assignment: { ...botAssignment, person_id: anna.id, actor_kind: 'human' },
    proposals: [],
  }
  const { requests } = mount([candidate, otherCandidate], (_init, replace) => {
    replace([human, otherCandidate])
    return receipt({
      source_account_id: candidate.id,
      outcome: 'saved',
      assignment_id: savedId,
      error_code: null,
    })
  })

  await row(candidate)
  expect(
    screen.getByRole('button', { name: 'Просмотреть выбранные' }),
  ).toBeDisabled()
  await selectCandidate()
  const dialog = review()
  expect(within(dialog).getByText('Было: Не сопоставлена')).toBeVisible()
  expect(
    within(dialog).getByText('Станет: Анна Смирнова · Человек'),
  ).toBeVisible()
  expect(
    within(dialog).queryByText(new RegExp(separate.native_account_id, 'u')),
  ).toBeNull()
  expect(requests).toHaveLength(0)
  confirm(dialog)
  await waitFor(async () => {
    expect(within(await row(candidate)).getByText('Подтверждено')).toBeVisible()
  })
  expect(body(requests[0])).toEqual({
    items: [
      {
        source_account_id: candidate.id,
        person_id: anna.id,
        actor_kind: 'human',
        create_person: false,
        base_assignment_id: null,
      },
    ],
  } satisfies Schemas['AccountAssignmentBatch'])
})

it('оставляет две записи с одинаковым именем отдельными и без предложения', async () => {
  const { requests } = mount(
    [{ ...candidate, proposals: [] }, separate],
    () => {
      throw new Error('Unexpected write')
    },
  )
  for (const account of [candidate, separate]) {
    const item = within(await row(account))
    expect(item.getByText('Анна Смирнова')).toBeVisible()
    expect(item.getByText(account.instance_id)).toBeVisible()
    expect(item.getByText('Нет предложений')).toBeVisible()
    expect(item.getByText('Не сопоставлена')).toBeVisible()
    expect(item.queryByRole('checkbox')).toBeNull()
  }
  expect(requests).toHaveLength(0)
})

it('показывает системное предложение бота и сохраняет подтверждённую пометку без человека', async () => {
  const { requests } = mount([bot], (_init, replace) => {
    replace([{ ...bot, assignment: botAssignment }])
    return receipt({
      source_account_id: bot.id,
      outcome: 'saved',
      assignment_id: savedId,
      error_code: null,
    })
  })

  expect(
    within(await row(bot)).getByText('Бот по данным источника'),
  ).toBeVisible()
  expect(within(await row(bot)).getByText('Не сопоставлена')).toBeVisible()
  await markBot()
  const dialog = review()
  expect(within(dialog).getByText(/новой ревизией/u)).toBeVisible()
  confirm(dialog)

  await waitFor(async () => {
    expect(
      within(await row(bot)).getByText('Бот', { exact: true }),
    ).toBeVisible()
  })
  expect(within(await row(bot)).getByText('Подтверждено')).toBeVisible()
  expect((await row(bot)).querySelector('time')).toHaveAttribute(
    'datetime',
    confirmedAt,
  )
  expect(body(requests[0])).toEqual({
    items: [
      {
        source_account_id: bot.id,
        person_id: null,
        actor_kind: 'bot',
        create_person: false,
        base_assignment_id: null,
      },
    ],
  } satisfies Schemas['AccountAssignmentBatch'])
})

it('перечитывает конфликт под фильтром и повторно предлагает только несохранённое решение', async () => {
  const concurrent: Account = {
    ...candidate,
    assignment: { ...botAssignment, id: updatedId, actor_kind: 'system' },
  }
  const saved: Account = { ...bot, assignment: botAssignment }
  const { requests, reads } = mount(
    [candidate, bot],
    (init, replace) => {
      replace([concurrent, saved])
      if (typeof init.body === 'string' && init.body.includes(updatedId)) {
        replace([
          {
            ...candidate,
            proposals: [],
            assignment: {
              ...botAssignment,
              id: '0193a5b0-bbbb-7000-8000-000000000003',
              person_id: anna.id,
              actor_kind: 'human',
            },
          },
          saved,
        ])
        return receipt({
          source_account_id: candidate.id,
          outcome: 'saved',
          assignment_id: '0193a5b0-bbbb-7000-8000-000000000003',
          error_code: null,
        })
      }
      return receipt(
        {
          source_account_id: candidate.id,
          outcome: 'version_conflict',
          assignment_id: null,
          error_code: null,
        },
        {
          source_account_id: bot.id,
          outcome: 'saved',
          assignment_id: savedId,
          error_code: null,
        },
      )
    },
    routePaths.sourceAccounts + '?assignment=unassigned',
  )
  await selectCandidate()
  await markBot()
  confirm()

  const results = await screen.findByRole('region', {
    name: 'Результат подтверждения',
  })
  expect(within(results).getByText('Результат подтверждения')).toHaveFocus()
  expect(results).toHaveTextContent('Эта привязка уже изменена')
  expect(results).toHaveTextContent('Сохранено')
  await waitFor(async () => {
    expect(
      within(await row(candidate)).getByText('Служебная учётная запись'),
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Просмотреть выбранные' }),
    ).toBeEnabled()
  })
  expect(within(await row(bot)).getByText('Бот', { exact: true })).toBeVisible()
  expect(
    reads.some(
      (q) =>
        q.get('instance_id') === candidate.instance_id && !q.has('assignment'),
    ),
  ).toBe(true)
  expect(body(requests[0])).toMatchObject({
    items: [
      { source_account_id: candidate.id, base_assignment_id: null },
      { source_account_id: bot.id, base_assignment_id: null },
    ],
  })
  const dialog = review()
  expect(
    within(dialog).getByText('Было: Служебная учётная запись'),
  ).toBeVisible()
  expect(
    within(dialog).queryByText(new RegExp(bot.native_account_id, 'u')),
  ).toBeNull()
  confirm(dialog)
  expect(body(requests[1])).toMatchObject({
    items: [{ source_account_id: candidate.id, base_assignment_id: updatedId }],
  })
  expect(new Headers(requests[1]?.headers).get('Idempotency-Key')).not.toBe(
    new Headers(requests[0]?.headers).get('Idempotency-Key'),
  )
})

it('показывает person_required у отклонённой строки и сохраняет её черновик', async () => {
  mount([candidate], () =>
    receipt({
      source_account_id: candidate.id,
      outcome: 'rejected',
      assignment_id: null,
      error_code: 'person_required',
    }),
  )
  await selectCandidate()
  confirm()
  await waitFor(async () => {
    expect(
      within(await row(candidate)).getByText('Выберите человека'),
    ).toBeVisible()
  })
  expect(
    within(await row(candidate)).getByText('Не сопоставлена'),
  ).toBeVisible()
  expect(within(await row(candidate)).getByRole('checkbox')).toBeChecked()
})

it.each([
  ['not_found', 'Нет доступа к учётной записи'],
  ['person_not_found', 'Выбранный человек недоступен. Выберите другого.'],
])(
  'показывает причину отклонения %s и сохраняет фокус при удалении решения',
  async (errorCode, message) => {
    mount([candidate], () =>
      receipt({
        source_account_id: candidate.id,
        outcome: 'rejected',
        assignment_id: null,
        error_code: errorCode,
      }),
    )
    await selectCandidate()
    confirm()
    const account = await row(candidate)
    expect(await within(account).findByText(message)).toBeVisible()
    const discard = within(account).getByRole('button', {
      name: 'Убрать решение',
    })
    discard.focus()
    fireEvent.click(discard)
    expect(account).toHaveFocus()
    expect(within(account).getByRole('checkbox')).not.toBeChecked()
  },
)

it('после таймаута явно повторяет тот же запрос с тем же Idempotency-Key', async () => {
  const timeout = new AbortController()
  let attempts = 0
  const { requests } = mount([bot], (init, replace) => {
    if (++attempts === 1) {
      return new Promise((_resolve, reject) => {
        init.signal?.addEventListener(
          'abort',
          () => {
            reject(new DOMException('Request timed out', 'TimeoutError'))
          },
          { once: true },
        )
      })
    }
    replace([{ ...bot, assignment: botAssignment }])
    return receipt({
      source_account_id: bot.id,
      outcome: 'saved',
      assignment_id: savedId,
      error_code: null,
    })
  })

  await markBot()
  vi.spyOn(AbortSignal, 'timeout').mockReturnValueOnce(timeout.signal)
  confirm()
  timeout.abort(new DOMException('Request timed out', 'TimeoutError'))
  const retry = await screen.findByRole('button', {
    name: 'Повторить сохранение',
  })
  expect(requests).toHaveLength(1)
  fireEvent.click(retry)
  await waitFor(async () => {
    expect(within(await row(bot)).getByText('Сохранено')).toBeVisible()
  })
  expect(requests).toHaveLength(2)
  const key = new Headers(requests[0]?.headers).get('Idempotency-Key')
  expect(key).toMatch(/^[\da-f-]{36}$/u)
  expect(new Headers(requests[1]?.headers).get('Idempotency-Key')).toBe(key)
  expect(requests[1]?.body).toBe(requests[0]?.body)
})

it('ищет человека на следующей странице справочника и управляет фокусом в UI-17', async () => {
  const { requests } = mount([separate], () => {
    throw new Error('Unexpected write')
  })
  const item = within(await row(separate))
  const edit = item.getByRole('button', { name: /Изменить привязку/u })
  edit.focus()
  // Native activation is simulated by a click. Tab wrapping is handled by UI-17.
  fireEvent.click(edit)
  const search = item.getByRole('searchbox', { name: 'Поиск человека' })
  search.focus()
  expect(search).toHaveFocus()
  fireEvent.change(search, { target: { value: 'Петров' } })
  await item.findByRole('option', { name: 'Иван Петров' })
  fireEvent.change(item.getByRole('combobox', { name: 'Человек' }), {
    target: { value: anotherPerson.id },
  })
  fireEvent.click(item.getByRole('button', { name: 'Выбрать человека' }))
  const opener = screen.getByRole('button', { name: 'Просмотреть выбранные' })
  opener.focus()
  const dialog = review()
  expect(within(dialog).getByRole('heading')).toHaveFocus()
  expect(
    within(dialog).getByText('Станет: Иван Петров · Человек'),
  ).toBeVisible()
  const cancel = within(dialog).getByRole('button', { name: 'Отмена' })
  const apply = within(dialog).getByRole('button', { name: 'Подтвердить' })
  apply.focus()
  expect(fireEvent.keyDown(apply, { key: 'Tab' })).toBe(false)
  expect(cancel).toHaveFocus()
  expect(fireEvent.keyDown(cancel, { key: 'Tab', shiftKey: true })).toBe(false)
  expect(apply).toHaveFocus()
  fireEvent(dialog, new Event('cancel', { cancelable: true }))
  await waitFor(() => {
    expect(opener).toHaveFocus()
  })
  expect(requests).toHaveLength(0)
})

it('сохраняет фильтр экземпляра в URL и сбрасывает курсор при смене статуса', async () => {
  const accounts: Account[] = Array.from({ length: 26 }, (_, index) => ({
    ...separate,
    id: `0193a5b0-9999-7000-8000-${String(index).padStart(12, '0')}`,
    native_account_id: `JIRA-${String(index)}`,
  }))
  const { reads, router } = mount(accounts, () => {
    throw new Error('Unexpected write')
  })
  const filter = await screen.findByRole('combobox', {
    name: 'Экземпляр источника',
  })
  const instanceOption = await within(filter).findByRole('option', {
    name: /Jira основной, Jira поддержка/u,
  })
  expect(instanceOption).toHaveValue(connectionExample.instance.id)
  expect(within(filter).getAllByRole('option')).toHaveLength(2)
  fireEvent.change(filter, { target: { value: connectionExample.instance.id } })
  await waitFor(() => {
    expect(reads.at(-1)?.get('instance_id')).toBe(connectionExample.instance.id)
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Следующая страница' }),
  )
  expect(await screen.findByText('JIRA-25', { exact: true })).toBeVisible()
  expect(reads.at(-1)?.get('cursor')).toBe('accounts-2')
  fireEvent.change(screen.getByRole('combobox', { name: 'Статус привязки' }), {
    target: { value: 'assigned' },
  })
  expect(
    await screen.findByText('Нет учётных записей по фильтру'),
  ).toBeVisible()
  expect(reads.at(-1)?.get('assignment')).toBe('assigned')
  expect(reads.at(-1)?.has('cursor')).toBe(false)
  expect(
    new URLSearchParams(router.state.location.search).get('instance_id'),
  ).toBe(connectionExample.instance.id)
})

it('после ошибки перечитывания повторяет только GET и показывает полученную привязку', async () => {
  let failRead = true
  const { requests } = mount(
    [{ ...bot, assignment: botAssignment }],
    () => {
      return receipt({
        source_account_id: bot.id,
        outcome: 'unchanged',
        assignment_id: savedId,
        error_code: null,
      })
    },
    routePaths.sourceAccounts,
    (query) => failRead && query.has('instance_id'),
  )

  await markBot()
  confirm()
  const results = await screen.findByRole('region', {
    name: 'Результат подтверждения',
  })
  expect(
    await within(results).findByText(
      'Не удалось перечитать затронутые привязки',
    ),
  ).toBeVisible()
  expect(results).toHaveTextContent('Привязка уже действовала')
  failRead = false
  fireEvent.click(within(results).getByRole('button', { name: /Повторить/u }))
  await waitFor(async () => {
    expect(
      within(await row(bot)).getByText('Бот', { exact: true }),
    ).toBeVisible()
  })
  expect(requests).toHaveLength(1)
})

it('создаёт человека из учётной записи только после подтверждения', async () => {
  const { requests } = mount([separate], (_init, replace) => {
    replace([
      {
        ...separate,
        assignment: {
          ...botAssignment,
          person_id: anna.id,
          actor_kind: 'human',
        },
      },
    ])
    return receipt({
      source_account_id: separate.id,
      outcome: 'saved',
      assignment_id: savedId,
      error_code: null,
    })
  })
  const account = within(await row(separate))
  fireEvent.click(account.getByRole('button', { name: /Изменить привязку/u }))
  fireEvent.click(account.getByRole('button', { name: 'Создать человека' }))
  const dialog = review()
  expect(
    within(dialog).getByText('Станет: Создать человека (Анна Смирнова)'),
  ).toBeVisible()
  expect(requests).toHaveLength(0)
  confirm(dialog)
  await waitFor(async () => {
    expect(within(await row(separate)).getByText('Подтверждено')).toBeVisible()
  })
  expect(body(requests[0])).toEqual({
    items: [
      {
        source_account_id: separate.id,
        person_id: null,
        actor_kind: 'human',
        create_person: true,
        base_assignment_id: null,
      },
    ],
  } satisfies Schemas['AccountAssignmentBatch'])
})

it('не подставляет логин как имя создаваемого человека, если у записи нет имени', async () => {
  const { requests } = mount([{ ...separate, display_name: null }], () => {
    throw new Error('Unexpected write')
  })
  const account = within(await row(separate))
  fireEvent.click(account.getByRole('button', { name: /Изменить привязку/u }))
  fireEvent.click(account.getByRole('button', { name: 'Создать человека' }))
  expect(within(review()).getByText('Станет: Создать человека')).toBeVisible()
  expect(requests).toHaveLength(0)
})
