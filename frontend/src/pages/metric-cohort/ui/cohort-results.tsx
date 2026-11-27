import { Button } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router'
import { isRevisionAccessDenied, useMetricPoint } from '@/entities/metric'
import type { MetricInspection } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError } from '@/shared/api'
import type { ApiClient, MetricRevision } from '@/shared/api'
import {
  technicalCohortTaskType,
  technicalReportFilters,
} from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'
import { loadCohort, parseRole } from '@/shared/api'
import { readCohortPagination } from '../model/pagination'
import { matchesReportSlice } from '../model/scope'
import { CohortTable } from './cohort-table'

export function CohortResults({
  api,
  selection,
  revision,
}: {
  api: ApiClient
  selection: MetricInspection
  revision: MetricRevision
}) {
  const returnState: unknown = useLocation().state
  const [pagination, setPagination] = useState(() =>
    readCohortPagination(returnState),
  )
  const { limit, cursors, page } = pagination
  const focusPage = useRef(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const role = parseRole(selection.search.get('role'))
  const projectIds = selection.project
    ? [selection.project]
    : selection.params.projects
  const { params, group } = selection
  const taskType =
    params.groupBy === 'task_type' && group !== null
      ? group || null
      : params.taskType
  const size =
    params.groupBy === 'size_label' && group !== null
      ? group || 'unknown'
      : params.size
  const technical = /^X0[3-6]$/u.test(selection.metricId)
  const cursor = cursors[page - 1]
  const query = {
    metric_id: selection.metricId,
    point_start: selection.point,
    ...(selection.estimator ? { estimator: selection.estimator } : {}),
    ...(role ? { role } : {}),
    ...(projectIds.length ? { project_id: projectIds } : {}),
    ...(technical ? technicalCohortTaskType(selection.metricId, taskType) : {}),
    ...(technical && size ? { size_label: size } : {}),
    ...(selection.metricId === 'X03' ? technicalReportFilters(params) : {}),
    limit,
    ...(cursor ? { cursor } : {}),
  }
  const result = useQuery({
    queryKey: ['revision-cohort', revision.id, query],
    queryFn: ({ signal }) => loadCohort(api, revision.id, query, signal),
    gcTime: 0,
  })
  const point = useMetricPoint(api, selection, revision)
  const data = result.isError ? undefined : result.data
  const next =
    data?.next && !cursors.slice(0, page).includes(data.next)
      ? data.next
      : undefined
  const items =
    data?.items.filter((item) => matchesReportSlice(item, selection)) ?? []
  const limited =
    data?.accessLimited === true || point.data?.accessLimited === true
  const definition = revision.definitions.find(
    ({ metric_id }) => metric_id === selection.metricId,
  )
  const value = point.isError ? undefined : point.data?.value
  const metricUnit = value?.unit ?? definition?.unit ?? ''
  const contributionUnit =
    selection.metricId === 'T15' || selection.metricId === 'X01'
      ? 'seconds'
      : selection.metricId === 'T11'
        ? 'ratio'
        : metricUnit === 'ratio'
          ? 'count'
          : metricUnit

  useEffect(() => {
    if (focusPage.current && !result.isPending) {
      heading.current?.focus()
      focusPage.current = false
    }
  }, [limit, page, result.isPending])

  if (
    isRevisionAccessDenied(result.error) ||
    isRevisionAccessDenied(point.error)
  )
    return (
      <UnavailableRedirect
        error={
          isRevisionAccessDenied(result.error) ? result.error : point.error
        }
      />
    )

  const diagnostic =
    result.error instanceof ApiError
      ? result.error.problem?.correlation_id
      : undefined
  return (
    <section aria-labelledby="cohort-results">
      <h2 id="cohort-results" tabIndex={-1} ref={heading}>
        Объекты точки
      </h2>
      {result.isPending ? (
        <LoadingState />
      ) : result.isError ? (
        <>
          <ErrorState
            reason="Не удалось загрузить страницу когорты"
            {...(diagnostic ? { correlationId: diagnostic } : {})}
            onRetry={() => void result.refetch()}
            retrying={result.isFetching}
          />
          {page > 0 && (
            <Button
              size="xl"
              onClick={() => {
                focusPage.current = true
                setPagination({ limit, page: 0, cursors: [] })
              }}
            >
              Загрузить когорту с начала
            </Button>
          )}
        </>
      ) : (
        <>
          <RestrictedResults access_limited={limited} />
          {!limited && value && (
            <p>
              Числитель: {value.numerator ?? 'нет данных'}. Знаменатель:{' '}
              {value.denominator ?? 'нет данных'}.
            </p>
          )}
          {items.length > 0 ? (
            <CohortTable
              key={page}
              api={api}
              items={items}
              unit={contributionUnit}
              asOf={revision.as_of}
              publishedAt={revision.published_at}
              taskParams={{
                revision_id: revision.id,
                as_of: revision.as_of,
                from: selection.search.get('from') ?? revision.window.from,
                to: selection.search.get('to') ?? revision.window.to,
                ...(selection.teamId ? { team_id: selection.teamId } : {}),
                ...(selection.project ? { project_id: selection.project } : {}),
              }}
              pagination={pagination}
            />
          ) : (
            <p role="status">
              {limited
                ? 'На странице нет доступных объектов.'
                : page > 0 ||
                    next ||
                    role ||
                    selection.group !== null ||
                    selection.params.taskType ||
                    selection.params.size
                  ? 'На странице нет объектов по выбранным фильтрам.'
                  : 'В точке нет объектов.'}
            </p>
          )}
        </>
      )}
      <div
        className="cp-cohort__pagination"
        role="group"
        aria-label="Страницы когорты"
      >
        <label>
          Строк на странице{' '}
          <select
            value={limit}
            onChange={(event) => {
              const size = Number(event.currentTarget.value)
              if (size !== 25 && size !== 50 && size !== 100) return
              setPagination({ limit: size, page: 0, cursors: [] })
              focusPage.current = true
            }}
          >
            <option value={25}>25</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>
        </label>
        <Button
          size="xl"
          disabled={page === 0 || result.isPending}
          onClick={() => {
            setPagination({ ...pagination, page: page - 1 })
            focusPage.current = true
          }}
        >
          Предыдущая страница
        </Button>
        <span role="status">
          Страница {page + 1}
          {data && !limited ? ' · получено ' + String(items.length) : ''}
        </span>
        <Button
          size="xl"
          disabled={!next || result.isPending}
          onClick={() => {
            if (!next) return
            setPagination({
              limit,
              page: page + 1,
              cursors: [...cursors.slice(0, page), next],
            })
            focusPage.current = true
          }}
        >
          Следующая страница
        </Button>
      </div>
    </section>
  )
}
