import { Label } from '@gravity-ui/uikit'
import { contractLabel, technicalLabels } from '@/shared/config'

export function CheckReportState({
  state,
  reasonCode,
  reportedAt,
}: {
  state: string
  reasonCode?: string | null
  reportedAt?: string | null
}) {
  return (
    <span>
      <Label
        theme={
          state === 'partial' || state === 'malformed' ? 'warning' : 'normal'
        }
      >
        {contractLabel(technicalLabels.reportState, state)}
      </Label>
      {state === 'missing' && ' · не проверено'}
      {reasonCode &&
        ` · ${contractLabel(technicalLabels.reportReason, reasonCode)}`}
      {reportedAt && (
        <>
          {' '}
          · Отчёт от <time dateTime={reportedAt}>{reportedAt}</time>
        </>
      )}
    </span>
  )
}
