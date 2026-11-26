import { Text } from '@gravity-ui/uikit'
import { useEffect, useRef, useState } from 'react'
import { EventRow, activitySourceName } from '@/entities/activity'
import type { TimelineEvent, ProjectTimelineParams } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'
import type { TimelineReturn } from '../model/timeline-return'
import { objectLabel } from '../model/timeline-search'
import { TimelineEventDetails } from './timeline-event-details'

function groupByObject(events: readonly TimelineEvent[]) {
  const groups: { first: TimelineEvent; events: TimelineEvent[] }[] = []
  for (const event of events) {
    const last = groups.at(-1)
    // Keep the overall chronology when changes to different objects are interleaved.
    if (
      last?.first.object.kind === event.object.kind &&
      last.first.object.id === event.object.id
    ) {
      last.events.push(event)
    } else {
      groups.push({ first: event, events: [event] })
    }
  }
  return groups
}

function rowId(event: TimelineEvent) {
  return 'timeline-event-' + event.id
}

export function TimelineEvents({
  events,
  projectId,
  params,
  returnPosition,
  pageCount,
}: {
  events: TimelineEvent[]
  projectId: string
  params: ProjectTimelineParams
  returnPosition: TimelineReturn
  pageCount: number
}) {
  const returnId = returnPosition?.focusId
  const scrollTop = returnPosition?.scrollTop
  const [selected, setSelected] = useState(returnId)
  const list = useRef<HTMLOListElement>(null)
  useEffect(() => {
    if (!returnId) return
    const target = document.getElementById(returnId)
    const active = document.activeElement
    if (!target || !list.current?.contains(target)) return
    if (
      active !== target &&
      active !== document.body &&
      !active?.matches('#main-content, #main-content h1')
    )
      return

    const rect = target.getBoundingClientRect()
    const topAtReturn = window.scrollY + rect.top - (scrollTop ?? 0)
    if (
      scrollTop !== undefined &&
      scrollTop !== window.scrollY &&
      topAtReturn >= 0 &&
      topAtReturn + rect.height <= window.innerHeight
    ) {
      target.focus({ preventScroll: true })
      window.scrollTo({ top: scrollTop })
    } else {
      target.focus()
    }
  }, [returnId, scrollTop])

  return (
    <ol
      ref={list}
      role="list"
      className="cp-timeline__groups"
      aria-label="События по времени"
    >
      {groupByObject(events).map(({ first, events }) => (
        <li key={first.id} className="cp-timeline__group">
          <Text as="h3" variant="subheader-2">
            {objectLabel(first.object)}
          </Text>
          <ol role="list" aria-label="События объекта">
            {events.map((event) => {
              const id = rowId(event)
              const open = selected === id
              const detailsId = id + '-details'
              const kind = contractLabel(
                contractLabels.eventKind,
                event.event_kind,
              )
              return (
                <li
                  key={event.id}
                  className="cp-timeline__event"
                  data-open={open}
                >
                  <div className="cp-timeline__row">
                    <EventRow
                      event={event}
                      sourceName={activitySourceName(event.object.kind)}
                      eventHeading={
                        <button
                          id={id}
                          type="button"
                          className="cp-timeline__event-button"
                          aria-expanded={open}
                          aria-controls={open ? detailsId : undefined}
                          onClick={() => {
                            setSelected(open ? undefined : id)
                          }}
                        >
                          {kind}
                        </button>
                      }
                      grouped
                      showSourceLink={!open}
                    />
                  </div>
                  {open && (
                    <TimelineEventDetails
                      id={detailsId}
                      rowId={id}
                      pageCount={pageCount}
                      event={event}
                      projectId={projectId}
                      params={params}
                    />
                  )}
                </li>
              )
            })}
          </ol>
        </li>
      ))}
    </ol>
  )
}
