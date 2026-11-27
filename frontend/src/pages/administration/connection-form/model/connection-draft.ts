import { adminLabels } from '@/shared/config'
import type { components, OpenCodes } from '@/shared/api'

type Schema = components['schemas']
type BuiltinProvider = Exclude<Schema['Provider'], 'client' | 'universal'>
type CatalogAdapter = OpenCodes<Schema['Adapter']>
export type Adapter = CatalogAdapter & { provider: BuiltinProvider }

export function isBuiltinAdapter(adapter: CatalogAdapter): adapter is Adapter {
  const { provider } = adapter
  return (
    provider !== 'client' &&
    provider !== 'universal' &&
    Object.hasOwn(adminLabels.provider, provider)
  )
}
export type Connection = OpenCodes<Schema['Connection']>
export type Credentials = Schema['Credentials']
type Scope = Schema['ConnectionScopeInput']
export type ScopeRow = Omit<OpenCodes<Scope>, 'native_scope_id'> & {
  key: string
  native_scope_id: string
  displayName?: string
}

export interface ConnectionDraft {
  name: string
  provider: string
  base_url: string
  scopes: ScopeRow[]
}

export const scopeLabels = {
  tracker_project: 'Проект трекера',
  repository: 'Репозиторий',
  wiki_space: 'Пространство Wiki',
  ci_project: 'Проект CI',
  analysis_project: 'Проект анализа',
} satisfies Record<Scope['kind'], string>

function isScopeKind(kind: string): kind is Scope['kind'] {
  return Object.hasOwn(scopeLabels, kind)
}

export function scopeKinds(adapter: Adapter | undefined) {
  return adapter?.scope_kinds.filter(isScopeKind) ?? []
}

export function newScope(kind = ''): ScopeRow {
  return {
    key: crypto.randomUUID(),
    kind,
    native_scope_id: '',
    history_from: null,
  }
}

export function connectionDraft(connection?: Connection): ConnectionDraft {
  return {
    provider: connection?.provider ?? '',
    name: connection?.name ?? '',
    base_url: connection?.instance.base_url ?? '',
    scopes: connection?.scopes.map((scope) => ({
      key: scope.id,
      kind: scope.kind,
      native_scope_id: scope.native_scope_id,
      displayName: scope.display_name,
      history_from: scope.history_from,
      ...(scope.repository_scope_id === undefined
        ? {}
        : { repository_scope_id: scope.repository_scope_id }),
    })) ?? [newScope()],
  }
}

function scopeInput(row: ScopeRow) {
  return {
    kind: row.kind,
    native_scope_id: row.native_scope_id,
    history_from: row.history_from,
    ...(row.repository_scope_id == null
      ? {}
      : { repository_scope_id: row.repository_scope_id }),
  }
}

export function connectionChanges(
  draft: ConnectionDraft,
  baseline: ConnectionDraft,
) {
  return {
    name: draft.name !== baseline.name,
    base_url: draft.base_url !== baseline.base_url,
    scopes:
      JSON.stringify(draft.scopes.map(scopeInput)) !==
      JSON.stringify(baseline.scopes.map(scopeInput)),
  }
}

export function scopeInputs(rows: ScopeRow[], adapter: Adapter | undefined) {
  const kinds = scopeKinds(adapter)
  return rows.flatMap((row): Scope[] => {
    const input = scopeInput(row)
    return isScopeKind(input.kind) && kinds.includes(input.kind)
      ? [{ ...input, kind: input.kind }]
      : []
  })
}

export function calendarValue(history: string | null): string {
  if (history === null || history === '') return ''
  const date = new Date(history)
  return Number.isFinite(date.getTime()) ? date.toISOString().slice(0, 23) : ''
}
