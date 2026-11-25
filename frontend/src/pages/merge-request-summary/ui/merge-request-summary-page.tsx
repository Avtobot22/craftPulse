import { useQuery } from '@tanstack/react-query'
import { generatePath, Link, useParams, useSearchParams } from 'react-router'
import { CommitSha, LastGoodResult } from '@/entities/check-report'
import { MetricValue } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import {
  contractLabel,
  contractLabels,
  routePaths,
  technicalLabels,
} from '@/shared/config'
import { DataStateBadge, ErrorState, LoadingState } from '@/shared/ui'
import { getSummaryQuery } from '../model/summary'
import { SummaryMetrics } from './summary-metrics'
import { SummarySignals } from './summary-signals'

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu
const gitSha = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u
const notEvaluatedReasons = {
  ...technicalLabels.reportReason,
  scan_failed: 'Анализ завершился с ошибкой',
  import_failed: 'Не удалось импортировать отчёт',
  scan_incomplete: 'Анализ выполнен частично',
}

export function MergeRequestSummaryPage({ api }: { api: ApiClient }) {
  const { mr = '' } = useParams()
  const [params] = useSearchParams()
  const head = params.get('head_sha') ?? undefined
  const raw = params.get('revision_no')
  const revision = raw === null ? undefined : Number(raw)
  if (
    !uuid.test(mr) ||
    (head !== undefined && !gitSha.test(head)) ||
    (raw !== null && (!/^[1-9][0-9]*$/u.test(raw) || Number(raw) > 2147483647))
  ) {
    return <p role="alert">Некорректная ссылка на сводку MR</p>
  }
  return (
    <SummaryContent
      api={api}
      mr={mr.toLowerCase()}
      revision={revision}
      head={head}
    />
  )
}

function SummaryContent({
  api,
  mr,
  revision,
  head,
}: {
  api: ApiClient
  mr: string
  revision: number | undefined
  head: string | undefined
}) {
  const query = useQuery(getSummaryQuery(api, mr, revision, head))
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
      {(s.status === 'stale' || s.status === 'superseded') && (
        <p>Историческая сводка</p>
      )}
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
        <>
          <p>
            Исход CI {contractLabel(contractLabels.ciOutcome, s.ci.outcome)}.
            Попыток {s.ci.attempts}, заданий с повтором {s.ci.retried_jobs}
          </p>
          <Link
            to={generatePath(routePaths.pipeline, {
              pipelineId: s.ci.pipeline_id,
            })}
          >
            Открыть pipeline
          </Link>
        </>
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
                  reason={contractLabel(notEvaluatedReasons, reason_code)}
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
