import { Button } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useSyncExternalStore } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router'
import { UnavailablePage } from '@/pages/unavailable'
import { ApiError } from '@/shared/api'
import { routePaths } from '@/shared/config'
import type { AppRuntime } from '../initialization/runtime'
import { expiredPath } from './session-navigation'

export function ProtectedRoute({ runtime }: { runtime: AppRuntime }) {
  const state = useSyncExternalStore(
    runtime.session.subscribe,
    runtime.session.getSnapshot,
  )
  const location = useLocation()
  if (state === 'expired')
    return (
      <Navigate
        to={expiredPath(location.pathname + location.search + location.hash)}
        replace
      />
    )
  if (state === 'signed-out') return <Navigate to={routePaths.login} replace />
  return <SessionGate runtime={runtime} />
}

function SessionGate({ runtime }: { runtime: AppRuntime }) {
  const query = useQuery(runtime.session.currentUserOptions())
  if (query.isPending) return <p role="status">Загрузка</p>
  if (query.error) {
    const error = query.error
    if (
      error instanceof ApiError &&
      (error.kind === 'not-found' || error.kind === 'forbidden')
    ) {
      return <UnavailablePage correlationId={error.problem?.correlation_id} />
    }
    return (
      <section role="alert">
        <p>Не удалось проверить сессию</p>
        {error instanceof ApiError && error.problem && (
          <p>Код обращения: {error.problem.correlation_id}</p>
        )}
        <Button
          onClick={() => {
            void query.refetch()
          }}
        >
          Повторить
        </Button>
      </section>
    )
  }
  return <Outlet />
}
