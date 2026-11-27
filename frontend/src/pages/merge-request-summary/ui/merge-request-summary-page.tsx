import { useQuery } from '@tanstack/react-query'
import {
  generatePath,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  baselineContext,
  contractLabel,
  readTechnicalId,
  routePaths,
  technicalLabels,
} from '@/shared/config'
import {
  DataStateBadge,
  ErrorState,
  InspectionBack,
  InspectionLink,
  LoadingState,
  useInspectionReturn,
} from '@/shared/ui'
import { getSummaryQuery } from '../model/summary'
import { summaryComparisonLink } from '../model/comparison-link'
import { SummaryAttempts } from './summary-attempts'
import { SummaryHeader } from './summary-header'
import { SummaryMetrics, SummaryCoverage } from './summary-metrics'
import { SummarySignals } from './summary-signals'
import './merge-request-summary-page.css'

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
    (raw !== null && !/^[1-9][0-9]{0,9}$/u.test(raw)) ||
    (revision !== undefined && revision > 2147483647)
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
  const user = useOutletContext<CurrentUser | undefined>()
  const [params] = useSearchParams()
  useInspectionReturn(query.isSuccess)
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    const kind = query.error instanceof ApiError ? query.error.kind : null
    let reason = 'Не удалось загрузить сводку MR'
    if (kind === 'not-found' || kind === 'forbidden') {
      reason =
        kind === 'not-found' && head && revision === undefined
          ? 'Для выбранного head пока нет сводки'
          : 'Ревизия сводки недоступна'
    }
    return (
      <ErrorState
        reason={reason}
        retrying={query.isFetching}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  const s = query.data
  const canReadFindings = user?.actions.includes('findings.read') ?? false
  const canReadCI = user?.actions.includes('ci.read') ?? false
  const comparisonSearch = new URLSearchParams()
  for (const key of ['org_slug', 'team_id', 'project_id']) {
    const value = params.get(key)
    if (value) comparisonSearch.set(key, value)
  }
  comparisonSearch.set('head_sha', s.head_sha)
  if (s.base_sha) comparisonSearch.set('base_sha', s.base_sha)

  const findingSearch = new URLSearchParams(comparisonSearch)
  findingSearch.set('mr_id', s.merge_request_id)
  findingSearch.set('revision_no', String(s.revision_no))

  const signals = s.items.filter(
    ({ finding_id, analysis_kind, item_kind }) =>
      finding_id !== null &&
      analysis_kind !== undefined &&
      Object.hasOwn(technicalLabels.analysisKind, analysis_kind) &&
      (item_kind === 'accepted' ||
        item_kind === 'newly_observed' ||
        (s.base_sha !== null &&
          (item_kind === 'new' || item_kind === 'resolved'))),
  )
  const comparisonItem =
    signals.find(({ component_id }) => component_id !== undefined) ??
    signals[0] ??
    s.items.find(
      ({ component_id, analysis_kind, item_kind }) =>
        item_kind === 'not_evaluated' &&
        component_id !== undefined &&
        analysis_kind !== undefined &&
        Object.hasOwn(technicalLabels.analysisKind, analysis_kind),
    )

  return (
    <article className="cp-mr-summary">
      <SummaryHeader
        api={api}
        summary={s}
        refresh={() => void query.refetch()}
      />
      <InspectionBack />
      {canReadCI && (
        <InspectionLink
          id="summary-baselines"
          to={{
            pathname: generatePath(routePaths.baselines, {
              projectId: readTechnicalId(params.get('project_id')) ?? null,
            }),
            search: baselineContext(findingSearch).toString(),
          }}
        >
          Базовые линии
        </InspectionLink>
      )}
      <div className="cp-summary-columns">
        <section>
          <h2>Находки</h2>
          <RestrictedResults
            access_limited={s.access_limited}
            access_notes={s.access_notes}
            redactions={s.redactions}
          />
          {canReadFindings && (
            <InspectionLink
              id="summary-comparison"
              to={summaryComparisonLink(comparisonSearch, comparisonItem)}
            >
              Причины изменения результата
            </InspectionLink>
          )}
          <SummarySignals
            items={s.items}
            hasBase={s.base_sha !== null}
            canReadFindings={canReadFindings}
            comparisonSearch={comparisonSearch}
            findingSearch={findingSearch}
          />
          {(s.not_evaluated.length > 0 ||
            s.redactions.includes('not_evaluated')) && (
            <section>
              <h2>Непроверенные области</h2>
              {s.redactions.includes('not_evaluated') && (
                <p>Часть непроверенных областей скрыта.</p>
              )}
              <ul>
                {s.not_evaluated.map(({ analysis_kind, reason_code }, i) => (
                  <li key={`${analysis_kind}-${reason_code}-${String(i)}`}>
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
        </section>
        <section>
          <h2>Проверки и структура изменения</h2>
          <SummaryAttempts api={api} ci={s.ci} canReadCI={canReadCI} />
          <h3>Покрытие изменённых строк</h3>
          <SummaryCoverage value={s.coverage_changed_lines} />
          <SummaryMetrics
            tests={s.tests}
            change={s.change}
            expected={s.expected}
          />
        </section>
      </div>
    </article>
  )
}
