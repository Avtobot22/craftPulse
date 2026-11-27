import { useQuery } from '@tanstack/react-query'
import { generatePath, Link } from 'react-router'
import { ApiError, getCheckProfile } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ErrorState, InspectionBack, LoadingState } from '@/shared/ui'
import { MatrixEditor } from './matrix-editor'
import './check-profile.css'

export function CheckProfilePage({
  api,
  projectId,
  canReadBaselines,
}: {
  api: ApiClient
  projectId: string
  canReadBaselines: boolean
}) {
  const query = useQuery({
    ...getCheckProfile(api, projectId),
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const loading =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)

  return (
    <div className="cp-check-matrix">
      <InspectionBack />
      <nav aria-label="Проверки проекта">
        <Link to={generatePath(routePaths.ciConfiguration, { projectId })}>
          Инструкция CI
        </Link>
        {canReadBaselines && (
          <Link to={generatePath(routePaths.baselines, { projectId })}>
            Базовые линии
          </Link>
        )}
      </nav>
      {loading ? (
        <LoadingState />
      ) : query.isSuccess ? (
        <MatrixEditor api={api} projectId={projectId} current={query.data} />
      ) : (
        <ErrorState
          reason={
            query.error instanceof ApiError && query.error.kind === 'not-found'
              ? 'Подтверждённая матрица проекта недоступна'
              : 'Не удалось загрузить матрицу проверок'
          }
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
    </div>
  )
}
