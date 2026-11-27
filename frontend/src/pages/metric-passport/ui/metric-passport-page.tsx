import { Card, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Link, Navigate, useLocation } from 'react-router'
import {
  InspectionReturn,
  isRevisionAccessDenied,
  MetricValue,
  metricDetailsLink,
  reportHistoryDepth,
  useMetricInspection,
  useMetricPoint,
  useMetricRevision,
} from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import {
  findMetricDefinition,
  followUpDays,
  loadRevisionSeries,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'
import { catalogHref, metricFormula } from '../model/definition'
import { RevisionInputs } from './revision-inputs'
import './metric-passport-page.css'

const reportTitles: Readonly<Record<string, string>> = {
  T10: 'Соблюдение целевых сроков',
  T23: 'Переоткрытия workflow',
}

const irreproducibleReasons: Readonly<Record<string, string>> = {
  basis_deleted: 'Основание удалено',
  basis_expired: 'Истёк срок хранения основания',
  definition_retired: 'Определение снято с использования',
}

export function MetricPassportPage({ api }: { api: ApiClient }) {
  const selection = useMetricInspection()
  const depth = reportHistoryDepth(useLocation().state)
  const revision = useMetricRevision(api, selection.revisionId)
  const data = revision.isError ? undefined : revision.data
  const point = useMetricPoint(api, selection, data)
  const definitionVersion = point.data?.definitionVersion
  const pinned = data?.definitions.find(
    (entry) =>
      entry.metric_id === selection.metricId &&
      (definitionVersion === undefined ||
        entry.definition_version === definitionVersion),
  )
  const catalog = useQuery({
    queryKey: ['metric-definition', selection.metricId, definitionVersion],
    queryFn: ({ signal }) =>
      findMetricDefinition(api, selection.metricId, definitionVersion, signal),
    enabled: Boolean(data) && !pinned && definitionVersion !== undefined,
  })
  const definition = pinned ?? catalog.data
  const href = definition && catalogHref(definition.catalog_ref)
  const title =
    (Object.hasOwn(reportTitles, selection.metricId)
      ? reportTitles[selection.metricId]
      : undefined) ??
    definition?.name ??
    selection.metricId
  const version = definition?.definition_version ?? definitionVersion
  const formula =
    version === undefined
      ? undefined
      : metricFormula(selection.metricId, version)
  const observationDays = definition && followUpDays(definition)
  const latest = useQuery({
    queryKey: ['metric-inspection-latest', data?.series_id, selection.metricId],
    queryFn: ({ signal }) =>
      data &&
      loadRevisionSeries(
        api,
        data.series_id,
        selection.metricId,
        undefined,
        signal,
      ),
    enabled: data?.status === 'superseded',
    gcTime: 0,
  })
  const current =
    !latest.isError &&
    latest.data?.publishedAt &&
    latest.data.revisionId !== selection.revisionId
      ? latest.data.revisionId
      : undefined

  if (
    isRevisionAccessDenied(revision.error) ||
    isRevisionAccessDenied(point.error)
  )
    return <Navigate to={routePaths.unavailable} replace />

  return (
    <section className="cp-metric-passport">
      <Text as="h1" variant="header-1">
        Паспорт показателя
      </Text>
      <InspectionReturn selection={selection} />
      {revision.isPending ? (
        <LoadingState />
      ) : revision.isError ? (
        <ErrorState
          reason="Не удалось загрузить паспорт ревизии"
          onRetry={() => void revision.refetch()}
          retrying={revision.isFetching}
        />
      ) : (
        data && (
          <>
            {data.status === 'superseded' && (
              <div role="status">
                Есть более новая ревизия.{' '}
                {current && (
                  <Link
                    replace={depth === 1}
                    state={depth === 1 ? { fromTeamReport: true } : undefined}
                    to={metricDetailsLink(selection, 'metricPassport', current)}
                  >
                    Открыть актуальную ревизию
                  </Link>
                )}
                {latest.isError && (
                  <ErrorState
                    reason="Не удалось найти актуальную ревизию"
                    onRetry={() => void latest.refetch()}
                    retrying={latest.isFetching}
                  />
                )}
              </div>
            )}
            {(data.loading || data.status === 'building') && (
              <p role="status">Ревизия ещё загружается.</p>
            )}
            {data.status === 'failed' && (
              <p role="alert">Расчёт ревизии завершился ошибкой.</p>
            )}
            {data.irreproducible_reason && (
              <p role="status">
                Ревизию нельзя воспроизвести.{' '}
                {Object.hasOwn(
                  irreproducibleReasons,
                  data.irreproducible_reason,
                )
                  ? irreproducibleReasons[data.irreproducible_reason]
                  : 'Описание причины недоступно'}
                .
              </p>
            )}
            <Card className="cp-metric-passport__value" view="outlined">
              <h2>{title}</h2>
              {definition && definition.name !== title && (
                <p>{definition.name}</p>
              )}
              {definition && (
                <>
                  <p>
                    {definition.metric_id} · Определение версии{' '}
                    {definition.definition_version}
                    {href && (
                      <>
                        {' '}
                        · <a href={href}>Каталог показателей</a>
                      </>
                    )}
                  </p>
                  <p>Единица измерения: {definition.unit}</p>
                </>
              )}
              {formula && <p>{formula}</p>}
              {!formula && href && (
                <p>Формула приведена в каталоге показателей.</p>
              )}
              {observationDays !== undefined && (
                <p>
                  Окно наблюдения:{' '}
                  <span>{observationDays} дн. после первого завершения</span>
                </p>
              )}
              {catalog.isError && (
                <ErrorState
                  reason="Не удалось загрузить определение"
                  onRetry={() => void catalog.refetch()}
                  retrying={catalog.isFetching}
                />
              )}
              {point.data?.estimator && (
                <p>Оценка ряда: {point.data.estimator}</p>
              )}
              {selection.point && (
                <p>
                  Начало точки{' '}
                  <time dateTime={selection.point}>{selection.point}</time>
                </p>
              )}
              {selection.validPoint &&
                data.published_at &&
                (point.isPending ? (
                  <LoadingState />
                ) : point.isError ? (
                  <ErrorState
                    reason="Не удалось загрузить значение точки"
                    onRetry={() => void point.refetch()}
                    retrying={point.isFetching}
                  />
                ) : (
                  <>
                    <RestrictedResults
                      access_limited={point.data?.accessLimited ?? false}
                    />
                    {point.data?.value ? (
                      <MetricValue
                        value={point.data.value}
                        showBreakdown
                        {...(selection.metricId === 'T23'
                          ? { excludedLabel: 'Незрелые задачи' }
                          : {})}
                      />
                    ) : (
                      <p>Значение точки недоступно.</p>
                    )}
                  </>
                ))}
              {selection.validPoint &&
                data.published_at &&
                !data.irreproducible_reason && (
                  <Link
                    to={metricDetailsLink(selection, 'metricCohort')}
                    state={depth === 1 ? { reportHistoryDepth: 2 } : undefined}
                  >
                    Открыть когорту точки
                  </Link>
                )}
            </Card>
            <RevisionInputs
              revision={data}
              step={point.data?.step ?? selection.search.get('step')}
            />
          </>
        )
      )}
    </section>
  )
}
