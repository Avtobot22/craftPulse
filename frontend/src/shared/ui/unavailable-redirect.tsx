import { Navigate, useLocation } from 'react-router'
import { ApiError } from '@/shared/api'
import { readTaskOrigin, routePaths } from '@/shared/config'

interface UnavailableNavigation {
  forbidden: boolean
  correlationId: string | undefined
  deniedFrom: string
  taskOrigin: ReturnType<typeof readTaskOrigin>
  inspectionFrom: string | undefined
}

export function UnavailableRedirect({
  error,
  forbidden,
}: {
  error?: Error | null | undefined
  forbidden?: boolean
}) {
  const location = useLocation()
  const state: unknown = location.state
  const inspectionFrom =
    typeof state === 'object' &&
    state !== null &&
    'inspectionFrom' in state &&
    typeof state.inspectionFrom === 'string'
      ? state.inspectionFrom
      : undefined
  const apiError = error instanceof ApiError ? error : null
  return (
    <Navigate
      to={routePaths.unavailable}
      replace
      state={
        {
          forbidden: forbidden ?? apiError?.kind === 'forbidden',
          correlationId: apiError?.problem?.correlation_id,
          deniedFrom: location.pathname + location.search + location.hash,
          taskOrigin: readTaskOrigin(state),
          inspectionFrom,
        } satisfies UnavailableNavigation
      }
    />
  )
}
