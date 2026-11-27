import { useLocation, useSearchParams } from 'react-router'
import { pageSizes } from './cursor-pagination'

function readHistory(state: unknown): string[] {
  const cursors =
    state && typeof state === 'object' && 'pageCursors' in state
      ? state.pageCursors
      : null
  return Array.isArray(cursors) &&
    cursors.every((cursor) => typeof cursor === 'string')
    ? cursors
    : []
}

export function useCursorSearch() {
  const [search, setSearch] = useSearchParams()
  const state: unknown = useLocation().state
  const cursor = search.get('cursor') ?? ''
  const limit =
    pageSizes.find((size) => String(size) === search.get('limit')) ??
    pageSizes[0]

  function filter(name: string, value: string) {
    const params = new URLSearchParams(search)
    if (value) params.set(name, value)
    else params.delete(name)
    params.delete('cursor')
    setSearch(params, { preventScrollReset: true, state: null })
  }

  function goTo(target: string, pageCursors: string[], size: string) {
    const params = new URLSearchParams(search)
    if (target) params.set('cursor', target)
    else params.delete('cursor')
    if (size === String(pageSizes[0])) params.delete('limit')
    else params.set('limit', size)
    setSearch(params, { preventScrollReset: true, state: { pageCursors } })
  }

  return {
    search,
    filter,
    page: { limit, cursor, previous: cursor ? readHistory(state) : [], goTo },
  }
}
