import type { MetricRevision } from '@/shared/api'

const inputLabels: Readonly<Record<string, string>> = {
  definition: 'Определение',
  mapping: 'Сопоставление статусов',
  calendar: 'Рабочий календарь',
  target: 'Целевой срок',
  decision: 'Решение',
  structure: 'Структура',
  accounts: 'Учётные записи',
  links: 'Связи',
  source_watermark: 'Watermark источника',
  ci_rate: 'Ставка CI',
}

const completenessLabels: Readonly<Record<string, string>> = {
  complete: 'Полные данные',
  partial: 'Частичные данные',
  unknown: 'Полнота неизвестна',
}

function label(labels: Readonly<Record<string, string>>, key: string) {
  return Object.hasOwn(labels, key) ? labels[key] : key
}

function RevisionTime({ value }: { value: string | null }) {
  return value ? <time dateTime={value}>{value}</time> : 'Нет данных'
}

export function RevisionInputs({
  revision,
  step,
}: {
  revision: MetricRevision
  step: string | null | undefined
}) {
  return (
    <>
      <section aria-labelledby="metric-revision">
        <h2 id="metric-revision">Ревизия {revision.revision_no}</h2>
        <dl className="cp-metric-passport__details">
          <dt>Период</dt>
          <dd>
            <RevisionTime value={revision.window.from} /> —{' '}
            <RevisionTime value={revision.window.to} />
          </dd>
          <dt>Шаг</dt>
          <dd>
            {step === 'day'
              ? 'День'
              : step === 'week'
                ? 'Неделя'
                : step === 'month'
                  ? 'Месяц'
                  : 'Нет данных'}
          </dd>
          <dt>Дата среза (as_of)</dt>
          <dd>
            <RevisionTime value={revision.as_of} />
          </dd>
          <dt>Часовой пояс</dt>
          <dd>{revision.timezone}</dd>
          <dt>Срез знания</dt>
          <dd>{revision.knowledge_seq}</dd>
          <dt>Опубликована</dt>
          <dd>
            <RevisionTime value={revision.published_at} />
          </dd>
        </dl>
      </section>
      <section aria-labelledby="metric-inputs">
        <h2 id="metric-inputs">Версии входов</h2>
        {revision.inputs.length ? (
          <ul>
            {revision.inputs.map((input, index) => (
              <li key={index}>
                {label(inputLabels, input.input_kind)} · {input.input_id} ·{' '}
                {input.version === null
                  ? 'Версия неизвестна'
                  : 'Версия ' + input.version}
              </li>
            ))}
          </ul>
        ) : (
          <p>Нет сведений о версиях входов.</p>
        )}
      </section>
      <section aria-labelledby="metric-watermarks">
        <h2 id="metric-watermarks">Полнота по подключениям</h2>
        {revision.source_watermarks.length ? (
          <ul>
            {revision.source_watermarks.map((source) => (
              <li key={source.connection_id}>
                {source.connection_id} ·{' '}
                {label(completenessLabels, source.completeness)} · Watermark{' '}
                <RevisionTime value={source.watermark} />
              </li>
            ))}
          </ul>
        ) : (
          <p>Нет сведений о полноте подключений.</p>
        )}
      </section>
    </>
  )
}
