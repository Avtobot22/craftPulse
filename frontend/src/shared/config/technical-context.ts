export const technicalIdPattern =
  '[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}'
const technicalId = new RegExp(`^${technicalIdPattern}$`, 'u')

export function readTechnicalId(value: string | null | undefined) {
  return value && technicalId.test(value) ? value.toLowerCase() : undefined
}

export function technicalScope(search: URLSearchParams) {
  const scope = new URLSearchParams()
  for (const key of ['org_slug', 'team_id', 'project_id']) {
    const value = search.get(key)
    if (value) scope.set(key, value)
  }
  return scope
}

export function baselineContext(search: URLSearchParams) {
  const context = technicalScope(search)
  const mr = readTechnicalId(search.get('mr_id'))
  if (mr) {
    context.set('mr_id', mr)
    const head = search.get('head_sha') ?? ''
    if (/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u.test(head))
      context.set('head_sha', head)
    const revision = search.get('revision_no') ?? ''
    if (/^[1-9][0-9]{0,9}$/u.test(revision) && Number(revision) <= 2147483647)
      context.set('revision_no', revision)
  }
  return context
}
