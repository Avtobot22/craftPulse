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
        {(Object.keys(labels) as K[]).map((key) => (
          <div key={key}>
            <dt>{labels[key]}</dt>
            <dd>
              <MetricValue value={values[key]} />
            </dd>
          </div>
        ))}
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
      {tests ? (
        <MetricGroup title="Тесты" values={tests} labels={testLabels} />
      ) : (
        <section>
          <h3>Тесты</h3>
          <p>Данные о тестах недоступны.</p>
        </section>
      )}
      {change ? (
        <MetricGroup title="Изменения" values={change} labels={changeLabels} />
      ) : (
        <section>
          <h3>Изменения</h3>
          <p>Данные о структуре изменения недоступны.</p>
        </section>
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

export function SummaryCoverage({
  value,
}: {
  value: Summary['coverage_changed_lines']
}) {
  if (value.state === 'NOT_APPLICABLE')
    return (
      <p>
        неприменимо
        {value.reason_code === 'no_executable_units' &&
          ' · Нет изменённых исполняемых строк'}
      </p>
    )
  if (value.state === 'NOT_EVALUATED')
    return (
      <p>
        не измерено
        {value.reason_code === 'report_missing' &&
          ' · Отчёт о покрытии отсутствует'}
      </p>
    )

  const { numerator, denominator } = value
  return (
    <div>
      <MetricValue value={value} />
      {value.state === 'ASSESSED' &&
        numerator !== null &&
        denominator !== null &&
        denominator > 0 && (
          <p>
            Покрыто строк: {numerator} из {denominator}
          </p>
        )}
    </div>
  )
}
