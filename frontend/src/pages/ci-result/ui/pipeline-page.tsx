import { Button } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useParams, useSearchParams } from 'react-router'
import { testExecutionPages } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { InspectionBack } from '@/shared/ui'
import { isAccessError, testOutcome, testOutcomeParam } from '../model/pipeline'
import type { Pipeline } from '../model/pipeline'
import { usePipelineAttempts } from '../model/use-pipeline-attempts'
import { PipelineJobs } from './pipeline-jobs'
import {
  PipelineCoverage,
  PipelineOverview,
  PipelineReports,
} from './pipeline-overview'
import { PipelineResult } from './pipeline-result'
import { PipelineTestExecutions, PipelineTests } from './pipeline-tests'

export function PipelinePage({ api }: { api: ApiClient }) {
  const { pipelineId = '' } = useParams()
  return (
    <PipelineResult
      api={api}
      pipelineId={pipelineId}
      title="Pipeline"
      navigation={<InspectionBack />}
    >
      {(pipeline, refresh, refreshing) => (
        <PipelineContent
          api={api}
          pipeline={pipeline}
          refresh={refresh}
          refreshing={refreshing}
        />
      )}
    </PipelineResult>
  )
}

function PipelineContent({
  api,
  pipeline,
  refresh,
  refreshing,
}: {
  api: ApiClient
  pipeline: Pipeline
  refresh: () => void
  refreshing: boolean
}) {
  const client = useQueryClient()
  const [params] = useSearchParams()
  const attempts = usePipelineAttempts(api, pipeline.id)
  const canListExecutions = pipeline.reports.some(
    ({ analysis_kind, state }) =>
      analysis_kind === 'junit' &&
      (state === 'received' || state === 'partial'),
  )
  function refreshAll() {
    refresh()
    void attempts.refetch()
    if (canListExecutions) {
      const outcome = testOutcome(params.get(testOutcomeParam))
      void client.invalidateQueries({
        queryKey: testExecutionPages(api, pipeline.id, outcome).queryKey,
        exact: true,
      })
    }
  }

  return (
    <>
      <div className="cp-ci-actions">
        <Button
          size="xl"
          view="outlined"
          onClick={refreshAll}
          disabled={refreshing}
        >
          Обновить
        </Button>
      </div>
      <PipelineOverview pipeline={pipeline} />
      <div className="cp-ci-summary">
        <PipelineReports pipeline={pipeline} />
        <PipelineTests
          pipeline={pipeline}
          canListExecutions={canListExecutions}
        />
        <PipelineCoverage coverage={pipeline.coverage} />
      </div>
      <PipelineJobs pipeline={pipeline} query={attempts} />
      {canListExecutions && (
        <PipelineTestExecutions
          api={api}
          pipelineId={pipeline.id}
          attempts={
            isAccessError(attempts.error)
              ? []
              : (attempts.data?.pages.flatMap(({ data }) => data) ?? [])
          }
        />
      )}
    </>
  )
}
