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
import { probeExample, syncStatusExample } from '../../../test/admin-fixtures'
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
const token = 'training-token-UI10'
const baseUrl = 'https://example.atlassian.net'
const apiRoot = '/api/v1/connections'
const newPath = '/settings/connections/new'
const id = '0193a5b0-5555-7000-8000-000000000001'
const detailPath = '/settings/connections/' + id
const detailApi = apiRoot + '/' + id
const user = {
  ...currentUser,
  actions: ['connections.manage', 'session.read', 'sync.read'],
}
const pendingProbe = {
  ...probeExample,
  status: 'pending',
  finished_at: null,
  edition: null,
  source_version: null,
  contract_version: null,
  capabilities: [],
  retry_after_seconds: 1,
} satisfies Schema['ConnectionProbe']
const adapter = {
  provider: 'jira_cloud',
  display_name: 'Jira Cloud',
  editions: ['cloud'],
  contract_versions: ['1'],
  data_kinds: ['tasks', 'accounts', 'permissions'],
  scope_kinds: ['tracker_project'],
  credential_fields: [
    { name: 'email', label: 'Email Atlassian', required: true, secret: false },
    {
      name: 'token',
      label: 'API token Atlassian',
      required: true,
      secret: true,
    },
  ],
} satisfies Schema['Adapter']
const draft = {
  id,
  name: 'Jira основной',
  provider: 'jira_cloud',
  instance: {
    id: '0193a5b0-aaaa-7000-8000-000000000001',
    base_url: baseUrl,
    edition: null,
    source_version: null,
  },
  status: 'draft',
  adapter_kind: 'builtin',
  access_mode: 'service_account',
  credentials_set_at: '2026-11-08T10:00:00Z',
  sync_paused: false,
  paused_until: null,
  last_success_at: null,
  scopes: [
    {
      id: '0193a5b0-5656-7000-8000-000000000001',
      kind: 'tracker_project',
      native_scope_id: '10200',
      display_name: '10200',
      area_id: null,
      history_from: '2026-01-01T00:00:00Z',
    },
  ],
  capabilities: [],
} satisfies Schema['Connection']
const active = {
  ...draft,
  status: 'active',
  capabilities: probeExample.capabilities,
} satisfies Schema['Connection']

beforeAll(() => {
  if (!Object.hasOwn(HTMLDialogElement.prototype, 'showModal')) {
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
  }
})

const dispose: (() => void)[] = []
afterEach(() => {
  for (const cleanup of dispose.splice(0)) cleanup()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

function mount({
  path = newPath,
  connection = draft,
  cachedConnection,
  read = () =>
    jsonResponse(connection, 200, { ETag: path === newPath ? '"2"' : '"1"' }),
  write = () =>
    jsonResponse(connection, path === newPath ? 201 : 200, { ETag: '"2"' }),
  start = () => jsonResponse(pendingProbe, 202),
  readProbe = () => jsonResponse(probeExample),
  activate = () => jsonResponse(active, 200, { ETag: '"2"' }),
  sync = () =>
    jsonResponse({
      ...syncStatusExample,
      data_kinds: syncStatusExample.data_kinds.map((entry) => ({
        ...entry,
        phase: 'backfill',
      })),
    }),
}: {
  path?: string
  connection?: Schema['Connection']
  cachedConnection?: Schema['Connection']
  read?: () => Response | Promise<Response>
  write?: (request: RequestInit) => Response | Promise<Response>
  start?: () => Response
  readProbe?: (probeId: string) => Response
  activate?: (request: RequestInit) => Response
  sync?: () => Response
} = {}) {
  const starts = vi.fn(start)
  const probeReads = vi.fn(readProbe)
  const activations = vi.fn(activate)
  const saves = vi.fn(write)
  const readConnection = vi.fn(read)
  const fetcher: typeof fetch = async (input, request) => {
    const url = new URL(
      input instanceof Request ? input.url : input,
      'https://craftpulse.test',
    )
    if (
      url.pathname === detailApi + '/activation' &&
      request?.method === 'POST'
    )
      return activations(request)
    if (url.pathname === detailApi + '/probes' && request?.method === 'POST')
      return starts()
    if (
      url.pathname.startsWith(detailApi + '/probes/') &&
      request?.method === 'GET'
    )
      return probeReads(url.pathname.slice((detailApi + '/probes/').length))
    if (request?.method === 'POST' || request?.method === 'PATCH')
      return saves(request)
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse(user)
      case '/api/v1/teams':
        return jsonResponse(teams)
      case '/api/v1/projects':
        return jsonResponse(projects)
      case '/api/v1/adapters':
        return jsonResponse({ data: [adapter] })
      case detailApi + '/sync-status':
        return sync()
      case detailApi:
        return readConnection()
      default:
        throw Error('Unexpected request ' + url.pathname)
    }
  }
  const runtime = createAppRuntime(fetcher)
  if (cachedConnection) {
    runtime.queryClient.setQueryData(['adapters', 'list'], { data: [adapter] })
    runtime.queryClient.setQueryData(['connections', 'detail', id], {
      data: cachedConnection,
      etag: '"0"',
    })
  }
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [path],
  })
  const view = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  dispose.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return {
    router,
    cache: runtime.queryClient,
    saves,
    readConnection,
    starts,
    probeReads,
    activations,
  }
}

function jsonBody(request: RequestInit | undefined): unknown {
  if (typeof request?.body !== 'string') throw Error('Expected a JSON body')
  return JSON.parse(request.body)
}

function fill(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}
function fillCredentials() {
  fill('Email Atlassian', 'service@example.org')
  fill('API token Atlassian', token)
}
async function fillDraft() {
  fireEvent.click(await screen.findByRole('combobox', { name: 'Адаптер' }))
  fireEvent.click(await screen.findByRole('option', { name: 'Jira Cloud' }))
  fill('Имя подключения', draft.name)
  fill('Базовый адрес источника', baseUrl)
  fill('Идентификатор области в источнике', '10200')
  fillCredentials()
}
function save(label = 'Сохранить черновик') {
  fireEvent.click(screen.getByRole('button', { name: label }))
}

it('создаёт черновик Jira Cloud с началом истории и открывает адрес подключения', async () => {
  const { router, cache, saves } = mount()
  await fillDraft()
  expect(screen.getByText('cloud')).toBeVisible()
  expect(
    screen.getByText('Задачи, Учётные записи, Права доступа'),
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Универсальный импорт' }),
  ).toHaveAttribute('href', '/settings/import')
  fireEvent.click(
    screen.getByRole('checkbox', { name: 'Вся доступная история' }),
  )
  fill('Начало истории (UTC)', '2026-01-01T00:00')
  save()

  expect(
    await screen.findByRole('heading', { name: 'Настройки подключения' }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe(detailPath)
  const request = saves.mock.calls[0]?.[0]
  expect(request?.method).toBe('POST')
  expect(jsonBody(request)).toEqual({
    provider: 'jira_cloud',
    name: draft.name,
    base_url: baseUrl,
    adapter_kind: 'builtin',
    adapter_url: null,
    adapter_key: null,
    credentials: { email: 'service@example.org', token },
    scopes: [
      {
        kind: 'tracker_project',
        native_scope_id: '10200',
        history_from: '2026-01-01T00:00:00.000Z',
      },
    ],
  })
  expect(cache.getQueryData(['connections', 'detail', id])).toMatchObject({
    etag: '"2"',
    data: { status: 'draft' },
  })
})

it('убирает учётные данные после ответа, сохраняя только дату', async () => {
  let respond: (response: Response) => void = () => {
    throw Error('Response resolver is not ready')
  }
  const response = new Promise<Response>((resolve) => {
    respond = resolve
  })
  const { router, cache, saves } = mount({ write: () => response })
  await fillDraft()
  expect(screen.getByLabelText('API token Atlassian')).toHaveAttribute(
    'type',
    'password',
  )
  expect(screen.getByLabelText('API token Atlassian')).toHaveAttribute(
    'autocomplete',
    'new-password',
  )
  save()
  await waitFor(() => {
    expect(saves).toHaveBeenCalledOnce()
  })
  const mutations = cache.getMutationCache()
  expect(mutations.getAll()).toHaveLength(1)
  expect(mutations.getAll()[0]?.options.gcTime).toBe(0)
  expect(
    JSON.stringify(mutations.getAll().map(({ state }) => state)),
  ).not.toContain(token)
  respond(jsonResponse(draft, 201, { ETag: '"2"' }))

  expect(
    await screen.findByText('Учётные данные заданы.', { exact: false }),
  ).toBeVisible()
  expect(document.querySelector('time')).toHaveAttribute(
    'datetime',
    draft.credentials_set_at,
  )
  expect(document.body.outerHTML).not.toContain(token)
  expect(screen.queryByLabelText('API token Atlassian')).not.toBeInTheDocument()
  expect(router.state.location).toMatchObject({
    pathname: detailPath,
    search: '',
    hash: '',
  })
  expect(
    JSON.stringify(
      cache
        .getQueryCache()
        .getAll()
        .map(({ queryKey, state }) => [queryKey, state]),
    ),
  ).not.toContain(token)
  expect(mutations.getAll()).toHaveLength(0)
  expect(JSON.stringify(window.localStorage)).not.toContain(token)
  expect(JSON.stringify(window.sessionStorage)).not.toContain(token)
  expect(document.cookie).not.toContain(token)
  expect(JSON.stringify(router.state.location)).not.toContain(token)
})

it('помечает адрес при 422 outside_scope и сохраняет несекретный ввод', async () => {
  const { cache } = mount({
    write: () =>
      jsonResponse(
        {
          ...problem('validation_failed', 422),
          detail: token,
          errors: [
            { field: '/base_url', code: 'outside_scope', detail: token },
          ],
        },
        422,
        { 'Content-Type': 'application/problem+json' },
      ),
  })
  await fillDraft()
  save()
  const summary = await screen.findByRole('alert')
  expect(summary).toHaveFocus()
  const message = 'Адрес вне разрешённой сетевой области установки'
  const address = screen.getByLabelText('Базовый адрес источника')
  expect(address).toHaveAccessibleDescription(message)
  fireEvent.click(within(summary).getByRole('link', { name: message }))
  expect(address).toHaveFocus()
  expect(address).toHaveValue(baseUrl)
  expect(screen.getByLabelText('Имя подключения')).toHaveValue(draft.name)
  expect(
    screen.getByLabelText('Идентификатор области в источнике'),
  ).toHaveValue('10200')
  expect(screen.getByLabelText('Email Atlassian')).toHaveValue('')
  expect(screen.getByLabelText('API token Atlassian')).toHaveValue('')
  expect(document.body.outerHTML).not.toContain(token)
  expect(cache.getMutationCache().getAll()).toHaveLength(0)
})

it('показывает ошибки JSON Pointer у строки области и поля учётных данных', async () => {
  mount({
    write: () =>
      jsonResponse(
        {
          ...problem('validation_failed', 422),
          errors: [
            { field: '/scopes/0/native_scope_id', code: 'unknown_reference' },
            { field: '/credentials/token', code: 'invalid_format' },
          ],
        },
        422,
        { 'Content-Type': 'application/problem+json' },
      ),
  })
  await fillDraft()
  save()
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(
    screen.getByLabelText('Идентификатор области в источнике'),
  ).toHaveAccessibleDescription('Значение не найдено')
  expect(
    screen.getByLabelText('API token Atlassian'),
  ).toHaveAccessibleDescription('Проверьте формат значения')
})

it('помечает блок областей при 409 state_conflict', async () => {
  mount({ write: () => problemResponse('state_conflict', 409) })
  await fillDraft()
  save()
  const message = 'Эту область уже собирает другое активное подключение'
  const summary = await screen.findByRole('alert')
  expect(
    within(screen.getByRole('group', { name: 'Область и история' })).getByText(
      message,
    ),
  ).toBeVisible()
  expect(within(summary).getByRole('link', { name: message })).toBeVisible()
  expect(
    screen.getByLabelText('Идентификатор области в источнике'),
  ).toHaveValue('10200')
  expect(screen.getByLabelText('API token Atlassian')).toHaveValue('')
})

it('отклоняет HTTP локально без отправки запроса', async () => {
  const { saves } = mount()
  await fillDraft()
  fill('Базовый адрес источника', 'http://example.atlassian.net')
  save()
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(
    screen.getByLabelText('Базовый адрес источника'),
  ).toHaveAccessibleDescription('Укажите адрес HTTPS длиной до 2048 символов')
  expect(saves).not.toHaveBeenCalled()
})

it('заменяет учётные данные через merge patch и показывает новую дату', async () => {
  const credentials_set_at = '2026-11-10T10:00:00Z'
  const { saves, cache } = mount({
    path: detailPath,
    write: () =>
      jsonResponse({ ...draft, credentials_set_at }, 200, { ETag: '"2"' }),
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Заменить учётные данные' }),
  )
  fillCredentials()
  save()
  expect(await screen.findByText('Подключение сохранено')).toBeVisible()
  const request = saves.mock.calls[0]?.[0]
  expect(request?.method).toBe('PATCH')
  expect(new Headers(request?.headers).get('Content-Type')).toBe(
    'application/merge-patch+json',
  )
  expect(new Headers(request?.headers).get('If-Match')).toBe('"1"')
  expect(jsonBody(request)).toEqual({
    credentials: { email: 'service@example.org', token },
  })
  expect(document.querySelector('time')).toHaveAttribute(
    'datetime',
    credentials_set_at,
  )
  expect(cache.getMutationCache().getAll()).toHaveLength(0)
  expect(screen.queryByLabelText('API token Atlassian')).not.toBeInTheDocument()
})

it('предупреждает о возврате в черновик перед заменой области активного подключения', async () => {
  let respond: (response: Response) => void = () => {
    throw Error('Response resolver is not ready')
  }
  const pendingRead = new Promise<Response>((resolve) => {
    respond = resolve
  })
  const { saves } = mount({
    path: detailPath,
    cachedConnection: draft,
    read: () => pendingRead,
  })

  await screen.findByRole('heading', { name: 'Настройки подключения' })
  expect(screen.queryByDisplayValue(draft.name)).not.toBeInTheDocument()
  respond(
    jsonResponse(
      { ...draft, status: 'active', webhook_url: baseUrl + '/webhook' },
      200,
      { ETag: '"1"' },
    ),
  )
  expect(await screen.findByText(baseUrl + '/webhook')).toBeVisible()
  const group = screen.getByRole('group', { name: 'Область и история' })
  fireEvent.click(
    within(group).getByRole('button', { name: 'Добавить область' }),
  )
  fireEvent.change(
    within(group).getAllByLabelText('Идентификатор области в источнике')[1] ??
      group,
    { target: { value: '10300' } },
  )
  fireEvent.click(
    within(group).getByRole('button', { name: 'Удалить область 1' }),
  )
  save('Сохранить')
  const dialog = await screen.findByRole('dialog', {
    name: 'Подтвердите изменение подключения',
  })
  expect(dialog).toHaveAccessibleDescription(/Подключение вернётся в черновик/u)
  expect(saves).not.toHaveBeenCalled()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  await waitFor(() => {
    expect(saves).toHaveBeenCalledOnce()
  })
  const request = saves.mock.calls[0]?.[0]
  expect(new Headers(request?.headers).get('If-Match')).toBe('"1"')
  expect(jsonBody(request)).toEqual({
    scopes: [
      { kind: 'tracker_project', native_scope_id: '10300', history_from: null },
    ],
  })
})

it('открывает конфликт версии при 412 и перечитывает данные по явному действию', async () => {
  const { saves, readConnection } = mount({
    path: detailPath,
    write: () => problemResponse('version_conflict', 412),
  })
  await screen.findByDisplayValue(draft.name)
  fill('Имя подключения', 'Локальное имя')
  save()
  const dialog = await screen.findByRole('dialog', {
    name: 'Данные подключения изменились',
  })
  expect(saves).toHaveBeenCalledOnce()
  const request = saves.mock.calls[0]?.[0]
  expect(new Headers(request?.headers).get('If-Match')).toBe('"1"')
  expect(jsonBody(request)).toEqual({ name: 'Локальное имя' })
  expect(screen.getByLabelText('Имя подключения')).toHaveValue('Локальное имя')
  expect(screen.queryByText('Подключение сохранено')).not.toBeInTheDocument()
  readConnection.mockImplementationOnce(() =>
    jsonResponse({ ...draft, name: 'Имя с сервера' }, 200, { ETag: '"3"' }),
  )
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Загрузить актуальную версию' }),
  )
  expect(await screen.findByDisplayValue('Имя с сервера')).toBeVisible()
  expect(saves).toHaveBeenCalledOnce()
})

it('открывает UI-18 при отмене и уходит без записи после явного сброса', async () => {
  const { router, saves } = mount()
  await fillDraft()
  fireEvent.click(screen.getByRole('button', { name: 'Отмена' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  expect(router.state.location.pathname).toBe(newPath)
  expect(within(dialog).getByText(/секретные значения/u)).toBeVisible()
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Отбросить изменения' }),
  )
  expect(
    await screen.findByRole('heading', { name: 'Подключения' }),
  ).toBeVisible()
  expect(saves).not.toHaveBeenCalled()
  expect(document.body.outerHTML).not.toContain(token)
})

async function startCheck() {
  const button = await screen.findByRole('button', {
    name: 'Проверить подключение',
  })
  expect(button).toBeEnabled()
  fireEvent.click(button)
  return button
}

function beginActivation() {
  return screen.getByRole('button', { name: 'Подтвердить и начать сбор' })
}

async function openActivation() {
  await screen.findByText('Проверка завершена', {}, { timeout: 3000 })
  fireEvent.click(beginActivation())
  return screen.getByRole('dialog', { name: 'Подтвердите подключение' })
}

it('ожидает интервал пробы и показывает возможности по группам до активации', async () => {
  const { starts, probeReads } = mount({ path: detailPath })
  const check = await startCheck()
  expect(await screen.findByText('Ожидание пробного чтения')).toBeVisible()
  expect(check).toBeDisabled()
  expect(beginActivation()).toBeDisabled()
  expect(starts).toHaveBeenCalledTimes(1)
  expect(probeReads).not.toHaveBeenCalled()

  expect(
    await screen.findByText('1001.0.0-SNAPSHOT', {}, { timeout: 3000 }),
  ).toBeVisible()
  const panel = screen.getByRole('group', { name: 'Проверка подключения' })
  expect(within(panel).getByText('cloud')).toBeVisible()
  expect(within(panel).getByText('1')).toBeVisible()
  expect(probeReads).toHaveBeenCalledWith(probeExample.id)
  const publication = within(panel)
    .getByRole('heading', { name: 'Публикация и запись' })
    .closest('section')
  expect(publication).not.toBeNull()
  if (publication === null) throw Error('Expected publication group')
  const denied = within(publication)
    .getByText('Создание задач в трекере')
    .closest('li')
  if (denied === null) throw Error('Expected missing capability')
  expect(within(denied).getByText('недоступно')).toBeVisible()
  expect(
    within(denied).getByText('Недостаточно прав в источнике'),
  ).toBeVisible()
  expect(within(panel).getByRole('heading', { name: 'История' })).toBeVisible()
  expect(within(panel).getByRole('heading', { name: 'Права' })).toBeVisible()
  const permissions = within(panel)
    .getByText('Чтение прав доступа')
    .closest('li')
  if (permissions === null)
    throw Error('Expected unchecked permission capability')
  expect(within(permissions).getByText('не проверено')).toBeVisible()
  expect(
    within(panel).getByText(
      'Показатели, которым нужны недостающие данные, будут неизвестны.',
    ),
  ).toBeVisible()
})

it.each([
  [
    'edition_mismatch',
    'Это другая редакция, этот адаптер её не поддерживает',
    'Воспользуйтесь универсальным импортом.',
  ],
  [
    'auth_failed',
    'Источник отклонил учётные данные',
    'Замените учётные данные в форме, сохраните их и повторите проверку.',
  ],
  [
    'timeout',
    'Источник не ответил вовремя',
    'Проверьте адрес источника или повторите проверку позже.',
  ],
  ['adapter_unavailable', 'Адаптер недоступен', 'Повторите проверку позже.'],
  [
    'outside_network_scope',
    'Адрес вне разрешённой сетевой области',
    'Проверьте базовый адрес и разрешённую сетевую область.',
  ],
  [
    'certificate_untrusted',
    'Сертификат источника не входит в список доверенных',
    'Проверьте адрес и настройте доверие к сертификату источника.',
  ],
  [
    'contract_unsupported',
    'Версия контракта адаптера не поддерживается',
    'Уточните поддерживаемую версию у администратора установки и повторите проверку.',
  ],
  [
    'configuration_changed',
    'Настройки подключения изменились во время проверки',
    'Сохраните актуальные настройки и запустите новую проверку.',
  ],
])(
  'блокирует активацию после failed с причиной %s',
  async (code, reason, remedy) => {
    mount({
      path: detailPath,
      readProbe: () =>
        jsonResponse({
          ...pendingProbe,
          status: 'failed',
          finished_at: probeExample.finished_at,
          reason_code: code,
          retry_after_seconds: null,
        } satisfies Schema['ConnectionProbe']),
    })
    const check = await startCheck()
    const alert = await screen.findByRole('alert', {}, { timeout: 3000 })
    expect(within(alert).getByText(reason)).toBeVisible()
    expect(within(alert).getByText(remedy)).toBeVisible()
    expect(check).toBeEnabled()
    expect(beginActivation()).toBeDisabled()
    if (code === 'edition_mismatch')
      expect(
        within(alert).getByRole('link', { name: 'Универсальный импорт' }),
      ).toHaveAttribute('href', '/settings/import')
  },
)

it('подтверждает показанную пробу с If-Match и открывает загрузку истории на UI-11', async () => {
  const { router, activations } = mount({
    path: detailPath,
    readProbe: () =>
      jsonResponse({
        ...probeExample,
        capabilities: [
          ...probeExample.capabilities,
          {
            capability: 'read_field_history',
            state: 'partial',
            reason_code: 'changelog_unavailable',
            checked_at: probeExample.finished_at,
          },
        ],
      } satisfies Schema['ConnectionProbe']),
  })
  await startCheck()
  const dialog = await openActivation()
  const confirm = within(dialog).getByRole('button', { name: 'Подтвердить' })
  expect(within(dialog).getByRole('button', { name: 'Отмена' })).toHaveFocus()
  fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
  expect(confirm).toHaveFocus()
  expect(
    within(dialog).getByText('Проект трекера 10200', { exact: false }),
  ).toBeVisible()
  expect(within(dialog).getByText('Чтение истории полей')).toBeVisible()
  expect(within(dialog).getByText('Создание задач в трекере')).toBeVisible()
  expect(within(dialog).getByText('Подключение будет частичным.')).toBeVisible()
  expect(
    within(dialog).getByText('После подтверждения начнётся загрузка истории.'),
  ).toBeVisible()
  fireEvent.click(confirm)
  expect(
    await screen.findByRole('heading', { name: 'Состояние сбора' }),
  ).toBeVisible()
  expect(await screen.findByText('Загрузка истории')).toBeVisible()
  expect(router.state.location.pathname).toBe(detailPath + '/sync')
  const request = activations.mock.calls[0]?.[0]
  expect(new Headers(request?.headers).get('If-Match')).toBe('"1"')
  expect(jsonBody(request)).toEqual({ probe_id: probeExample.id })
})

it('при 409 заменяет завершённую пробу и требует повторного подтверждения', async () => {
  const nextId = '0193a5b0-5757-7000-8000-000000000002'
  const start = vi
    .fn(() => jsonResponse(pendingProbe, 202))
    .mockImplementationOnce(() => jsonResponse(pendingProbe, 202))
    .mockImplementationOnce(() =>
      jsonResponse({ ...pendingProbe, id: nextId }, 202),
    )
  const { activations, probeReads, readConnection } = mount({
    path: detailPath,
    start,
    readProbe: (probeId) => jsonResponse({ ...probeExample, id: probeId }),
    activate: () => problemResponse('state_conflict', 409),
  })
  await startCheck()
  const dialog = await openActivation()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByText(
      'Сервер отклонил подтверждение. Для него нужна последняя успешная проверка.',
    ),
  ).toBeVisible()
  expect(beginActivation()).toBeDisabled()
  expect(screen.queryByText('1001.0.0-SNAPSHOT')).toBeNull()
  await openActivation()
  expect(probeReads).toHaveBeenLastCalledWith(nextId)
  expect(readConnection).toHaveBeenCalledTimes(2)
  expect(activations).toHaveBeenCalledTimes(1)
})

it('при 412 показывает конфликт версии в UI-17 и сохраняет блокировку старой пробы', async () => {
  const { starts } = mount({
    path: detailPath,
    activate: () => problemResponse('version_conflict', 412),
  })
  await startCheck()
  const dialog = await openActivation()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  const conflict = await screen.findByRole('dialog', {
    name: 'Данные подключения изменились',
  })
  expect(
    within(conflict).getByRole('button', {
      name: 'Загрузить актуальную версию',
    }),
  ).toBeVisible()
  fireEvent.click(within(conflict).getByRole('button', { name: 'Отмена' }))
  expect(beginActivation()).toBeDisabled()
  expect(starts).toHaveBeenCalledTimes(1)
})

it('после изменения и сохранения области требует новую пробу', async () => {
  const nextId = '0193a5b0-5757-7000-8000-000000000002'
  const start = vi
    .fn(() => jsonResponse(pendingProbe, 202))
    .mockImplementationOnce(() => jsonResponse(pendingProbe, 202))
    .mockImplementationOnce(() =>
      jsonResponse(
        { ...pendingProbe, id: nextId, configuration_version: 2 },
        202,
      ),
    )
  const { activations } = mount({
    path: detailPath,
    start,
    readProbe: (probeId) =>
      jsonResponse({
        ...probeExample,
        id: probeId,
        configuration_version: probeId === nextId ? 2 : 1,
      }),
    write: () =>
      jsonResponse(
        {
          ...draft,
          scopes: draft.scopes.map((scope) => ({
            ...scope,
            display_name: '10300',
            native_scope_id: '10300',
          })),
        },
        200,
        { ETag: '"2"' },
      ),
  })
  await startCheck()
  await screen.findByText('Проверка завершена', {}, { timeout: 3000 })
  fill('Идентификатор области в источнике', '10300')
  expect(beginActivation()).toBeDisabled()
  expect(
    screen.getByText('Результат проверки устарел. Запустите новую проверку.'),
  ).toBeVisible()
  save()
  expect(await screen.findByText('Подключение сохранено')).toBeVisible()
  expect(beginActivation()).toBeDisabled()
  await startCheck()
  const dialog = await openActivation()
  expect(
    within(dialog).getByText('Проект трекера 10300', { exact: false }),
  ).toBeVisible()
  expect(activations).not.toHaveBeenCalled()
})

it('возвращается с UI-11, сохраняет новые учётные данные и подтверждает восстановление', async () => {
  let connection: Schema['Connection'] = { ...draft, status: 'access_lost' }
  const { router, saves, activations } = mount({
    path: detailPath + '/sync',
    sync: () =>
      jsonResponse({
        ...syncStatusExample,
        connection_status: connection.status,
      }),
    read: () => jsonResponse(connection, 200, { ETag: '"1"' }),
    write: () => {
      connection = { ...connection, credentials_set_at: '2026-11-08T10:00:30Z' }
      return jsonResponse(connection, 200, { ETag: '"2"' })
    },
    start: () =>
      jsonResponse({ ...pendingProbe, configuration_version: 2 }, 202),
    readProbe: () =>
      jsonResponse({ ...probeExample, configuration_version: 2 }),
    activate: () => {
      connection = active
      return jsonResponse(active, 200, { ETag: '"3"' })
    },
  })
  fireEvent.click(
    await screen.findByRole('link', { name: 'Заменить учётные данные' }),
  )
  fireEvent.click(
    await screen.findByRole('button', { name: 'Заменить учётные данные' }),
  )
  fillCredentials()
  save()
  expect(await screen.findByText('Подключение сохранено')).toBeVisible()
  expect(jsonBody(saves.mock.calls[0]?.[0])).toEqual({
    credentials: { email: 'service@example.org', token },
  })
  await startCheck()
  const dialog = await openActivation()
  expect(
    within(dialog).getByText('Сбор продолжится с сохранённого места.'),
  ).toBeVisible()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByRole('heading', { name: 'Состояние сбора' }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe(detailPath + '/sync')
  expect(
    new Headers(activations.mock.calls[0]?.[0]?.headers).get('If-Match'),
  ).toBe('"2"')
})
