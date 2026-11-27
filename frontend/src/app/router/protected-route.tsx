import { useQuery } from '@tanstack/react-query'
import { useSyncExternalStore } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router'
import { UnavailablePage } from '@/pages/unavailable'
import { ApiError, isUnavailableError } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'
import type { AppRuntime } from '../initialization/runtime'
import { sessionEndPath } from './session-navigation'

export function ProtectedRoute({ runtime }: { runtime: AppRuntime }) {
  const state = useSyncExternalStore(
    runtime.session.subscribe,
    runtime.session.getSnapshot,
  )
  const location = useLocation()
  if (state === 'expired' || state === 'signed-out')
    return <Navigate to={sessionEndPath(state, location)} replace />
  return <SessionGate runtime={runtime} />
}

function SessionGate({ runtime }: { runtime: AppRuntime }) {
  const query = useQuery(runtime.session.currentUserOptions())
  if (query.isPending) return <LoadingState />
  if (query.error) {
    const error = query.error
    if (isUnavailableError(error)) {
      return (
        <UnavailablePage
          correlationId={error.problem?.correlation_id}
          forbidden={error.kind === 'forbidden'}
        />
      )
    }
    return (
      <ErrorState
        reason="Не удалось проверить сессию"
        {...(error instanceof ApiError && error.problem
          ? { correlationId: error.problem.correlation_id }
          : {})}
        retrying={query.isFetching}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  return <Outlet context={query.data} />
}
