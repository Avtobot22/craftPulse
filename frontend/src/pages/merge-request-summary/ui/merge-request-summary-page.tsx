import { useQuery } from '@tanstack/react-query'
import { useParams, useSearchParams } from 'react-router'
import { ApiError, InvalidResponse } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'
import { publicationLabels, readSummary } from '../model/summary'

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu

export function MergeRequestSummaryPage({ api }: { api: ApiClient }) {
  const { mr = '' } = useParams()
  const [params] = useSearchParams()
  const raw = params.get('revision_no')
  const revision = raw === null ? undefined : Number(raw)
  if (
    !uuid.test(mr) ||
    (raw !== null && (!/^[1-9][0-9]*$/u.test(raw) || Number(raw) > 2147483647))
  ) {
    return <p role="alert">Некорректная ссылка на сводку MR</p>
  }
  return <SummaryContent api={api} mr={mr.toLowerCase()} revision={revision} />
}

function SummaryContent({
  api,
  mr,
  revision,
}: {
  api: ApiClient
  mr: string
  revision: number | undefined
}) {
  const query = useQuery({
    queryKey: ['merge-request-summary', mr, revision],
    queryFn: async ({ signal }) => {
      const { data } = await api.read(
        '/merge-requests/{merge_request_id}/summary',
        readSummary,
        {
          signal,
          pathParams: { merge_request_id: mr },
          query: { revision_no: revision },
        },
      )
      if (
        data.mr !== mr ||
        (revision !== undefined && data.revision !== revision)
      )
        throw new InvalidResponse()
      return data
    },
    retry: false,
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    const unavailable =
      query.error instanceof ApiError &&
      ['not-found', 'forbidden'].includes(query.error.kind)
    return (
      <ErrorState
        reason={
          unavailable
            ? 'Ревизия сводки недоступна'
            : 'Не удалось загрузить сводку MR'
        }
        retrying={query.isFetching}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  const s = query.data
  const publication =
    Object.entries(publicationLabels).find(
      ([status]) => status === s.publication.status,
    )?.[1] ?? s.publication.status
  return (
    <article>
      <h1>Сводка MR, ревизия {s.revision}</h1>
      <p>
        Head <code>{s.head}</code>
      </p>
      {s.base && (
        <p>
          База <code>{s.base}</code>
        </p>
      )}
      <p>
        Состояние сводки {s.status}
        {s.staleReason && ` (${s.staleReason})`}
      </p>
      {s.builtAt && (
        <p>
          Собрана <time dateTime={s.builtAt}>{s.builtAt}</time>
        </p>
      )}
      <h2>Комментарий в MR</h2>
      <p>{publication}</p>
      {s.publication.error === 'write_permission_missing' ? (
        <p>У бота нет права записи в MR.</p>
      ) : (
        s.publication.error && <p>{s.publication.error}</p>
      )}
      {s.publication.publishedAt && (
        <p>
          Опубликован{' '}
          <time dateTime={s.publication.publishedAt}>
            {s.publication.publishedAt}
          </time>
        </p>
      )}
      {s.publication.externalId && (
        <p>
          ID комментария <code>{s.publication.externalId}</code>
        </p>
      )}
      <h2>Проверки</h2>
      <p>Состояние CI {s.ci?.outcome ?? s.pipelineState}</p>
      {s.ci && (
        <p>
          Попыток {s.ci.attempts}, заданий с повтором {s.ci.retried}
        </p>
      )}
      <p>Покрытие изменённых строк {metric(s.coverage, true)}</p>
      {s.tests && (
        <p>
          Тесты: прошло {metric(s.tests.passed)}, упало {metric(s.tests.failed)}
          , ошибок {metric(s.tests.errored)}, пропущено{' '}
          {metric(s.tests.skipped)}
        </p>
      )}
      {s.change && (
        <p>
          Файлов {metric(s.change.files)}, строк добавлено{' '}
          {metric(s.change.added)}, удалено {metric(s.change.removed)}, функций
          с ростом сложности {metric(s.change.complexity)}, новых групп клонов{' '}
          {metric(s.change.clones)}
        </p>
      )}
      {s.missing.length > 0 && (
        <section>
          <h2>Непроверенные области</h2>
          <ul>
            {s.missing.map(({ kind, reason }, i) => (
              <li key={i}>
                {kind} ({reason})
              </li>
            ))}
          </ul>
        </section>
      )}
      <h2>Находки</h2>
      {s.restricted && <p>Есть закрытые результаты.</p>}
      {s.signals.length > 0 && (
        <ul>
          {s.signals.map((item) => (
            <li
              key={item.no}
              id={
                item.kind === 'new' && item.findingId
                  ? `finding-${item.findingId}`
                  : undefined
              }
            >
              {item.rank !== null && `№${String(item.rank)} · `}
              {item.kind}
              {item.rule && (
                <>
                  {' · '}
                  <code>{item.rule}</code>
                </>
              )}
              {item.severity && ` · ${item.severity}`}
              {item.path && (
                <>
                  {' · '}
                  <code>
                    {item.path}
                    {item.line !== null && `:${String(item.line)}`}
                  </code>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
      {s.signals.length === 0 && <p>Нет доступных находок.</p>}
    </article>
  )
}

function metric(
  point: ReturnType<typeof readSummary>['coverage'],
  ratio = false,
) {
  if (point.value !== null)
    return ratio
      ? new Intl.NumberFormat('ru', {
          style: 'percent',
          maximumFractionDigits: 2,
        }).format(point.value)
      : String(point.value)
  return point.reason ? `${point.state} (${point.reason})` : point.state
}
