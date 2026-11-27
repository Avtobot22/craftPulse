import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode, Ref } from 'react'
import { getPipeline } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'
import { isAccessError } from '../model/pipeline'
import type { Pipeline } from '../model/pipeline'
import './pipeline.css'

export function PipelineResult({
  api,
  pipelineId,
  title,
  titleRef,
  navigation,
  children,
}: {
  api: ApiClient
  pipelineId: string
  title: string
  titleRef?: Ref<HTMLHeadingElement>
  navigation: ReactNode
  children: (
    pipeline: Pipeline,
    refresh: () => void,
    refreshing: boolean,
  ) => ReactNode
}) {
  const query = useQuery({ ...getPipeline(api, pipelineId), retry: false })
  return (
    <article className="cp-ci-result">
      <div className="cp-ci-heading">
        <Text
          as="h1"
          variant="header-1"
          ref={titleRef}
          tabIndex={titleRef ? -1 : undefined}
        >
          {title}
        </Text>
        {navigation}
      </div>
      {query.isPending ? (
        <LoadingState />
      ) : query.isError ? (
        isAccessError(query.error) ? (
          <p role="status">Pipeline недоступен или ещё не получен.</p>
        ) : (
          <ErrorState
            reason="Не удалось загрузить pipeline"
            retrying={query.isFetching}
            onRetry={() => void query.refetch()}
          />
        )
      ) : (
        children(query.data, () => void query.refetch(), query.isFetching)
      )}
    </article>
  )
}
