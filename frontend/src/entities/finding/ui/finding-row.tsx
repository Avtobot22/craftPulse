import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'

type FindingRowData = Pick<
  OpenCodes<components['schemas']['Finding']>,
  'finding_kind' | 'rule_id' | 'location' | 'severity' | 'state' | 'confidence'
>

export function FindingRow({ finding }: { finding: FindingRowData }) {
  const { location, severity } = finding
  const firstLine = location?.line_start ?? location?.line_end
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
          <code>
            {location.path}
            {firstLine !== null &&
              firstLine !== undefined &&
              `:${String(firstLine)}`}
            {location.line_end !== null &&
              location.line_end !== firstLine &&
              `-${String(location.line_end)}`}
          </code>
        </>
      )}
      <div>Исходная серьёзность: {severity.native ?? 'неизвестно'}</div>
      <div>
        Сопоставленная серьёзность:{' '}
        {severity.mapped === null
          ? 'неизвестно'
          : contractLabel(technicalLabels.severity, severity.mapped)}
      </div>
      <div>Уверенность: {finding.confidence ?? 'неизвестно'}</div>
      <div>
        Состояние: {contractLabel(technicalLabels.findingState, finding.state)}
      </div>
    </div>
  )
}
