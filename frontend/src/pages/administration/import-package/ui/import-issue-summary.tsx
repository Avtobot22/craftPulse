import { useEffect, useRef } from 'react'
import { Link } from 'react-router'
import { adminLinks } from '@/shared/config'
import { fieldLabels } from '../model/import-issues'
import type { ImportIssue } from '../model/import-issues'
import { fieldId } from './import-fields'

export function ImportIssueSummary({
  issues,
  journalAllowed,
}: {
  issues: readonly ImportIssue[]
  journalAllowed: boolean
}) {
  const summary = useRef<HTMLDivElement>(null)
  useEffect(() => {
    summary.current?.focus()
  }, [])

  return (
    <div
      className="cp-import__summary"
      ref={summary}
      tabIndex={-1}
      role="alert"
    >
      <strong>Не удалось передать файл</strong>
      <ul>
        {issues.map(({ field, message }, index) => (
          <li key={index}>
            {field ? (
              <a
                href={'#' + fieldId(field)}
                onClick={(event) => {
                  event.preventDefault()
                  document.getElementById(fieldId(field))?.focus()
                }}
              >
                {fieldLabels[field]} · {message}
              </a>
            ) : (
              message
            )}
          </li>
        ))}
      </ul>
      {journalAllowed &&
        issues.some(
          ({ message }) => message === 'Исход передачи пока не известен',
        ) && (
          <Link to={adminLinks.receiptJournal()}>Проверить журнал приёма</Link>
        )}
    </div>
  )
}
