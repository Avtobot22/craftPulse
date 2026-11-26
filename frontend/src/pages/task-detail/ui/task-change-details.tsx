import { useQueries } from '@tanstack/react-query'
import { generatePath, Link } from 'react-router'
import { ActivityTime, ActorKindBadge, SourceLink } from '@/entities/activity'
import { ReviewWaitDetails } from '@/entities/merge-request'
import { getMergeRequest } from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import { contractLabel, routePaths, taskAnalysisLinks } from '@/shared/config'
import { ErrorState, InspectionLink, LoadingState } from '@/shared/ui'
import { basisLabel } from '../model/task-evidence'
import type { TaskDetail } from '../model/task-evidence'
import { taskLinks } from '../model/task-links'
import type { TaskLink } from '../model/task-links'
import { TaskLinks } from './task-links'
import { TaskPipelines } from './task-pipelines'

type MergeRequest = OpenCodes<components['schemas']['MergeRequest']>

const mrStates = {
  open: 'Открыт',
  merged: 'Слит',
  closed: 'Закрыт',
} satisfies Record<components['schemas']['MergeRequest']['state'], string>

function MergeRequestDetails({
  mr,
  reference,
  task,
}: {
  mr: MergeRequest
  reference: TaskLink['to']
  task: TaskDetail
}) {
  const label = [mr.iid === null ? null : `!${String(mr.iid)}`, mr.title]
    .filter(Boolean)
    .join(' · ')
  return (
    <>
      <h3>
        <SourceLink url={mr.source_url ?? reference.source_url}>
          {label || basisLabel(reference) || 'Запрос на слияние'}
        </SourceLink>
      </h3>
      <dl className="cp-task-detail__meta">
        <div>
          <dt>Состояние</dt>
          <dd>
            {contractLabel(mrStates, mr.state)}
            {mr.draft && ' · Черновик'}
          </dd>
        </div>
        <div>
          <dt>Готовность к ревью</dt>
          <dd>
            {mr.ready_at ? (
              <ActivityTime instant={mr.ready_at} />
            ) : (
              'Дата готовности неизвестна'
            )}
          </dd>
        </div>
        <div>
          <dt>Автор</dt>
          <dd>
            {mr.author.display_name ?? 'Неизвестный автор'} ·{' '}
            <ActorKindBadge kind={mr.author.kind} />
          </dd>
        </div>
        <div>
          <dt>Текущий head SHA</dt>
          <dd>
            <code>{mr.head_sha}</code>
          </dd>
        </div>
        <div>
          <dt>Ожидание ревью</dt>
          <dd>
            <ReviewWaitDetails wait={mr.review_wait} />
          </dd>
        </div>
      </dl>
      <p>
        <InspectionLink
          id={`task-mr-${mr.id}`}
          to={
            generatePath(routePaths.mergeRequestSummary, {
              mr: encodeURIComponent(mr.id),
            }) +
            '?' +
            new URLSearchParams({ head_sha: mr.head_sha }).toString()
          }
        >
          Сводка MR
        </InspectionLink>
      </p>
      {task.project_id && (
        <p>
          <Link
            to={taskAnalysisLinks.mergeRequestQueue({
              project_id: task.project_id,
              ...(task.team_id ? { team_id: task.team_id } : {}),
            })}
          >
            Очередь ревью проекта
          </Link>
        </p>
      )}
    </>
  )
}

export function TaskChangeDetails({
  api,
  task,
  canDecide,
  refreshTask,
}: {
  api: ApiClient
  task: TaskDetail
  canDecide: boolean
  refreshTask: () => Promise<void>
}) {
  const { confirmed, candidates, runs } = taskLinks(task)
  const requests = useQueries({
    queries: confirmed.map(({ to }) => getMergeRequest(api, to.id)),
  })
  const heads = new Map<string, string>()
  const linkedRunMRs = new Set(runs.map(({ from_object_id }) => from_object_id))
  let waitingForHead = false
  let headUnavailable = false
  let hasMerged = false
  for (const [index, { to }] of confirmed.entries()) {
    const request = requests[index]
    if (request?.isSuccess) {
      heads.set(to.id, request.data.head_sha)
      if (request.data.state === 'merged') hasMerged = true
    } else if (linkedRunMRs.has(to.id)) {
      if (request?.isError) headUnavailable = true
      else waitingForHead = true
    }
  }

  return (
    <>
      <TaskLinks
        api={api}
        task={task}
        confirmed={confirmed}
        candidates={candidates}
        hasMerged={hasMerged}
        canDecide={canDecide}
        refreshTask={refreshTask}
      />
      {confirmed.length > 0 && (
        <section aria-labelledby="task-merge-requests">
          <h2 id="task-merge-requests">Подтверждённые MR</h2>
          <ul className="cp-task-detail__cards">
            {confirmed.map(({ to }, index) => {
              const request = requests[index]
              return (
                <li key={to.id}>
                  {request?.isSuccess ? (
                    <MergeRequestDetails
                      mr={request.data}
                      reference={to}
                      task={task}
                    />
                  ) : (
                    <>
                      <h3>{basisLabel(to) || 'Запрос на слияние'}</h3>
                      {request?.isError ? (
                        <ErrorState
                          reason="Не удалось загрузить данные MR"
                          onRetry={() => void request.refetch()}
                          retrying={request.isFetching}
                        />
                      ) : (
                        <LoadingState />
                      )}
                    </>
                  )}
                </li>
              )
            })}
          </ul>
        </section>
      )}
      <TaskPipelines
        api={api}
        links={runs}
        heads={heads}
        waitingForHead={waitingForHead}
        headUnavailable={headUnavailable}
      />
    </>
  )
}
