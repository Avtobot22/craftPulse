import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { allTeamProjects, allTeams } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  outsideAreas,
  projectParam,
  readLastScope,
  rememberScope,
  teamParam,
} from './scope'

export function useScope(
  api: ApiClient,
  hasAreas: boolean,
  storageKey: string,
  params: URLSearchParams,
  restore: boolean,
  areas: CurrentUser['areas'],
  standalone: boolean,
  blocked: boolean,
) {
  const remembered = restore ? readLastScope(storageKey) : null
  const requested =
    remembered && !outsideAreas(areas, remembered) ? remembered : params
  const requestedTeam = requested.get(teamParam) ?? ''
  const requestedProject = requested.get(projectParam) ?? ''

  const teams = useQuery({
    ...allTeams(api, storageKey),
    enabled: hasAreas && !blocked,
  })
  const teamChoices = teams.data?.filter(({ id }) =>
    areas.teams.some((area) => area.id === id),
  )
  const team =
    !blocked && hasAreas && teams.isSuccess
      ? teamChoices?.find(({ id }) => id === requestedTeam)
      : undefined
  const projects = useQuery({
    ...allTeamProjects(api, storageKey, team?.id ?? ''),
    enabled: Boolean(team),
  })
  const projectChoices = requestedTeam
    ? projects.isSuccess
      ? projects.data.filter(({ id }) =>
          areas.projects.some((area) => area.id === id),
        )
      : undefined
    : standalone
      ? areas.projects
      : undefined
  const project = projectChoices?.find(({ id }) => id === requestedProject)
  const ready =
    (standalone && requestedTeam === '') ||
    !hasAreas ||
    (teams.isSuccess && (!team || projects.isSuccess))
  const invalid =
    ready &&
    ((requestedTeam !== '' && !team) || (requestedProject !== '' && !project))

  useEffect(() => {
    if (!restore && ready && !invalid && team) {
      rememberScope(storageKey, team.id, project?.id ?? '')
    }
  }, [restore, ready, invalid, storageKey, team, project])

  return {
    teams,
    teamChoices,
    team,
    projects,
    project,
    projectChoices,
    remembered,
    ready,
    invalid,
  }
}
