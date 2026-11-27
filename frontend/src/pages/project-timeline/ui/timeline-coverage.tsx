import { Text } from '@gravity-ui/uikit'
import { ActivityTime, HistoryBoundary } from '@/entities/activity'
import type { ProjectTimelinePageData } from '@/shared/api'
import { providerLabel } from '@/shared/config'

function isBeforeHistory(from: string, historyFrom: string | null) {
  return historyFrom !== null && Date.parse(from) < Date.parse(historyFrom)
}

export function timelineCoverage(page: ProjectTimelinePageData, from: string) {
  const sources = page.sources ?? []
  return {
    historyMissing:
      isBeforeHistory(from, page.history_from) ||
      sources.some((source) => isBeforeHistory(from, source.history_from)),
    historyUncertain: sources.some(
      (source) => source.completeness !== 'complete',
    ),
    noSources: page.sources?.length === 0 && page.history_from === null,
  }
}

export function TimelineCoverage({
  page,
  from,
  to,
}: {
  page: ProjectTimelinePageData
  from: string
  to: string
}) {
  const sources = page.sources ?? []
  if (!sources.length && page.history_from === null) return null
  return (
    <section
      className="cp-timeline__coverage"
      aria-labelledby="timeline-coverage"
    >
      <Text as="h2" variant="subheader-2" id="timeline-coverage">
        История по источникам
      </Text>
      {sources.length ? (
        <ul>
          {sources.map(
            ({ connection_id, provider, history_from, completeness }) => {
              const name = providerLabel(provider)
              const label = sources.some(
                (source) =>
                  source.connection_id !== connection_id &&
                  providerLabel(source.provider) === name,
              )
                ? name + ' · ' + connection_id
                : name
              const gap = isBeforeHistory(from, history_from)
              const fullyMissing =
                history_from !== null &&
                Date.parse(to) <= Date.parse(history_from)
              const status =
                completeness === 'complete'
                  ? 'Полная история'
                  : completeness === 'partial'
                    ? 'Частичная история'
                    : 'Полнота истории неизвестна'
              return (
                <li key={connection_id}>
                  <Text as="strong">{label}</Text>
                  <Text as="p">
                    {gap
                      ? fullyMissing
                        ? 'История за выбранный период не загружена.'
                        : 'История за часть выбранного периода не загружена.'
                      : status}
                  </Text>
                  <Text as="p">
                    {history_from ? (
                      <>
                        Граница истории: <ActivityTime instant={history_from} />
                      </>
                    ) : (
                      'Граница истории неизвестна'
                    )}
                  </Text>
                </li>
              )
            },
          )}
        </ul>
      ) : (
        <HistoryBoundary
          sourceName="История проекта"
          historyFrom={page.history_from}
        />
      )}
    </section>
  )
}
