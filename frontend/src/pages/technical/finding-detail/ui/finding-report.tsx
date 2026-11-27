import { CommitSha } from '@/entities/check-report'
import { contractLabel } from '@/shared/config'
import type { components } from '@/shared/api'
import type { FindingDetail } from '../model/finding-links'

type Report = components['schemas']['FindingDetail']['report']

const reportModes = {
  import: 'Отчёт клиента',
  builtin: 'Штатный профиль',
} satisfies Record<NonNullable<Report['mode']>, string>

const completenessLabels = {
  complete: 'Полный',
  partial: 'Частичный',
  unknown: 'Неизвестна',
} satisfies Record<Report['completeness'], string>

export function FindingReport({ report }: { report: FindingDetail['report'] }) {
  return (
    <section>
      <h2>Отчёт</h2>
      <dl>
        <dt>ID отчёта</dt>
        <dd>
          <code>{report.id}</code>
        </dd>
        <dt>Инструмент</dt>
        <dd>{report.producer}</dd>
        <dt>Версия инструмента</dt>
        <dd>{report.producer_version}</dd>
        <dt>Профиль проверки</dt>
        <dd>
          {report.profile_id ? (
            <code>{report.profile_id}</code>
          ) : (
            'Профиль не указан'
          )}
        </dd>
        <dt>Режим</dt>
        <dd>
          {report.mode ? contractLabel(reportModes, report.mode) : 'неизвестно'}
        </dd>
        <dt>Версия правил</dt>
        <dd>{report.rules_version ?? 'неизвестно'}</dd>
        <dt>Версия базы</dt>
        <dd>{report.db_version ?? 'неизвестно'}</dd>
        <dt>SHA</dt>
        <dd>{report.sha ? <CommitSha sha={report.sha} /> : 'неизвестно'}</dd>
        <dt>Полнота</dt>
        <dd>{contractLabel(completenessLabels, report.completeness)}</dd>
      </dl>
    </section>
  )
}
