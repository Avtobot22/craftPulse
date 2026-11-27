import { Card, Text } from '@gravity-ui/uikit'
import { useEffect, useRef } from 'react'
import { useParams, useSearchParams } from 'react-router'
import { ActivityTime, SourceLink } from '@/entities/activity'
import type { ApiClient } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'
import { InspectionBack } from '@/shared/ui'
import {
  attemptUsage,
  configParam,
  groupJobs,
  jobPipelineParam,
  pipelinePath,
  secondsText,
} from '../model/pipeline'
import type { Job, Pipeline } from '../model/pipeline'
import { usePipelineAttempts } from '../model/use-pipeline-attempts'
import { PipelineAttemptList } from './pipeline-attempt-list'
import { AttemptHistory } from './pipeline-overview'
import { PipelineResult } from './pipeline-result'
import { PipelineTable } from './pipeline-table'

export function PipelineJobPage({ api }: { api: ApiClient }) {
  const { pipelineId = '', jobKey = '' } = useParams()
  const [params] = useSearchParams()
  const config = params.get(configParam)
  const jobPipelineId = params.get(jobPipelineParam)
  const returnParams = new URLSearchParams(params)
  returnParams.delete(configParam)
  returnParams.delete(jobPipelineParam)
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => {
    heading.current?.focus()
  }, [pipelineId, jobKey, config, jobPipelineId])

  return (
    <PipelineResult
      api={api}
      pipelineId={pipelineId}
      title="Попытки задания CI"
      titleRef={heading}
      navigation={
        <InspectionBack
          label="Назад к pipeline"
          fallbackTo={{
            pathname: pipelinePath(pipelineId),
            search: returnParams.toString(),
          }}
        />
      }
    >
      {(pipeline) => (
        <JobContent
          api={api}
          pipeline={pipeline}
          jobKey={jobKey}
          config={config}
          jobPipelineId={jobPipelineId}
        />
      )}
    </PipelineResult>
  )
}

function JobContent({
  api,
  pipeline,
  jobKey,
  config,
  jobPipelineId,
}: {
  api: ApiClient
  pipeline: Pipeline
  jobKey: string
  config: string | null
  jobPipelineId: string | null
}) {
  const query = usePipelineAttempts(api, pipeline.id)
  return (
    <>
      <Text as="h2" variant="subheader-2">
        {jobKey}
      </Text>
      <p>
        История попыток: <AttemptHistory state={pipeline.attempt_history} />
      </p>
      <PipelineAttemptList query={query}>
        {({ attempts, hasNextPage }) => {
          const jobs = groupJobs(
            attempts.filter(
              ({ job_key, config_fingerprint }) =>
                job_key === jobKey &&
                (config === null || config === config_fingerprint),
            ),
            pipeline.id,
          ).filter(
            (job) => jobPipelineId === null || job.pipelineId === jobPipelineId,
          )
          if (jobs.length === 0) {
            return (
              <p>
                {hasNextPage
                  ? 'Среди загруженных попыток совпадений пока нет.'
                  : 'Нет известных попыток для выбранного задания.'}
              </p>
            )
          }
          const incomplete =
            hasNextPage || pipeline.attempt_history !== 'complete'
          return jobs.map((job) => (
            <JobAttempts
              key={job.key}
              job={job}
              isChildPipeline={job.pipelineId !== pipeline.id}
              incomplete={incomplete}
            />
          ))
        }}
      </PipelineAttemptList>
    </>
  )
}

function AttemptTime({ instant }: { instant: string | null }) {
  return instant ? <ActivityTime instant={instant} /> : 'Неизвестно'
}

function JobAttempts({
  job,
  isChildPipeline,
  incomplete,
}: {
  job: Job
  isChildPipeline: boolean
  incomplete: boolean
}) {
  const { attempts } = job
  const retries = attempts.filter(({ attempt_no }) => attempt_no > 1)
  return (
    <Card className="cp-ci-panel">
      <Text as="h3" variant="subheader-2">
        Конфигурация
      </Text>
      <code>{attempts[0].config_fingerprint}</code>
      {isChildPipeline && (
        <p>
          Дочерний pipeline <code>{job.pipelineId}</code>
        </p>
      )}
      <dl className="cp-ci-facts">
        <div>
          <dt>Расход задания</dt>
          <dd>{attemptUsage(attempts, incomplete)}</dd>
        </div>
        <div>
          <dt>Известные повторы</dt>
          <dd>{retries.length}</dd>
        </div>
        <div>
          <dt>Время повторов</dt>
          <dd>{attemptUsage(retries, incomplete)}</dd>
        </div>
      </dl>
      <PipelineTable
        label="Попытки задания"
        columns={[
          'Попытка',
          'Исход',
          'Разрешено падение',
          'Причина падения',
          'Класс раннера',
          'Начало',
          'Завершение',
          'Длительность',
        ]}
        rows={attempts.map((attempt) => ({
          key: attempt.id,
          cells: [
            <SourceLink key="source" url={attempt.source_url}>
              № {attempt.attempt_no}
            </SourceLink>,
            <>
              {contractLabel(contractLabels.ciOutcome, attempt.outcome)}
              {attempt.native_status && <div>{attempt.native_status}</div>}
            </>,
            attempt.allow_failure ? 'Да' : 'Нет',
            attempt.failure_kind
              ? contractLabel(contractLabels.failureKind, attempt.failure_kind)
              : attempt.outcome === 'failed'
                ? 'Причина неизвестна'
                : 'Не указана',
            attempt.runner_class ?? 'Неизвестен',
            <AttemptTime key="start" instant={attempt.started_at} />,
            <AttemptTime key="finish" instant={attempt.finished_at} />,
            attempt.duration_seconds === null
              ? 'Неизвестно'
              : secondsText(attempt.duration_seconds),
          ],
        }))}
      />
    </Card>
  )
}
