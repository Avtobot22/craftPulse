import { useQuery } from '@tanstack/react-query'
import {
  Link,
  Navigate,
  useLocation,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, getTask } from '@/shared/api'
import type { ApiClient, Team } from '@/shared/api'
import { readTaskOrigin, routePaths, taskAnalysisLinks } from '@/shared/config'
import {
  ErrorState,
  LoadingState,
  StaleState,
  TaskReturnLink,
} from '@/shared/ui'
import { useTaskHistory } from '../model/use-task-history'
import { TaskHeader } from './task-header'
import { TaskHistory } from './task-history'
import { TaskObservations } from './task-observations'
import { TaskBlocks, TaskTiming } from './task-timing'
import './task-detail-page.css'

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu

function unavailable(error: Error | null) {
  return (
    error instanceof ApiError && ['not-found', 'forbidden'].includes(error.kind)
  )
}

export function TaskDetailPage({ api }: { api: ApiClient }) {
  const { taskId } = useParams()
  const state: unknown = useLocation().state
  const origin = readTaskOrigin(state)
  if (!taskId || !uuid.test(taskId)) {
    return (
      <Navigate
        to={routePaths.unavailable}
        replace
        state={{ taskOrigin: origin }}
      />
    )
  }
  return (
    <TaskDetailContent key={taskId} api={api} taskId={taskId.toLowerCase()} />
  )
}

function TaskDetailContent({
  api,
  taskId,
}: {
  api: ApiClient
  taskId: string
}) {
  const state: unknown = useLocation().state
  const context = useOutletContext<{ teams: Team[] | undefined } | undefined>()
  const [params] = useSearchParams()
  const revisionId = params.get('revision_id')
  const revision = revisionId && uuid.test(revisionId) ? revisionId : null
  const origin = readTaskOrigin(state)
  const task = useQuery({
    ...getTask(api, taskId),
    refetchOnMount: 'always',
    gcTime: 0,
  })
  const waiting =
    task.isPending || (task.isFetching && !task.isFetchedAfterMount)
  const {
    history,
    pending: historyPending,
    pages,
    events,
  } = useTaskHistory(
    api,
    taskId,
    !waiting && !unavailable(task.error) ? task.data : undefined,
  )

  if (unavailable(task.error) || unavailable(history.error))
    return (
      <Navigate
        to={routePaths.unavailable}
        replace
        state={{ taskOrigin: origin }}
      />
    )

  const detail = task.data
  const teamId = params.get('team_id')
  const projectId = params.get('project_id')
  const back = origin ? (
    <TaskReturnLink origin={origin} />
  ) : (
    <Link
      to={taskAnalysisLinks.taskList({
        ...(teamId ? { team_id: teamId } : {}),
        ...(projectId ? { project_id: projectId } : {}),
      })}
      replace
    >
      Назад
    </Link>
  )
  const correlationId =
    task.error instanceof ApiError
      ? task.error.problem?.correlation_id
      : undefined
  const failure = task.isError && (
    <ErrorState
      reason="Не удалось загрузить карточку задачи"
      {...(correlationId ? { correlationId } : {})}
      onRetry={() => {
        void task.refetch()
      }}
      retrying={task.isFetching}
    />
  )
  const team = context?.teams?.find(({ id }) => id === detail?.team_id)
  const content = !waiting && detail && (
    <>
      <RestrictedResults redactions={detail.redactions} />
      <TaskObservations task={detail} events={events} />
      <TaskTiming task={detail} revision={revision} />
      <TaskBlocks
        task={detail}
        revision={revision}
        events={events}
        receivedAt={task.dataUpdatedAt}
      />
      <TaskHistory
        events={events}
        historyFrom={pages[0]?.history_from ?? null}
        accessLimited={pages.some((page) => page.access_limited)}
        accessNotes={pages.flatMap((page) => page.access_notes)}
        pending={historyPending}
        fetching={history.isFetching}
        more={history.hasNextPage}
        error={history.error}
        snapshotAt={history.dataUpdatedAt}
        loadMore={() => {
          void history.fetchNextPage({ cancelRefetch: false })
        }}
        retry={() => {
          if (history.isFetchNextPageError) void history.fetchNextPage()
          else void history.refetch()
        }}
      />
    </>
  )
  return (
    <article className="cp-task-detail">
      {back}
      <TaskHeader
        task={waiting ? undefined : detail}
        teamName={team?.name ?? detail?.team_id ?? undefined}
      />
      {waiting ? (
        <LoadingState />
      ) : failure && !detail ? (
        failure
      ) : failure ? (
        <StaleState
          snapshotAt={new Date(task.dataUpdatedAt).toISOString()}
          currentError={failure}
        >
          {content}
        </StaleState>
      ) : (
        content
      )}
    </article>
  )
}
