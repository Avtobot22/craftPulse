import { ActivityTime } from './activity-time'

export function HistoryBoundary({
  historyFrom,
  sourceName,
}: {
  historyFrom: string | null
  sourceName: string
}) {
  if (historyFrom === null) return null
  return (
    <p>
      {sourceName}: раньше этой даты данных нет (
      <ActivityTime instant={historyFrom} />
      ).
    </p>
  )
}
