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
  projects,
} from '../../../test/fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const path = '/settings/import'
const project = projects.data[0]
if (!project) throw new Error('Expected a project fixture')
const projectId = project.id

const receiptId = '0193a5b0-5858-7000-8000-000000000002'
const receiptPath = '/settings/receipts/' + receiptId
const accepted = () =>
  jsonResponse(
    { receipt_id: receiptId, status: 'quarantined', retry_after_seconds: 3 },
    202,
  )
const user = {
  ...currentUser,
  actions: ['org.read', 'imports.create', 'receipts.read'],
  roles: [
    {
      role: 'tech_lead',
      project_id: projectId,
      team_id: null,
      service_id: null,
      assigned_at: '2026-11-01T00:00:00Z',
    },
  ],
} satisfies Schema['CurrentUser']
const receipt = {
  id: receiptId,
  channel: 'import',
  status: 'rejected',
  received_at: '2026-11-01T00:00:00Z',
  schema_id: 'tasks',
  schema_version: '2',
  record_count: 0,
  accepted_count: 0,
  rejected_count: 0,
  replay_count: 0,
  conflict_count: 0,
  reason_code: 'schema_unsupported',
  duplicate_of: null,
  affected_metrics: [],
  retry_after_seconds: null,
  result_url: null,
} satisfies Schema['Receipt']

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal === 'function') return
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

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  window.localStorage.clear()
  window.sessionStorage.clear()
})

function mount({
  url = path,
  actor = user,
  upload = () => accepted(),
  readProjects = () => projects,
}: {
  url?: string
  actor?: Schema['CurrentUser']
  upload?: (options: RequestInit | undefined) => Response | Promise<Response>
  readProjects?: (query: URLSearchParams) => Schema['ProjectPage']
} = {}) {
  const send = vi.fn(upload)
  const fetcher: typeof fetch = async (input, options) => {
    const url = new URL(
      input instanceof Request ? input.url : input,
      'https://craftpulse.test',
    )
    switch (url.pathname) {
      case '/api/v1/me':
        return jsonResponse(actor)
      case '/api/v1/projects':
        return jsonResponse(readProjects(url.searchParams))
      case '/api/v1/imports':
        return send(options)
      case '/api/v1/receipts/' + receiptId:
        return jsonResponse(receipt)
      default:
        throw new Error('Unexpected request ' + url.pathname)
    }
  }
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [url],
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
  return { router, send }
}

function fill(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

function chooseFile(file: File) {
  fireEvent.change(screen.getByLabelText('Файл'), { target: { files: [file] } })
}

const tasksJSON = () =>
  new File(
    [
      JSON.stringify([
        {
          record_class: 'object',
          record_kind: 'task',
          payload_schema: 'task/1',
          native_id: '1',
          payload: {
            project_native_id: 'IMP',
            task_key: 'IMP-1',
            title: 'Example',
            type_native_id: 'Story',
            status_native_id: 'Done',
            created_at: '2026-02-03T06:00:00Z',
          },
        },
      ]),
    ],
    'tasks.json',
    { type: 'application/json' },
  )

async function fillDraft(schema = 'tasks', file = tasksJSON()) {
  const form = await screen.findByRole('form', { name: 'Импорт' })
  const select = within(form).getByRole('combobox', { name: 'Проект' })
  fireEvent.change(select, { target: { value: projectId } })
  fill('Вид данных', schema)
  fill('Версия схемы', '1')
  fill('Провайдер источника', 'universal')
  fill('Ключ экземпляра источника', 'instance-1')
  chooseFile(file)
}

function start() {
  fireEvent.click(screen.getByRole('button', { name: 'Передать файл' }))
}

function confirmButton() {
  return within(
    screen.getByRole('dialog', { name: 'Передать пакет?' }),
  ).getByRole('button', { name: 'Подтвердить отправку' })
}

function formData(options: RequestInit | undefined) {
  if (!(options?.body instanceof FormData))
    throw new Error('Expected multipart body')
  return options.body
}

function fieldErrors(
  status: number,
  errors: { field: string; code: string }[] = [],
) {
  return jsonResponse(
    {
      ...problem('validation_failed', status),
      errors,
    },
    status,
    { 'Content-Type': 'application/problem+json' },
  )
}

it('передаёт JSON задач одним multipart-запросом и открывает полученную квитанцию', async () => {
  const { send, router } = mount()
  await fillDraft()
  start()
  expect(send).not.toHaveBeenCalled()
  fireEvent.click(confirmButton())

  const heading = await screen.findByRole('heading', {
    name: 'Квитанция приёма',
  })
  expect(heading).toHaveFocus()
  expect(router.state.location.pathname).toBe(receiptPath)
  expect(send).toHaveBeenCalledTimes(1)
  const options = send.mock.calls[0]?.[0]
  expect(options?.method).toBe('POST')
  expect(options?.credentials).toBe('same-origin')
  expect(new Headers(options?.headers).has('Content-Type')).toBe(false)
  const data = formData(options)
  expect(Object.fromEntries(data)).toMatchObject({
    project_id: projectId,
    schema_id: 'tasks',
    schema_version: '1',
    source_provider: 'universal',
    source_instance_key: 'instance-1',
    snapshot: 'false',
  })
  expect(data.get('file')).toMatchObject({
    name: 'tasks.json',
    type: 'application/json',
    size: tasksJSON().size,
  })
  expect(data.has('commit_sha')).toBe(false)
  expect(data.has('report_format')).toBe(false)
})

it('помечает CSV задач как снимок и объясняет отсутствие прошлого WIP', async () => {
  const { send } = mount()
  await fillDraft(
    'tasks',
    new File(
      [
        'native_id,project_native_id,task_key,type_native_id,status_native_id,created_at\n' +
          '1,IMP,IMP-1,Story,Done,2026-02-03T06:00:00Z\n',
      ],
      'tasks.CSV',
      {
        type: 'application/vnd.ms-excel',
      },
    ),
  )
  expect(
    screen.getByRole('checkbox', {
      name: 'Файл содержит только текущее состояние, прошлый WIP из него не строится',
    }),
  ).toBeChecked()
  start()
  expect(screen.getByText('Только текущее состояние')).toBeVisible()
  fireEvent.click(confirmButton())
  await screen.findByRole('heading', { name: 'Квитанция приёма' })
  const data = formData(send.mock.calls[0]?.[0])
  expect(data.get('snapshot')).toBe('true')
  expect(data.get('file')).toMatchObject({
    name: 'tasks.CSV',
    type: 'text/csv',
  })
})

it.each([
  [
    'файл 26 MiB',
    () => new File([new Uint8Array(26 * 1024 * 1024)], 'tasks.json'),
    'Пакет превышает лимит 25 MiB',
  ],
  [
    'неподдерживаемое расширение',
    () => new File(['data'], 'tasks.zip', { type: 'application/zip' }),
    'Выберите файл поддержанного формата',
  ],
] as const)('не передаёт %s', async (_name, file, message) => {
  const { send } = mount()
  await fillDraft('tasks', file())
  start()

  expect(screen.getByLabelText('Файл')).toHaveAccessibleDescription(
    new RegExp(message, 'u'),
  )
  expect(
    within(screen.getByRole('form', { name: 'Импорт' })).getByRole('alert'),
  ).toHaveFocus()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(send).not.toHaveBeenCalled()
})

it.each([
  [413, 'Пакет превышает лимит 25 MiB'],
  [415, 'Сервер не поддерживает тип этого файла'],
] as const)(
  'показывает HTTP %s у поля файла и сохраняет черновик',
  async (status, message) => {
    const { send } = mount({ upload: () => new Response(null, { status }) })
    await fillDraft()
    start()
    fireEvent.click(confirmButton())

    expect(
      await within(screen.getByRole('form', { name: 'Импорт' })).findByRole(
        'alert',
      ),
    ).toHaveFocus()
    expect(screen.getByLabelText('Файл')).toHaveAccessibleDescription(
      new RegExp(message, 'u'),
    )
    expect(screen.getByText('tasks.json', { exact: false })).toBeVisible()
    expect(screen.getByLabelText('Ключ экземпляра источника')).toHaveValue(
      'instance-1',
    )
    expect(send).toHaveBeenCalledTimes(1)
  },
)

it('привязывает ошибки 422 по JSON Pointer к полям и фокусирует сводку', async () => {
  mount({
    upload: () =>
      fieldErrors(422, [
        { field: '/schema_version', code: 'not_allowed_value' },
        { field: '/file/0', code: 'invalid_format' },
      ]),
  })
  await fillDraft()
  start()
  fireEvent.click(confirmButton())

  const summary = await within(
    screen.getByRole('form', { name: 'Импорт' }),
  ).findByRole('alert')
  expect(summary).toHaveFocus()
  const version = screen.getByLabelText('Версия схемы')
  expect(version).toHaveValue('1')
  expect(version).toHaveAccessibleDescription('Значение не поддерживается')
  expect(screen.getByLabelText('Файл')).toHaveAccessibleDescription(
    /Проверьте формат/u,
  )
  fireEvent.click(within(summary).getByRole('link', { name: /Версия схемы/ }))
  expect(version).toHaveFocus()
})

it('показывает отсутствие права на выбранный проект при 403', async () => {
  mount({ upload: () => new Response(null, { status: 403 }) })
  await fillDraft()
  start()
  fireEvent.click(confirmButton())
  await within(screen.getByRole('form', { name: 'Импорт' })).findByRole('alert')
  expect(
    within(screen.getByRole('form', { name: 'Импорт' })).getByLabelText(
      'Проект',
    ),
  ).toHaveAccessibleDescription('Нет права импорта в этот проект')
})

it('после 503 позволяет отправить те же значения и файл позже', async () => {
  const upload = vi
    .fn<() => Response>()
    .mockReturnValueOnce(new Response(null, { status: 503 }))
    .mockImplementation(accepted)
  const { send } = mount({ upload })
  await fillDraft()
  start()
  fireEvent.click(confirmButton())

  expect(
    await within(screen.getByRole('form', { name: 'Импорт' })).findByRole(
      'alert',
    ),
  ).toHaveTextContent('Повторите отправку позже')
  start()
  fireEvent.click(confirmButton())
  await screen.findByRole('heading', { name: 'Квитанция приёма' })
  expect(send).toHaveBeenCalledTimes(2)
  expect([...formData(send.mock.calls[0]?.[0])]).toEqual([
    ...formData(send.mock.calls[1]?.[0]),
  ])
})

it('при двойном нажатии отправляет один запрос и показывает ожидание квитанции', async () => {
  let respond: (response: Response) => void = () => {
    throw new Error('POST has not started')
  }
  const response = new Promise<Response>((resolve) => {
    respond = resolve
  })
  const { send } = mount({ upload: () => response })
  await fillDraft()
  start()
  const button = confirmButton()
  fireEvent.click(button)
  fireEvent.click(button)
  expect(button).toBeDisabled()
  expect(screen.getByRole('status')).toHaveTextContent('Ожидаем квитанцию')
  expect(send).toHaveBeenCalledTimes(1)
  respond(accepted())
  await screen.findByRole('heading', { name: 'Квитанция приёма' })
})

it('при уходе показывает UI-18, позволяет вернуться и отменить импорт без запроса', async () => {
  const { router, send } = mount()
  await fillDraft()
  const file = screen.getByLabelText('Файл')
  file.focus()
  await act(async () => {
    await router.navigate(receiptPath)
  })
  const dialog = screen.getByRole('dialog', { name: 'Изменения не сохранены' })
  const stay = within(dialog).getByRole('button', {
    name: 'Продолжить редактирование',
  })
  expect(stay).toHaveFocus()
  fireEvent.click(stay)
  await waitFor(() => {
    expect(file).toHaveFocus()
  })
  expect(router.state.location.pathname).toBe(path)
  expect(screen.getByText('tasks.json', { exact: false })).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Отмена' }))
  fireEvent.click(screen.getByRole('button', { name: 'Отбросить изменения' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/')
  })
  expect(send).not.toHaveBeenCalled()
})

it('подставляет вид и версию из квитанции и требует выбрать файл заново', async () => {
  const { send, router } = mount({ url: receiptPath })
  fireEvent.click(
    await screen.findByRole('link', { name: 'Исправить и повторить' }),
  )
  expect(await screen.findByLabelText('Вид данных')).toHaveValue('tasks')
  expect(screen.getByLabelText('Версия схемы')).toHaveValue('2')
  expect(screen.getByLabelText('Файл')).toHaveValue('')
  fireEvent.click(screen.getByRole('button', { name: 'Отмена' }))
  await waitFor(() => {
    expect(router.state.location.pathname).toBe(receiptPath)
  })
  expect(send).not.toHaveBeenCalled()
})

it('для покрытия требует ревизию и формат, отправляет LCOV с нормализованным SHA', async () => {
  const { send } = mount()
  await fillDraft('coverage', new File(['TN:\nend_of_record\n'], 'lcov.info'))
  start()
  expect(
    within(screen.getByRole('form', { name: 'Импорт' })).getByRole('alert'),
  ).toHaveTextContent('Выберите формат отчёта')
  expect(screen.getByLabelText('Ревизия (commit SHA)')).toBeInvalid()
  expect(send).not.toHaveBeenCalled()

  fill('Формат отчёта', 'lcov')
  fill('Ревизия (commit SHA)', 'A'.repeat(64))
  start()
  fireEvent.click(confirmButton())
  await screen.findByRole('heading', { name: 'Квитанция приёма' })
  const data = formData(send.mock.calls[0]?.[0])
  expect(Object.fromEntries(data)).toMatchObject({
    schema_id: 'coverage',
    report_format: 'lcov',
    commit_sha: 'a'.repeat(64),
  })
  expect(data.get('file')).toMatchObject({
    type: 'text/plain',
    name: 'lcov.info',
  })
})

it('скрывает пункт и форму без imports.create', async () => {
  const { send } = mount({
    actor: { ...user, actions: ['org.read', 'receipts.read'] },
  })
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(screen.queryByRole('link', { name: 'Импорт' })).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Файл')).not.toBeInTheDocument()
  expect(send).not.toHaveBeenCalled()
})

it('ограничивает список проектов областью техлида и читает следующую страницу', async () => {
  const foreign = {
    ...project,
    id: '0193a5b0-4444-7000-8000-000000000002',
    name: 'Другой проект',
  }
  const readProjects = vi.fn((query: URLSearchParams) =>
    query.has('cursor')
      ? projects
      : { ...projects, data: [foreign], next_cursor: 'next' },
  )
  mount({ readProjects })

  const form = await screen.findByRole('form', { name: 'Импорт' })
  expect(within(form).getByRole('option', { name: project.name })).toBeVisible()
  expect(
    within(form).queryByRole('option', { name: foreign.name }),
  ).not.toBeInTheDocument()
  expect(readProjects.mock.calls.map(([query]) => query.get('cursor'))).toEqual(
    [null, 'next'],
  )
})

it('фокусирует заголовок подтверждения и удерживает Tab в диалоге', async () => {
  const { send } = mount()
  await fillDraft()
  const file = screen.getByLabelText('Файл')
  file.focus()
  expect(file).toHaveFocus()
  start()
  const dialog = screen.getByRole('dialog', { name: 'Передать пакет?' })
  expect(within(dialog).getByRole('heading')).toHaveFocus()
  fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
  expect(confirmButton()).toHaveFocus()
  fireEvent.keyDown(dialog, { key: 'Tab' })
  expect(
    within(dialog).getByRole('button', { name: 'Вернуться к форме' }),
  ).toHaveFocus()
  fireEvent(dialog, new Event('cancel', { cancelable: true }))
  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Передать файл' })).toHaveFocus()
  })
  expect(send).not.toHaveBeenCalled()
})
