import { formatTime, latestPoint, seriesKey } from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'
import { MetricCard } from './metric-block'

const completenessMetrics = new Set(['T38', 'T39', 'T40', 'T41', 'T42'])

export function CompletenessBlock({ block }: { block: ReportBlockProps }) {
  const { report, metadata } = block
  const series = report.metrics.filter(({ metric_id }) =>
    completenessMetrics.has(metric_id),
  )
  const timeZone = metadata?.timeZone ?? 'UTC'
  const watermarks =
    block.projectIds?.length || report.access_limited
      ? []
      : (metadata?.watermarks ?? [])
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-completeness"
    >
      <h2 id="team-report-completeness">Полнота данных</h2>
      {watermarks.length > 0 && (
        <ul aria-label="Отметки загрузки источников">
          {watermarks.map((watermark, index) => (
            <li key={index}>
              Отметка загрузки (watermark):{' '}
              {watermark ? (
                <time dateTime={watermark}>
                  {formatTime(watermark, timeZone)}
                </time>
              ) : (
                'неизвестна'
              )}
            </li>
          ))}
        </ul>
      )}
      {series.length ? (
        <div className="cp-team-report__cards">
          {series.map((item) => {
            const value = latestPoint(item)?.value
            const isSuccessAge = item.estimator === 'last_success_age'
            const sourceDate =
              item.metric_id === 'T40' &&
              (isSuccessAge ||
                (item.estimator === 'lag' && !watermarks.length)) &&
              value?.state === 'ASSESSED' &&
              value.unit === 'seconds' &&
              value.value !== null
                ? new Date(
                    Date.parse(report.revision.as_of) - value.value * 1000,
                  )
                : null
            return (
              <MetricCard key={seriesKey(item)} block={block} series={item}>
                {sourceDate && Number.isFinite(sourceDate.getTime()) && (
                  <p>
                    {isSuccessAge
                      ? 'Самая ранняя из последних успешных синхронизаций:'
                      : 'Самая ранняя отметка загрузки (watermark):'}{' '}
                    {value?.value === 0 && 'не ранее '}
                    <time dateTime={sourceDate.toISOString()}>
                      {formatTime(sourceDate.toISOString(), timeZone)}
                    </time>
                  </p>
                )}
              </MetricCard>
            )
          })}
        </div>
      ) : (
        <p>Нет данных о полноте за этот период.</p>
      )}
    </section>
  )
}
