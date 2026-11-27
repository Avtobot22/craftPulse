import type { CurrentUser } from '@/shared/auth'

export const teamParam = 'team_id'
export const projectParam = 'project_id'

const compatible = new Set(['from', 'to', 'as_of', 'period', 'view'])

export function changeScope(
  current: URLSearchParams,
  team: string,
  project = '',
  { projectOnly = false, keepDate = false } = {},
) {
  const params = new URLSearchParams()
  let reset = current.has(projectParam) && current.get(projectParam) !== project
  for (const [key, value] of current) {
    if (compatible.has(key) || (keepDate && key === 'date'))
      params.append(key, value)
    else if (key !== teamParam && key !== projectParam) reset = true
  }
  if (team) params.set(teamParam, team)
  if ((team || projectOnly) && project) params.set(projectParam, project)
  return { params, reset }
}

export function readLastScope(key: string) {
  try {
    return new URLSearchParams(localStorage.getItem(key) ?? '')
  } catch {
    return new URLSearchParams()
  }
}

export function rememberScope(key: string, team: string, project: string) {
  try {
    if (team) {
      const { params } = changeScope(new URLSearchParams(), team, project)
      localStorage.setItem(key, params.toString())
    } else localStorage.removeItem(key)
  } catch {
    // The address remains the source of truth when browser storage is disabled.
  }
}

export function outsideAreas(
  areas: CurrentUser['areas'],
  params: URLSearchParams,
  projectId?: string,
) {
  const teams = params.getAll(teamParam).filter(Boolean)
  const projects = [
    ...params.getAll(projectParam),
    ...params.getAll('report_project_id'),
    projectId,
  ].filter(Boolean)
  return (
    teams.some((team) => !areas.teams.some(({ id }) => id === team)) ||
    projects.some((project) => !areas.projects.some(({ id }) => id === project))
  )
}
