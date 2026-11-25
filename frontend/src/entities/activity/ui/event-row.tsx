import { ArrowDownToLine, Clock } from '@gravity-ui/icons'
import { Flex, Text } from '@gravity-ui/uikit'
import type { TimelineEvent } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'
import { ActivityBadge } from './activity-badge'
import { ActivityTime } from './activity-time'
import { ActorKindBadge } from './actor-kind-badge'
import { SourceLink } from './source-link'

export function EventRow({
  event,
  sourceName,
}: {
  event: TimelineEvent
  sourceName?: string
}) {
  const {
    object,
    actor,
    field,
    before_value: before,
    after_value: after,
  } = event
  const objectLabel = [object.display_key, object.title]
    .filter(Boolean)
    .join(' · ')
  return (
    <Flex direction="column" gap="spacing-2">
      <Text as="strong">{objectLabel || 'Объект события'}</Text>
      <Text as="div">
        {contractLabel(contractLabels.eventKind, event.event_kind)}
      </Text>
      {(field !== null || before !== null || after !== null) && (
        <Text as="div">
          {field === null ? null : field + ': '}
          {before === null && field === null
            ? after
            : `${before ?? 'Не задано'} → ${after ?? 'Не задано'}`}
        </Text>
      )}
      <Flex alignItems="center" gap="spacing-2" wrap>
        <ActorKindBadge kind={actor.kind} />
        {actor.display_name && <Text>{actor.display_name}</Text>}
      </Flex>
      <Text as="div">
        Время события: <ActivityTime instant={event.event_time} />
      </Text>
      <Text as="div">
        Время в источнике:{' '}
        <ActivityTime
          instant={event.event_time}
          offsetSeconds={event.event_offset_seconds}
        />
      </Text>
      <Text as="div">
        Время получения: <ActivityTime instant={event.observed_at} />
      </Text>
      {(event.observed_via === 'backfill' || event.late) && (
        <Flex gap="spacing-2" wrap>
          {event.observed_via === 'backfill' && (
            <ActivityBadge icon={ArrowDownToLine} theme="info">
              Загружено с историей
            </ActivityBadge>
          )}
          {event.late && (
            <ActivityBadge icon={Clock} theme="warning">
              Получено с опозданием
            </ActivityBadge>
          )}
        </Flex>
      )}
      <SourceLink
        url={object.source_url}
        {...(sourceName === undefined ? {} : { sourceName })}
      />
    </Flex>
  )
}
