import { Text } from '@gravity-ui/uikit'
import {
  ActivityTime,
  ActorKindBadge,
  SourceLink,
  activitySourceName,
} from '@/entities/activity'
import type { TimelineEvent, ProjectTimelineParams } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'
import { TaskDetailLink } from '@/shared/ui'
import { objectLabel } from '../model/timeline-search'

const methods = {
  webhook: 'Вебхук',
  incremental: 'Плановая синхронизация',
  backfill: 'Загрузка истории',
  reconcile: 'Сверка с источником',
  import: 'Импорт',
}

export function TimelineEventDetails({
  id,
  event,
  rowId,
  pageCount,
  projectId,
  params,
}: {
  id: string
  event: TimelineEvent
  rowId: string
  pageCount: number
  projectId: string
  params: ProjectTimelineParams
}) {
  const { object, actor } = event
  return (
    <section
      id={id}
      className="cp-timeline__details"
      aria-labelledby={id + '-heading'}
    >
      <Text as="h4" variant="subheader-2" id={id + '-heading'}>
        Подробности события
      </Text>
      <dl>
        <dt>Объект</dt>
        <dd>{objectLabel(object)}</dd>
        <dt>Событие</dt>
        <dd>{contractLabel(contractLabels.eventKind, event.event_kind)}</dd>
        <dt>Поле</dt>
        <dd>{event.field ?? 'Не указано'}</dd>
        <dt>До изменения</dt>
        <dd>{event.before_value ?? 'Не задано'}</dd>
        <dt>После изменения</dt>
        <dd>{event.after_value ?? 'Не задано'}</dd>
        <dt>Участник</dt>
        <dd>
          <ActorKindBadge kind={actor.kind} /> {actor.display_name}
        </dd>
        <dt>Время события</dt>
        <dd>
          <ActivityTime instant={event.event_time} />
        </dd>
        <dt>Время получения</dt>
        <dd>
          <ActivityTime instant={event.observed_at} />
        </dd>
        <dt>Способ получения</dt>
        <dd>{contractLabel(methods, event.observed_via)}</dd>
      </dl>
      <div className="cp-timeline__actions">
        <SourceLink
          url={object.source_url}
          sourceName={activitySourceName(object.kind)}
        />
        {object.kind === 'task' && (
          <TaskDetailLink
            taskId={object.id}
            focusId={rowId + '-task'}
            returnFocusId={rowId}
            returnPageCount={pageCount}
            params={{
              project_id: projectId,
              from: params.from,
              to: params.to,
              ...(params.revision_id
                ? { revision_id: params.revision_id }
                : {}),
            }}
            preserveHistoryFocus
          >
            Открыть карточку задачи
          </TaskDetailLink>
        )}
      </div>
    </section>
  )
}
