import { Button } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { Navigate } from 'react-router'
import { isRevisionAccessDenied, useMetricPoint } from '@/entities/metric'
import type { MetricInspection } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError } from '@/shared/api'
import type { ApiClient, MetricRevision } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'
import { loadCohort, parseRole } from '../api/cohort-api'
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
  const [limit, setLimit] = useState<25 | 50 | 100>(25)
  const [cursors, setCursors] = useState<string[]>([])
  const [page, setPage] = useState(0)
  const focusPage = useRef(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const role = parseRole(selection.search.get('role'))
  const projectIds = selection.project
    ? [selection.project]
    : selection.params.projects
  const cursor = cursors[page - 1]
  const query = {
    metric_id: selection.metricId,
    point_start: selection.point,
    ...(selection.estimator ? { estimator: selection.estimator } : {}),
    ...(role ? { role } : {}),
    ...(projectIds.length ? { project_id: projectIds } : {}),
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
    selection.metricId === 'T15'
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
    return <Navigate to={routePaths.unavailable} replace />

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
                setPage(0)
                setCursors([])
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
              setLimit(size)
              setPage(0)
              setCursors([])
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
            setPage(page - 1)
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
            setCursors([...cursors.slice(0, page), next])
            setPage(page + 1)
            focusPage.current = true
          }}
        >
          Следующая страница
        </Button>
      </div>
    </section>
  )
}
