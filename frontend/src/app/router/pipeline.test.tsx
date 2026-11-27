import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { createBrowserRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { afterEach, expect, it, vi } from 'vitest'
import type { components } from '@/shared/api'
import {
  currentUser,
  jsonResponse,
  problemResponse,
  teams,
} from '../../../test/fixtures'
import {
  executionRows,
  jobKey,
  linux,
  noReports,
  pipelineId,
  v28,
  v28Attempts,
  windows,
} from '../../../test/pipeline-fixtures'
import { AppProviders } from '../initialization/app-providers'
import { createAppRuntime } from '../initialization/runtime'
import { createAppRoutes } from './routes'

type Schema = components['schemas']
const path = `/pipelines/${pipelineId}`
const job = `${path}/jobs/test:%20matrix%2Fbuild`
const originalUrl = window.location.href
const originalState: unknown = window.history.state
const apiPath = `/api/v1${path}`
const cleanups: (() => void)[] = []

function mount({
  initial = path,
  pipeline = v28,
  attempts = () => jsonResponse(v28Attempts),
  executions = (params) => {
    const outcome = params.get('outcome')
    return jsonResponse({
      data: outcome
        ? executionRows.filter((row) => row.outcome === outcome)
        : executionRows,
      next_cursor: null,
    } satisfies Schema['TestExecutionPage'])
  },
}: {
  initial?: string
  pipeline?: Schema['Pipeline']
  attempts?: (params: URLSearchParams) => Response
  executions?: (params: URLSearchParams) => Response
} = {}) {
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input) => {
    if (typeof input !== 'string') throw new Error('Expected a request URL')
    const url = new URL(input, 'https://craftpulse.test')
    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(
          jsonResponse({
            ...currentUser,
            actions: ['session.read', 'ci.read', 'mr.read'],
          }),
        )
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case apiPath:
        return Promise.resolve(jsonResponse(pipeline))
      case apiPath + '/attempts':
        return Promise.resolve(attempts(url.searchParams))
      case apiPath + '/test-executions':
        return Promise.resolve(executions(url.searchParams))
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
  vi.spyOn(window, 'scrollTo').mockReturnValue(undefined)
  window.history.replaceState(null, '', initial)
  const runtime = createAppRuntime(fetcher)
  const router = createBrowserRouter(createAppRoutes(runtime))
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
  return { router, fetcher }
}

afterEach(() => {
  for (const clean of cleanups.splice(0)) clean()
  window.history.replaceState(originalState, '', originalUrl)
  localStorage.clear()
})

// jsdom does not synthesize a click on Enter. Dispatch its keyboard click
// after moving focus to the control.
function activateFromKeyboard(control: HTMLElement) {
  control.focus()
  expect(control).toHaveFocus()
  fireEvent.keyDown(control, { key: 'Enter' })
  fireEvent.click(control, { detail: 0 })
  fireEvent.keyUp(control, { key: 'Enter' })
}

async function jobFor(config: string) {
  const table = await screen.findByRole('table', { name: 'Задания CI' })
  const row = within(table).getByText(config).closest('tr')
  if (!row) throw new Error('Expected a matrix job row')
  return { row, link: within(row).getByRole('link', { name: jobKey }) }
}

it('показывает V-28 с расходом 130 с, повтором 30 с и отдельными исходами matrix', async () => {
  mount()
  const failedJob = await jobFor(linux)
  const parallelJob = await jobFor(windows)
  expect(
    screen.getByText('Известные попытки').nextElementSibling,
  ).toHaveTextContent('3')
  expect(
    screen.getByText('Общий расход CI').nextElementSibling,
  ).toHaveTextContent(/^130 с$/u)
  expect(
    screen.getByText('Время повторов').nextElementSibling,
  ).toHaveTextContent(/^30 с$/u)
  expect(
    screen.getByText('Статус в источнике').nextElementSibling,
  ).toHaveTextContent('passed')
  expect(screen.getByText(v28.sha)).toBeVisible()
  expect(
    screen.getByRole('link', { name: v28.merge_request_id ?? '' }),
  ).toHaveAttribute(
    'href',
    `/merge-requests/${v28.merge_request_id ?? ''}/summary?head_sha=${v28.sha}`,
  )
  expect(
    screen.getByRole('link', { name: 'Открыть CI в GitLab' }),
  ).toHaveAttribute('href', v28.source_url)
  expect(within(failedJob.row).getAllByRole('cell').slice(1, 3)).toEqual([
    expect.toHaveTextContent('Сбой'),
    expect.toHaveTextContent('Успешно'),
  ])
  expect(
    within(failedJob.row).getByText('Сбой → успех после повтора'),
  ).toBeVisible()
  expect(within(parallelJob.row).getAllByRole('cell').slice(1, 3)).toEqual([
    expect.toHaveTextContent('Успешно'),
    expect.toHaveTextContent('Успешно'),
  ])
  expect(
    within(parallelJob.row).queryByText('Сбой → успех после повтора'),
  ).not.toBeInTheDocument()
})

const childPipelineId = '0193a5b0-d1d1-7000-8000-000000000002'

it('разделяет одноимённые задания родительского и дочернего pipeline при переходе к попыткам', async () => {
  const search = '?org_slug=example&test_outcome=failed'
  const data = v28Attempts.data
    .filter(({ attempt_no }) => attempt_no === 1)
    .map((attempt) => ({
      ...attempt,
      pipeline_id:
        attempt.config_fingerprint === linux ? pipelineId : childPipelineId,
      config_fingerprint: linux,
    }))
  mount({
    initial: path + search,
    pipeline: { ...v28, attempts_total: 2, retried_jobs: 0 },
    attempts: () =>
      jsonResponse({
        ...v28Attempts,
        data,
        usage_seconds: { ...v28Attempts.usage_seconds, value: 100, n: 2 },
      } satisfies Schema['AttemptPage']),
  })

  for (const { origin, outcome, usage } of [
    { origin: 'Текущий pipeline', outcome: 'Сбой', usage: '60 с' },
    { origin: childPipelineId, outcome: 'Успешно', usage: '40 с' },
  ]) {
    const jobs = await screen.findByRole('table', { name: 'Задания CI' })
    expect(within(jobs).getAllByRole('rowheader')).toHaveLength(2)
    const row = within(jobs).getByText(origin).closest('tr')
    if (!row) throw new Error('Expected a job row')
    expect(within(row).getAllByText(outcome)).toHaveLength(2)
    expect(
      within(row).queryByText('Сбой → успех после повтора'),
    ).not.toBeInTheDocument()
    activateFromKeyboard(within(row).getByRole('link', { name: jobKey }))

    const table = await screen.findByRole('table', { name: 'Попытки задания' })
    expect(within(table).getAllByRole('rowheader')).toHaveLength(1)
    expect(within(table).getByText(outcome)).toBeVisible()
    expect(
      screen.getByText('Расход задания').nextElementSibling?.textContent,
    ).toBe(usage)

    activateFromKeyboard(
      screen.getByRole('button', { name: 'Назад к pipeline' }),
    )
    await waitFor(() => {
      const returnedRow = screen.getByText(origin).closest('tr')
      if (!returnedRow) throw new Error('Expected a job row on return')
      expect(
        within(returnedRow).getByRole('link', { name: jobKey }),
      ).toHaveFocus()
    })
    expect(window.location.search).toBe(search)
  }
})

it.each([
  ['V-28', v28, linux, 'failed'],
  [
    'pipeline без отчётов',
    { ...noReports, attempts_total: 3, retried_jobs: 1 },
    windows,
    '',
  ],
] as const)(
  'возвращает фокус к исходной конфигурации для %s',
  async (_, pipeline, config, outcome) => {
    const search = outcome
      ? `?org_slug=example&test_outcome=${outcome}`
      : '?org_slug=example'
    const { router } = mount({ initial: path + search, pipeline })
    const { link } = await jobFor(config)
    activateFromKeyboard(link)

    const title = await screen.findByRole('heading', {
      level: 1,
      name: 'Попытки задания CI',
    })
    expect(title).toHaveFocus()
    expect(router.state.location.pathname).toBe(job)
    expect(
      new URLSearchParams(router.state.location.search).get(
        'config_fingerprint',
      ),
    ).toBe(config)
    const table = await screen.findByRole('table', { name: 'Попытки задания' })
    expect(screen.getByText(config)).toBeVisible()
    expect(
      screen.queryByText(config === linux ? windows : linux),
    ).not.toBeInTheDocument()
    expect(
      within(table)
        .getAllByRole('rowheader')
        .map(({ textContent }) => textContent),
    ).toEqual(config === linux ? ['№ 1', '№ 2'] : ['№ 1'])
    if (config === linux) {
      expect(within(table).getByText('Причина неизвестна')).toBeVisible()
      expect(within(table).getAllByText('linux-medium')).toHaveLength(2)
      expect(
        screen.getByText('Расход задания').nextElementSibling,
      ).toHaveTextContent('90 с')
    }

    activateFromKeyboard(
      screen.getByRole('button', { name: 'Назад к pipeline' }),
    )
    const returned = await jobFor(config)
    await waitFor(() => {
      expect(returned.link).toHaveFocus()
    })
    expect(router.state.location.search).toBe(search)
    if (outcome)
      expect(screen.getByRole('combobox', { name: 'Исход теста' })).toHaveValue(
        outcome,
      )
  },
)

it('не подставляет процент и нули вместо отсутствующих JUnit и покрытия', async () => {
  const { fetcher } = mount({ pipeline: noReports })
  await screen.findByText('Покрытие строк')
  expect(screen.getAllByText('Не измерено')).toHaveLength(2)
  expect(screen.getByText('Прошли').nextElementSibling).toHaveTextContent(
    'Нет данных',
  )
  expect(
    screen.queryByText('Доля успешных без пропущенных'),
  ).not.toBeInTheDocument()
  expect(
    fetcher.mock.calls.some(
      ([url]) => typeof url === 'string' && url.includes('/test-executions'),
    ),
  ).toBe(false)
})

it('показывает состояние I19 и измеряет строки и ветви отдельно', async () => {
  mount()
  expect(
    (await screen.findByText('Уникальные тесты')).nextElementSibling,
  ).toHaveTextContent('4')
  expect(screen.getByText('Выполнения').nextElementSibling).toHaveTextContent(
    '5',
  )
  expect(
    screen.getByText('Ошибки', { selector: 'dt' }).nextElementSibling,
  ).toHaveTextContent('1')
  expect(
    screen.getByText('Пропущены', { selector: 'dt' }).nextElementSibling,
  ).toHaveTextContent('1')
  expect(
    screen.getByText('Доля успешных без пропущенных').nextElementSibling,
  ).toHaveTextContent(/50\s*%/u)
  expect(
    screen.getByText('Покрытие строк').nextElementSibling,
  ).toHaveTextContent(/50\s*%/u)
  expect(
    screen.getByText('Покрытие ветвей').nextElementSibling,
  ).toHaveTextContent(/25\s*%/u)
})

it('показывает пропущенный отчёт, повреждённый JUnit и разрешённое падение при успехе', async () => {
  mount({
    pipeline: {
      ...noReports,
      attempts_total: 1,
      allowed_failures: 1,
      reports: [
        { analysis_kind: 'junit', state: 'malformed' },
        { analysis_kind: 'coverage', state: 'missing' },
        { analysis_kind: 'code', state: 'partial' },
      ],
      tests: { ...noReports.tests, state: 'UNKNOWN' },
    },
    attempts: () =>
      jsonResponse({
        ...v28Attempts,
        data: v28Attempts.data
          .filter(({ outcome }) => outcome === 'failed')
          .map((attempt) => ({ ...attempt, allow_failure: true })),
      } satisfies Schema['AttemptPage']),
  })
  const { row } = await jobFor(linux)
  expect(
    screen.getByText('Разрешённые падения').nextElementSibling,
  ).toHaveTextContent('1')
  expect(within(row).getByText('Разрешённое падение')).toBeVisible()
  expect(
    screen.getByText('Отчёт о тестах JUnit').nextElementSibling,
  ).toHaveTextContent('повреждён')
  expect(
    screen.getByText('Покрытие тестами').nextElementSibling,
  ).toHaveTextContent('Не получен')
  expect(screen.getByText('Анализ кода').nextElementSibling).toHaveTextContent(
    'Частичный',
  )
  expect(
    screen.queryByText('Доля успешных без пропущенных'),
  ).not.toBeInTheDocument()
  expect(screen.getAllByText('Успешно')).toHaveLength(1)
})

it('показывает нижнюю границу при усечённой истории на обоих экранах', async () => {
  mount({
    pipeline: { ...v28, attempt_history: 'truncated' },
    attempts: () =>
      jsonResponse({
        ...v28Attempts,
        usage_seconds: {
          ...v28Attempts.usage_seconds,
          state: 'BOUNDED',
          value: null,
          bound: { kind: 'lower', value: 130 },
          reason_code: 'history_incomplete',
        },
      } satisfies Schema['AttemptPage']),
  })
  const { link } = await jobFor(linux)
  expect(screen.getByText('Известны не все повторы')).toBeVisible()
  expect(
    screen.getByText('Общий расход CI').nextElementSibling,
  ).toHaveTextContent('Не менее 130 с')
  expect(
    screen.getByRole('columnheader', { name: 'Последний известный исход' }),
  ).toBeVisible()
  fireEvent.click(link)
  expect(
    (await screen.findByText('Расход задания')).nextElementSibling,
  ).toHaveTextContent('Не менее 90 с')
  expect(
    screen.getByText('Время повторов').nextElementSibling,
  ).toHaveTextContent('Не менее 30 с')
})

it('сохраняет два выполнения одного теста и применяет фильтр на API', async () => {
  const { fetcher } = mount()
  const table = await screen.findByRole('table', { name: 'Выполнения тестов' })
  expect(
    within(table).getAllByRole('rowheader', { name: 'billing.TestExport/csv' }),
  ).toHaveLength(2)
  expect(within(table).getByText(linux)).toBeVisible()
  expect(within(table).getByText(windows)).toBeVisible()
  expect(within(table).getAllByText('1840 мс')).toHaveLength(2)
  activateFromKeyboard(
    screen.getByRole('link', { name: 'Перейти к выполнениям тестов' }),
  )
  expect(
    screen.getByRole('heading', { name: 'Выполнения тестов' }),
  ).toHaveFocus()
  fireEvent.change(screen.getByRole('combobox', { name: 'Исход теста' }), {
    target: { value: 'failed' },
  })
  const filtered = await screen.findByRole('table', {
    name: 'Выполнения тестов',
  })
  expect(within(filtered).getAllByRole('rowheader')).toHaveLength(1)
  expect(within(filtered).queryByText(windows)).not.toBeInTheDocument()
  expect(fetcher.mock.calls.map(([url]) => url)).toContain(
    apiPath + '/test-executions?limit=100&outcome=failed',
  )
})

it('догружает попытки по курсору после сбоя и сохраняет уже показанные строки', async () => {
  let failNext = true
  mount({
    initial: `${job}?config_fingerprint=${linux}&job_pipeline_id=${pipelineId}&test_outcome=failed`,
    attempts: (params) => {
      if (!params.has('cursor'))
        return jsonResponse({
          ...v28Attempts,
          data: v28Attempts.data.slice(0, 1),
          next_cursor: 'next+1',
        })
      if (failNext) {
        failNext = false
        return problemResponse('internal_error', 500)
      }
      return jsonResponse({ ...v28Attempts, data: v28Attempts.data.slice(1) })
    },
  })
  const table = await screen.findByRole('table', { name: 'Попытки задания' })
  expect(within(table).getAllByRole('rowheader')).toHaveLength(1)
  expect(
    screen.getByText('Расход задания').nextElementSibling,
  ).toHaveTextContent('Не менее 30 с')
  fireEvent.click(screen.getByRole('button', { name: 'Загрузить ещё попытки' }))
  const retry = await screen.findByRole('button', { name: 'Повторить' })
  expect(within(table).getAllByRole('rowheader')).toHaveLength(1)
  fireEvent.click(retry)
  await waitFor(() => {
    expect(within(table).getAllByRole('rowheader')).toHaveLength(2)
  })
  expect(
    screen.getByText('Расход задания').nextElementSibling,
  ).toHaveTextContent(/^90 с$/u)
  expect(
    screen.getByRole('link', { name: 'Назад к pipeline' }),
  ).toHaveAttribute('href', path + '?test_outcome=failed')
})
