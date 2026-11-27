import { useQuery } from '@tanstack/react-query'
import { generatePath, useParams, useSearchParams } from 'react-router'
import {
  ApiError,
  loadMetricRevision,
  loadReport,
  loadRevisionSeries,
} from '@/shared/api'
import type { ApiClient, MetricRevision, ReportSeries } from '@/shared/api'
import { readReportParams, reportQuery, routePaths } from '@/shared/config'
import { isDateTime } from '@/shared/lib/date-time'

export function useMetricInspection() {
  const { revisionId = '', metricId = '', pointStart } = useParams()
  const [search] = useSearchParams()
  const point = pointStart ?? search.get('point_start') ?? ''
  const params = readReportParams(search)
  const group = search.get('group_value')
  const teamId = search.get('team_id')
  const project = search.get('project_id')
  return {
    revisionId,
    metricId,
    point,
    validPoint: isDateTime(point),
    search,
    teamId,
    project,
    params,
    group,
    estimator: search.get('estimator') ?? undefined,
  }
}

export type MetricInspection = ReturnType<typeof useMetricInspection>

export function useMetricRevision(api: ApiClient, id: string) {
  return useQuery({
    queryKey: ['metric-revision', id],
    queryFn: ({ signal }) => loadMetricRevision(api, id, signal),
    enabled: Boolean(id),
    gcTime: 0,
  })
}

export function useMetricPoint(
  api: ApiClient,
  selection: MetricInspection,
  revision: MetricRevision | undefined,
) {
  const { metricId, point, teamId, params, project, group, estimator, search } =
    selection
  const query = reportQuery(
    {
      ...params,
      from: search.get('from') ?? revision?.window.from ?? params.from,
      to: search.get('to') ?? revision?.window.to ?? params.to,
      revision: selection.revisionId,
    },
    project ?? undefined,
  )

  return useQuery({
    queryKey: [
      'metric-inspection-point',
      revision?.series_id,
      teamId,
      query,
      metricId,
      point,
      group,
      estimator,
    ],
    enabled: Boolean(revision?.published_at) && selection.validPoint,
    gcTime: 0,
    queryFn: async ({ signal }) => {
      if (!revision) return null
      let metrics: ReportSeries[]
      let step: string | null
      let accessLimited: boolean
      if (teamId) {
        const report = await loadReport(api, teamId, query, signal)
        metrics = report.metrics
        step = report.step
        accessLimited = report.access_limited
      } else {
        const series = await loadRevisionSeries(
          api,
          revision.series_id,
          metricId,
          revision.id,
          signal,
        )
        metrics = series.metrics
        step = search.get('step')
        accessLimited = series.accessLimited
      }
      const matching = metrics.filter(
        (series) =>
          series.metric_id === metricId &&
          (estimator === undefined ||
            (series.estimator ?? series.group?.signal) === estimator) &&
          (params.groupBy && group !== null
            ? (series.group?.[params.groupBy] ?? '') === group &&
              series.group !== undefined
            : !series.group ||
              (series.group.signal !== undefined &&
                series.group.task_type === undefined &&
                series.group.size_label === undefined)),
      )
      const selected = matching[0]
      const value = selected?.points.find(
        ({ point_start }) => Date.parse(point_start) === Date.parse(point),
      )?.value
      return {
        step,
        accessLimited,
        // An assessed T05 is the exact count of numerator tasks. The API
        // stores the count in value rather than repeating it in numerator.
        value:
          metricId === 'T05' && value?.state === 'ASSESSED'
            ? { ...value, numerator: value.numerator ?? value.value }
            : value,
        estimator: selected?.estimator,
        definitionVersion: selected?.definition_version,
      }
    },
  })
}

export function isRevisionAccessDenied(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.kind === 'forbidden' ||
      error.kind === 'not-found' ||
      [403, 404].includes(error.problem?.status ?? 0))
  )
}

export function metricDetailsLink(
  selection: MetricInspection,
  page: 'metricPassport' | 'metricCohort',
  revisionId = selection.revisionId,
) {
  const search = new URLSearchParams(selection.search)
  search.delete('role')
  if (page === 'metricPassport' && selection.point)
    search.set('point_start', selection.point)
  else search.delete('point_start')

  return {
    pathname: generatePath(routePaths[page], {
      revisionId: revisionId,
      metricId: selection.metricId,
      pointStart: selection.point,
    }),
    search: search.toString(),
  }
}
