import { SourceLink } from '@/entities/activity'
import { formatFindingLocation } from '@/entities/finding'
import { ReportSha } from '@/entities/check-report'
import type { FindingDetail } from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'

const completeness = {
  complete: 'Полный',
  partial: 'Частичный',
  unknown: 'Неизвестно',
}

function linkOutcome(rule: string, status: string | undefined) {
  if (
    status === '403' ||
    status === '429' ||
    status === 'timeout' ||
    status === 'network_error' ||
    [
      'network-error',
      'link-timeout',
      'link-unverified',
      'link-access-denied',
    ].includes(rule)
  ) {
    return 'Цель не проверена из-за ошибки сети или ограничения доступа.'
  }
  if (rule === 'missing-anchor') return 'В целевом файле отсутствует якорь.'
  if (rule === 'missing-file' || status === '404' || status === '410')
    return 'Проверка подтвердила отсутствие цели.'
  return undefined
}

export function DocumentationEvidence({ finding }: { finding: FindingDetail }) {
  const { location, report, evidence } = finding
  const isSpecification = finding.finding_kind === 'openapi'
  const outcome = isSpecification
    ? undefined
    : linkOutcome(
        finding.rule_id,
        evidence.http_status ?? evidence.network_status,
      )
  return (
    <div className="cp-doc-finding__evidence">
      <section aria-labelledby="documentation-location">
        <h2 id="documentation-location">Место и правило</h2>
        <p role="status">
          Состояние:{' '}
          {contractLabel(technicalLabels.findingState, finding.state)}
        </p>
        <dl>
          <dt>{isSpecification ? 'Спецификация OpenAPI' : 'Файл'}</dt>
          <dd>{location ? formatFindingLocation(location) : 'Неизвестно'}</dd>
          <dt>Правило</dt>
          <dd>
            <code>{finding.rule_id}</code>
          </dd>
          {evidence.target_path && (
            <>
              <dt>Цель ссылки</dt>
              <dd>{evidence.target_path}</dd>
            </>
          )}
          {evidence.anchor && (
            <>
              <dt>Якорь</dt>
              <dd>
                <code>#{evidence.anchor}</code>
              </dd>
            </>
          )}
        </dl>
        {finding.state !== 'resolved' && outcome && <p>{outcome}</p>}
        {evidence.rule_description && <p>{evidence.rule_description}</p>}
        <SourceLink url={finding.source_url ?? null} sourceName="GitLab" />
      </section>
      <section aria-labelledby="documentation-report">
        <h2 id="documentation-report">Отчёт проверки</h2>
        <dl>
          <dt>Версия документа</dt>
          <dd>{report.sha ? <ReportSha sha={report.sha} /> : 'Неизвестно'}</dd>
          <dt>ID отчёта</dt>
          <dd>{report.id}</dd>
          <dt>Инструмент</dt>
          <dd>
            {report.producer} {report.producer_version}
          </dd>
          {report.profile_id && (
            <>
              <dt>Профиль</dt>
              <dd>{report.profile_id}</dd>
            </>
          )}
          <dt>Полнота</dt>
          <dd>{contractLabel(completeness, report.completeness)}</dd>
        </dl>
      </section>
    </div>
  )
}
