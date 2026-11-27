import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Navigate, useOutletContext } from 'react-router'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import type { TeamReportScope } from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'
import { workflowApi } from '../api/workflow-api'
import { accessLost, WorkflowEditor } from './workflow-editor'
import './workflow-settings.css'

type Team = NonNullable<TeamReportScope['team']>
type WorkflowScope = Pick<
  TeamReportScope,
  'team' | 'canManageProcess' | 'projects'
> & { teams?: readonly Team[] }

export function WorkflowSettingsPage({ api }: { api: ApiClient }) {
  const scope = useOutletContext<WorkflowScope>()
  if (!scope.canManageProcess || !scope.team)
    return <Navigate to={routePaths.unavailable} replace />
  return (
    <WorkflowSettings
      key={scope.team.id}
      api={api}
      team={scope.team}
      teams={scope.teams ?? [scope.team]}
      projects={scope.projects}
    />
  )
}

function WorkflowSettings({
  api,
  team,
  teams,
  projects,
}: {
  api: ApiClient
  team: Team
  teams: readonly Team[]
  projects: WorkflowScope['projects']
}) {
  const resource = workflowApi(api, team.id)
  const query = useQuery({
    queryKey: ['workflow-settings', team.id],
    queryFn: ({ signal }) => resource.load(signal),
    retry: false,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  if (accessLost(query.error))
    return <UnavailableRedirect error={query.error} />

  return (
    <section className="cp-workflow-page">
      <Text as="h1" variant="header-1">
        Настройки процесса
      </Text>
      <p>
        Команда {team.name}
        {projects.length
          ? ` · Проекты: ${projects.map((project) => project.name).join(', ')}`
          : ''}
      </p>
      {query.isPending && <LoadingState />}
      {query.error && (
        <ErrorState
          reason="Не удалось загрузить справочники и версии команды"
          {...(query.error instanceof ApiError && query.error.problem
            ? { correlationId: query.error.problem.correlation_id }
            : {})}
          onRetry={() => {
            void query.refetch()
          }}
          retrying={query.isFetching}
        />
      )}
      {query.data && !query.error && (
        <WorkflowEditor
          resource={resource}
          data={query.data}
          team={team}
          teams={teams}
        />
      )}
    </section>
  )
}
