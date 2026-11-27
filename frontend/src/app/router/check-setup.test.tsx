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
const projectId = '0193a5b0-4444-7000-8000-000000000001'
const componentId = '0193a5b0-7777-7000-8000-000000000001'
const matrixPath = `/checks/profile/${projectId}`
const setupPath = `/checks/setup/${projectId}`
const profileEndpoint = `/api/v1/projects/${projectId}/check-profiles`
const ciEndpoint = `/api/v1/projects/${projectId}/ci-configuration`
const tokensEndpoint = '/api/v1/upload-tokens'
const rulesHash = 'a'.repeat(64)
const secret = 'cpt_' + 'x'.repeat(43)
const matrix = {
  version_no: 3,
  base_version_no: 2,
  entries: [
    {
      component_id: componentId,
      language: 'go',
      analysis_kind: 'code',
      mode: 'import',
      profile_id: 'sonar-import',
      tool_versions: {},
    },
    {
      component_id: componentId,
      language: '',
      analysis_kind: 'secrets',
      mode: 'builtin',
      profile_id: 'gitleaks-default',
      tool_versions: { gitleaks: '8.28.0' },
      rules_hash: rulesHash,
    },
    {
      component_id: componentId,
      language: '',
      analysis_kind: 'sca',
      mode: 'builtin',
      profile_id: 'trivy-default',
      tool_versions: { trivy: '0.76.2' },
    },
  ],
  uncovered: [
    {
      component_id: componentId,
      language: 'kotlin',
      reason_code: 'language_unsupported',
    },
  ],
  confirmed_by: currentUser.user.id,
  confirmed_at: '2026-11-08T10:00:00Z',
} satisfies Omit<Schema['CheckProfileVersion'], 'entries'> & {
  entries: (Schema['CheckProfileVersion']['entries'][number] & {
    rules_hash?: string
  })[]
}
const configuration = {
  ci_system: 'gitlab_ci',
  profile_version_no: 3,
  snippet:
    'craftpulse:\n  allow_failure: true\n  script: [craftpulse-ci run]\n',
  variables: [
    { name: 'CRAFTPULSE_UPLOAD_TOKEN', masked: true },
    { name: 'CRAFTPULSE_URL', masked: false },
  ],
} satisfies Schema['CiConfiguration']
const token = {
  id: '0193a5b0-dddd-7000-8000-000000000001',
  project_id: projectId,
  slot: 1,
  created_at: '2026-11-08T10:00:00Z',
  revoked_at: null,
} satisfies Schema['UploadToken']
const disposers: (() => void)[] = []

function mount({
  at = matrixPath,
  manage = true,
  handle,
}: {
  at?: string
  manage?: boolean
  handle?: (url: URL, request: RequestInit | undefined) => Response | undefined
} = {}) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) =>
    Promise.resolve().then(() => {
      if (typeof input !== 'string') throw new Error('Expected a request URL')
      const url = new URL(input, 'https://craftpulse.test')
      const response = handle?.(url, init)
      if (response) return response
      switch (url.pathname) {
        case '/api/v1/me':
          return jsonResponse({
            ...currentUser,
            actions: [
              'session.read',
              'check_profiles.manage',
              'ci.read',
              ...(manage ? ['upload_tokens.manage'] : []),
            ],
          } satisfies Schema['CurrentUser'])
        case '/api/v1/teams':
          return jsonResponse(teams)
        case '/api/v1/projects':
          return jsonResponse(projects)
        case profileEndpoint:
          return jsonResponse(matrix, 200, { ETag: '"3"' })
        case `/api/v1/projects/${projectId}/baselines`:
          return jsonResponse({ data: [] } satisfies Schema['BaselineList'])
        case ciEndpoint:
          return jsonResponse(configuration)
        case tokensEndpoint:
          return jsonResponse({
            data: [],
            next_cursor: null,
          } satisfies Schema['UploadTokenPage'])
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
  return { fetcher, router, client: runtime.queryClient }
}

beforeAll(() => {
  // jsdom does not provide a native modal dialog implementation.
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
  vi.unstubAllGlobals()
})

function mockClipboard() {
  const writeText = vi
    .fn<(value: string) => Promise<void>>()
    .mockResolvedValue(undefined)
  vi.stubGlobal(
    'navigator',
    Object.create(navigator, { clipboard: { value: { writeText } } }),
  )
  return writeText
}

const codeMode = /^Режим .* · go · code$/u
async function changeCode() {
  fireEvent.change(await screen.findByRole('combobox', { name: codeMode }), {
    target: { value: 'builtin' },
  })
}

function confirm(dialog: HTMLElement) {
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
}

async function previewChange() {
  await changeCode()
  fireEvent.click(screen.getByRole('button', { name: 'Подтвердить профиль' }))
  return screen.getByRole('dialog', { name: 'Подтвердить профиль проверок' })
}

function body(request: RequestInit | undefined): unknown {
  if (typeof request?.body !== 'string')
    throw new Error('Expected a JSON request body')
  return JSON.parse(request.body)
}

it('shows imported code, builtin security checks and the uncovered language', async () => {
  mount({ at: '/checks/profile' })
  fireEvent.click(await screen.findByRole('link', { name: 'Core' }))
  expect(await screen.findByRole('combobox', { name: codeMode })).toHaveValue(
    'import',
  )
  expect(
    screen.getByRole('combobox', { name: /^Режим .* · secrets$/u }),
  ).toHaveValue('builtin')
  expect(
    screen.getByRole('combobox', { name: /^Режим .* · sca$/u }),
  ).toHaveValue('builtin')
  expect(screen.getByText('gitleaks 8.28.0')).toBeInTheDocument()
  expect(screen.getByText(rulesHash)).toBeInTheDocument()
  const uncovered = within(
    screen.getByRole('region', { name: 'Непокрытые области' }),
  )
  expect(uncovered.getByText(/kotlin/u)).toBeInTheDocument()
  expect(uncovered.getByText(/Язык не поддерживается/u)).toBeInTheDocument()
})

it('opens the project baseline matrix and preserves the inspection return', async () => {
  const { router } = mount()
  const navigation = await screen.findByRole('navigation', {
    name: 'Проверки проекта',
  })
  fireEvent.click(
    within(navigation).getByRole('link', { name: 'Базовые линии' }),
  )
  expect(router.state.location.pathname).toBe(`/checks/baselines/${projectId}`)

  const main = within(screen.getByRole('main'))
  const profile = await main.findByRole('link', { name: 'Матрица проверок' })
  fireEvent.click(profile)
  const back = await main.findByRole('button', { name: 'Назад' })
  fireEvent.click(back)

  await waitFor(() => {
    expect(main.getByRole('link', { name: 'Матрица проверок' })).toHaveFocus()
  })
})

it('saves the complete matrix with the previewed ETag and an idempotency key', async () => {
  const { fetcher } = mount({
    handle: (url, init) => {
      if (url.pathname !== profileEndpoint || init?.method !== 'POST') return
      return jsonResponse(
        {
          ...matrix,
          version_no: 4,
          entries: matrix.entries.map((entry) =>
            entry.analysis_kind === 'code'
              ? { ...entry, mode: 'builtin', profile_id: 'go-default' }
              : entry,
          ),
        } satisfies Schema['CheckProfileVersion'],
        201,
        { ETag: '"4"' },
      )
    },
  })
  const dialog = await previewChange()
  expect(
    within(dialog).getByText('Было: Импорт · sonar-import'),
  ).toBeInTheDocument()
  expect(
    within(dialog).getByText('Станет: Штатный · go-default'),
  ).toBeInTheDocument()
  confirm(dialog)
  expect(await screen.findByText('Подтверждённая версия 4')).toBeInTheDocument()
  const request = fetcher.mock.calls.find(
    ([, init]) => init?.method === 'POST',
  )?.[1]
  const headers = new Headers(request?.headers)
  expect(headers.get('If-Match')).toBe('"3"')
  expect(headers.get('Idempotency-Key')).toMatch(/^[a-f0-9-]{36}$/u)
  expect(body(request)).toEqual({
    entries: [
      {
        component_id: componentId,
        language: 'go',
        analysis_kind: 'code',
        mode: 'builtin',
        profile_id: 'go-default',
      },
      {
        component_id: componentId,
        language: '',
        analysis_kind: 'secrets',
        mode: 'builtin',
        profile_id: 'gitleaks-default',
      },
      {
        component_id: componentId,
        language: '',
        analysis_kind: 'sca',
        mode: 'builtin',
        profile_id: 'trivy-default',
      },
    ],
  })
})

it('explains a missing secrets row when validation returns 422', async () => {
  mount({
    handle: (url, init) => {
      if (url.pathname !== profileEndpoint) return
      if (init?.method !== 'POST') {
        return jsonResponse(
          {
            ...matrix,
            entries: matrix.entries.filter(
              ({ analysis_kind }) => analysis_kind !== 'secrets',
            ),
          },
          200,
          { ETag: '"3"' },
        )
      }
      return jsonResponse(
        {
          ...problem('validation_failed', 422),
          errors: [
            {
              field: '/entries',
              code: 'required',
              detail: 'component needs a row of secrets',
            },
          ],
        } satisfies Schema['Problem'],
        422,
        { 'Content-Type': 'application/problem+json' },
      )
    },
  })
  confirm(await previewChange())
  expect(
    await screen.findByText(
      `${componentId} · Без языка · secrets — обязательная проверка отсутствует`,
    ),
  ).toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: codeMode })).toHaveValue(
    'builtin',
  )
})

it('resolves an indexed validation error against the submitted row', async () => {
  mount({
    handle: (url, init) =>
      url.pathname === profileEndpoint && init?.method === 'POST'
        ? jsonResponse(
            {
              ...problem('validation_failed', 422),
              errors: [
                { field: '/entries/0/profile_id', code: 'not_allowed_value' },
              ],
            } satisfies Schema['Problem'],
            422,
            { 'Content-Type': 'application/problem+json' },
          )
        : undefined,
  })
  confirm(await previewChange())
  expect(
    await screen.findByText(
      /go · code \(profile_id\) — Профиль, режим, язык или вид анализа не поддерживается/u,
    ),
  ).toBeInTheDocument()
})

it('reopens the preview against the latest version after 412 without losing concurrent changes', async () => {
  let conflict = false
  const current = {
    ...matrix,
    version_no: 4,
    entries: matrix.entries.map((entry) =>
      entry.analysis_kind === 'sca'
        ? { ...entry, mode: 'import', profile_id: 'sca-import' }
        : entry,
    ),
  } satisfies Schema['CheckProfileVersion']
  const { fetcher } = mount({
    handle: (url, init) => {
      if (url.pathname !== profileEndpoint) return
      if (!conflict && init?.method === 'POST') {
        conflict = true
        return problemResponse('version_conflict', 412)
      }
      if (conflict && init?.method === 'POST') {
        return jsonResponse({ ...current, version_no: 5 }, 201, { ETag: '"5"' })
      }
      return conflict ? jsonResponse(current, 200, { ETag: '"4"' }) : undefined
    },
  })
  const dialog = await previewChange()
  confirm(dialog)
  expect(
    await within(dialog).findByText(
      'Данные изменились. Просмотрите актуальную версию',
    ),
  ).toBeInTheDocument()
  expect(within(dialog).getByText('"4"')).toBeInTheDocument()
  expect(screen.getByText('Подтверждённая версия 4')).toBeInTheDocument()
  confirm(dialog)
  expect(await screen.findByText('Подтверждённая версия 5')).toBeInTheDocument()
  const requests = fetcher.mock.calls
    .filter(([, init]) => init?.method === 'POST')
    .map(([, init]) => init)
  const headers = requests.map((request) => new Headers(request?.headers))
  expect(headers.map((values) => values.get('If-Match'))).toEqual([
    '"3"',
    '"4"',
  ])
  expect(headers[0]?.get('Idempotency-Key')).not.toBe(
    headers[1]?.get('Idempotency-Key'),
  )
  expect(body(requests[1])).toMatchObject({
    entries: [
      { profile_id: 'go-default' },
      { profile_id: 'gitleaks-default' },
      { profile_id: 'sca-import' },
    ],
  })
})

it('asks before leaving a changed matrix and keeps the draft if the user stays', async () => {
  const { router } = mount()
  await changeCode()
  const link = screen.getByRole('link', { name: 'Инструкция CI' })
  fireEvent.click(link)
  const dialog = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Продолжить редактирование' }),
  )
  expect(router.state.location.pathname).toBe(matrixPath)
  expect(screen.getByRole('combobox', { name: codeMode })).toHaveValue(
    'builtin',
  )
  fireEvent.click(link)
  fireEvent.click(
    within(
      await screen.findByRole('dialog', { name: 'Изменения не сохранены' }),
    ).getByRole('button', { name: 'Отбросить изменения' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Подключение шага CI' }),
  ).toBeInTheDocument()
})

it('copies the complete GitLab snippet from the configuration response', async () => {
  const writeText = mockClipboard()
  const { fetcher } = mount({ at: setupPath })
  const copyButton = await screen.findByRole('button', {
    name: 'Скопировать конфигурацию',
  })
  expect(screen.getByRole('combobox', { name: 'Система CI' })).toHaveValue(
    'gitlab_ci',
  )
  expect(
    screen.getByText('CRAFTPULSE_UPLOAD_TOKEN').closest('li'),
  ).toHaveTextContent('Маскированная')
  expect(screen.getByLabelText('Конфигурация GitLab CI').textContent).toBe(
    configuration.snippet,
  )
  copyButton.focus()
  expect(copyButton).toHaveFocus()
  // Keyboard activation emits a click with detail zero.
  fireEvent.click(copyButton, { detail: 0 })
  expect(await screen.findByText('Скопировано')).toBeInTheDocument()
  expect(writeText).toHaveBeenCalledExactlyOnceWith(configuration.snippet)
  expect(fetcher).toHaveBeenCalledWith(
    `${ciEndpoint}?ci_system=gitlab_ci`,
    expect.objectContaining({ method: 'GET' }),
  )
})

it('hides token management and makes no token requests without the separate permission', async () => {
  const { fetcher } = mount({ at: setupPath, manage: false })
  expect(
    await screen.findByRole('button', { name: 'Скопировать конфигурацию' }),
  ).toBeInTheDocument()
  expect(
    screen.getByText(/обратитесь к администратору проекта/u),
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Выпустить токен' }),
  ).not.toBeInTheDocument()
  expect(
    fetcher.mock.calls.some(
      ([input]) =>
        typeof input === 'string' && input.startsWith(tokensEndpoint),
    ),
  ).toBe(false)
})

async function startIssuing() {
  const opener = await screen.findByRole('button', { name: 'Выпустить токен' })
  opener.focus()
  fireEvent.click(opener)
  confirm(screen.getByRole('dialog', { name: 'Выпустить токен загрузки' }))
}

it('issues a token with an idempotency key and removes the one-time value on close', async () => {
  const writeText = mockClipboard()
  let issued = false
  const { client, fetcher } = mount({
    at: setupPath,
    handle: (url, init) => {
      if (url.pathname !== tokensEndpoint) return
      if (init?.method === 'POST') {
        issued = true
        return jsonResponse(
          { ...token, value: secret } satisfies Schema['UploadTokenCreated'],
          201,
        )
      }
      return jsonResponse({
        data: issued ? [token] : [],
        next_cursor: null,
      } satisfies Schema['UploadTokenPage'])
    },
  })
  await startIssuing()
  const dialog = await screen.findByRole('dialog', {
    name: 'Токен загрузки выпущен',
  })
  expect(within(dialog).getByText(secret)).toBeInTheDocument()
  expect(
    within(dialog).getByText(/нельзя посмотреть повторно/u),
  ).toBeInTheDocument()
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Скопировать токен' }),
  )
  expect(writeText).toHaveBeenCalledExactlyOnceWith(secret)
  expect(
    client
      .getMutationCache()
      .getAll()
      .map((mutation) => mutation.state.data),
  ).toEqual([undefined])
  fireEvent.click(within(dialog).getByRole('button', { name: 'Закрыть' }))
  expect(screen.queryByText(secret)).not.toBeInTheDocument()
  await waitFor(() => {
    expect(
      screen.getByRole('button', { name: 'Выпустить токен' }),
    ).toHaveFocus()
  })
  await waitFor(() => {
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  })
  expect(
    await screen.findByRole('button', { name: 'Отозвать токен (слот 1)' }),
  ).toBeInTheDocument()
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data),
    ),
  ).not.toContain(secret)
  expect(screen.getByLabelText('Конфигурация GitLab CI')).not.toHaveTextContent(
    secret,
  )
  const request = fetcher.mock.calls.find(
    ([, init]) => init?.method === 'POST',
  )?.[1]
  expect(new Headers(request?.headers).get('Idempotency-Key')).toMatch(
    /^[a-f0-9-]{36}$/u,
  )
  expect(body(request)).toEqual({ project_id: projectId })
})

it('explains the active token limit when a third issuance returns 409', async () => {
  mount({
    at: setupPath,
    handle: (url, init) => {
      if (url.pathname !== tokensEndpoint) return
      if (init?.method === 'POST') return problemResponse('state_conflict', 409)
      return jsonResponse({
        data: [
          token,
          { ...token, slot: 2, id: '0193a5b0-dddd-7000-8000-000000000002' },
        ],
        next_cursor: null,
      } satisfies Schema['UploadTokenPage'])
    },
  })
  await startIssuing()
  expect(
    await screen.findByText(/Сначала отзовите прежний токен/u),
  ).toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('revokes the selected token through confirmation and refreshes its status', async () => {
  let revoked = false
  const lastUsed = '2026-11-08T10:05:00Z'
  const { fetcher } = mount({
    at: setupPath,
    handle: (url, init) => {
      if (
        url.pathname === `${tokensEndpoint}/${token.id}` &&
        init?.method === 'DELETE'
      ) {
        revoked = true
        return new Response(null, { status: 204 })
      }
      if (url.pathname !== tokensEndpoint) return
      return jsonResponse({
        data: [
          {
            ...token,
            revoked_at: revoked ? '2026-11-08T11:00:00Z' : null,
            last_used_at: lastUsed,
          },
        ],
        next_cursor: null,
      })
    },
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Отозвать токен (слот 1)' }),
  )
  expect(
    screen
      .getByText('Последнее использование')
      .nextElementSibling?.querySelector('time'),
  ).toHaveAttribute('datetime', lastUsed)
  const dialog = screen.getByRole('dialog', { name: 'Отозвать токен загрузки' })
  expect(fetcher.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(
    false,
  )
  confirm(dialog)
  expect(await screen.findByText('Токен отозван.')).toBeInTheDocument()
  await waitFor(() => {
    expect(
      screen.queryByRole('button', { name: 'Отозвать токен (слот 1)' }),
    ).not.toBeInTheDocument()
  })
  expect(fetcher).toHaveBeenCalledWith(
    `${tokensEndpoint}/${token.id}`,
    expect.objectContaining({ method: 'DELETE' }),
  )
})
