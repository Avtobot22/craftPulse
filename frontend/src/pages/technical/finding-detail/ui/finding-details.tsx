import { generatePath, useOutletContext, useSearchParams } from 'react-router'
import { FindingRow } from '@/entities/finding'
import { RestrictedResults } from '@/entities/result-access'
import type { CurrentUser } from '@/shared/auth'
import { routePaths } from '@/shared/config'
import { InspectionLink } from '@/shared/ui'
import {
  findingComparisonLink,
  gitLabLocationUrl,
  readFindingContext,
} from '../model/finding-links'
import type { FindingDetail } from '../model/finding-links'
import { FindingDecisions } from './finding-decisions'
import { FindingReport } from './finding-report'

const evidenceLabels: Record<string, string> = {
  rule_name: 'Название правила',
  rule_description: 'Описание правила',
  snippet: 'Фрагмент',
  target_path: 'Целевой путь',
  anchor: 'Якорь',
}

export function FindingDetails({ finding }: { finding: FindingDetail }) {
  const [search] = useSearchParams()
  const context = readFindingContext(search)
  const current = useOutletContext<CurrentUser | undefined>()
  const canReadMr = current?.actions.includes('mr.read') ?? false
  const locationUrl = gitLabLocationUrl(finding.source_url)
  const secret = finding.finding_kind === 'secret'
  const evidence = secret ? [] : Object.entries(finding.evidence)
  const exceptionSearch = new URLSearchParams(context.scope)
  exceptionSearch.set('finding_id', finding.id)
  exceptionSearch.set('state', 'pending')

  return (
    <>
      <RestrictedResults redactions={finding.redactions} />
      <div className="cp-finding-detail__columns">
        <section>
          <h2>Правило и место</h2>
          <FindingRow finding={finding} locationUrl={locationUrl} />
          {finding.package && !secret && (
            <p>
              Пакет: <code>{finding.package.purl}</code>
              {finding.package.version && ` · ${finding.package.version}`}
            </p>
          )}
        </section>
        <section>
          <h2>Основание</h2>
          {secret ? (
            <p>
              Значение секрета скрыто. Проверьте указанное место, отзовите или
              замените секрет, удалите его из дерева и проверьте историю Git.
            </p>
          ) : evidence.length ? (
            <dl>
              {evidence.map(([key, value]) => (
                <div key={key}>
                  <dt>
                    {Object.hasOwn(evidenceLabels, key)
                      ? evidenceLabels[key]
                      : key}
                  </dt>
                  <dd className="cp-finding-detail__evidence">{value}</dd>
                </div>
              ))}
            </dl>
          ) : (
            <p>Очищенное основание отсутствует.</p>
          )}
        </section>
      </div>
      <FindingReport report={finding.report} />
      <FindingDecisions finding={finding} />
      <nav aria-label="Действия и связанные результаты">
        {finding.actions.includes('create_exception') && (
          <InspectionLink
            id="finding-create-exception"
            to={{
              pathname: generatePath(routePaths.exceptionCreate, {
                findingId: encodeURIComponent(finding.id),
              }),
              search: context.scope.toString(),
            }}
          >
            Оформить исключение
          </InspectionLink>
        )}
        {finding.actions.includes('review_exception') && (
          <InspectionLink
            id="finding-review-exception"
            to={{
              pathname: generatePath(routePaths.exceptions),
              search: exceptionSearch.toString(),
            }}
          >
            Решение ревьюера
          </InspectionLink>
        )}
        <InspectionLink
          id="finding-comparison"
          to={findingComparisonLink(finding, context)}
        >
          Причины изменения результата
        </InspectionLink>
        {canReadMr && context.summary && (
          <InspectionLink id="finding-summary" to={context.summary}>
            Сводка MR
          </InspectionLink>
        )}
      </nav>
    </>
  )
}
