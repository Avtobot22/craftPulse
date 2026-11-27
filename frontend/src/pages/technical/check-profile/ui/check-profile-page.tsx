import { useQuery } from '@tanstack/react-query'
import { generatePath, Link } from 'react-router'
import { ApiError, getCheckProfile, getCheckProfileOptions } from '@/shared/api'
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

  const missing =
    query.error instanceof ApiError && query.error.kind === 'not-found'
  const options = useQuery({
    ...getCheckProfileOptions(api, projectId),
    enabled: missing,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const loading =
    query.isPending ||
    (query.isFetching && !query.isFetchedAfterMount) ||
    (missing && options.isPending)
  const canEdit = query.isSuccess || (missing && options.isSuccess)

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
      ) : canEdit ? (
        <MatrixEditor
          api={api}
          projectId={projectId}
          current={query.isSuccess ? query.data : null}
          options={options.data}
        />
      ) : (
        <ErrorState
          reason={
            missing
              ? 'Не удалось загрузить компоненты и профили проверок'
              : 'Не удалось загрузить матрицу проверок'
          }
          onRetry={() => void (missing ? options.refetch() : query.refetch())}
          retrying={query.isFetching || options.isFetching}
        />
      )}
    </div>
  )
}
