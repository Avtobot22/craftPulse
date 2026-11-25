import { Text } from '@gravity-ui/uikit'
import { DataStateBadge } from '@/shared/ui'
import type { Summary } from '../api/read-summary'

const number = new Intl.NumberFormat('ru-RU')
const percent = new Intl.NumberFormat('ru-RU', {
  style: 'percent',
  maximumFractionDigits: 2,
})

const reasons = new Map([
  ['scan_missing', 'Отчёт отсутствует'],
  ['scan_failed', 'Анализ завершился с ошибкой'],
  ['import_failed', 'Не удалось импортировать отчёт'],
  ['scan_incomplete', 'Анализ выполнен частично'],
  ['mr_diff_missing', 'Нет данных об изменённых строках'],
  ['incomparable_base', 'Нет сопоставимой базы'],
])

function reasonLabel(code: string): string {
  return reasons.get(code) ?? code
}

function Metric({ metric }: { metric: Summary['coverage'] }) {
  const format = metric.unit === 'ratio' ? percent : number
  const value = metric.value !== null ? format.format(metric.value) : null
  const count =
    metric.numerator !== null && metric.denominator !== null
      ? `${number.format(metric.numerator)} из ${number.format(metric.denominator)}`
      : null
  const bound = metric.bound
  return (
    <>
      {value !== null && (
        <span>
          {value}
          {count && ` (${count})`}{' '}
        </span>
      )}
      {bound && (
        <span>
          {bound.kind === 'lower' && '≥ '}
          {bound.kind === 'upper' && '≤ '}
          {format.format(bound.value)}{' '}
        </span>
      )}
      <DataStateBadge
        state={metric.state}
        reason={metric.reason ? reasonLabel(metric.reason) : ''}
      />
    </>
  )
}

function MetricSection({
  title,
  rows,
}: {
  title: string
  rows: Summary['tests']
}) {
  if (rows.length === 0) return null
  return (
    <section>
      <Text as="h2" variant="subheader-2">
        {title}
      </Text>
      <dl>
        {rows.map(({ key, label, metric }) => (
          <div key={key}>
            <dt>{label}</dt>
            <dd>
              <Metric metric={metric} />
            </dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

export function SummaryMetrics({ summary }: { summary: Summary }) {
  return (
    <>
      <MetricSection title="Тесты" rows={summary.tests} />
      <MetricSection title="Изменение" rows={summary.change} />
      <MetricSection
        title="Покрытие"
        rows={[
          {
            key: 'coverage',
            label: 'Покрытие изменённых строк',
            metric: summary.coverage,
          },
        ]}
      />
      <MetricSection
        title="Ожидаемые проверки и отчёты"
        rows={summary.expected}
      />
      {summary.notEvaluated.length > 0 && (
        <section>
          <Text as="h2" variant="subheader-2">
            Непроверенные области
          </Text>
          <ul>
            {summary.notEvaluated.map(({ kind, reason }, index) => (
              <li key={index}>
                {kind} · {reasonLabel(reason)}
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}
