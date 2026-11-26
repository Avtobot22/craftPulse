import { useLayoutEffect, useRef } from 'react'
import type { FieldIssues } from '../model/validation'

function fieldId(scope: string, path: string) {
  return scope + path.replaceAll('/', '-')
}

export function fieldProps(scope: string, issues: FieldIssues, path: string) {
  const id = fieldId(scope, path)
  return issues.has(path)
    ? ({ id, 'aria-invalid': true, 'aria-describedby': id + '-error' } as const)
    : { id }
}

export function FieldMessage({
  scope,
  issues,
  path,
}: {
  scope: string
  issues: FieldIssues
  path: string
}) {
  const error = issues.get(path)
  return (
    error && (
      <span
        aria-hidden="true"
        className="cp-workflow__error"
        id={fieldId(scope, path) + '-error'}
      >
        {error}
      </span>
    )
  )
}

export function FormErrors({
  scope,
  issues,
  describe,
}: {
  scope: string
  issues: FieldIssues
  describe: (path: string) => string
}) {
  const summary = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    if (issues.size) summary.current?.focus()
  }, [issues])

  if (!issues.size) return null
  return (
    <div
      ref={summary}
      role="alert"
      tabIndex={-1}
      className="cp-workflow__error"
    >
      <strong>Исправьте ошибки в форме</strong>
      <ul>
        {Array.from(issues, ([path, message]) => {
          const id = fieldId(scope, path)
          return (
            <li key={path}>
              {path ? (
                <a
                  href={'#' + id}
                  onClick={(event) => {
                    event.preventDefault()
                    const field = document.getElementById(id)
                    if (field instanceof HTMLFieldSetElement) {
                      field
                        .querySelector<HTMLElement>('input, select, button')
                        ?.focus()
                    } else {
                      field?.focus()
                    }
                  }}
                >
                  {describe(path)} · {message}
                </a>
              ) : (
                message
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
