import { useEffect, useRef } from 'react'
import type { MouseEvent, ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router'
import { readTaskReturnFocus, taskAnalysisLinks } from '@/shared/config'
import type { TaskDetailSearch, TaskOrigin } from '@/shared/config'

export function TaskDetailLink({
  taskId,
  params,
  focusId = `task-link-${taskId}`,
  children,
  preserveHistoryFocus = false,
}: {
  taskId: string
  params?: TaskDetailSearch
  focusId?: string
  children: ReactNode
  preserveHistoryFocus?: boolean
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const { pathname, search, hash } = location
  const to = taskAnalysisLinks.taskDetail(taskId, params)
  const taskOrigin: TaskOrigin = { pathname, search, hash, focusId }
  const ref = useRef<HTMLAnchorElement>(null)
  const opening = useRef(false)
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

  async function openTask(event: MouseEvent<HTMLAnchorElement>) {
    if (!isSameTabActivation(event)) return
    event.preventDefault()
    if (opening.current) return
    opening.current = true
    try {
      const state: unknown = location.state
      // Save the opener on the list entry so both the Back link and browser Back restore it.
      await navigate(
        { pathname, search, hash },
        {
          replace: true,
          preventScrollReset: true,
          state: {
            ...(typeof state === 'object' && state !== null ? state : {}),
            taskReturnFocus: focusId,
          },
        },
      )
      await navigate(to, {
        state: { taskOrigin: { ...taskOrigin, returnViaHistory: true } },
      })
    } finally {
      opening.current = false
    }
  }

  return (
    <Link
      ref={ref}
      id={focusId}
      to={to}
      state={{ taskOrigin }}
      onClick={
        preserveHistoryFocus ? (event) => void openTask(event) : undefined
      }
    >
      {children}
    </Link>
  )
}

export function TaskReturnLink({ origin }: { origin: TaskOrigin }) {
  const navigate = useNavigate()
  return (
    <Link
      to={origin}
      replace
      state={{ taskReturnFocus: origin.focusId }}
      onClick={(event) => {
        if (!origin.returnViaHistory || !isSameTabActivation(event)) return
        event.preventDefault()
        void navigate(-1)
      }}
    >
      Назад
    </Link>
  )
}

function isSameTabActivation(event: MouseEvent<HTMLAnchorElement>) {
  return (
    !event.defaultPrevented &&
    event.button === 0 &&
    !event.metaKey &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.shiftKey
  )
}
