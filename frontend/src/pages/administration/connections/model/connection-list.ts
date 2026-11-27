import type { ConnectionListParams } from '@/shared/api'

type ConnectionStatus = NonNullable<ConnectionListParams['status']>

export const connectionStatuses = [
  'draft',
  'active',
  'access_lost',
  'deleted',
] as const satisfies readonly ConnectionStatus[]

export const connectionStatusLabels = {
  draft: 'Черновик',
  active: 'Активно',
  access_lost: 'Доступ потерян',
  deleted: 'Удалено',
} satisfies Record<ConnectionStatus, string>

export const connectionPageSize = 25

export function readConnectionCursorHistory(state: unknown): string[] {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('connectionCursors' in state)
  ) {
    return []
  }
  const cursors: unknown = state.connectionCursors
  return Array.isArray(cursors) &&
    cursors.every((cursor) => typeof cursor === 'string')
    ? cursors
    : []
}
