import { eventTime } from '../lib/event-time'

export function ActivityTime({
  instant,
  offsetSeconds,
}: {
  instant: string
  offsetSeconds?: number
}) {
  const text = eventTime(instant, offsetSeconds)
  return text === null ? (
    'Неизвестное время'
  ) : (
    <time dateTime={instant}>{text}</time>
  )
}
