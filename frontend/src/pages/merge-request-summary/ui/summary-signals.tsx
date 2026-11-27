import { generatePath } from 'react-router'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { InspectionLink } from '@/shared/ui'
import { summaryComparisonLink } from '../model/comparison-link'

type Item = OpenCodes<components['schemas']['SummaryItem']>

const sections = [
  { kind: 'new', title: 'Новые находки' },
  { kind: 'resolved', title: 'Устранённые находки' },
  { kind: 'newly_observed', title: 'Впервые наблюдено' },
  { kind: 'accepted', title: 'Находки с действующим исключением' },
  { kind: 'unchanged', title: 'Находки без изменений' },
] satisfies {
  kind: components['schemas']['SummaryItem']['item_kind']
  title: string
}[]

const modes = {
  import: 'Отчёт клиента',
  builtin: 'Штатный профиль',
} satisfies Record<
  NonNullable<components['schemas']['SummaryItem']['mode']>,
  string
>

export function SummarySignals({
  items,
  hasBase,
  canReadFindings,
  comparisonSearch,
  findingSearch,
}: {
  items: Item[]
  hasBase: boolean
  canReadFindings: boolean
  comparisonSearch: URLSearchParams
  findingSearch: URLSearchParams
}) {
  const groups = sections.filter(
    ({ kind }) => hasBase || kind === 'accepted' || kind === 'newly_observed',
  )
  return groups.map(({ kind, title }) => {
    const group = items.filter((item) => item.item_kind === kind)
    const ranked =
      kind === 'new'
        ? group
            .filter(({ rank }) => rank !== null && rank >= 1 && rank <= 5)
            .sort((left, right) => (left.rank ?? 0) - (right.rank ?? 0))
        : []
    const rest = group.filter((item) => !ranked.includes(item))
    const renderItem = (item: Item) => (
      <SignalItem
        key={item.item_no}
        item={item}
        canReadFindings={canReadFindings}
        comparisonSearch={comparisonSearch}
        findingSearch={findingSearch}
      />
    )

    if (
      group.length === 0 &&
      (kind === 'unchanged' || (hasBase && kind === 'newly_observed'))
    )
      return null
    return (
      <section key={kind}>
        <h3>{title}</h3>
        {ranked.length > 0 && (
          <>
            <h4 id="summary-priority-heading">Приоритетные находки</h4>
            <ol
              aria-labelledby="summary-priority-heading"
              className="cp-summary-priority"
            >
              {ranked.map(renderItem)}
            </ol>
          </>
        )}
        {rest.length > 0 && <ul>{rest.map(renderItem)}</ul>}
        {!group.length && <p>В сводке нет доступных строк этой категории.</p>}
      </section>
    )
  })
}

function SignalItem({
  item,
  canReadFindings,
  comparisonSearch,
  findingSearch,
}: {
  item: Item
  canReadFindings: boolean
  comparisonSearch: URLSearchParams
  findingSearch: URLSearchParams
}) {
  const {
    finding_id,
    rule_id,
    rank,
    location,
    severity,
    analysis_kind,
    mode,
    item_kind,
    item_no,
  } = item
  const findingPath =
    finding_id &&
    generatePath(routePaths.findingDetail, {
      findingId: encodeURIComponent(finding_id),
    })
  const context = new URLSearchParams(findingSearch)
  if (
    analysis_kind &&
    Object.hasOwn(technicalLabels.analysisKind, analysis_kind)
  )
    context.set('analysis_kind', analysis_kind)
  const rule = rule_id ? <code>{rule_id}</code> : 'Находка'

  return (
    <li
      id={
        item_kind === 'new' && finding_id ? `finding-${finding_id}` : undefined
      }
    >
      {item_kind === 'new' && rank !== null && rank >= 1 && rank <= 5 && (
        <strong>Приоритет {rank} · </strong>
      )}
      {canReadFindings && findingPath ? (
        <InspectionLink
          id={`summary-item-${String(item_no)}`}
          to={{ pathname: findingPath, search: context.toString() }}
        >
          {rule}
        </InspectionLink>
      ) : (
        rule
      )}
      {severity && ` · ${contractLabel(technicalLabels.severity, severity)}`}
      {analysis_kind &&
        ` · ${contractLabel(technicalLabels.analysisKind, analysis_kind)}`}
      {mode && ` · ${contractLabel(modes, mode)}`}
      {location && (
        <>
          {' '}
          ·{' '}
          <code>
            {location.path}
            {location.line_start !== null && `:${String(location.line_start)}`}
          </code>
        </>
      )}
      {item_kind === 'accepted' && ' · Принята по исключению'}
      {canReadFindings && finding_id && analysis_kind && (
        <div>
          <InspectionLink
            id={`summary-comparison-${String(item_no)}`}
            to={summaryComparisonLink(comparisonSearch, item)}
          >
            Причины изменения · {rule}
          </InspectionLink>
        </div>
      )}
    </li>
  )
}
