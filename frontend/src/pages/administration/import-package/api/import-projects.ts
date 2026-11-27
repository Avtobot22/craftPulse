import { useQueries } from '@tanstack/react-query'
import { allProjects } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'

export function useImportProjects(api: ApiClient, current: CurrentUser) {
  const identity = current.organization.id + ':' + current.user.id
  const roles = current.roles.filter(
    ({ role }) => role === 'administrator' || role === 'tech_lead',
  )
  const unrestricted = roles.some(
    ({ project_id, team_id, service_id }) =>
      !project_id && !team_id && !service_id,
  )
  const direct = new Set(
    roles.flatMap(({ project_id }) => (project_id ? [project_id] : [])),
  )
  const teams = new Set(
    roles.flatMap(({ project_id, team_id }) =>
      !project_id && team_id ? [team_id] : [],
    ),
  )
  const scopes = unrestricted
    ? [undefined]
    : [...(direct.size ? [undefined] : []), ...teams]
  const queries = useQueries({
    queries: scopes.map((teamId) => allProjects(api, identity, teamId)),
  })
  const available = new Map(
    queries.flatMap((query, index) =>
      (query.data ?? [])
        .filter(
          ({ id }) => unrestricted || Boolean(scopes[index]) || direct.has(id),
        )
        .map((project) => [project.id, project] as const),
    ),
  )
  return {
    projects: [...available.values()].sort((a, b) =>
      a.name.localeCompare(b.name, 'ru'),
    ),
    loading: queries.some(({ isPending }) => isPending),
    fetching: queries.some(({ isFetching }) => isFetching),
    error: queries.find(({ isError }) => isError)?.error,
    retry: () => {
      for (const query of queries) {
        if (query.isError) void query.refetch()
      }
    },
  }
}
