import { Card } from '@gravity-ui/uikit'
import { MetricValue } from '@/entities/metric'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel } from '@/shared/config'
import { DataStateBadge, StatusTime } from '@/shared/ui'

type Fact = OpenCodes<
  components['schemas']['ComponentOverview']['facts'][number]
>

const metricLabels = new Map([
  ['C01', 'Объём проанализированного кода'],
  ['C04', 'Цикломатическая сложность'],
  ['C05', 'Доля функций выше порога сложности'],
  ['C06', 'Длина функций'],
  ['C08', 'Доля дублированных строк'],
  ['C09', 'Группы копий'],
])

const sources = {
  builtin: 'Штатная проверка (builtin)',
  import: 'Импортированный отчёт (import)',
}

export function OverviewCodeFacts({ facts }: { facts: Fact[] }) {
  const visible = facts.filter(({ metric_id }) => metricLabels.has(metric_id))
  return (
    <section aria-labelledby="overview-facts-heading">
      <h2 id="overview-facts-heading">Структура кода</h2>
      {visible.length === 0 ? (
        <DataStateBadge
          state="NOT_EVALUATED"
          reason="Нет данных о структуре кода по отчётам"
        />
      ) : (
        <ul className="cp-component-overview__cards">
          {visible.map(({ metric_id, source, window, value }, index) => (
            <li
              key={`${metric_id}-${source}-${window.from}-${window.to}-${String(index)}`}
            >
              <Card className="cp-component-overview__card">
                <h3>
                  {metricLabels.get(metric_id)} ({metric_id})
                </h3>
                <p>{contractLabel(sources, source)}</p>
                <p>
                  Окно: <StatusTime value={window.from} /> -{' '}
                  <StatusTime value={window.to} />
                </p>
                <MetricValue value={value} />
              </Card>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
