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
import { getFinding } from '@/shared/api'
import type { components } from '@/shared/api'
import {
  acceptedDetail,
  acceptedFinding,
  componentId,
  findingProfile,
  restrictedFinding,
  secretDetail,
  secretFinding,
} from '../../../test/finding-fixtures'
import {
  currentUser,
  jsonResponse,
  problem,
  problemResponse,
  projects,
  services,
  teams,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const apiPath = '/api/v1/findings'
const project = projects.data[0]
const service = services.data[0]
if (!project || !service)
  throw new Error('Expected project and service fixtures')
const projectId = project.id
const serviceId = service.id
const cleanups: (() => void)[] = []
const cursor = 'next+/='

function page(
  data: Schema['Finding'][],
  next_cursor: string | null = null,
  access_limited = false,
) {
  return jsonResponse({
    data,
    next_cursor,
    access_limited,
    access_notes: [],
  } satisfies Schema['FindingPage'])
}

function mount(path: string, respond?: (url: URL) => Response | undefined) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    const response = respond?.(url)
    if (response) return Promise.resolve(response)
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(
          jsonResponse({
            ...currentUser,
            actions: [
              'session.read',
              'findings.read',
              'mr.read',
              'exceptions.manage',
            ],
          } satisfies Schema['CurrentUser']),
        )
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case '/api/v1/projects':
        return Promise.resolve(jsonResponse(projects))
      case apiPath:
        return Promise.resolve(page([secretFinding]))
      case `${apiPath}/${secretFinding.id}`:
        return Promise.resolve(jsonResponse(secretDetail))
      case `${apiPath}/${acceptedFinding.id}`:
        return Promise.resolve(jsonResponse(acceptedDetail))
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
  vi.spyOn(window, 'scrollTo').mockReturnValue(undefined)
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
  return { fetcher, router, runtime }
}

afterEach(() => {
  for (const clean of cleanups.splice(0)) clean()
  localStorage.clear()
})

it('показывает правило, строки и метаданные отчёта секрета, исключая всё основание', async () => {
  const value = 'synthetic-value-for-test'
  mount(`/findings/${secretFinding.id}`, (url) =>
    url.pathname === `${apiPath}/${secretFinding.id}`
      ? jsonResponse({
          ...secretDetail,
          evidence: { ...secretDetail.evidence, payload: value },
        } satisfies Schema['FindingDetail'])
      : undefined,
  )

  expect(await screen.findByText(secretFinding.rule_id)).toBeVisible()
  expect(screen.getByText('deploy/config.yaml:12-14')).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'deploy/config.yaml:12-14' }),
  ).toBeNull()
  expect(screen.getByText('Уверенность: неизвестно')).toBeVisible()
  expect(screen.getByText('Версия сопоставления: 2')).toBeVisible()
  for (const text of [
    'gitleaks',
    '8.28.0',
    findingProfile,
    'Штатный профиль',
    'r2026.11',
    'Полный',
  ]) {
    expect(screen.getByText(text)).toBeVisible()
  }
  expect(screen.getByLabelText(secretDetail.report.sha)).toBeVisible()
  expect(document.body.innerHTML).not.toContain(value)
  expect(document.body.innerHTML).not.toContain('3f9c2a7b')
  expect(document.body.innerHTML).not.toContain('[REDACTED]')
  expect(screen.queryByRole('link', { name: 'Оформить исключение' })).toBeNull()
})

it('показывает принятую находку, срок исключения и ссылки на разрешённые действия', async () => {
  mount(`/findings/${acceptedFinding.id}`)

  expect(
    await screen.findByText('Состояние: Принята (принят риск)'),
  ).toBeVisible()
  expect(screen.queryByText('Состояние: Исправлена')).toBeNull()
  expect(screen.getByText('Действует · Срок:', { exact: false })).toBeVisible()
  expect(screen.getByText(/09\.12\.2026/u)).toHaveAttribute(
    'datetime',
    acceptedDetail.exceptions[0]?.expires_at,
  )
  expect(
    screen.getByRole('link', { name: 'Оформить исключение' }),
  ).toHaveAttribute('href', `/findings/${acceptedFinding.id}/exception`)
  expect(
    screen.getByRole('link', { name: 'Решение ревьюера' }),
  ).toHaveAttribute(
    'href',
    `/exceptions?finding_id=${acceptedFinding.id}&state=pending`,
  )
  expect(
    screen.getByRole('link', { name: 'Причины изменения результата' }),
  ).toHaveAttribute(
    'href',
    `/components/${componentId}/comparison?head_sha=${acceptedDetail.report.sha}&analysis_kind=sast`,
  )
})

it('показывает очищенное основание и внешний триаж со ссылкой на источник', async () => {
  mount(`/findings/${acceptedFinding.id}`)

  expect(await screen.findByText('SQL-запрос собран из строк')).toBeVisible()
  const triage = screen.getByRole('heading', {
    name: 'Внешний триаж',
  }).parentElement
  if (!triage) throw new Error('Expected triage section')
  expect(within(triage).getByText(/Ложное срабатывание/u)).toBeVisible()
  expect(
    within(triage).getByRole('link', { name: 'SonarQube' }),
  ).toHaveAttribute('href', acceptedDetail.external_triage[0]?.source_url)
  expect(within(triage).queryByRole('button')).toBeNull()
})

it('применяет фильтры и сортировку через URL, сбрасывая входной курсор', async () => {
  const { router, fetcher } = mount('/findings?org_slug=example&cursor=old')
  await screen.findByRole('link', { name: secretFinding.rule_id })
  const form = screen.getByRole('form', { name: 'Фильтры находок' })
  for (const [name, value] of [
    ['Вид', 'security'],
    ['Состояние', 'accepted_risk'],
    ['Серьёзность', 'high'],
    ['Сортировка', '-first_seen_at'],
  ] as const) {
    fireEvent.change(within(form).getByRole('combobox', { name }), {
      target: { value },
    })
  }
  for (const [name, value] of [
    ['Проект', projectId],
    ['Компонент', componentId],
    ['Сервис', serviceId],
  ] as const) {
    fireEvent.change(within(form).getByRole('textbox', { name }), {
      target: { value },
    })
  }
  fireEvent.click(within(form).getByRole('button', { name: 'Применить' }), {
    detail: 0,
  })

  const expected = {
    finding_kind: 'security',
    state: 'accepted_risk',
    severity: 'high',
    sort: '-first_seen_at',
    project_id: projectId,
    component_id: componentId,
    service_id: serviceId,
  }
  await waitFor(() => {
    expect(
      Object.fromEntries(new URLSearchParams(router.state.location.search)),
    ).toEqual({
      org_slug: 'example',
      ...expected,
    })
    expect(
      fetcher.mock.calls.some(([input]) => {
        if (typeof input !== 'string') return false
        const url = new URL(input, 'https://craftpulse.test')
        return (
          url.pathname === apiPath &&
          Array.from(url.searchParams).length ===
            Object.keys(expected).length &&
          Object.entries(expected).every(
            ([key, value]) => url.searchParams.get(key) === value,
          )
        )
      }),
    ).toBe(true)
  })
})

it('догружает курсор, повторяет ошибку и восстанавливает фокус на строке второй страницы', async () => {
  let attempts = 0
  const initial = `/findings?finding_kind=security&sort=-first_seen_at&project_id=${projectId}`
  const { router, fetcher, runtime } = mount(initial, (url) => {
    if (url.pathname === `${apiPath}/${restrictedFinding.id}`)
      return jsonResponse({
        ...acceptedDetail,
        ...restrictedFinding,
      } satisfies Schema['FindingDetail'])
    if (url.pathname !== apiPath) return undefined
    if (url.searchParams.get('cursor') !== cursor)
      return page([acceptedFinding], cursor)
    attempts += 1
    return attempts === 1
      ? problemResponse('service_unavailable', 503)
      : page([restrictedFinding], cursor, true)
  })

  expect(
    await screen.findByRole('link', { name: acceptedFinding.rule_id }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Ещё' }), { detail: 0 })
  expect(
    await screen.findByText('Не удалось загрузить следующие находки'),
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: acceptedFinding.rule_id }),
  ).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Повторить' }), {
    detail: 0,
  })
  const row = await screen.findByRole('link', {
    name: restrictedFinding.rule_id,
  })
  expect(screen.getByText('Есть закрытые результаты.')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Ещё' })).toBeNull()
  expect(
    fetcher.mock.calls.some(
      ([input]) =>
        typeof input === 'string' && input.includes('cursor=next%2B%2F%3D'),
    ),
  ).toBe(true)
  row.focus()
  fireEvent.click(row, { detail: 0 })
  await screen.findByText('Состояние: Принята (принят риск)')
  runtime.queryClient.removeQueries({ queryKey: ['findings', 'pages'] })
  await act(() => router.navigate(-1))
  const restored = await screen.findByRole('link', {
    name: restrictedFinding.rule_id,
  })
  await waitFor(() => {
    expect(restored).toHaveFocus()
  })
  expect(router.state.location.search).toBe(initial.slice('/findings'.length))
  expect(screen.getByRole('combobox', { name: 'Вид' })).toHaveValue('security')
})

it('открывает исходный файл по SHA и строке из карточки отфильтрованной находки', async () => {
  const source = `https://gitlab.example/team/billing/-/blob/${acceptedDetail.report.sha}/services/billing/repo.go#L88`
  const { router } = mount('/findings?finding_kind=security', (url) => {
    if (
      url.pathname === apiPath &&
      url.searchParams.get('finding_kind') === 'security'
    )
      return page([acceptedFinding])
    if (url.pathname === `${apiPath}/${acceptedFinding.id}`)
      return jsonResponse({
        ...acceptedDetail,
        source_url: source,
      } satisfies Schema['FindingDetail'])
    return undefined
  })

  const row = await screen.findByRole('link', { name: acceptedFinding.rule_id })
  row.focus()
  fireEvent.click(row, { detail: 0 })
  expect(
    await screen.findByRole('link', { name: 'services/billing/repo.go:88' }),
  ).toHaveAttribute('href', source)
  expect(new URLSearchParams(router.state.location.search).has('mr_id')).toBe(
    false,
  )
})

it('заменяет карточку находки с ответом 404 на UI-04 без старых или переданных в ошибке данных', async () => {
  let denied = false
  const { router, runtime } = mount(
    '/findings?finding_kind=security',
    (url) => {
      if (url.pathname === apiPath) return page([acceptedFinding])
      if (url.pathname === `${apiPath}/${acceptedFinding.id}` && denied) {
        return jsonResponse(
          {
            ...problem('not_found', 404),
            title: acceptedFinding.rule_id,
            detail: acceptedFinding.location.path,
          },
          404,
          { 'Content-Type': 'application/problem+json' },
        )
      }
      return undefined
    },
  )
  const row = await screen.findByRole('link', { name: acceptedFinding.rule_id })
  await runtime.queryClient.query(getFinding(runtime.api, acceptedFinding.id))
  denied = true

  row.focus()
  expect(row).toHaveFocus()
  fireEvent.click(row, { detail: 0 })
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe('/access-denied')
  expect(screen.queryByText(acceptedFinding.rule_id)).toBeNull()
  expect(screen.queryByText(/billing\/repo\.go/u)).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'К списку' }), {
    detail: 0,
  })
  const restored = await screen.findByRole('link', {
    name: acceptedFinding.rule_id,
  })
  await waitFor(() => {
    expect(restored).toHaveFocus()
  })
})
