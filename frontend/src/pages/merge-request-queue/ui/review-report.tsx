import { useQuery } from '@tanstack/react-query'
import { ActivityTime } from '@/entities/activity'
import { MetricValue } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, loadReport, RevisionNotPublished } from '@/shared/api'
import type { ApiClient, ReportQuery, TeamReport } from '@/shared/api'
import { EmptyState, ErrorState, LoadingState, StaleState } from '@/shared/ui'

const metrics = [
  ['p50', 'A07 · p50'],
  ['p85', 'A07 · p85'],
  ['A05', 'A05 · Очередь ревью'],
  ['A10', 'A10 · Доля MR без ревьюера'],
] as const
type Metric = (typeof metrics)[number][0]
type Value = TeamReport['metrics'][number]['points'][number]['value']

function reviewPoints(report: TeamReport) {
  const points = new Map<string, Partial<Record<Metric, Value>>>()
  for (const {
    metric_id,
    estimator,
    points: series,
    group,
  } of report.metrics) {
    if (group) continue
    const metric = metric_id === 'A07' ? estimator : metric_id
    if (
      metric !== 'A05' &&
      metric !== 'A10' &&
      !(metric_id === 'A07' && (metric === 'p50' || metric === 'p85'))
    )
      continue
    for (const { point_start, value } of series) {
      const values = points.get(point_start) ?? {}
      values[metric] = value
      points.set(point_start, values)
    }
  }
  return [...points.entries()].sort(
    ([left], [right]) => Date.parse(right) - Date.parse(left),
  )
}

const steps = {
  day: 'день',
  week: 'неделя',
  month: 'месяц',
  unknown: 'неизвестно',
}

function ReportValues({ report }: { report: TeamReport }) {
  const points = reviewPoints(report)
  return (
    <>
      <p>
        Период отчёта с <ActivityTime instant={report.period.from} /> до{' '}
        <ActivityTime instant={report.period.to} />
      </p>
      <p>
        Данные на <ActivityTime instant={report.revision.as_of} /> · Шаг:{' '}
        {steps[report.step]}
      </p>
      <RestrictedResults access_limited={report.access_limited} />
      {report.sourceAccessLost && (
        <p role="status">Подключение к источнику потеряло доступ.</p>
      )}
      {report.revision.loading && (
        <p role="status">Данные отчёта ещё загружаются.</p>
      )}
      {points.length === 0 && (
        <EmptyState
          reason={report.revision.loading ? 'history' : 'filters'}
          title="Нет доступных значений за период"
        />
      )}
      {points.map(([start, values]) => (
        <section className="cp-mr-queue__report-point" key={start}>
          <h3>
            Интервал с <ActivityTime instant={start} />
          </h3>
          {(values.p50?.reason_code === 'first_external_response' ||
            values.p85?.reason_code === 'first_external_response') && (
            <p>Первый внешний ответ</p>
          )}
          <div className="cp-mr-queue__metrics">
            {metrics.map(([id, label]) => {
              const value = values[id]
              return (
                <div key={id} className="cp-mr-queue__metric">
                  <h4>{label}</h4>
                  {value ? (
                    <MetricValue
                      value={value}
                      showBreakdown={id === 'A05' || id === 'A10'}
                    />
                  ) : (
                    <p>Нет значения за интервал.</p>
                  )}
                </div>
              )
            })}
          </div>
        </section>
      ))}
    </>
  )
}

export function ReviewReport({
  api,
  teamId,
  params,
}: {
  api: ApiClient
  teamId: string
  params: ReportQuery
}) {
  const query = useQuery({
    queryKey: ['team-report', teamId, params],
    queryFn: ({ signal }) => loadReport(api, teamId, params, signal),
    refetchOnMount: 'always',
  })
  if (query.isPending || (query.isFetching && !query.isFetchedAfterMount))
    return <LoadingState />

  if (query.isError) {
    const error = query.error instanceof ApiError ? query.error : undefined
    const correlationId = error?.problem?.correlation_id
    const unavailable =
      error?.kind === 'forbidden' || error?.kind === 'not-found'
    const failure = (
      <ErrorState
        reason={
          query.error instanceof RevisionNotPublished
            ? 'Ревизия отчёта ещё не опубликована'
            : unavailable
              ? 'Отчёт команды недоступен'
              : 'Не удалось загрузить показатели ревью'
        }
        {...(correlationId ? { correlationId } : {})}
        onRetry={() => void query.refetch()}
        retrying={query.isFetching}
      />
    )
    const transient =
      error?.kind === 'network' || (error?.problem?.status ?? 0) >= 500
    return transient && query.data && !query.data.sourceAccessLost ? (
      <StaleState
        snapshotAt={new Date(query.dataUpdatedAt).toISOString()}
        currentError={failure}
      >
        <ReportValues report={query.data} />
      </StaleState>
    ) : (
      failure
    )
  }
  return <ReportValues report={query.data} />
}
