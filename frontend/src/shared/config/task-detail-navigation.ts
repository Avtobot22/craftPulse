import { matchPath } from 'react-router'
import { routePaths } from './routes'

export interface TaskOrigin {
  pathname: string
  search: string
  hash: string
  focusId: string
  returnViaHistory?: true
}

export function readTaskOrigin(state: unknown): TaskOrigin | null {
  if (typeof state !== 'object' || state === null || !('taskOrigin' in state))
    return null
  const origin = state.taskOrigin
  if (
    typeof origin !== 'object' ||
    origin === null ||
    !('pathname' in origin) ||
    typeof origin.pathname !== 'string' ||
    !('search' in origin) ||
    typeof origin.search !== 'string' ||
    (origin.search !== '' && !origin.search.startsWith('?')) ||
    !('hash' in origin) ||
    typeof origin.hash !== 'string' ||
    (origin.hash !== '' && !origin.hash.startsWith('#')) ||
    !('focusId' in origin) ||
    typeof origin.focusId !== 'string'
  )
    return null

  const { pathname, search, hash, focusId } = origin
  const isSource = [
    routePaths.taskList,
    routePaths.teamReport,
    routePaths.metricCohort,
    routePaths.projectTimeline,
  ].some((path) => matchPath(path, pathname))
  return isSource && pathname.startsWith('/') && !/[\\?#]/u.test(pathname)
    ? {
        pathname,
        search,
        hash,
        focusId,
        ...(matchPath(routePaths.taskList, pathname) &&
        'returnViaHistory' in origin &&
        origin.returnViaHistory === true
          ? { returnViaHistory: true }
          : {}),
      }
    : null
}

export function readTaskReturnFocus(state: unknown): string | undefined {
  if (
    typeof state === 'object' &&
    state !== null &&
    'taskReturnFocus' in state &&
    typeof state.taskReturnFocus === 'string'
  )
    return state.taskReturnFocus
  return undefined
}
