import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { allTeamProjects, allTeams } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { projectParam, readLastScope, rememberScope, teamParam } from './scope'

export function useScope(
  api: ApiClient,
  hasAreas: boolean,
  storageKey: string,
  params: URLSearchParams,
  restore: boolean,
) {
  const remembered = restore ? readLastScope(storageKey) : null
  const requested = remembered ?? params
  const requestedTeam = requested.get(teamParam) ?? ''
  const requestedProject = requested.get(projectParam) ?? ''

  const teams = useQuery({ ...allTeams(api, storageKey), enabled: hasAreas })
  const team =
    hasAreas && teams.isSuccess
      ? teams.data.find(({ id }) => id === requestedTeam)
      : undefined
  const projects = useQuery({
    ...allTeamProjects(api, storageKey, team?.id ?? ''),
    enabled: Boolean(team),
  })
  const project = projects.isSuccess
    ? projects.data.find(({ id }) => id === requestedProject)
    : undefined
  const ready = !hasAreas || (teams.isSuccess && (!team || projects.isSuccess))
  const invalid =
    ready &&
    ((requestedTeam !== '' && !team) || (requestedProject !== '' && !project))

  useEffect(() => {
    if (!restore && ready && !invalid && team) {
      rememberScope(storageKey, team.id, project?.id ?? '')
    }
  }, [restore, ready, invalid, storageKey, team, project])

  return { teams, team, projects, project, remembered, ready, invalid }
}
