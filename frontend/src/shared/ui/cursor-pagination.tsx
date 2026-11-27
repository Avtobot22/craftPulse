import { Button } from '@gravity-ui/uikit'
import './cursor-pagination.css'

export const pageSizes = [25, 50, 100] as const

export function CursorPagination({
  label,
  cursor,
  next,
  previous,
  limit,
  received,
  fixedSize = false,
  goTo,
}: {
  label: string
  cursor: string
  next: string | null
  previous: readonly string[]
  limit: number
  received: number | undefined
  fixedSize?: boolean
  goTo: (cursor: string, previous: string[], limit: string) => void
}) {
  const page = !cursor ? 1 : previous[0] === '' ? previous.length + 1 : null
  const nextCursor =
    next && next !== cursor && !previous.includes(next) ? next : null
  return (
    <nav className="cp-cursor-pagination" aria-label={label}>
      {!fixedSize && (
        <label>
          Строк на странице
          <select
            value={limit}
            onChange={(event) => {
              goTo('', [], event.target.value)
            }}
          >
            {pageSizes.map((size) => (
              <option key={size}>{size}</option>
            ))}
          </select>
        </label>
      )}
      <Button
        size="xl"
        disabled={!cursor}
        onClick={() => {
          goTo(previous.at(-1) ?? '', previous.slice(0, -1), String(limit))
        }}
      >
        {cursor && previous.length === 0
          ? 'К первой странице'
          : 'Предыдущая страница'}
      </Button>
      <span role="status" aria-current="page">
        {page === null ? 'Текущая страница' : `Страница ${String(page)}`}
        {received !== undefined && ` · получено ${String(received)}`}
      </span>
      <Button
        size="xl"
        disabled={!nextCursor}
        onClick={() => {
          if (nextCursor) goTo(nextCursor, [...previous, cursor], String(limit))
        }}
      >
        Следующая страница
      </Button>
    </nav>
  )
}
