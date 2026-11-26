import { Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { MetricValue } from '@/entities/metric'
import { loadReport, loadRevisionMetadata } from '@/shared/api'
import type {
  ApiClient,
  ReportQuery,
  ReportSeries,
  RevisionMetadata,
} from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'

const titles = {
  T10: 'Соблюдение целевых сроков',
  T23: 'Переоткрытия workflow',
}

const inputLabels: Readonly<Record<string, string>> = {
  mapping: 'Сопоставление',
  calendar: 'Календарь',
  target: 'Целевой срок',
  decision: 'Решение',
  structure: 'Структура',
  accounts: 'Сопоставление аккаунтов',
  links: 'Связи',
}

const irreproducible: Readonly<Record<string, string>> = {
  basis_deleted: 'основания удалены',
  basis_expired: 'срок хранения оснований истёк',
  definition_retired: 'определение выведено из использования',
}

function CalculationBasis({
  revision,
  series,
}: {
  revision: RevisionMetadata
  series: ReportSeries
}) {
  const { metric_id: metricId, definition_version: version } = series
  const followUpDays = revision.reopenWindows[version]
  return (
    <section aria-label="Основания расчёта">
      <Text as="h3" variant="subheader-2">
        Основания расчёта
      </Text>
      {version === 1 && metricId === 'T10' && (
        <p>
          Числитель - задачи, завершённые в пределах целевого срока. Знаменатель
          - завершённые задачи с известными началом работы, размером и целевым
          сроком.
        </p>
      )}
      {version === 1 && metricId === 'T23' && (
        <p>
          Числитель - задачи, переоткрытые в течение окна наблюдения после
          первого завершения. Знаменатель - задачи с полным окном наблюдения.
        </p>
      )}
      <dl>
        <dt>Версия определения</dt>
        <dd>{version}</dd>
        {metricId === 'T23' && (
          <>
            <dt>Окно наблюдения</dt>
            <dd>
              {followUpDays === undefined
                ? 'Не указано'
                : String(followUpDays) + ' дн. после первого завершения'}
            </dd>
          </>
        )}
        <dt>Ревизия</dt>
        <dd>№ {revision.number}</dd>
        <dt>Окно расчёта ревизии</dt>
        <dd>
          <time dateTime={revision.window.from}>{revision.window.from}</time>
          {' - '}
          <time dateTime={revision.window.to}>{revision.window.to}</time>
        </dd>
        <dt>Срез данных</dt>
        <dd>
          <time dateTime={revision.asOf}>{revision.asOf}</time>
        </dd>
        <dt>Часовой пояс расчёта</dt>
        <dd>{revision.timeZone}</dd>
      </dl>
      {revision.irreproducibleReason && (
        <p role="status">
          Ревизия невоспроизводима.{' '}
          {Object.hasOwn(irreproducible, revision.irreproducibleReason)
            ? irreproducible[revision.irreproducibleReason]
            : 'Причина не указана'}
          .
        </p>
      )}
      {revision.inputs.length > 0 && (
        <>
          <Text as="h4" variant="subheader-1">
            Данные и версии настроек ревизии
          </Text>
          <ul>
            {revision.inputs.map(({ kind, id, version }) => (
              <li key={JSON.stringify([kind, id, version])}>
                {Object.hasOwn(inputLabels, kind) ? inputLabels[kind] : kind}
                {' · '}
                {id}
                {' · '}
                {version === null ? 'Версия не указана' : 'Версия ' + version}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}

export function ReportPointPassport({
  api,
  teamId,
  query,
  revisionId,
  metricId,
  pointStart,
  estimator,
  groupValue,
}: {
  api: ApiClient
  teamId: string
  query: ReportQuery
  revisionId: string
  metricId: string
  pointStart: string
  estimator: string | null
  groupValue: string | null
}) {
  const passport = useQuery({
    queryKey: ['metric-passport', teamId, query, revisionId],
    queryFn: async ({ signal }) => {
      const [report, revision] = await Promise.all([
        loadReport(api, teamId, query, signal),
        loadRevisionMetadata(api, revisionId, signal),
      ])
      return { report, revision }
    },
  })
  if (passport.isError)
    return (
      <ErrorState
        reason="Не удалось загрузить паспорт выбранной точки"
        onRetry={() => void passport.refetch()}
        retrying={passport.isFetching}
      />
    )
  if (!passport.data || !passport.isFetchedAfterMount) return <LoadingState />

  const { report, revision } = passport.data
  const groupBy = query.group_by
  const series = report.metrics.find(
    (item) =>
      item.metric_id === metricId &&
      (item.estimator ?? null) === estimator &&
      (groupValue === null
        ? !item.group
        : groupBy && item.group && (item.group[groupBy] ?? '') === groupValue),
  )
  const point = series?.points.find(
    ({ point_start }) => Date.parse(point_start) === Date.parse(pointStart),
  )
  if (!series || !point)
    return <p role="status">В выбранном срезе нет этой точки показателя.</p>

  const definition = revision.definitions.find(
    ({ metricId: id, version }) =>
      id === metricId && version === series.definition_version,
  )
  const title =
    metricId === 'T10' || metricId === 'T23'
      ? titles[metricId]
      : (definition?.name ?? metricId)
  const { value } = point
  return (
    <Flex as="section" direction="column" gap="spacing-2">
      <Text as="h2" variant="header-2">
        {title}
      </Text>
      {report.access_limited && (
        <p role="status">Значение рассчитано по доступным данным.</p>
      )}
      <MetricValue value={value} />
      <dl>
        <dt>Числитель</dt>
        <dd>{value.numerator ?? 'Неизвестен'}</dd>
        <dt>Знаменатель</dt>
        <dd>{value.denominator ?? 'Неизвестен'}</dd>
        <dt>Неизвестно</dt>
        <dd>{value.missing_count}</dd>
        <dt>{metricId === 'T23' ? 'Незрелые задачи' : 'Исключено'}</dt>
        <dd>{value.excluded_count}</dd>
      </dl>
      <CalculationBasis revision={revision} series={series} />
    </Flex>
  )
}
