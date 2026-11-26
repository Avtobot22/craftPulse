import { useQueries } from '@tanstack/react-query'
import { generatePath } from 'react-router'
import { ActivityTime, SourceLink } from '@/entities/activity'
import { CheckReportState } from '@/entities/check-report'
import { getPipeline } from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import {
  contractLabel,
  contractLabels,
  routePaths,
  technicalLabels,
} from '@/shared/config'
import { ErrorState, InspectionLink, LoadingState } from '@/shared/ui'
import type { TaskLink } from '../model/task-links'
import { TaskPipelineAttempts } from './task-pipeline-attempts'

type Pipeline = OpenCodes<components['schemas']['Pipeline']>
const attemptHistory = {
  complete: 'Полная',
  truncated: 'Неполная',
  unknown: 'Полнота неизвестна',
} satisfies Record<components['schemas']['Pipeline']['attempt_history'], string>

function PipelineDetails({
  api,
  pipeline,
  reference,
}: {
  api: ApiClient
  pipeline: Pipeline
  reference: TaskLink['to']
}) {
  return (
    <>
      <h3>
        <SourceLink url={pipeline.source_url ?? reference.source_url}>
          {reference.display_key
            ? `Pipeline ${reference.display_key}`
            : `Pipeline · ${pipeline.ref_name}`}
        </SourceLink>
      </h3>
      <dl className="cp-task-detail__meta">
        <div>
          <dt>Исход</dt>
          <dd>{contractLabel(contractLabels.ciOutcome, pipeline.outcome)}</dd>
        </div>
        <div>
          <dt>Статус в источнике</dt>
          <dd>{pipeline.native_status || 'Неизвестен'}</dd>
        </div>
        <div>
          <dt>Начало запуска</dt>
          <dd>
            {pipeline.started_at ? (
              <ActivityTime instant={pipeline.started_at} />
            ) : (
              'Неизвестно'
            )}
          </dd>
        </div>
        <div>
          <dt>Известные попытки</dt>
          <dd>{pipeline.attempts_total}</dd>
        </div>
        <div>
          <dt>Задания с повтором</dt>
          <dd>{pipeline.retried_jobs}</dd>
        </div>
        <div>
          <dt>Разрешённые падения</dt>
          <dd>{pipeline.allowed_failures}</dd>
        </div>
        <div>
          <dt>История попыток</dt>
          <dd>{contractLabel(attemptHistory, pipeline.attempt_history)}</dd>
        </div>
      </dl>
      {pipeline.reports.length > 0 && (
        <ul aria-label="Отчёты CI">
          {pipeline.reports.map((report) => (
            <li key={report.analysis_kind}>
              {contractLabel(
                technicalLabels.analysisKind,
                report.analysis_kind,
              )}
              {' · '}
              <CheckReportState state={report.state} />
            </li>
          ))}
        </ul>
      )}
      <p>
        <InspectionLink
          id={`task-ci-${pipeline.id}`}
          to={generatePath(routePaths.pipeline, {
            pipelineId: encodeURIComponent(pipeline.id),
          })}
        >
          Результаты pipeline
        </InspectionLink>
      </p>
      <TaskPipelineAttempts api={api} pipelineId={pipeline.id} />
    </>
  )
}

export function TaskPipelines({
  api,
  links,
  heads,
  waitingForHead,
  headUnavailable,
}: {
  api: ApiClient
  links: TaskLink[]
  heads: ReadonlyMap<string, string>
  waitingForHead: boolean
  headUnavailable: boolean
}) {
  const byPipeline = new Map<
    string,
    { reference: TaskLink['to']; shas: Set<string> }
  >()
  for (const { to, from_object_id } of links) {
    const head = heads.get(from_object_id)
    if (!head) continue
    const run = byPipeline.get(to.id) ?? {
      reference: to,
      shas: new Set<string>(),
    }
    run.shas.add(head)
    byPipeline.set(to.id, run)
  }
  const pipelines = [...byPipeline.entries()]
  const queries = useQueries({
    queries: pipelines.map(([id]) => getPipeline(api, id)),
  })
  const items = pipelines.flatMap(([id, { reference, shas }], index) => {
    const query = queries[index]
    if (!query) return []
    if (query.isSuccess) {
      const { data } = query
      return shas.has(data.sha)
        ? [
            <li key={id}>
              <PipelineDetails
                api={api}
                pipeline={data}
                reference={reference}
              />
            </li>,
          ]
        : []
    }
    return [
      <li key={id}>
        {query.isError ? (
          <ErrorState
            reason={
              reference.display_key
                ? `Не удалось загрузить pipeline ${reference.display_key}`
                : 'Не удалось загрузить pipeline'
            }
            onRetry={() => void query.refetch()}
            retrying={query.isFetching}
          />
        ) : (
          <LoadingState />
        )}
      </li>,
    ]
  })
  return (
    <section aria-labelledby="task-pipelines">
      <h2 id="task-pipelines">CI текущего head</h2>
      {waitingForHead && <LoadingState />}
      {headUnavailable && (
        <p>Часть запусков CI нельзя проверить, пока head MR недоступен.</p>
      )}
      {items.length > 0 ? (
        <ul className="cp-task-detail__cards">{items}</ul>
      ) : (
        !waitingForHead &&
        !headUnavailable && (
          <p>
            Нет доступных запусков CI для подтверждённых MR на текущем head.
          </p>
        )
      )}
    </section>
  )
}
