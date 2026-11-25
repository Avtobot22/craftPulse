import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'

const nonFindings = new Set([
  'not_evaluated',
  'ci_attempts',
  'coverage',
  'tests',
])

export function SummarySignals({
  items,
}: {
  items: OpenCodes<components['schemas']['SummaryItem']>[]
}) {
  const signals = items.filter((item) => !nonFindings.has(item.item_kind))
  if (signals.length === 0) return <p>Нет доступных находок.</p>
  return (
    <ul>
      {signals.map((item) => (
        <li
          key={item.item_no}
          id={
            item.item_kind === 'new' && item.finding_id
              ? `finding-${item.finding_id}`
              : undefined
          }
        >
          {item.rank !== null && `№${String(item.rank)} · `}
          {contractLabel(technicalLabels.summaryItem, item.item_kind)}
          {item.rule_id && (
            <>
              {' '}
              · <code>{item.rule_id}</code>
            </>
          )}
          {item.severity &&
            ` · ${contractLabel(technicalLabels.severity, item.severity)}`}
          {item.location && (
            <>
              {' '}
              ·{' '}
              <code>
                {item.location.path}
                {item.location.line_start !== null &&
                  `:${String(item.location.line_start)}`}
              </code>
            </>
          )}
        </li>
      ))}
    </ul>
  )
}
