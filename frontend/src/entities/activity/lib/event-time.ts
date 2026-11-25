const clock = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'UTC',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
})

function utcOffset(offsetSeconds: number) {
  const absolute = Math.abs(offsetSeconds)
  const hours = Math.floor(absolute / 3600)
  const minutes = Math.floor(absolute / 60) % 60
  const seconds = absolute % 60
  const parts = seconds === 0 ? [hours, minutes] : [hours, minutes, seconds]
  return (
    (offsetSeconds < 0 ? '-' : '+') +
    parts.map((part) => String(part).padStart(2, '0')).join(':')
  )
}

export function eventTime(instant: string, offsetSeconds?: number) {
  const timestamp = Date.parse(instant)
  const offset = offsetSeconds ?? 0
  if (
    !Number.isFinite(timestamp) ||
    !Number.isInteger(offset) ||
    Math.abs(offset) > 64800
  ) {
    return null
  }
  const localTime = clock.format(timestamp + offset * 1000)
  return `${localTime} UTC${offsetSeconds === undefined ? '' : utcOffset(offset)}`
}
