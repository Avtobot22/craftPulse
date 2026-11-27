import { Button, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import {
  generatePath,
  Link,
  useLocation,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import { isUnavailableError, listBaselines } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  baselineContext,
  readTechnicalId,
  routePaths,
  technicalScope,
} from '@/shared/config'
import {
  ErrorState,
  InspectionBack,
  InspectionLink,
  LoadingState,
  UnavailableRedirect,
  useInspectionReturn,
} from '@/shared/ui'
import { BaselineMatrix, BaselineReport } from './baseline-matrix'
import './baselines.css'

export function BaselinesPage({ api }: { api: ApiClient }) {
  const route = useParams()
  const state: unknown = useLocation().state
  const fromInspection =
    state !== null &&
    typeof state === 'object' &&
    'fromInspection' in state &&
    state.fromInspection === true
  const [search] = useSearchParams()
  const { actions, areas } = useOutletContext<CurrentUser>()
  const projectId = readTechnicalId(route.projectId ?? search.get('project_id'))
  const context = baselineContext(search)
  const matrixPath = generatePath(routePaths.baselines, {
    projectId: projectId ?? null,
  })
  const reportId = readTechnicalId(route.reportId)
  if (projectId) context.set('project_id', projectId)
  else context.delete('project_id')

  return (
    <article className="cp-baselines">
      <Text as="h1" id="baselines-heading" tabIndex={-1} variant="header-1">
        {route.reportId === undefined ? 'Базовые линии' : 'Отчёт базовой линии'}
      </Text>
      <div>
        <InspectionBack
          {...(route.reportId !== undefined
            ? {
                fallbackTo: {
                  pathname: matrixPath,
                  search: context.toString(),
                },
              }
            : {})}
        />
      </div>
      {route.reportId !== undefined && !reportId ? (
        <p role="alert">Некорректный ID отчёта.</p>
      ) : projectId ? (
        <BaselineContent
          key={JSON.stringify([projectId, reportId])}
          api={api}
          projectId={projectId}
          reportId={reportId}
          context={context}
          actions={actions}
        />
      ) : (
        <section aria-labelledby="baseline-project-heading">
          {route.projectId && <p role="alert">Некорректный ID проекта.</p>}
          <h2 id="baseline-project-heading">Проект</h2>
          {areas.projects.length === 0 ? (
            <p>Нет доступных проектов.</p>
          ) : (
            <ul>
              {areas.projects.map(({ id, name }) => (
                <li key={id}>
                  <Link
                    replace={fromInspection}
                    state={fromInspection ? state : null}
                    to={{
                      pathname: generatePath(routePaths.baselines, {
                        projectId: id,
                      }),
                      search: context.toString(),
                    }}
                  >
                    {name}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </article>
  )
}

function BaselineContent({
  api,
  projectId,
  reportId,
  context,
  actions,
}: {
  api: ApiClient
  projectId: string
  reportId: string | undefined
  context: URLSearchParams
  actions: CurrentUser['actions']
}) {
  const query = useQuery({
    ...listBaselines(api, projectId),
    refetchOnMount: 'always',
  })
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const ready = query.isSuccess && !waiting
  useInspectionReturn(ready, 'baselines-heading')

  if (waiting) return <LoadingState />
  if (query.isError) {
    if (isUnavailableError(query.error))
      return <UnavailableRedirect error={query.error} />
    return (
      <ErrorState
        reason="Не удалось загрузить базовые линии"
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }

  if (reportId) {
    const rows = query.data.data.filter(
      ({ report_id }) => report_id === reportId,
    )
    return rows.length ? (
      <BaselineReport reportId={reportId} rows={rows} />
    ) : (
      <p role="status">Отчёт базовой линии недоступен.</p>
    )
  }

  const scope = technicalScope(context)
  const mr = context.get('mr_id')
  const summary = new URLSearchParams(scope)
  for (const key of ['revision_no', 'head_sha']) {
    const value = context.get(key)
    if (value) summary.set(key, value)
  }

  return (
    <>
      <div className="cp-baselines__controls">
        <Button
          size="l"
          aria-disabled={query.isFetching}
          onClick={() => {
            if (!query.isFetching) void query.refetch()
          }}
        >
          Обновить
        </Button>
        {actions.includes('check_profiles.manage') && (
          <InspectionLink
            id="baseline-profile"
            to={{
              pathname: generatePath(routePaths.checkProfile, { projectId }),
              search: scope.toString(),
            }}
          >
            Матрица проверок
          </InspectionLink>
        )}
        {actions.includes('mr.read') && (
          <InspectionLink
            id="baseline-summary"
            to={{
              pathname: mr
                ? generatePath(routePaths.mergeRequestSummary, { mr })
                : routePaths.mergeRequestQueue,
              search: (mr ? summary : scope).toString(),
            }}
          >
            {mr ? 'Открыть сводку MR' : 'Выбрать MR для просмотра сводки'}
          </InspectionLink>
        )}
      </div>
      <div className="cp-baselines__sr-only" role="status">
        {query.isFetching
          ? 'Обновляются базовые линии'
          : 'Базовые линии загружены'}
      </div>
      <BaselineMatrix
        rows={query.data.data}
        projectId={projectId}
        context={context}
        canReadFindings={actions.includes('findings.read')}
      />
    </>
  )
}
