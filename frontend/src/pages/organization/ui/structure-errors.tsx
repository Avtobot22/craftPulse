import { useEffect, useRef } from 'react'
import { fieldLabels, sections } from '../model/structure-draft'
import type { Issue } from '../model/structure-errors'
import { fieldId } from './structure-fields'

export function StructureErrors({
  issues,
  showIssue,
}: {
  issues: Issue[]
  showIssue: (issue: Issue) => void
}) {
  const summary = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!issues.length) return
    // UI-17 restores focus when it closes. Move to the summary after that.
    const frame = requestAnimationFrame(() => summary.current?.focus())
    return () => {
      cancelAnimationFrame(frame)
    }
  }, [issues])

  if (!issues.length) return null
  return (
    <div ref={summary} tabIndex={-1} role="alert" className="cp-org__error">
      <strong>Исправьте ошибки в структуре</strong>
      <ul>
        {issues.map((issue, index) => {
          const target =
            issue.section &&
            fieldId(
              issue.section,
              issue.index === undefined ? undefined : issue.field,
            )
          const label = [
            issue.section && sections[issue.section],
            issue.index !== undefined && 'Запись ' + String(issue.index + 1),
            issue.field && fieldLabels[issue.field],
            issue.message,
          ]
            .filter(Boolean)
            .join(' · ')
          return (
            <li key={index}>
              {target ? (
                <a
                  href={'#' + target}
                  onClick={(event) => {
                    event.preventDefault()
                    showIssue(issue)
                  }}
                >
                  {label}
                </a>
              ) : (
                label
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

export function RowErrors({ issues }: { issues: Issue[] }) {
  const messages = issues.filter(
    ({ field }) =>
      !field ||
      field === 'id' ||
      field === 'valid_from' ||
      field === 'valid_to',
  )
  return (
    messages.length > 0 && (
      <ul className="cp-org__error">
        {messages.map(({ message }, index) => (
          <li key={index}>{message}</li>
        ))}
      </ul>
    )
  )
}
