import type { ReactNode } from 'react'
import './data-table.css'

export function DataTable({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div
      className="cp-data-table"
      role="region"
      aria-label={label}
      tabIndex={0}
    >
      <table aria-label={label}>{children}</table>
    </div>
  )
}
