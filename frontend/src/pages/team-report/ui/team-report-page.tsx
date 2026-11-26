import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import {
  generatePath,
  Link,
  Navigate,
  useLocation,
  useNavigate,
  useOutletContext,
  useSearchParams,
} from 'react-router'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths, teamReportReturnState } from '@/shared/config'
import type { TeamReportScope } from '@/shared/config'
import { EmptyState, ErrorState, LoadingState, StaleState } from '@/shared/ui'
import {
  loadReport,
  loadRevisionMetadata,
  RevisionNotPublished,
} from '../api/report-api'
import type { TeamReport } from '../api/report-api'
import { seriesKey } from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'
import {
  periodError,
  readReportParams,
  reportQuery,
  writeReportParams,
} from '../model/report-params'
import type { ReportParams } from '../model/report-params'
import { CompletenessBlock } from './completeness-block'
import { CycleTimeBlock } from './cycle-time-block'
import { WorkAgeBlock } from './work-age-block'
import { ReopeningsBlock, TargetComplianceBlock } from './outcome-metrics-block'
import { FlowBalanceBlock } from './flow-balance-block'
import { ReportControls } from './report-controls'
import { ReportRevision } from './report-revision'
import { ThroughputBlock } from './throughput-block'
import './team-report.css'

function isAccessFailure(error: unknown) {
  return (
    error instanceof ApiError &&
    (['forbidden', 'not-found', 'auth-required', 'session-ended'].includes(
      error.kind,
    ) ||
      [401, 403, 404].includes(error.problem?.status ?? 0))
  )
}

function metricPoints(report: TeamReport, metric: string) {
  return report.metrics
    .filter(({ metric_id }) => metric_id === metric)
    .flatMap(({ points }) => points)
}

function availability(report: TeamReport, groupedWithoutMapping: boolean) {
  const values = report.metrics.flatMap(({ points }) =>
    points.map(({ value }) => value),
  )
  if (
    (groupedWithoutMapping && values.length === 0) ||
    values.some(
      ({ state, reason_code }) =>
        state === 'NOT_EVALUATED' && reason_code === 'mapping_missing',
    )
  )
    return 'mapping'
  const freshness = metricPoints(report, 'T40').filter(
    ({ value }) => value.reason_code !== 'as_of_only',
  )
  const history = metricPoints(report, 'T39')
  const hasTasks = history.some(
    ({ value }) => (value.denominator ?? value.n ?? 0) > 0,
  )
  if (
    !report.access_limited &&
    (values.some(
      ({ reason_code }) => reason_code === 'source_not_configured',
    ) ||
      (!hasTasks &&
        freshness.length > 0 &&
        freshness.every(
          ({ value }) => value.state === 'NOT_APPLICABLE' && value.n === 0,
        ) &&
        !values.some(({ value }) => value !== null && value !== 0)))
  )
    return 'source'
  if (
    !values.some(({ value, bound }) => value !== null || bound !== null) &&
    (report.revision.loading ||
      values.some(({ reason_code }) => reason_code === 'history_incomplete'))
  )
    return 'history'
  if (
    !report.revision.loading &&
    !report.access_limited &&
    (values.length === 0 ||
      (history.length > 0 &&
        history.every(
          ({ value }) =>
            value.denominator === 0 ||
            (value.state === 'NOT_APPLICABLE' && value.n === 0),
        )))
  )
    return 'filters'
  if (
    values.length > 0 &&
    values.every(({ state }) => state === 'NOT_APPLICABLE')
  )
    return 'inapplicable'
  return 'data'
}

export function TeamReportPage({ api }: { api: ApiClient }) {
  const scope = useOutletContext<TeamReportScope>()
  const [search] = useSearchParams()
  const params = readReportParams(search)
  params.revision ||= scope.focus?.revisionId ?? ''
  const needsDefaults =
    !search.has('from') ||
    !search.has('to') ||
    search.get('step') !== params.step
  return (
    <section className="cp-team-report">
      <Text as="h1" variant="header-1">
        Отчёт команды
      </Text>
      {!scope.team ? (
        <p>Выберите команду в области отчёта.</p>
      ) : needsDefaults ? (
        <Navigate
          replace
          to={{ search: writeReportParams(search, params).toString() }}
        />
      ) : (
        <TeamReportContent
          api={api}
          scope={scope}
          teamId={scope.team.id}
          params={params}
          search={search}
        />
      )}
    </section>
  )
}

function TeamReportContent({
  api,
  scope,
  teamId,
  params,
  search,
}: {
  api: ApiClient
  scope: TeamReportScope
  teamId: string
  params: ReportParams
  search: URLSearchParams
}) {
  const [, setSearch] = useSearchParams()
  const navigate = useNavigate()
  const location = useLocation()
  const restoredFocus = useRef<string | undefined>(undefined)
  const query = reportQuery(params, scope.project?.id)
  const invalidPeriod = periodError(params)
  const report = useQuery({
    queryKey: ['team-report', teamId, query],
    queryFn: ({ signal }) => loadReport(api, teamId, query, signal),
    enabled: !invalidPeriod,
  })
  const accessError = isAccessFailure(report.error)
  const readError = report.isError
  const awaitingPublication = report.error instanceof RevisionNotPublished
  const transientFailure =
    report.error instanceof ApiError &&
    (report.error.kind === 'network' ||
      (report.error.problem?.status ?? 0) >= 500)
  const snapshot =
    (!readError || transientFailure || awaitingPublication) &&
    report.isFetchedAfterMount
      ? report.data
      : undefined

  const revisionId = snapshot?.revision.id
  const revision = useQuery({
    queryKey: ['team-report', 'revision', revisionId],
    queryFn: ({ signal }) =>
      loadRevisionMetadata(api, revisionId ?? '', signal),
    enabled: Boolean(revisionId),
  })
  const metadata = revision.isError ? undefined : revision.data
  const readingMetadata = Boolean(revisionId) && revision.isPending
  const unsafeMetadata = isAccessFailure(revision.error)
  const historicalStep =
    params.revision && (snapshot?.step === 'week' || snapshot?.step === 'month')
      ? snapshot.step
      : undefined
  const visible =
    Boolean(snapshot) && !readingMetadata && !unsafeMetadata && !invalidPeriod
  const focusedPoint = scope.focus?.targetId

  const denial = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (accessError || unsafeMetadata) denial.current?.focus()
  }, [accessError, unsafeMetadata])

  useEffect(() => {
    if (!visible || !focusedPoint || restoredFocus.current === location.key)
      return
    const fallback = document.getElementById('team-report-results')
    function restore(immediate: boolean) {
      if (!focusedPoint) return false
      const target = document.getElementById(focusedPoint)
      if (!target) return false
      if (
        immediate ||
        document.activeElement === fallback ||
        document.activeElement === document.body
      )
        target.focus()
      restoredFocus.current = location.key
      return true
    }
    const observer = new MutationObserver(() => {
      if (restore(false)) observer.disconnect()
    })
    const frame = requestAnimationFrame(() => {
      if (restore(true) || !fallback) return
      fallback.focus()
      observer.observe(fallback.parentElement ?? fallback, {
        childList: true,
        subtree: true,
      })
    })
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [focusedPoint, location.key, visible])

  function apply(next: ReportParams) {
    const updated = writeReportParams(search, next)
    if (updated.toString() !== search.toString()) setSearch(updated)
  }
  function reset() {
    apply({ ...params, projects: [], taskType: '', size: '', groupBy: '' })
  }

  const hasProjectFilter = (query.project_id?.length ?? 0) > 0
  const hasFilters =
    hasProjectFilter ||
    Boolean(query.task_type ?? query.size_label ?? query.group_by)
  const controls = (
    <ReportControls
      key={JSON.stringify([teamId, scope.project?.id])}
      applied={historicalStep ? { ...params, step: historicalStep } : params}
      projects={scope.projects}
      scopeProject={scope.project}
      onApply={(next) => {
        apply({ ...next, step: params.step, revision: params.revision })
      }}
      onReset={reset}
      onStep={(step) => {
        apply({ ...params, step, revision: '' })
      }}
    />
  )
  const retry = () => void report.refetch()
  const error = report.error instanceof ApiError ? report.error : undefined
  const diagnostic = error?.problem?.correlation_id
  const readFailure = (
    <ErrorState
      reason={
        awaitingPublication
          ? 'Ревизия ещё не опубликована'
          : accessError
            ? 'Отчёт или ревизия недоступны'
            : 'Не удалось прочитать отчёт команды'
      }
      retrying={report.isFetching}
      {...(diagnostic ? { correlationId: diagnostic } : {})}
      onRetry={retry}
    />
  )
  if (historicalStep && historicalStep !== params.step)
    return (
      <Navigate
        replace
        to={{
          search: writeReportParams(search, {
            ...params,
            step: historicalStep,
          }).toString(),
        }}
      />
    )
  if (invalidPeriod)
    return (
      <>
        {controls}
        <p role="alert">{invalidPeriod}</p>
      </>
    )
  if (!snapshot || readingMetadata || unsafeMetadata)
    return (
      <>
        {controls}
        {unsafeMetadata || accessError ? (
          <div ref={denial} tabIndex={-1}>
            {unsafeMetadata ? (
              <ErrorState
                reason="Ревизия отчёта недоступна"
                onRetry={() => void revision.refetch()}
              />
            ) : (
              readFailure
            )}
          </div>
        ) : readError ? (
          readFailure
        ) : (
          <ReportLoading />
        )}
      </>
    )

  const block: ReportBlockProps = {
    report: snapshot,
    metadata,
    groupBy: params.groupBy,
    focus: scope.focus,
    projectIds: query.project_id,
    taskType: query.task_type,
    size: query.size_label,
    selectPoint: (series, pointStart, targetId, chartSeries, cohort) => {
      scope.rememberPoint({
        locationKey: location.key,
        targetId,
        seriesId: seriesKey(series),
        pointStart,
        revisionId: snapshot.revision.id,
        ...(chartSeries ? { chartSeries } : {}),
      })
      const details = new URLSearchParams(search)
      details.set('point_start', pointStart)
      details.delete('estimator')
      details.delete('group_value')
      if (series.estimator) details.set('estimator', series.estimator)
      if (series.group && params.groupBy)
        details.set('group_value', series.group[params.groupBy] ?? '')
      if (cohort) {
        details.set('size_label', cohort.size ?? 'unknown')
        if (cohort.taskType === null) {
          details.delete('task_type')
          details.set('group_by', 'task_type')
          details.set('group_value', '')
        } else {
          if (cohort.taskType !== undefined)
            details.set('task_type', cohort.taskType)
          details.delete('group_by')
          details.delete('group_value')
        }
      }
      void navigate(
        {
          pathname: generatePath(routePaths.metricPassport, {
            revisionId: snapshot.revision.id,
            metricId: series.metric_id,
          }),
          search: details.toString(),
        },
        { state: teamReportReturnState },
      )
    },
  }
  const state = availability(
    snapshot,
    Boolean(query.group_by) && metadata?.hasMapping === false,
  )
  const results = (
    <>
      <div id="team-report-results" tabIndex={-1}>
        <ReportRevision
          key={snapshot.revision.id}
          api={api}
          report={snapshot}
          metadata={metadata}
          isHistorical={Boolean(params.revision)}
          isReading={report.isFetching}
          onLatest={() => {
            scope.rememberPoint(undefined)
            apply({ ...params, revision: '' })
          }}
          onRead={() => {
            if (revision.isError) void revision.refetch()
            retry()
          }}
        />
      </div>
      {awaitingPublication && (
        <p role="status">Новая ревизия ещё не опубликована.</p>
      )}
      {snapshot.sourceAccessLost && (
        <p role="status">Доступ к одному или нескольким источникам потерян.</p>
      )}
      {state === 'mapping' && (
        <div role="status" className="cp-team-report__panel">
          <p>
            Не проверено (NOT_EVALUATED). Сопоставление статусов не
            подтверждено.
          </p>
          {scope.canManageProcess && (
            <Link
              to={{
                pathname: routePaths.workflowSettings,
                search: new URLSearchParams({
                  team_id: teamId,
                  ...(scope.project ? { project_id: scope.project.id } : {}),
                }).toString(),
              }}
            >
              Настроить сопоставление
            </Link>
          )}
        </div>
      )}
      {state !== 'data' && state !== 'mapping' && (
        <EmptyState
          reason={state}
          {...(state === 'filters'
            ? {
                description:
                  'Нет задач за выбранный период в доступной области.',
                ...(hasFilters
                  ? { action: { label: 'Сбросить фильтры', onClick: reset } }
                  : {}),
              }
            : {})}
        />
      )}
      {snapshot.metrics.length > 0 && (
        <div className="cp-team-report__blocks" key={snapshot.revision.id}>
          <ThroughputBlock block={block} api={api} />
          <CycleTimeBlock block={block} api={api} />
          <FlowBalanceBlock block={block} />
          <WorkAgeBlock block={block} api={api} />
          <TargetComplianceBlock
            block={block}
            api={api}
            canReadVersions={scope.canManageProcess}
          />
          <ReopeningsBlock block={block} />
          <CompletenessBlock block={block} />
        </div>
      )}
    </>
  )
  return (
    <>
      {controls}
      {readError && !awaitingPublication ? (
        <StaleState
          snapshotAt={snapshot.revision.published_at ?? snapshot.revision.as_of}
          currentError={readFailure}
        >
          {results}
        </StaleState>
      ) : (
        results
      )}
    </>
  )
}

function ReportLoading() {
  return (
    <div className="cp-team-report__loading">
      <LoadingState />
    </div>
  )
}
