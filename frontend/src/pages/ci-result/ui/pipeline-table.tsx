import type { ReactNode } from 'react'

export function PipelineTable({
  label,
  columns,
  rows,
}: {
  label: string
  columns: readonly string[]
  rows: readonly { key: string; cells: readonly ReactNode[] }[]
}) {
  return (
    <table className="cp-ci-table" aria-label={label} role="table">
      <thead role="rowgroup">
        <tr role="row">
          {columns.map((column) => (
            <th key={column} scope="col" role="columnheader">
              {column}
            </th>
          ))}
        </tr>
      </thead>
      <tbody role="rowgroup">
        {rows.map(({ key, cells }) => (
          <tr key={key} role="row">
            {columns.map((column, index) =>
              index === 0 ? (
                <th key={column} scope="row" role="rowheader">
                  {cells[index]}
                </th>
              ) : (
                <td key={column} role="cell">
                  <span className="cp-ci-cell-label" aria-hidden="true">
                    {column}
                  </span>
                  {cells[index]}
                </td>
              ),
            )}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
