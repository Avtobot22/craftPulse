import { formatExceptionDate } from '../lib/exception'

export function ExceptionPeriod({
  start,
  end,
  expired = false,
}: {
  start: string
  end: string
  expired?: boolean
}) {
  return (
    <dl>
      <dt>Начало действия</dt>
      <dd>
        <time dateTime={start}>{formatExceptionDate(start)}</time>
      </dd>
      <dt>{expired ? 'Срок истёк' : 'Срок пересмотра'}</dt>
      <dd>
        <time dateTime={end}>{formatExceptionDate(end)}</time>
      </dd>
    </dl>
  )
}
