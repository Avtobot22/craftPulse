import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Link, useOutletContext, useParams } from 'react-router'
import { allTeamProjects, ApiError, loadRevisionMetadata } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { routePaths, writeReportParams } from '@/shared/config'
import type { ReportParams, TeamReportScope } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'
import { getSavedView, savedViewKey } from '../api/saved-views'
import type { OpenedSavedView } from '../api/saved-views'
import { useReportPointNavigation } from '../model/use-report-point-navigation'
import { CopySavedView } from './copy-saved-view'
import { ReportBlocks } from './report-blocks'
import { ReportRevision } from './report-revision'
import { SavedViewFilters } from './saved-view-filters'
import './team-report.css'

export function SavedViewPage({ api }: { api: ApiClient }) {
  const { savedViewId = '' } = useParams()
  const view = useQuery({
    queryKey: savedViewKey(savedViewId),
    queryFn: ({ signal }) => getSavedView(api, savedViewId, signal),
    refetchOnMount: 'always',
  })
  const unavailable =
    view.error instanceof ApiError &&
    [403, 404].includes(view.error.problem?.status ?? 0)
  return (
    <section className="cp-team-report">
      <Text as="h1" variant="header-1">
        Отчёт команды
      </Text>
      <Link to={routePaths.savedViews}>Сохранённые срезы</Link>
      {unavailable ? (
        <p role="alert">Сохранённый срез недоступен</p>
      ) : view.isError ? (
        <ErrorState
          reason="Не удалось открыть срез"
          onRetry={() => void view.refetch()}
        />
      ) : view.data && view.isFetchedAfterMount ? (
        <SavedViewReport
          key={savedViewId}
          api={api}
          view={view.data}
          readAt={view.dataUpdatedAt}
          isReading={view.isFetching}
          onRead={() => void view.refetch()}
        />
      ) : (
        <LoadingState />
      )}
    </section>
  )
}

function SavedViewReport({
  api,
  view,
  readAt,
  isReading,
  onRead,
}: {
  api: ApiClient
  view: OpenedSavedView
  readAt: number
  isReading: boolean
  onRead: () => void
}) {
  const { report, filters } = view
  const scope = useOutletContext<CurrentUser & TeamReportScope>()
  const params: ReportParams = {
    ...report.period,
    step: view.step,
    projects: filters.project_id ?? [],
    taskType: filters.task_type ?? '',
    size: filters.size_label ?? '',
    groupBy: filters.group_by ?? '',
    revision: '',
  }
  const search = writeReportParams(
    new URLSearchParams({ team_id: report.team_id }),
    params,
  )
  const latest = { pathname: routePaths.teamReport, search: search.toString() }
  const selectPoint = useReportPointNavigation(
    search,
    params.groupBy,
    report.revision.id,
    scope.rememberPoint,
  )
  const revision = useQuery({
    queryKey: ['team-report', 'revision', report.revision.id],
    queryFn: ({ signal }) =>
      loadRevisionMetadata(api, report.revision.id, signal),
  })
  const projects = useQuery({
    ...allTeamProjects(
      api,
      `craftpulse.scope.${scope.organization.id}.${scope.user.id}`,
      report.team_id,
    ),
    enabled: params.projects.length > 0,
    refetchOnMount: 'always',
  })
  const metadata = revision.isError ? undefined : revision.data
  const isPinned = view.pinned_revision_id !== null
  const projectChoices =
    projects.isFetchedAfterMount && !projects.isError
      ? projects.data
      : undefined
  return (
    <>
      <div className="cp-team-report__panel">
        <Text as="h2" variant="subheader-2">
          {view.name}
        </Text>
        <SavedViewFilters
          {...params}
          groupBy={filters.group_by}
          projects={params.projects.map(
            (id) =>
              projectChoices?.find((project) => project.id === id)?.name ??
              'Недоступный проект',
          )}
        />
        <CopySavedView id={view.id} name={view.name} />
      </div>
      <ReportRevision
        api={api}
        report={report}
        metadata={metadata}
        isHistorical={isPinned}
        isReading={isReading}
        onRead={onRead}
        savedViewLatest={latest}
      />
      {report.sourceAccessLost && (
        <p role="status">Доступ к одному или нескольким источникам потерян.</p>
      )}
      {report.metrics.length === 0 && (
        <p>Нет данных за выбранный период в доступной области.</p>
      )}
      <ReportBlocks
        api={api}
        canReadVersions={false}
        block={{
          report,
          // Read details again with the permissions in effect for this opening.
          detailsCacheKey: view.id + ':' + String(readAt),
          metadata,
          groupBy: params.groupBy,
          focus: scope.focus,
          projectIds: filters.project_id,
          taskType: filters.task_type,
          size: filters.size_label,
          selectPoint,
        }}
      />
    </>
  )
}
