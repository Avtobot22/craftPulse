import { useState } from 'react'
import { matchPath } from 'react-router'
import type { Location } from 'react-router'
import type { CurrentUser } from '@/shared/auth'
import { readTaskOrigin, routePaths } from '@/shared/config'
import { adminNavigation } from './admin-navigation'
import { taskAnalysisSections } from './task-analysis-navigation'
import { technicalNavigation } from './technical-navigation'
import { outsideAreas, projectParam, teamParam } from './scope'

const lists = [
  ...taskAnalysisSections.flatMap(({ path, action, children }) => [
    { path, action },
    ...children.map((child) => ({ path: child.path, action })),
  ]),
  ...adminNavigation.filter(({ path }) => path !== routePaths.importPackage),
  ...technicalNavigation.filter(({ path }) =>
    [routePaths.findings, routePaths.exceptions].some((list) => list === path),
  ),
  { path: routePaths.users, action: 'users.manage' },
  { path: routePaths.teamReport, action: 'report.read' },
  { path: routePaths.savedViews, action: 'report.read' },
]

function listScope(path: string) {
  const [pathname, search] = path.split('#')[0]?.split('?') ?? []
  const params = new URLSearchParams(search)
  return JSON.stringify([
    pathname,
    params.get(teamParam),
    params.get(projectParam),
  ])
}

export function useUnavailableReturn(
  current: CurrentUser,
  identity: string,
  location: Location,
  canRemember: boolean,
) {
  const state: unknown = location.state
  const deniedFrom =
    location.pathname === routePaths.unavailable &&
    typeof state === 'object' &&
    state !== null &&
    'deniedFrom' in state
      ? state.deniedFrom
      : undefined
  const deniedScope =
    typeof deniedFrom === 'string' ? listScope(deniedFrom) : null
  const path = location.pathname + location.search
  const list =
    canRemember &&
    lists.some(({ path: listPath }) => listPath === location.pathname)
      ? path + location.hash
      : null
  const [visits, setVisits] = useState<{
    identity: string
    list: string | null
    deniedFrom: unknown
    recent: string[]
  }>({ identity, list: null, deniedFrom: undefined, recent: [] })
  let recent = visits.identity === identity ? visits.recent : []
  if (
    visits.identity !== identity ||
    visits.list !== list ||
    visits.deniedFrom !== deniedFrom
  ) {
    if (deniedScope) {
      recent = recent.filter((entry) => listScope(entry) !== deniedScope)
    }
    if (list)
      recent = [list, ...recent.filter((entry) => entry !== list)].slice(0, 20)
    setVisits({ identity, list, deniedFrom, recent })
  }
  if (location.pathname !== routePaths.unavailable) return undefined

  function allowed(to: { pathname: string; search: string }, origin = false) {
    const action =
      lists.find(({ path }) => to.pathname === path)?.action ??
      (origin && matchPath(routePaths.metricCohort, to.pathname)
        ? 'report.read'
        : undefined)
    return (
      action !== undefined &&
      current.actions.includes(action) &&
      !outsideAreas(current.areas, new URLSearchParams(to.search)) &&
      listScope(to.pathname + to.search) !== deniedScope
    )
  }

  const origin = readTaskOrigin(state)
  if (origin && allowed(origin, true))
    return { to: origin, focusId: origin.focusId }
  for (const entry of recent) {
    const url = new URL(entry, 'https://craftpulse.test')
    if (allowed(url))
      return {
        to: entry,
        viaHistory:
          typeof state === 'object' &&
          state !== null &&
          'inspectionFrom' in state &&
          state.inspectionFrom === entry,
      }
  }
  return undefined
}
