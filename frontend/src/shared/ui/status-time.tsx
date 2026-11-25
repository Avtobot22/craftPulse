const dateTime = new Intl.DateTimeFormat('ru-RU', {
  dateStyle: 'short',
  timeStyle: 'medium',
  timeZone: 'UTC',
})

export function StatusTime({ value }: { value: string | null }) {
  if (value === null) return 'Нет данных'
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return 'Неизвестное время'
  return <time dateTime={value}>{dateTime.format(timestamp)} UTC</time>
}
