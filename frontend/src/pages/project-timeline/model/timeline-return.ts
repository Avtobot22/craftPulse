import { readTaskReturnFocus } from '@/shared/config'

export function readTimelineReturn(state: unknown) {
  const focusId = readTaskReturnFocus(state)
  if (
    !focusId?.startsWith('timeline-event-') ||
    typeof state !== 'object' ||
    state === null
  ) {
    return undefined
  }
  const scrollTop =
    'taskReturnScrollTop' in state &&
    typeof state.taskReturnScrollTop === 'number' &&
    Number.isFinite(state.taskReturnScrollTop) &&
    state.taskReturnScrollTop >= 0
      ? state.taskReturnScrollTop
      : undefined
  const pageCount =
    'taskReturnPageCount' in state &&
    typeof state.taskReturnPageCount === 'number' &&
    Number.isSafeInteger(state.taskReturnPageCount) &&
    state.taskReturnPageCount > 0
      ? state.taskReturnPageCount
      : 1
  return { focusId, scrollTop, pageCount }
}

export type TimelineReturn = ReturnType<typeof readTimelineReturn>
