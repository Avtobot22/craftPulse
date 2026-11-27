import { Button, Pagination } from '@gravity-ui/uikit'
import { useRef, useState } from 'react'
import type { ReactNode } from 'react'

export interface TableRow {
  index: number
  cells: ReactNode[]
  actionLabel?: string
  editText?: string
  actions?: ReactNode
  onEdit: () => void
}

const pageSizes = [25, 50, 100]

export function StructureTable({
  title,
  columns,
  rows,
  activeIndex,
  onAdd,
  addLabel,
  unavailable,
  empty = 'Записей пока нет',
}: {
  title: string
  columns: string[]
  rows: TableRow[]
  activeIndex?: number | undefined
  onAdd?: (() => void) | undefined
  addLabel?: string
  unavailable?: string | undefined
  empty?: string
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(25)
  const last = Math.max(1, Math.ceil(rows.length / pageSize))
  const current = Math.min(page, last)
  return (
    <section className="cp-org__list">
      <header className="cp-org__bar">
        <h2 ref={heading} tabIndex={-1}>
          {title}
        </h2>
        {onAdd && (
          <Button disabled={Boolean(unavailable)} onClick={onAdd}>
            {addLabel ?? 'Добавить'}
          </Button>
        )}
      </header>
      {unavailable && <p>{unavailable}</p>}
      {rows.length ? (
        <>
          <div
            className="cp-org__table"
            role="region"
            aria-label={title}
            tabIndex={0}
          >
            <table>
              <thead>
                <tr>
                  {columns.map((name) => (
                    <th key={name} scope="col">
                      {name}
                    </th>
                  ))}
                  <th scope="col">Действия</th>
                </tr>
              </thead>
              <tbody>
                {rows
                  .slice((current - 1) * pageSize, current * pageSize)
                  .map((row) => (
                    <tr
                      key={row.index}
                      data-selected={row.index === activeIndex || undefined}
                    >
                      {row.cells.map((cell, index) => (
                        <td key={index}>{cell}</td>
                      ))}
                      <td>
                        <div className="cp-org__row-actions">
                          <Button
                            type="button"
                            view={
                              row.index === activeIndex
                                ? 'outlined-action'
                                : 'flat'
                            }
                            aria-label={row.actionLabel}
                            onClick={row.onEdit}
                          >
                            {row.editText ?? 'Изменить'}
                          </Button>
                          {row.actions}
                        </div>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          {rows.length > 25 && (
            <p role="status">
              Страница {current} из {last}. Всего записей: {rows.length}
            </p>
          )}
          {rows.length > 25 && (
            <Pagination
              page={current}
              pageSize={pageSize}
              total={rows.length}
              pageSizeOptions={pageSizes}
              size="xl"
              onUpdate={(nextPage, nextSize) => {
                setPage(nextSize === pageSize ? nextPage : 1)
                setPageSize(nextSize)
                heading.current?.focus()
              }}
            />
          )}
        </>
      ) : (
        <p>{empty}</p>
      )}
    </section>
  )
}
