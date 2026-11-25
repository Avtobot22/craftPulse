import { MetricValue } from '@/entities/metric'
import type { components, OpenCodes } from '@/shared/api'

type Summary = OpenCodes<components['schemas']['MergeRequestSummary']>

const testLabels = {
  passed: 'Прошли',
  failed: 'Упали',
  errored: 'Завершились с ошибкой',
  skipped: 'Пропущены',
} satisfies Record<keyof NonNullable<Summary['tests']>, string>

const changeLabels = {
  files_changed: 'Изменённые файлы',
  lines_added: 'Добавленные строки',
  lines_removed: 'Удалённые строки',
  complexity_increased_functions: 'Функции с ростом сложности',
  new_clone_groups: 'Новые группы клонов',
} satisfies Record<keyof NonNullable<Summary['change']>, string>

const expectedLabels = {
  missing_checks: 'Отсутствующие проверки',
  missing_reports: 'Отсутствующие отчёты',
} satisfies Record<keyof NonNullable<Summary['expected']>, string>

const changeReasons = new Map([
  ['incomparable_base', 'Нет сопоставимой базы'],
  ['mr_diff_missing', 'Нет данных об изменённых строках'],
])

function MetricGroup<K extends string>({
  title,
  values,
  labels,
}: {
  title: string
  values: Record<K, OpenCodes<components['schemas']['MetricValue']>>
  labels: Record<K, string>
}) {
  return (
    <section>
      <h3>{title}</h3>
      <dl>
        {(Object.keys(labels) as K[]).map((key) => {
          const value = values[key]
          const reason =
            value.reason_code && changeReasons.get(value.reason_code)
          return (
            <div key={key}>
              <dt>{labels[key]}</dt>
              <dd>
                <MetricValue value={value} />
                {reason && <p>{reason}</p>}
              </dd>
            </div>
          )
        })}
      </dl>
    </section>
  )
}

export function SummaryMetrics({
  tests,
  change,
  expected,
}: {
  tests: Summary['tests']
  change: Summary['change']
  expected: Summary['expected']
}) {
  return (
    <>
      {tests && (
        <MetricGroup title="Тесты" values={tests} labels={testLabels} />
      )}
      {change && (
        <MetricGroup title="Изменения" values={change} labels={changeLabels} />
      )}
      {expected && (
        <MetricGroup
          title="Ожидаемые проверки и отчёты"
          values={expected}
          labels={expectedLabels}
        />
      )}
    </>
  )
}
