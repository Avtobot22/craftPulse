import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { Link, useLocation } from 'react-router'
import { readTaskReturnFocus, taskAnalysisLinks } from '@/shared/config'
import type { TaskDetailSearch } from '@/shared/config'

export function TaskDetailLink({
  taskId,
  params,
  focusId = `task-link-${taskId}`,
  children,
}: {
  taskId: string
  params?: TaskDetailSearch
  focusId?: string
  children: ReactNode
}) {
  const location = useLocation()
  const { pathname, search, hash } = location
  const ref = useRef<HTMLAnchorElement>(null)
  const restoreFocus = readTaskReturnFocus(location.state) === focusId
  useEffect(() => {
    if (!restoreFocus) return
    const active = document.activeElement
    // A paginated source may finish loading after the shell has focused its heading.
    if (
      active === document.body ||
      active?.matches('#main-content, #main-content h1')
    )
      ref.current?.focus()
  }, [restoreFocus])

  return (
    <Link
      ref={ref}
      id={focusId}
      to={taskAnalysisLinks.taskDetail(taskId, params)}
      state={{ taskOrigin: { pathname, search, hash, focusId } }}
    >
      {children}
    </Link>
  )
}
