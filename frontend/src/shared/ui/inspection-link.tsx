import { useEffect } from 'react'
import type { MouseEvent } from 'react'
import { Link, useLocation, useNavigate, useNavigationType } from 'react-router'
import type { LinkProps } from 'react-router'

const returnPositions = new Map<string, { id: string; top: number }>()

export function InspectionLink({
  id,
  ...props
}: Pick<LinkProps, 'to' | 'children'> & { id: string }) {
  const { key } = useLocation()

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
    returnPositions.set(key, { id, top: window.scrollY })
  }

  return (
    <Link
      {...props}
      id={id}
      state={{ fromInspection: true }}
      onClick={remember}
    />
  )
}

export function useInspectionReturn(ready: boolean) {
  const { key, hash } = useLocation()
  const navigationType: string = useNavigationType()
  const isHistoryNavigation = navigationType === 'POP'
  useEffect(() => {
    if (!ready) return
    const position = isHistoryNavigation ? returnPositions.get(key) : undefined
    if (!position && !hash) return

    const frame = window.requestAnimationFrame(() => {
      const link = position && document.getElementById(position.id)
      if (link) {
        link.focus({ preventScroll: true })
        window.scrollTo({ top: position.top })
      } else if (hash) {
        document.getElementById(hash.slice(1))?.scrollIntoView()
      }
    })
    return () => {
      window.cancelAnimationFrame(frame)
    }
  }, [key, hash, isHistoryNavigation, ready])
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
