import { useQuery } from '@tanstack/react-query'
import { useParams, useSearchParams } from 'react-router'
import { CommitSha, LastGoodResult } from '@/entities/check-report'
import { MetricValue } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, getMergeRequestSummary } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { contractLabel, contractLabels, technicalLabels } from '@/shared/config'
import { DataStateBadge, ErrorState, LoadingState } from '@/shared/ui'
import { SummaryMetrics } from './summary-metrics'
import { SummarySignals } from './summary-signals'

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
  const query = useQuery(
    getMergeRequestSummary(
      api,
      mr,
      revision === undefined ? {} : { revision_no: revision },
    ),
  )
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
  const publication = s.publication
  return (
    <article>
      <h1>Сводка MR, ревизия {s.revision_no}</h1>
      <p>
        Head <CommitSha sha={s.head_sha} />
      </p>
      {s.base_sha && (
        <p>
          База <CommitSha sha={s.base_sha} />
        </p>
      )}
      {s.last_good && <LastGoodResult result={s.last_good} />}
      <p>
        Состояние сводки{' '}
        {contractLabel(technicalLabels.summaryStatus, s.status)}
        {s.stale_reason && ` (${s.stale_reason})`}
      </p>
      {s.built_at && (
        <p>
          Собрана <time dateTime={s.built_at}>{s.built_at}</time>
        </p>
      )}
      <h2>Комментарий в MR</h2>
      <p>
        {contractLabel(technicalLabels.publicationStatus, publication.status)}
      </p>
      {publication.error_code && (
        <p>
          {contractLabel(
            technicalLabels.publicationError,
            publication.error_code,
          )}
        </p>
      )}
      {publication.published_at && (
        <p>
          Опубликован{' '}
          <time dateTime={publication.published_at}>
            {publication.published_at}
          </time>
        </p>
      )}
      {publication.external_id && (
        <p>
          ID комментария <code>{publication.external_id}</code>
        </p>
      )}
      <h2>Проверки</h2>
      <p>
        Состояние pipeline{' '}
        {contractLabel(technicalLabels.pipelineState, s.pipeline_state)}
      </p>
      {s.ci && (
        <p>
          Исход CI {contractLabel(contractLabels.ciOutcome, s.ci.outcome)}.
          Попыток {s.ci.attempts}, заданий с повтором {s.ci.retried_jobs}
        </p>
      )}
      <h3>Покрытие изменённых строк</h3>
      <MetricValue value={s.coverage_changed_lines} />
      <SummaryMetrics tests={s.tests} change={s.change} expected={s.expected} />
      {s.not_evaluated.length > 0 && (
        <section>
          <h2>Непроверенные области</h2>
          <ul>
            {s.not_evaluated.map(({ analysis_kind, reason_code }, i) => (
              <li key={i}>
                {contractLabel(technicalLabels.analysisKind, analysis_kind)}{' '}
                <DataStateBadge
                  state="NOT_EVALUATED"
                  reason={contractLabel(
                    technicalLabels.reportReason,
                    reason_code,
                  )}
                />
              </li>
            ))}
          </ul>
        </section>
      )}
      <h2>Находки</h2>
      <RestrictedResults
        access_limited={s.access_limited}
        access_notes={s.access_notes}
        redactions={s.redactions}
      />
      <SummarySignals items={s.items} />
    </article>
  )
}
