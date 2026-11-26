import type { ReactNode } from 'react'
import { ErrorState, LoadingState } from '@/shared/ui'
import type { ReportSeries } from '../api/report-api'
import { formatTime } from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'

export function DetailPoint({
  block,
  series,
  step,
  id,
  children,
  cohort,
}: {
  block: ReportBlockProps
  series: ReportSeries | undefined
  step: string
  id: string
  children: ReactNode
  cohort?: { taskType: string | null | undefined; size: string | null }
}) {
  if (!series) return children
  return (
    <button
      type="button"
      className="cp-team-report__point-link"
      id={id}
      onClick={() => {
        block.selectPoint(series, step, id, undefined, cohort)
      }}
    >
      {children}
    </button>
  )
}

export function StepSelect({
  steps,
  step,
  label,
  timeZone,
  onChange,
}: {
  steps: string[]
  step: string
  label: string
  timeZone: string
  onChange: (step: string) => void
}) {
  if (steps.length < 2)
    return <time dateTime={step}>{formatTime(step, timeZone)}</time>
  return (
    <label className="cp-team-report__point-period">
      {label}
      <select
        value={step}
        onChange={(event) => {
          onChange(event.currentTarget.value)
        }}
      >
        {steps.map((start) => (
          <option key={start} value={start}>
            {formatTime(start, timeZone)}
          </option>
        ))}
      </select>
    </label>
  )
}

export function DetailsStatus({
  query,
  reason,
}: {
  query: {
    isError: boolean
    isFetching: boolean
    refetch: () => Promise<unknown>
  }
  reason: string
}) {
  if (query.isError)
    return (
      <ErrorState
        reason={reason}
        onRetry={() => void query.refetch()}
        retrying={query.isFetching}
      />
    )
  if (query.isFetching) return <LoadingState />
  return null
}
