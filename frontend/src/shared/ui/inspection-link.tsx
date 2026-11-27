import { useEffect } from 'react'
import type { MouseEvent } from 'react'
import { Link, useLocation, useNavigate, useNavigationType } from 'react-router'
import type { LinkProps } from 'react-router'

const returnPositions = new Map<
  string,
  { id: string; top: number; pageCount: number }
>()

export function InspectionLink({
  id,
  pageCount = 1,
  ...props
}: Pick<LinkProps, 'to' | 'children'> & { id: string; pageCount?: number }) {
  const { key, pathname, search, hash } = useLocation()

  function remember(event: MouseEvent<HTMLAnchorElement>) {
    if (
      event.button ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey
    )
      return
    if (returnPositions.size >= 20) {
      const oldest = returnPositions.keys().next().value
      if (oldest !== undefined) returnPositions.delete(oldest)
    }
    returnPositions.set(key, { id, top: window.scrollY, pageCount })
  }

  return (
    <Link
      {...props}
      id={id}
      state={{ fromInspection: true, inspectionFrom: pathname + search + hash }}
      onClick={remember}
    />
  )
}

function useHistoryReturnPosition() {
  const { key } = useLocation()
  const navigationType: string = useNavigationType()
  return navigationType === 'POP' ? returnPositions.get(key) : undefined
}

export function useInspectionPageCount() {
  return useHistoryReturnPosition()?.pageCount ?? 1
}

export function useInspectionReturn(ready: boolean, fallbackId?: string) {
  const { hash } = useLocation()
  const position = useHistoryReturnPosition()
  useEffect(() => {
    if (!ready) return
    if (!position && !hash) return

    const frame = window.requestAnimationFrame(() => {
      const link = position && document.getElementById(position.id)
      if (link) {
        link.focus({ preventScroll: true })
        window.scrollTo({ top: position.top })
      } else if (position && fallbackId) {
        document.getElementById(fallbackId)?.focus()
      } else if (hash) {
        document.getElementById(hash.slice(1))?.scrollIntoView()
      }
    })
    return () => {
      window.cancelAnimationFrame(frame)
    }
  }, [position, hash, ready, fallbackId])
}

export function InspectionBack({
  label = 'Назад',
  fallbackTo,
}: { label?: string; fallbackTo?: LinkProps['to'] } = {}) {
  const state: unknown = useLocation().state
  const navigate = useNavigate()
  if (
    !state ||
    typeof state !== 'object' ||
    !('fromInspection' in state) ||
    state.fromInspection !== true
  )
    return fallbackTo ? <Link to={fallbackTo}>{label}</Link> : null

  return (
    <button type="button" onClick={() => void navigate(-1)}>
      {label}
    </button>
  )
}
