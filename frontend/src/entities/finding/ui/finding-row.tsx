import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'
import { formatFindingLocation } from '../lib/finding-location'

type FindingRowData = Pick<
  OpenCodes<components['schemas']['Finding']>,
  'finding_kind' | 'rule_id' | 'location' | 'severity' | 'state' | 'confidence'
>

export function FindingRow({
  finding,
  locationUrl,
}: {
  finding: FindingRowData
  locationUrl?: string | undefined
}) {
  const { location, severity } = finding
  return (
    <div>
      <strong>
        {contractLabel(technicalLabels.findingKind, finding.finding_kind)}
      </strong>
      {' · '}
      <code>{finding.rule_id}</code>
      {location && (
        <>
          {' · '}
          {locationUrl ? (
            <a href={locationUrl}>
              <code>{formatFindingLocation(location)}</code>
            </a>
          ) : (
            <code>{formatFindingLocation(location)}</code>
          )}
        </>
      )}
      <div>Исходная серьёзность: {severity.native ?? 'неизвестно'}</div>
      <div>
        Сопоставленная серьёзность:{' '}
        {severity.mapped === null
          ? 'неизвестно'
          : contractLabel(technicalLabels.severity, severity.mapped)}
      </div>
      <div>
        Версия сопоставления: {severity.mapping_version ?? 'неизвестно'}
      </div>
      <div>Уверенность: {finding.confidence ?? 'неизвестно'}</div>
      <div>
        Состояние: {contractLabel(technicalLabels.findingState, finding.state)}
      </div>
    </div>
  )
}
