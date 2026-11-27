import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
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
const team = teams.data[0]
if (!team) throw new Error('Expected a team fixture')
const teamId = team.id
const root = `/api/v1/teams/${teamId}`
const period = { from: '2026-08-03T00:00:00Z', to: '2026-11-02T00:00:00Z' }
const settingsUrl =
  '/settings/targets?' +
  new URLSearchParams({ team_id: teamId, ...period }).toString()
const group = { task_type: 'Bug', size_label: 'S' }
const author = currentUser.user.id
const revision = '0193a5b0-a1a1-7000-8000-000000000001'
const confirmedAt = '2026-11-08T11:30:00Z'
const mapping = {
  data: [
    {
      version_no: 1,
      base_version_no: null,
      timezone: 'UTC',
      calendar_version_no: null,
      confirmed_by: author,
      confirmed_at: confirmedAt,
      rules: [
        {
          rule_kind: 'status',
          instance_id: revision,
          native_ref: '3',
          native_value: '',
          meaning: 'active',
          rank: null,
        },
      ],
    },
  ],
  next_cursor: null,
} satisfies Schemas['MappingVersionPage']
const p85 = {
  state: 'ASSESSED',
  value: 259201,
  unit: 'seconds',
  numerator: null,
  denominator: null,
  n: 9,
  bound: null,
  low_sample: true,
  excluded_count: 2,
  missing_count: 1,
  reason_code: null,
} satisfies Schemas['MetricValue']
const baseline = {
  revision_id: revision,
  data: [
    { ...group, p85 },
    {
      task_type: 'Task',
      size_label: null,
      p85: { ...p85, reason_code: 'size_unknown' },
    },
    {
      task_type: 'Story',
      size_label: 'M',
      p85: {
        ...p85,
        state: 'UNKNOWN',
        value: null,
        n: 0,
        reason_code: 'history_incomplete',
      },
    },
  ],
} satisfies Schemas['TargetBaselineList']
const v1 = {
  ...group,
  version_no: 1,
  base_version_no: null,
  duration_seconds: 172800,
  basis: 'manual',
  basis_revision_id: null,
  basis_n: null,
  basis_period: null,
  basis_value_seconds: null,
  confirmed_by: author,
  confirmed_at: confirmedAt,
} satisfies Schemas['TargetVersion']
const v2 = {
  ...v1,
  version_no: 2,
  base_version_no: 1,
  duration_seconds: 259201,
  basis: 'historical_p85',
  basis_revision_id: revision,
  basis_n: 9,
  basis_period: period,
  basis_value_seconds: 259201,
} satisfies Schemas['TargetVersion']
const otherVersion = { ...v2, duration_seconds: 345600 }
const v3 = { ...v2, version_no: 3, base_version_no: 2 }

const cleanups: (() => void)[] = []
beforeAll(() => {
  // jsdom has no native modal focus or keyboard activation.
  for (const [name, method] of Object.entries({
    showModal(this: HTMLDialogElement) {
      this.setAttribute('open', '')
    },
    close(this: HTMLDialogElement) {
      this.removeAttribute('open')
    },
  })) {
    if (!Object.hasOwn(HTMLDialogElement.prototype, name)) {
      Object.defineProperty(HTMLDialogElement.prototype, name, {
        value: method,
        configurable: true,
      })
    }
  }
})
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

function mount({
  mappingPage = mapping,
  history = [v1],
  readBaseline = () => jsonResponse(baseline),
  write,
}: {
  mappingPage?: Schemas['MappingVersionPage']
  history?: Schemas['TargetVersion'][]
  readBaseline?: (search: URLSearchParams) => Response
  write?: (headers: Headers) => Response
} = {}) {
  const posts = vi
    .fn<(headers: Headers, body: unknown) => Response>()
    .mockImplementation((headers) => {
      if (write) return write(headers)
      current = [v2, ...current]
      return jsonResponse(v2, 201, { ETag: '"2"' })
    })
  let current = history
  function respond(input: RequestInfo | URL, init?: RequestInit) {
    if (typeof input !== 'string') throw new Error('Expected a URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse({
          ...currentUser,
          roles: [
            ...currentUser.roles,
            {
              team_id: teamId,
              role: 'process_owner',
              project_id: null,
              service_id: null,
              assigned_at: confirmedAt,
            },
          ],
          actions: [...currentUser.actions, 'process.manage'],
        } satisfies Schemas['CurrentUser'])
      case '/api/v1/teams':
        return jsonResponse(teams)
      case '/api/v1/projects':
        return jsonResponse(projects)
      case root + '/mapping-versions':
        return jsonResponse(mappingPage)
      case root + '/target-baselines':
        return readBaseline(url.searchParams)
      case root + '/target-versions': {
        if (init?.method === 'POST') {
          if (typeof init.body !== 'string') throw new Error('Expected JSON')
          const body: unknown = JSON.parse(init.body)
          return posts(new Headers(init.headers), body)
        }
        const data = current.filter(
          (row) =>
            row.task_type === url.searchParams.get('task_type') &&
            row.size_label === url.searchParams.get('size_label'),
        )
        return jsonResponse(
          { data, next_cursor: null } satisfies Schemas['TargetVersionPage'],
          200,
          {
            ETag: `"${String(data[0]?.version_no ?? 0)}"`,
          },
        )
      }
      default:
        throw new Error(`Unexpected URL ${url.pathname}`)
    }
  }
  const fetcher = vi.fn<typeof fetch>((input, init) =>
    Promise.resolve(respond(input, init)),
  )
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [settingsUrl],
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
  return {
    fetcher,
    posts,
    replaceVersions: (versions: Schemas['TargetVersion'][]) => {
      current = versions
    },
  }
}

async function selectGroup() {
  fireEvent.click(
    await screen.findByRole('link', { name: 'Задать срок: Bug, S' }),
  )
  return screen.findByRole('button', { name: 'Сохранить срок' })
}

async function openConfirmation() {
  const save = await selectGroup()
  fireEvent.change(screen.getByLabelText('Основание'), {
    target: { value: 'historical_p85' },
  })
  fireEvent.click(save)
  return screen.getByRole('dialog', { name: 'Подтверждение целевого срока' })
}

describe('Целевые сроки команды', () => {
  it('показывает текст малой выборки, n и полноту в строке p85', async () => {
    mount()
    const link = await screen.findByRole('link', {
      name: 'Задать срок: Bug, S',
    })
    const row = link.closest('tr')
    if (!row) throw new Error('Expected a baseline row')
    expect(within(row).getByText('Малая выборка')).toBeVisible()
    expect(within(row).getByText('n = 9')).toBeVisible()
    expect(within(row).getByText('Пропущено: 1. Исключено: 2.')).toBeVisible()
  })

  it('показывает задачи без размера отдельно и не предлагает задать для них срок', async () => {
    mount()
    const section = (
      await screen.findByRole('heading', { name: 'Без размера' })
    ).closest('section')
    if (!section) throw new Error('Expected a section for unknown sizes')
    expect(
      within(section).getByText(
        'Для задач без размера целевой срок не применяется.',
      ),
    ).toBeVisible()
    expect(within(section).queryByRole('link')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Сохранить срок' }),
    ).not.toBeInTheDocument()
  })

  it('показывает причину отсутствия p85 по группе', async () => {
    mount()
    await screen.findByRole('link', { name: 'Задать срок: Story, M' })
    expect(screen.getByText('История получена не полностью')).toBeVisible()
    expect(screen.getByText('Не рассчитано')).toBeVisible()
  })

  it('открывает предыдущую группу через форму после перехода по ссылке', async () => {
    mount()
    await selectGroup()
    fireEvent.change(screen.getByLabelText('Тип задачи'), {
      target: { value: 'Task' },
    })
    fireEvent.click(screen.getByRole('link', { name: 'Задать срок: Story, M' }))
    expect(
      await screen.findByRole('heading', { name: 'Срок группы Story, M' }),
    ).toHaveFocus()

    const taskType = screen.getByLabelText('Тип задачи')
    const size = screen.getByLabelText('Размер задачи')
    expect(taskType).toHaveValue('Story')
    expect(size).toHaveValue('M')
    fireEvent.change(taskType, { target: { value: 'Bug' } })
    fireEvent.change(size, { target: { value: 'S' } })
    fireEvent.click(screen.getByRole('button', { name: 'Открыть группу' }))

    expect(
      await screen.findByRole('heading', { name: 'Срок группы Bug, S' }),
    ).toBeVisible()
  })

  it('при отсутствии подтверждённого сопоставления предлагает UI-14', async () => {
    const { fetcher } = mount({ mappingPage: { data: [], next_cursor: null } })
    expect(
      await screen.findByRole('link', { name: 'Настроить сопоставление' }),
    ).toHaveAttribute('href', `/settings/process?team_id=${teamId}`)
    expect(
      fetcher.mock.calls.some(
        ([url]) => typeof url === 'string' && url.includes('target-baselines'),
      ),
    ).toBe(false)
  })

  it('передаёт новый период чтения и оставляет ручной ввод при ошибке ориентира', async () => {
    const load = vi.fn((params: URLSearchParams) =>
      params.get('from') === period.from
        ? jsonResponse(baseline)
        : problemResponse('internal_error', 500),
    )
    mount({ readBaseline: load })
    await selectGroup()
    fireEvent.change(screen.getByLabelText('Начало периода'), {
      target: { value: '2026-08-10' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Показать ориентир' }))
    expect(
      await screen.findByText(
        'Не удалось загрузить ориентир. Срок можно задать вручную.',
      ),
    ).toBeVisible()
    expect(load.mock.lastCall?.[0].get('from')).toBe('2026-08-10T00:00:00Z')
    expect(load.mock.lastCall?.[0].get('to')).toBe(period.to)
    expect(
      screen.getByRole('option', { name: 'Исторический p85' }),
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Сохранить срок' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await waitFor(() => {
      expect(load).toHaveBeenCalledTimes(3)
    })
  })

  it.each(['', '0'])(
    'показывает ошибку срока «%s» у полей и в сводке без отправки',
    async (value) => {
      const { posts } = mount()
      const save = await selectGroup()
      const days = screen.getByLabelText('Сутки')
      fireEvent.change(days, { target: { value } })
      fireEvent.click(save)
      expect(days).toHaveAttribute('aria-invalid', 'true')
      expect(days).toHaveAccessibleDescription(/укажите срок больше нуля/iu)
      const alert = screen.getByRole('alert')
      expect(alert).toHaveFocus()
      expect(
        within(alert).getByRole('link', { name: /укажите срок больше нуля/iu }),
      ).toHaveAttribute('href', '#target-days')
      expect(posts).not.toHaveBeenCalled()
    },
  )

  it('подтверждает p85 с ревизией и периодом и оставляет v1 в истории под v2', async () => {
    const { posts } = mount()
    const dialog = await openConfirmation()
    expect(within(dialog).getByText('Станет: 3 д 1 с')).toBeVisible()
    expect(posts).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    expect(await screen.findByText('Целевой срок подтверждён')).toBeVisible()
    const [headers, draft] = posts.mock.calls[0] ?? []
    expect(headers?.get('If-Match')).toBe('"1"')
    expect(headers?.get('Idempotency-Key')).toMatch(/^[\da-f-]{36}$/u)
    expect(draft).toEqual({
      ...group,
      duration_seconds: 259201,
      basis: 'historical_p85',
      basis_revision_id: revision,
      basis_period: period,
    } satisfies Schemas['TargetDraft'])
    expect(
      screen
        .getAllByRole('rowheader', { name: /^v/u })
        .map((item) => item.textContent),
    ).toEqual(['v2 (действует)', 'v1'])
  })

  it('при 412 перечитывает чужую v2 и ждёт повторного подтверждения с новым ETag', async () => {
    const { posts, replaceVersions } = mount({
      write: (headers) => {
        if (headers.get('If-Match') === '"1"')
          return problemResponse('version_conflict', 412)
        replaceVersions([v3, otherVersion, v1])
        return jsonResponse(v3, 201, { ETag: '"3"' })
      },
    })
    const dialog = await openConfirmation()
    replaceVersions([otherVersion, v1])
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    expect(await within(dialog).findByText('Было: 4 д')).toBeVisible()
    expect(posts).toHaveBeenCalledTimes(1)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    expect(
      await screen.findByRole('rowheader', { name: 'v3 (действует)' }),
    ).toBeVisible()
    expect(
      posts.mock.calls.map(([headers]) => headers.get('If-Match')),
    ).toEqual(['"1"', '"2"'])
    expect(
      within(
        screen.getByRole('rowheader', { name: 'v2' }).closest('tr') ??
          document.body,
      ).getByText('4 д'),
    ).toBeVisible()
  })

  it('возвращает ошибку 422 к полю основания и сохраняет введённый срок', async () => {
    mount({
      write: () =>
        jsonResponse(
          {
            ...problem('validation_failed', 422),
            errors: [
              { field: '/basis_revision_id', code: 'unknown_reference' },
            ],
          },
          422,
          { 'Content-Type': 'application/problem+json' },
        ),
    })
    const dialog = await openConfirmation()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    const error = await screen.findByRole('alert')
    expect(error).toHaveFocus()
    expect(screen.getByLabelText('Основание')).toHaveAccessibleDescription(
      /основание недоступно/iu,
    )
    expect(screen.getByLabelText('Сутки')).toHaveValue('3')
  })

  it('после отмены конфликтного подтверждения открывает сравнение с актуальной версией', async () => {
    const { posts, replaceVersions } = mount({
      write: () => problemResponse('version_conflict', 412),
    })
    const dialog = await openConfirmation()
    replaceVersions([otherVersion, v1])
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    await within(dialog).findByText('Было: 4 д')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Отмена' }))
    fireEvent.change(screen.getByLabelText('Сутки'), { target: { value: '5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить срок' }))
    const next = screen.getByRole('dialog', {
      name: 'Подтверждение целевого срока',
    })
    expect(within(next).getByText('Было: 4 д')).toBeVisible()
    expect(within(next).getByText('"2"')).toBeVisible()
    expect(posts).toHaveBeenCalledTimes(1)
  })

  it('при неизвестном исходе и прежней версии повторяет запрос с тем же ключом', async () => {
    const write = vi
      .fn<() => Response>()
      .mockReturnValueOnce(problemResponse('internal_error', 500))
    const { posts, replaceVersions } = mount({ write })
    write.mockImplementation(() => {
      replaceVersions([v2, v1])
      return jsonResponse(v2, 201, { ETag: '"2"' })
    })
    const dialog = await openConfirmation()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
    fireEvent.click(
      await within(dialog).findByRole('button', {
        name: 'Повторить сохранение',
      }),
    )
    expect(await screen.findByText('Целевой срок подтверждён')).toBeVisible()
    expect(posts).toHaveBeenCalledTimes(2)
    const [first, second] = posts.mock.calls
    expect(first?.[0].get('Idempotency-Key')).toBeTruthy()
    expect(second?.[0].get('Idempotency-Key')).toBe(
      first?.[0].get('Idempotency-Key'),
    )
    expect(second?.[0].get('If-Match')).toBe('"1"')
    expect(second?.[1]).toEqual(first?.[1])
  })

  it('защищает ввод при смене периода и возвращает фокус к форме по UI-18', async () => {
    mount()
    await selectGroup()
    const days = screen.getByLabelText('Сутки')
    days.focus()
    fireEvent.change(days, { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText('Начало периода'), {
      target: { value: '2026-08-10' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Показать ориентир' }))
    const dialog = await screen.findByRole('dialog', {
      name: 'Изменения не сохранены',
    })
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Продолжить редактирование' }),
    )
    await waitFor(() => {
      expect(days).toHaveFocus()
    })
    expect(days).toHaveValue('2')
  })

  it('переводит фокус к группе и даёт выполнить подтверждение с клавиатуры', async () => {
    const { posts, replaceVersions } = mount({
      history: [],
      write: () => {
        const saved = { ...v1, duration_seconds: 86400 }
        replaceVersions([saved])
        return jsonResponse(saved, 201, { ETag: '"1"' })
      },
    })
    const groupLink = await screen.findByRole('link', {
      name: 'Задать срок: Bug, S',
    })
    if (!(groupLink instanceof HTMLAnchorElement))
      throw new Error('Expected a link')
    groupLink.focus()
    expect(groupLink).toHaveFocus()
    // Emulate native activation and text entry, which jsdom does not perform for key events.
    fireEvent.keyDown(groupLink, { key: 'Enter' })
    groupLink.click()
    const save = await screen.findByRole('button', { name: 'Сохранить срок' })
    expect(
      screen.getByRole('heading', { name: 'Срок группы Bug, S' }),
    ).toHaveFocus()
    const days = screen.getByLabelText('Сутки')
    days.focus()
    expect(days).toHaveFocus()
    fireEvent.input(days, { target: { value: '1' } })
    if (!(save instanceof HTMLButtonElement))
      throw new Error('Expected a button')
    save.focus()
    fireEvent.keyDown(save, { key: 'Enter' })
    save.click()
    const dialog = await screen.findByRole('dialog', {
      name: 'Подтверждение целевого срока',
    })
    const confirm = within(dialog).getByRole('button', { name: 'Подтвердить' })
    expect(within(dialog).getByRole('heading')).toHaveFocus()
    fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
    expect(confirm).toHaveFocus()
    if (!(confirm instanceof HTMLButtonElement))
      throw new Error('Expected a button')
    fireEvent.keyDown(confirm, { key: 'Enter' })
    confirm.click()
    await waitFor(() => {
      expect(posts).toHaveBeenCalledTimes(1)
    })
    expect(posts.mock.calls[0]?.[0].get('If-Match')).toBe('"0"')
    expect(posts.mock.calls[0]?.[1]).toEqual({
      ...group,
      duration_seconds: 86400,
      basis: 'manual',
      basis_revision_id: null,
      basis_period: null,
    } satisfies Schemas['TargetDraft'])
  })
})
