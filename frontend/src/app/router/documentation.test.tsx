import {
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
import {
  acceptedCheck,
  anchorDetail,
  anchorFinding,
  documentPage,
  emptyCorpus,
  owner,
  root,
  service,
  specificationDetail,
  specificationFinding,
} from '../../../test/documentation-fixtures'
import {
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
const rootsPath = `/documentation/roots/${service.id}`
const corpusPath = `/documentation/services/${service.id}`
const findingPath = (id: string) =>
  `/documentation/findings/${id}?service_id=${service.id}`
const serviceApi = `/api/v1/services/${service.id}`
const rootsApi = `${serviceApi}/documentation-roots`
const documentsApi = `${serviceApi}/documents`
const checksApi = `${serviceApi}/documentation-checks`
const operationApi = `/api/v1/operations/${acceptedCheck.id}`

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
const cleanup: (() => void)[] = []
afterEach(() => {
  for (const dispose of cleanup.splice(0)) dispose()
})

function mount(
  path: string,
  override?: (
    url: URL,
    request: RequestInit | undefined,
  ) => Response | undefined,
) {
  const fetcher = vi.fn<typeof fetch>((input, request) => {
    const url = new URL(
      input instanceof Request ? input.url : input,
      'https://craftpulse.test',
    )
    const response = override?.(url, request)
    if (response) return Promise.resolve(response)

    switch (url.pathname) {
      case '/api/v1/me':
        return Promise.resolve(jsonResponse(owner))
      case '/api/v1/teams':
        return Promise.resolve(jsonResponse(teams))
      case '/api/v1/projects':
        return Promise.resolve(jsonResponse(projects))
      case rootsApi:
        return Promise.resolve(
          jsonResponse(
            { data: [root] } satisfies Schema['DocumentationRootList'],
            200,
            { ETag: '"1"' },
          ),
        )
      case documentsApi:
        return Promise.resolve(jsonResponse(documentPage))
      case '/api/v1/findings': {
        const finding =
          url.searchParams.get('finding_kind') === 'docs'
            ? anchorFinding
            : specificationFinding
        expect(url.searchParams.get('service_id')).toBe(service.id)
        return Promise.resolve(
          jsonResponse({
            data: [finding],
            next_cursor: null,
            access_limited: false,
            access_notes: [],
          } satisfies Schema['FindingPage']),
        )
      }
      case `/api/v1/findings/${anchorDetail.id}`:
        return Promise.resolve(jsonResponse(anchorDetail))
      case `/api/v1/findings/${specificationDetail.id}`:
        return Promise.resolve(jsonResponse(specificationDetail))
      default:
        throw new Error(`Unexpected request: ${url.pathname}`)
    }
  })
  const runtime = createAppRuntime(fetcher)
  const router = createMemoryRouter(createAppRoutes(runtime), {
    initialEntries: [path],
  })
  const view = render(
    <AppProviders runtime={runtime}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  cleanup.push(() => {
    view.unmount()
    router.dispose()
    runtime.session.dispose()
    runtime.queryClient.clear()
  })
  return fetcher
}

it('показывает неизвестное состояние сервиса без корней', async () => {
  mount(corpusPath, (url) =>
    url.pathname === documentsApi ? jsonResponse(emptyCorpus) : undefined,
  )
  expect(await screen.findByText('Документация не подключена.')).toBeVisible()
  expect(screen.getByText('неизвестно')).toBeVisible()
  expect(screen.queryByText('0%')).not.toBeInTheDocument()
})

it('подписывает корень без владельца', async () => {
  mount(rootsPath)
  expect(await screen.findByText('Владелец не назначен')).toBeVisible()
  expect(screen.getByLabelText(/Репозиторий/)).toHaveValue(root.area_id)
})

it('открывает битый якорь из корпуса и показывает исходный файл и цель', async () => {
  mount(corpusPath)
  expect(
    await screen.findByRole('heading', { name: 'Руководство Billing' }),
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Открыть документ в GitLab' }),
  ).toHaveAttribute('href', documentPage.data[0]?.source_url)
  fireEvent.click(await screen.findByRole('link', { name: /missing-anchor/ }))
  expect(await screen.findByText('#install')).toBeVisible()
  expect(screen.getByText('docs/README.md:12')).toBeVisible()
  expect(screen.getByText('docs/guide.md')).toBeVisible()
})

it('показывает файл спецификации и правило OpenAPI', async () => {
  mount(findingPath(specificationDetail.id))
  expect(await screen.findByText('oas3-schema')).toBeVisible()
  expect(screen.getByText('docs/openapi.yaml:8')).toBeVisible()
  expect(screen.getByText('Невалидная схема ответа')).toBeVisible()
})

it('различает сетевой отказ и подтверждённое отсутствие якоря', async () => {
  mount(findingPath(anchorDetail.id), (url) =>
    url.pathname === `/api/v1/findings/${anchorDetail.id}`
      ? jsonResponse({
          ...anchorDetail,
          evidence: { ...anchorDetail.evidence, http_status: '429' },
        } satisfies Schema['FindingDetail'])
      : undefined,
  )
  expect(
    await screen.findByText(
      'Цель не проверена из-за ошибки сети или ограничения доступа.',
    ),
  ).toBeVisible()
  expect(
    screen.queryByText('В целевом файле отсутствует якорь.'),
  ).not.toBeInTheDocument()
})

it('ожидает pipeline после 202 и обновляет нарушение по принятому отчёту', async () => {
  let reports = 0
  let resolved = false
  const fetcher = mount(findingPath(anchorDetail.id), (url, request) => {
    if (url.pathname === checksApi && request?.method === 'POST') {
      expect(request.body).toBe(JSON.stringify({ root_id: root.id }))
      return jsonResponse(acceptedCheck, 202, { Location: operationApi })
    }
    if (url.pathname === operationApi) {
      resolved = true
      return jsonResponse({
        ...acceptedCheck,
        status: 'succeeded',
        retry_after_seconds: null,
        result_uri: documentsApi,
      } satisfies Schema['BackgroundOperation'])
    }
    if (url.pathname === `/api/v1/findings/${anchorDetail.id}`) {
      reports += 1
      return jsonResponse(
        resolved
          ? ({
              ...anchorDetail,
              state: 'resolved',
              report: {
                ...anchorDetail.report,
                id: '0193a5b0-d4d4-7000-8000-000000000012',
                sha: '2'.repeat(40),
              },
            } satisfies Schema['FindingDetail'])
          : anchorDetail,
      )
    }
    return undefined
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Проверить снова' }),
  )
  expect(
    await screen.findByText(
      'Запрос принят. Ожидаем отчёт следующего pipeline.',
    ),
  ).toBeVisible()
  expect(screen.getByText('Состояние: Открыта')).toBeVisible()
  expect(fetcher).toHaveBeenCalledWith(
    checksApi,
    expect.objectContaining({ method: 'POST' }),
  )
  expect(
    await screen.findByText('Состояние: Исправлена', {}, { timeout: 4000 }),
  ).toBeVisible()
  expect(reports).toBeGreaterThan(1)
})

it('оставляет нарушение открытым после публикации новой версии документа', async () => {
  let newVersion = false
  mount(findingPath(anchorDetail.id), (url) => {
    if (url.pathname !== `/api/v1/findings/${anchorDetail.id}`) return undefined
    return jsonResponse(
      newVersion
        ? ({
            ...anchorDetail,
            report: { ...anchorDetail.report, sha: '3'.repeat(40) },
          } satisfies Schema['FindingDetail'])
        : anchorDetail,
    )
  })
  expect(await screen.findByText('Состояние: Открыта')).toBeVisible()
  newVersion = true
  fireEvent.click(screen.getByRole('button', { name: 'Обновить нарушение' }))
  expect(await screen.findByLabelText('3'.repeat(40))).toBeVisible()
  expect(screen.getByText('Состояние: Открыта')).toBeVisible()
})

it('показывает актуальный набор после 412 и требует повторного подтверждения', async () => {
  const writes: Headers[] = []
  const latest = { ...root, path_prefix: 'docs/changed-by-teammate' }
  mount(rootsPath, (url, request) => {
    if (
      url.pathname !== rootsApi ||
      (!writes.length && request?.method !== 'PUT')
    )
      return undefined
    if (request?.method !== 'PUT')
      return jsonResponse(
        { data: [latest] } satisfies Schema['DocumentationRootList'],
        200,
        { ETag: '"2"' },
      )
    writes.push(new Headers(request.headers))
    if (writes.length === 1) return problemResponse('version_conflict', 412)
    expect(request.body).toBe(
      JSON.stringify({
        data: [
          {
            area_id: root.area_id,
            path_prefix: 'docs/new',
            profile_id: root.profile_id,
            owner_person_id: null,
            audience: root.audience,
          },
        ],
      } satisfies Schema['DocumentationRootListInput']),
    )
    return jsonResponse(
      { data: [{ ...latest, path_prefix: 'docs/new' }] },
      200,
      { ETag: '"3"' },
    )
  })
  fireEvent.change(await screen.findByLabelText('Путь в репозитории'), {
    target: { value: 'docs/new' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить набор' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Подтверждение корней документации',
  })
  expect(writes).toHaveLength(0)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await within(dialog).findByText(
      'Данные изменились. Просмотрите актуальную версию',
    ),
  ).toBeVisible()
  expect(within(dialog).getByText('"2"')).toBeVisible()
  expect(within(dialog).getByText(/docs\/changed-by-teammate/)).toBeVisible()
  expect(writes).toHaveLength(1)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(
    await screen.findByRole('heading', { name: 'Документация сервиса' }),
  ).toBeVisible()
  expect(writes.map((headers) => headers.get('If-Match'))).toEqual([
    '"1"',
    '"2"',
  ])
  expect(writes[0]?.get('Idempotency-Key')).toMatch(/^[0-9a-f-]{36}$/u)
  expect(writes[1]?.get('Idempotency-Key')).not.toBe(
    writes[0]?.get('Idempotency-Key'),
  )
})

it('показывает причину 422 и сохраняет черновик под защитой UI-18', async () => {
  mount(rootsPath, (url, request) => {
    if (url.pathname !== rootsApi || request?.method !== 'PUT') return undefined
    return jsonResponse(
      {
        ...problem('validation_failed', 422),
        errors: [{ field: '/data/0/area_id', code: 'outside_scope' }],
      },
      422,
      { 'Content-Type': 'application/problem+json' },
    )
  })
  const path = await screen.findByLabelText('Путь в репозитории')
  fireEvent.change(path, { target: { value: 'docs/draft' } })
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить набор' }))
  const confirmation = await screen.findByRole('dialog', {
    name: 'Подтверждение корней документации',
  })
  fireEvent.click(
    within(confirmation).getByRole('button', { name: 'Подтвердить' }),
  )
  expect(await screen.findByText('Корень вне вашей области.')).toBeVisible()
  expect(path).toHaveValue('docs/draft')
  fireEvent.click(screen.getByRole('link', { name: 'К документам сервиса' }))
  const guard = await screen.findByRole('dialog', {
    name: 'Изменения не сохранены',
  })
  fireEvent.click(
    within(guard).getByRole('button', { name: 'Продолжить редактирование' }),
  )
  await waitFor(() => {
    expect(guard).not.toBeInTheDocument()
  })
  expect(path).toHaveValue('docs/draft')
})

it('удаляет последний корень только после подтверждения всего набора', async () => {
  let saved = false
  mount(rootsPath, (url, request) => {
    if (url.pathname === documentsApi && saved) return jsonResponse(emptyCorpus)
    if (url.pathname !== rootsApi || request?.method !== 'PUT') return undefined
    expect(request.body).toBe(
      JSON.stringify({
        data: [],
      } satisfies Schema['DocumentationRootListInput']),
    )
    saved = true
    return jsonResponse(
      { data: [] } satisfies Schema['DocumentationRootList'],
      200,
      { ETag: '"2"' },
    )
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Удалить корень 1' }),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Сохранить набор' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Подтверждение корней документации',
  })
  expect(within(dialog).getByText(/Удаление ·/u)).toBeVisible()
  expect(saved).toBe(false)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Подтвердить' }))
  expect(await screen.findByText('Документация не подключена.')).toBeVisible()
  expect(saved).toBe(true)
})
