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
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
type Structure = Schema['OrganizationStructure']
type Write = Schema['OrganizationStructureInput']
const team = {
  id: '0193a5b0-3333-7000-8000-000000000001',
  name: 'Платформа',
  status: 'active',
} as const
const project = {
  id: '0193a5b0-4444-7000-8000-000000000001',
  name: 'Core',
  status: 'active',
} as const
const service = {
  id: '0193a5b0-6666-7000-8000-000000000001',
  name: 'billing',
  status: 'active',
} as const
const area = {
  id: '0193a5b0-8888-7000-8000-000000000001',
  kind: 'repository',
  display_name: 'platform/core',
  connection_id: '0193a5b0-5555-7000-8000-000000000002',
} as const
const person = {
  id: '0193a5b0-2222-7000-8000-000000000001',
  display_name: 'Анна Смирнова',
  kind: 'human',
} as const
const component = {
  id: '0193a5b0-7777-7000-8000-000000000001',
  service_id: service.id,
  repository_area_id: area.id,
  path_prefix: 'services/billing',
  origin: 'manifest',
  status: 'proposed',
} as const
const baseline: Structure = {
  version: 7,
  teams: [team],
  projects: [project],
  services: [service],
  components: [component],
  team_members: [],
  team_projects: [],
  project_areas: [],
}
const newTeamId = '0193a5b0-3333-7000-8000-000000000002'
const newProjectId = '0193a5b0-4444-7000-8000-000000000002'
const secondComponent = {
  ...component,
  id: '0193a5b0-7777-7000-8000-000000000002',
  path_prefix: 'services/api',
}
beforeAll(() => {
  const prototype = HTMLDialogElement.prototype
  if (!Object.hasOwn(prototype, 'showModal')) {
    Object.defineProperty(prototype, 'showModal', {
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
      configurable: true,
    })
  }
  if (!Object.hasOwn(prototype, 'close')) {
    Object.defineProperty(prototype, 'close', {
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
      configurable: true,
    })
  }
})

const cleanup: (() => void)[] = []
afterEach(() => {
  for (const dispose of cleanup.splice(0)) dispose()
  vi.restoreAllMocks()
})

function requestUrl(input: RequestInfo | URL) {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

function setup({
  initial = baseline,
  areas = [area],
  onWrite,
  authorized = true,
  route = '/organization/structure',
}: {
  initial?: Structure
  areas?: Schema['AreaPage']['data']
  onWrite?: (
    number: number,
    snapshot: Structure,
    update: (value: Structure) => void,
  ) => Response | undefined
  authorized?: boolean
  route?: string
} = {}) {
  let snapshot = initial
  const writes: { body: Write; tag: string | null }[] = []
  const appUser = {
    ...currentUser,
    actions: authorized
      ? ['session.read', 'report.read', 'org.manage', 'accounts.manage']
      : ['session.read', 'report.read'],
  }
  const get = (value: Structure) =>
    jsonResponse(value, 200, { ETag: '"' + String(value.version) + '"' })
  function response(input: RequestInfo | URL, options?: RequestInit) {
    const url = new URL(requestUrl(input), 'http://localhost')
    if (url.pathname === '/api/v1/me') return jsonResponse(appUser)
    if (url.pathname === '/api/v1/areas')
      return jsonResponse({
        data: areas,
        next_cursor: null,
      } satisfies Schema['AreaPage'])
    if (url.pathname === '/api/v1/persons')
      return jsonResponse({
        data: url.searchParams.has('cursor') ? [person] : [],
        next_cursor: url.searchParams.has('cursor') ? null : 'people-next',
      } satisfies Schema['PersonPage'])
    if (url.pathname === '/api/v1/organization/structure') {
      if (options?.method !== 'PUT') return get(snapshot)
      if (typeof options.body !== 'string')
        throw new Error('Expected a JSON body')
      const body = JSON.parse(options.body) as Write
      writes.push({ body, tag: new Headers(options.headers).get('If-Match') })
      const response = onWrite?.(writes.length, snapshot, (value) => {
        snapshot = value
      })
      if (response) return response
      const date = '2026-10-01T00:00:00Z'
      snapshot = {
        ...body,
        version: snapshot.version + 1,
        teams: body.teams.map((row) => ({ ...row, id: row.id ?? newTeamId })),
        projects: body.projects.map((row) => ({
          ...row,
          id: row.id ?? newProjectId,
        })),
        services: body.services.map((row) => ({
          ...row,
          id: row.id ?? service.id,
        })),
        components: [
          ...snapshot.components.filter(
            (row) =>
              row.status === 'proposed' &&
              !body.components.some(({ id }) => id === row.id),
          ),
          ...body.components.map(
            (row) =>
              ({
                ...row,
                id: row.id ?? secondComponent.id,
                origin:
                  snapshot.components.find(({ id }) => id === row.id)?.origin ??
                  'manual',
              }) as const,
          ),
        ],
        team_projects: body.team_projects.map((row) => ({
          ...row,
          valid_from: date,
        })),
        team_members: body.team_members.map((row) => ({
          ...row,
          valid_from: date,
        })),
        project_areas: body.project_areas.map((row) => ({
          ...row,
          valid_from: date,
        })),
      }
      return get(snapshot)
    }
    return jsonResponse({ data: [], next_cursor: null })
  }
  const fetcher = vi.fn<typeof fetch>((input, options) =>
    Promise.resolve(response(input, options)),
  )
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [route],
  })
  const rendered = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  cleanup.push(() => {
    rendered.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return { writes, fetcher, router }
}

function button(name: string) {
  return screen.getByRole('button', { name })
}
function press(name: string) {
  fireEvent.click(button(name))
}
async function fill(name: string, value: string) {
  const editor = await screen.findByRole('region', { name: /редактирование/ })
  const field = await within(editor).findByLabelText(name, { exact: true })
  fireEvent.change(field, { target: { value } })
  return field
}
function addLinkIn(name: string) {
  const list = screen.getByRole('heading', { name }).closest('section')
  if (!list) throw new Error('Missing link list')
  return within(list).getByRole('button', { name: 'Добавить связь' })
}

async function confirm() {
  press('Просмотреть изменения')
  const dialog = await screen.findByRole('dialog', {
    name: 'Подтвердить структуру организации',
  })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
}

it('сохраняет новые команду и проект, затем связывает их и добавляет участника по ID', async () => {
  const { writes } = setup()
  expect(
    await screen.findByText('Сохранённая версия структуры: 7'),
  ).toBeVisible()
  expect(screen.getByRole('cell', { name: 'Платформа' })).toBeVisible()
  press('Добавить')
  const name = await fill('Имя', 'Инфраструктура')
  expect(name).toHaveFocus()
  expect(screen.getByText(/Сначала сохраните запись/)).toBeVisible()
  press('Проекты')
  press('Добавить')
  await fill('Имя', 'Data')
  await confirm()
  expect(
    await screen.findByText('Сохранённая версия структуры: 8'),
  ).toBeVisible()
  expect(writes[0]).toEqual({
    tag: '"7"',
    body: {
      teams: [team, { id: null, name: 'Инфраструктура', status: 'active' }],
      projects: [project, { id: null, name: 'Data', status: 'active' }],
      services: [service],
      components: [],
      team_projects: [],
      team_members: [],
      project_areas: [],
    },
  })
  press('Связи')
  fireEvent.click(addLinkIn('Команды и проекты'))
  await fill('Команда', newTeamId)
  await fill('Проект', newProjectId)
  fireEvent.click(addLinkIn('Участники команд'))
  await fill('Команда', newTeamId)
  await fill('Участник', person.id)
  await confirm()
  expect(
    await screen.findByText('Сохранённая версия структуры: 9'),
  ).toBeVisible()
  expect(writes[1]?.body.team_projects).toEqual([
    {
      team_id: newTeamId,
      project_id: newProjectId,
      valid_from: null,
      valid_to: null,
    },
  ])
  expect(writes[1]?.body.team_members).toEqual([
    {
      team_id: newTeamId,
      person_id: person.id,
      valid_from: null,
      valid_to: null,
    },
  ])
})

it('сохраняет явные решения по манифестам и исправленный путь', async () => {
  const { writes } = setup({
    initial: { ...baseline, components: [component, secondComponent] },
  })
  await screen.findByText('Сохранённая версия структуры: 7')
  press('Сервисы и компоненты')
  press('Исправить путь services/billing')
  await fill('Префикс пути', 'apps/billing')
  press('Подтвердить компонент')
  press('Отклонить services/api')
  await confirm()
  expect(
    await screen.findByText('Сохранённая версия структуры: 8'),
  ).toBeVisible()
  expect(writes[0]?.body.components).toEqual([
    {
      id: component.id,
      service_id: service.id,
      repository_area_id: area.id,
      path_prefix: 'apps/billing',
      status: 'confirmed',
    },
    {
      id: secondComponent.id,
      service_id: service.id,
      repository_area_id: area.id,
      path_prefix: 'services/api',
      status: 'rejected',
    },
  ])
})

it('после 412 показывает чужие правки и требует нового подтверждения с новым ETag', async () => {
  const { writes } = setup({
    onWrite: (count, snapshot, update) => {
      if (count !== 1) return undefined
      update({
        ...snapshot,
        version: 8,
        teams: [{ ...team, status: 'archived' }],
        projects: [
          ...snapshot.projects,
          { ...project, id: newProjectId, name: 'Чужой проект' },
        ],
      })
      return problemResponse('version_conflict', 412)
    },
  })
  await screen.findByText('Сохранённая версия структуры: 7')
  press('Изменить Платформа')
  await fill('Имя', 'Платформа v2')
  await confirm()
  const dialog = await screen.findByRole('dialog')
  expect(await within(dialog).findByText('Станет: Чужой проект')).toBeVisible()
  expect(within(dialog).getByText('Станет: Платформа v2')).toBeVisible()
  expect(writes).toHaveLength(1)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByText('Сохранённая версия структуры: 9'),
  ).toBeVisible()
  expect(writes.map(({ tag }) => tag)).toEqual(['"7"', '"8"'])
  expect(writes[1]?.body.teams).toEqual([
    { ...team, name: 'Платформа v2', status: 'archived' },
  ])
  expect(writes[1]?.body.projects).toContainEqual({
    ...project,
    id: newProjectId,
    name: 'Чужой проект',
  })
})

it('показывает 422 у полей и переводит фокус из сводки к сохранённому вводу', async () => {
  const { writes } = setup({
    onWrite: () =>
      jsonResponse(
        {
          ...problem('validation_failed', 422),
          errors: [
            { field: '/project_areas/0/area_id', code: 'unknown_reference' },
            { field: '/project_areas/0', code: 'duplicate' },
          ],
        },
        422,
        { 'Content-Type': 'application/problem+json' },
      ),
  })
  await screen.findByText('Сохранённая версия структуры: 7')
  press('Связи')
  const add = addLinkIn('Области источников проектов')
  await waitFor(() => {
    expect(add).toBeEnabled()
  })
  fireEvent.click(add)
  await fill('Проект', project.id)
  const field = await fill('Область источника', area.id)
  await confirm()
  const summary = await screen.findByRole('alert')
  const link = within(summary).getByRole('link', {
    name: /Выбранная запись больше недоступна/,
  })
  const overlap = within(summary).getByRole('link', {
    name: /Интервалы связи пересекаются/,
  })
  fireEvent.click(overlap)
  expect(field).toHaveFocus()
  fireEvent.click(link)
  expect(field).toHaveFocus()
  expect(field).toHaveValue(area.id)
  expect(field).toHaveAccessibleDescription(
    'Выбранная запись больше недоступна. Интервалы связи пересекаются',
  )
  expect(field).toHaveAttribute('aria-invalid', 'true')
  expect(writes).toHaveLength(1)
})

it('объясняет пустой список областей в разделах проекта и компонентов', async () => {
  setup({ areas: [] })
  await screen.findByText('Сохранённая версия структуры: 7')
  press('Связи')
  expect(await screen.findByText('Источник не настроен')).toBeVisible()
  press('Сервисы и компоненты')
  expect(await screen.findByText('Источник не настроен')).toBeVisible()
  press('Исправить путь services/billing')
  expect(await screen.findByLabelText('Репозиторий')).toBeDisabled()
})

it('сохраняет черновик при отмене UI-17 и отбрасывает его через UI-18 без PUT', async () => {
  const { writes, router } = setup()
  await screen.findByText('Сохранённая версия структуры: 7')
  press('Изменить Платформа')
  const name = await fill('Имя', 'Черновик')
  const form = name.closest('form')
  if (!form) throw new Error('Missing structure form')
  fireEvent.submit(form)
  const confirmation = await screen.findByRole('dialog')
  fireEvent.keyDown(confirmation, { key: 'Tab' })
  expect(
    within(confirmation).getByRole('button', { name: 'Отмена' }),
  ).toHaveFocus()
  fireEvent.click(within(confirmation).getByRole('button', { name: 'Отмена' }))
  expect(name).toHaveValue('Черновик')
  name.focus()
  await act(async () => {
    await router.navigate('/users')
  })
  const unsaved = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  const keep = within(unsaved).getByRole('button', {
    name: 'Продолжить редактирование',
  })
  expect(keep).toHaveFocus()
  fireEvent.click(keep)
  await waitFor(() => {
    expect(name).toHaveFocus()
  })
  press('Отмена')
  fireEvent.click(
    within(await screen.findByRole('dialog')).getByRole('button', {
      name: 'Отбросить изменения',
    }),
  )
  expect(await screen.findByDisplayValue('Платформа')).toBeVisible()
  expect(writes).toEqual([])
})

it('закрывает прямой маршрут без org.manage до чтения структуры', async () => {
  const { fetcher } = setup({ authorized: false })
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(
    fetcher.mock.calls.some(([url]) =>
      requestUrl(url).includes('/organization/structure'),
    ),
  ).toBe(false)
})

it('не создаёт вторую команду, если сохранение прошло, но ответ не удалось прочитать', async () => {
  const { writes } = setup({
    onWrite: (_count, snapshot, update) => {
      update({
        ...snapshot,
        version: 8,
        teams: [
          ...snapshot.teams,
          { id: newTeamId, name: 'Инфраструктура', status: 'active' },
        ],
      })
      return jsonResponse(null)
    },
  })
  await screen.findByText('Сохранённая версия структуры: 7')
  press('Добавить')
  await fill('Имя', 'Инфраструктура')
  await confirm()
  const dialog = await screen.findByRole('dialog')
  fireEvent.click(await within(dialog).findByRole('button', { name: 'Готово' }))
  expect(
    await screen.findByText('Сохранённая версия структуры: 8'),
  ).toBeVisible()
  expect(screen.getAllByRole('cell', { name: 'Инфраструктура' })).toHaveLength(
    1,
  )
  expect(writes).toHaveLength(1)
})

it('сохраняет строки, скрытые пагинацией и поиском', async () => {
  const extraTeams = Array.from({ length: 26 }, (_, index) => ({
    ...team,
    id: '0193a5b0-aaaa-7000-8000-' + String(index).padStart(12, '0'),
    name: 'Команда ' + String(index),
  }))
  const allTeams = [team, ...extraTeams]
  const { writes } = setup({ initial: { ...baseline, teams: allTeams } })
  await screen.findByText('Сохранённая версия структуры: 7')
  expect(screen.queryByRole('cell', { name: 'Команда 25' })).toBeNull()
  fireEvent.change(screen.getByLabelText('Поиск в списке'), {
    target: { value: 'Команда 25' },
  })
  press('Изменить Команда 25')
  await fill('Имя', 'Отчёты')
  await confirm()
  await screen.findByText('Сохранённая версия структуры: 8')
  expect(writes[0]?.body.teams).toEqual(
    allTeams.map((row) =>
      row.name === 'Команда 25' ? { ...row, name: 'Отчёты' } : row,
    ),
  )
})
