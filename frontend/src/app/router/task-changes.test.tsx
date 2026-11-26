import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import type { components } from '@/shared/api'
import { taskAnalysisLinks } from '@/shared/config'
import { jsonResponse, problemResponse, teams } from '../../../test/fixtures'
import {
  candidate,
  confirmed,
  failedAttempt,
  manager,
  mergeRequest,
  otherConfiguration,
  pipeline,
  runLink,
  successfulRetry,
  usageSeconds,
} from '../../../test/task-changes-fixtures'
import {
  mrReference,
  taskDetail,
  taskHistory,
  taskId,
  taskTitle,
} from '../../../test/task-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']

const candidateTask: Schema['TaskDetail'] = {
  ...taskDetail,
  links: [candidate],
  observations: [
    { kind: 'unknown', since: null, basis: [], action: 'clarify_reason' },
  ],
}
const decisionPath = `/api/v1/links/${candidate.id}/decisions`
const pipelinePath = `/api/v1/pipelines/${pipeline.id}`
const cleanups: (() => void)[] = []

// jsdom exposes dialog elements without the native modal methods.
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

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
})

function mount({
  user = manager,
  loadTask = () => jsonResponse(candidateTask),
  loadMR = () => jsonResponse(mergeRequest),
  loadPipeline = () => jsonResponse(pipeline),
  loadAttempts,
  decide,
}: {
  user?: Schema['CurrentUser']
  loadTask?: () => Response
  loadMR?: () => Response
  loadPipeline?: () => Response
  loadAttempts?: (cursor: string | null) => Schema['AttemptPage']
  decide?: (request: RequestInit | undefined) => Response | Promise<Response>
} = {}) {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementation(async (input, init) => {
      if (typeof input !== 'string') throw new Error('Expected a request URL')
      const url = new URL(input, 'https://craftpulse.test')
      switch (url.pathname) {
        case '/api/v1/me':
          return jsonResponse(user)
        case '/api/v1/teams':
          return jsonResponse(teams)
        case `/api/v1/tasks/${taskId}`:
          return loadTask()
        case `/api/v1/tasks/${taskId}/events`:
          return jsonResponse(taskHistory)
        case `/api/v1/merge-requests/${mrReference.id}`:
          return loadMR()
        case pipelinePath:
          return loadPipeline()
        case pipelinePath + '/attempts':
          if (loadAttempts)
            return jsonResponse(loadAttempts(url.searchParams.get('cursor')))
          break
        case decisionPath:
          if (decide) return decide(init)
      }
      throw new Error(`Unexpected request: ${url.pathname}`)
    })
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [taskAnalysisLinks.taskDetail(taskId)],
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
  return router
}

async function openDecision(label = 'Подтвердить связь') {
  const candidateLinks = within(
    await screen.findByRole('list', { name: 'Кандидаты связи' }),
  )
  fireEvent.click(candidateLinks.getByRole('button', { name: label }))
  const dialog = within(screen.getByRole('dialog', { name: label }))
  return { dialog, submit: dialog.getByRole('button', { name: label }) }
}

it('shows a branch-key candidate separately and reloads the confirmed link and observation basis', async () => {
  const decide = vi.fn((request: RequestInit | undefined) => {
    expect(request?.method).toBe('POST')
    expect(request?.body).toBe(JSON.stringify({ decision: 'confirm' }))
    const headers = new Headers(request?.headers)
    expect(headers.has('If-Match')).toBe(false)
    expect(headers.get('Idempotency-Key')).toMatch(/^[\da-f-]{36}$/)
    return jsonResponse(confirmed)
  })
  mount({
    loadTask: () =>
      jsonResponse(
        decide.mock.calls.length
          ? { ...taskDetail, links: [confirmed] }
          : candidateTask,
      ),
    decide,
  })
  expect(
    await screen.findByText('Есть только кандидаты связи с MR'),
  ).toBeVisible()
  expect(
    screen.getByText('Ключ CORE-15 найден в имени ветки MR.'),
  ).toBeVisible()
  expect(
    screen.queryByRole('list', { name: 'Подтверждённые связи' }),
  ).not.toBeInTheDocument()
  const { dialog, submit } = await openDecision()
  expect(dialog.getByText(taskTitle)).toBeVisible()
  expect(dialog.getByText('!42 · Перенос отчёта')).toBeVisible()
  expect(
    dialog.getByText('MR станет основанием наблюдений задачи.'),
  ).toBeVisible()
  fireEvent.click(submit)
  const links = within(
    await screen.findByRole('list', { name: 'Подтверждённые связи' }),
  )
  expect(links.getAllByRole('listitem')).toHaveLength(1)
  expect(links.getByText(/Подтверждение пользователем/)).toBeVisible()
  expect(
    screen.queryByRole('list', { name: 'Кандидаты связи' }),
  ).not.toBeInTheDocument()
  const observations = within(
    screen.getByRole('region', { name: 'Наблюдения' }),
  )
  expect(
    observations.getByRole('link', { name: '!42 · Перенос отчёта · GitLab' }),
  ).toBeVisible()
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  expect(decide).toHaveBeenCalledTimes(1)
})

it('reloads the task after rejection and removes the candidate from the card', async () => {
  const titleKey = {
    ...candidate,
    method: 'title_key',
  } satisfies Schema['Link']
  const reject = vi.fn(() =>
    jsonResponse({
      ...candidate,
      status: 'rejected',
      decided_at: confirmed.decided_at,
    } satisfies Schema['Link']),
  )
  mount({
    loadTask: () =>
      jsonResponse({
        ...candidateTask,
        links: reject.mock.calls.length
          ? [{ ...titleKey, status: 'rejected' }]
          : [titleKey],
      } satisfies Schema['TaskDetail']),
    decide: reject,
  })
  expect(
    await screen.findByText('Ключ CORE-15 найден в названии MR.'),
  ).toBeVisible()
  const { dialog, submit } = await openDecision('Отклонить связь')
  expect(
    dialog.getByText('Кандидат уйдёт из оснований наблюдений задачи.'),
  ).toBeVisible()
  fireEvent.click(submit)
  expect(await screen.findByText('Связь с MR не найдена')).toBeVisible()
  expect(reject).toHaveBeenCalledWith(
    expect.objectContaining({ body: JSON.stringify({ decision: 'reject' }) }),
  )
  expect(
    screen.queryByRole('list', { name: 'Кандидаты связи' }),
  ).not.toBeInTheDocument()
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  expect(screen.queryByRole('link', { name: /!42/ })).not.toBeInTheDocument()
})

it('locks duplicate submits while one POST is pending', async () => {
  let respond: ((response: Response) => void) | undefined
  const decide = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        respond = resolve
      }),
  )
  mount({ decide })
  const { submit } = await openDecision()
  act(() => {
    submit.click()
    submit.click()
  })
  expect(decide).toHaveBeenCalledTimes(1)
  expect(submit).toBeDisabled()
  if (!respond) throw new Error('Expected the decision request to start')
  respond(problemResponse('forbidden', 403))
  expect(await screen.findByText(/Действие недоступно/)).toBeVisible()
})

it('rechecks the task after a lost response and replays the same key and body', async () => {
  const timeout = new AbortController()
  const clock = vi
    .spyOn(AbortSignal, 'timeout')
    .mockReturnValueOnce(timeout.signal)
  const requests: { key: string | null; body: BodyInit | null | undefined }[] =
    []
  const loadTask = vi.fn(() =>
    jsonResponse(
      requests.length ? { ...taskDetail, links: [confirmed] } : candidateTask,
    ),
  )
  mount({
    loadTask,
    decide: (request) => {
      requests.push({
        key: new Headers(request?.headers).get('Idempotency-Key'),
        body: request?.body,
      })
      if (requests.length > 1) return jsonResponse(confirmed)
      timeout.abort(new DOMException('Timed out', 'TimeoutError'))
      return Promise.reject(
        timeout.signal.reason instanceof Error
          ? timeout.signal.reason
          : new Error('Timed out'),
      )
    },
  })
  const { dialog, submit } = await openDecision()
  fireEvent.click(submit)
  const retry = await dialog.findByRole('button', { name: 'Повторить запрос' })
  expect(clock).toHaveBeenCalledWith(30_000)
  expect(loadTask).toHaveBeenCalledTimes(2)
  fireEvent.click(retry)
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  expect(requests).toHaveLength(2)
  expect(requests[0]?.key).toBeTruthy()
  expect(requests[1]).toEqual(requests[0])
  expect(loadTask).toHaveBeenCalledTimes(3)
  expect(
    within(
      screen.getByRole('list', { name: 'Подтверждённые связи' }),
    ).getAllByRole('listitem'),
  ).toHaveLength(1)
})

it('explains a source-confirmed conflict and reloads the authoritative link', async () => {
  const decide = vi.fn(() => problemResponse('state_conflict', 409))
  mount({
    loadTask: () =>
      jsonResponse(decide.mock.calls.length ? taskDetail : candidateTask),
    decide,
  })
  const { dialog, submit } = await openDecision('Отклонить связь')
  fireEvent.click(submit)
  expect(
    await dialog.findByText('Связь уже подтверждена источником.'),
  ).toBeVisible()
  expect(await dialog.findByRole('button', { name: 'Закрыть' })).toBeEnabled()
  fireEvent.click(dialog.getByRole('button', { name: 'Закрыть' }))
  expect(
    within(
      screen.getByRole('list', { name: 'Подтверждённые связи' }),
    ).getByText(/Явная связь в источнике/),
  ).toBeVisible()
  expect(
    screen.queryByRole('list', { name: 'Кандидаты связи' }),
  ).not.toBeInTheDocument()
})

it('keeps the decision dialog open when the refresh fails and retries only the read', async () => {
  const decide = vi.fn(() => jsonResponse(confirmed))
  const loadTask = vi
    .fn()
    .mockImplementationOnce(() => jsonResponse(candidateTask))
    .mockImplementationOnce(() => problemResponse('internal_error', 500))
    .mockImplementation(() =>
      jsonResponse({ ...taskDetail, links: [confirmed] }),
    )
  mount({ loadTask, decide })
  const { dialog, submit } = await openDecision()
  fireEvent.click(submit)
  const refresh = await dialog.findByRole('button', {
    name: 'Перечитать карточку',
  })
  expect(dialog.getByRole('alert')).toHaveTextContent(
    'Не удалось перечитать карточку',
  )
  fireEvent.click(refresh)
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  expect(
    screen.getByRole('list', { name: 'Подтверждённые связи' }),
  ).toBeVisible()
  expect(decide).toHaveBeenCalledTimes(1)
})

it('shows a forbidden decision and removes decision controls', async () => {
  mount({ decide: () => problemResponse('forbidden', 403) })
  const { dialog, submit } = await openDecision()
  fireEvent.click(submit)
  const alert = await dialog.findByRole('alert')
  expect(alert).toHaveTextContent('Действие недоступно')
  expect(alert).toHaveFocus()
  fireEvent.click(dialog.getByRole('button', { name: 'Закрыть' }))
  expect(
    screen.queryByRole('button', { name: /(?:Подтвердить|Отклонить) связь/ }),
  ).not.toBeInTheDocument()
})

it('redirects to UI-04 when the candidate is no longer accessible', async () => {
  const router = mount({ decide: () => problemResponse('not_found', 404) })
  const { submit } = await openDecision()
  fireEvent.click(submit)
  expect(
    await screen.findByRole('heading', {
      name: 'Объект или область недоступны',
    }),
  ).toBeVisible()
  expect(router.state.location.pathname).toBe('/access-denied')
  expect(screen.queryByText(taskTitle)).not.toBeInTheDocument()
})

it.each([
  {
    ...manager,
    actions: manager.actions.filter((action) => action !== 'links.decide'),
  },
  {
    ...manager,
    roles: manager.roles.map((role) => ({
      ...role,
      project_id: '0193a5b0-4444-7000-8000-000000000002',
    })),
  },
])('gates decisions by the action and the project scope', async (user) => {
  mount({ user })
  const candidates = within(
    await screen.findByRole('list', { name: 'Кандидаты связи' }),
  )
  expect(candidates.queryAllByRole('button')).toHaveLength(0)
})

it('renders the merged MR and unknown review state using the task project for UI-27', async () => {
  mount({
    loadTask: () => jsonResponse(taskDetail),
    loadMR: () =>
      jsonResponse({
        ...mergeRequest,
        state: 'merged',
      } satisfies Schema['MergeRequest']),
  })
  expect(await screen.findByText('Есть подтверждённый слитый MR')).toBeVisible()
  const mr = within(screen.getByRole('region', { name: 'Подтверждённые MR' }))
  expect(
    mr.getByText(/Состояние неизвестно: история MR неполная/),
  ).toBeVisible()
  expect(mr.getByText('Build Bot', { exact: false })).toBeVisible()
  expect(mr.getByText('Бот')).toBeVisible()
  expect(
    mr.getByRole('link', { name: '!42 · Перенос отчёта' }),
  ).toHaveAttribute('href', mrReference.source_url)
  expect(
    mr.getByRole('link', { name: 'Очередь ревью проекта' }),
  ).toHaveAttribute(
    'href',
    taskAnalysisLinks.mergeRequestQueue({
      project_id: taskDetail.project_id ?? '',
      team_id: taskDetail.team_id ?? '',
    }),
  )
})

it('shows paginated attempts by configuration, fail-to-pass, missing reports and the lower usage bound', async () => {
  mount({
    loadTask: () =>
      jsonResponse({ ...taskDetail, links: [...taskDetail.links, runLink] }),
    loadAttempts: (cursor) => ({
      data:
        cursor === 'retry'
          ? [successfulRetry]
          : [failedAttempt, otherConfiguration],
      next_cursor: cursor === 'retry' ? null : 'retry',
      usage_seconds: usageSeconds,
    }),
  })
  const ci = within(
    await screen.findByRole('region', { name: 'CI текущего head' }),
  )
  const pipelineLink = await ci.findByRole('link', { name: 'Pipeline #8841' })
  expect(pipelineLink).toHaveAttribute('href', pipeline.source_url)
  pipelineLink.focus()
  expect(pipelineLink).toHaveFocus()
  expect(ci.getByText('Неполная')).toBeVisible()
  expect(ci.getByText(/Не получен/)).toBeVisible()
  expect(await ci.findByText(/Не менее/)).toBeVisible()
  expect(ci.getByText(/Причина неизвестна/)).toBeVisible()
  expect(ci.queryByText('Сбой → успех после повтора')).not.toBeInTheDocument()
  fireEvent.click(ci.getByRole('button', { name: 'Загрузить ещё попытки' }))
  expect(await ci.findByText('Сбой → успех после повтора')).toBeVisible()
  expect(ci.getByText(failedAttempt.config_fingerprint)).toBeVisible()
  expect(ci.getByText(otherConfiguration.config_fingerprint)).toBeVisible()
  const jobs = ci.getAllByRole('link', { name: /Попытка \d/ })
  expect(jobs.map((link) => link.getAttribute('href'))).toEqual([
    failedAttempt.source_url,
    successfulRetry.source_url,
    otherConfiguration.source_url,
  ])
  for (const link of jobs) {
    link.focus()
    expect(link).toHaveFocus()
  }
})

it('excludes a linked pipeline for an older head without requesting its attempts', async () => {
  const loadPipeline = vi.fn(() =>
    jsonResponse({ ...pipeline, sha: 'b'.repeat(40) }),
  )
  const loadAttempts = vi.fn(() => ({
    data: [],
    next_cursor: null,
    usage_seconds: usageSeconds,
  }))
  mount({
    loadTask: () =>
      jsonResponse({ ...taskDetail, links: [...taskDetail.links, runLink] }),
    loadPipeline,
    loadAttempts,
  })
  await waitFor(() => {
    expect(loadPipeline).toHaveBeenCalledTimes(1)
  })
  const ci = within(screen.getByRole('region', { name: 'CI текущего head' }))
  expect(await ci.findByText(/Нет доступных запусков CI/)).toBeVisible()
  expect(
    ci.queryByRole('link', { name: 'Pipeline #8841' }),
  ).not.toBeInTheDocument()
  expect(loadAttempts).not.toHaveBeenCalled()
})

it('waits for the MR head before deciding which linked CI runs to show', async () => {
  const loadMR = vi
    .fn<() => Response>()
    .mockImplementationOnce(() => problemResponse('internal_error', 500))
    .mockImplementation(() => jsonResponse(mergeRequest))
  const loadPipeline = vi.fn(() => jsonResponse(pipeline))
  mount({
    loadTask: () =>
      jsonResponse({ ...taskDetail, links: [...taskDetail.links, runLink] }),
    loadMR,
    loadPipeline,
    loadAttempts: () => ({
      data: [],
      next_cursor: null,
      usage_seconds: usageSeconds,
    }),
  })
  const ci = within(
    await screen.findByRole('region', { name: 'CI текущего head' }),
  )
  expect(await ci.findByText(/пока head MR недоступен/)).toBeVisible()
  expect(ci.queryByText(/Нет доступных запусков CI/)).not.toBeInTheDocument()
  expect(loadPipeline).not.toHaveBeenCalled()

  const mr = within(screen.getByRole('region', { name: 'Подтверждённые MR' }))
  fireEvent.click(mr.getByRole('button', { name: 'Повторить' }))
  expect(await ci.findByRole('link', { name: 'Pipeline #8841' })).toBeVisible()
  expect(ci.queryByText(/пока head MR недоступен/)).not.toBeInTheDocument()
})
