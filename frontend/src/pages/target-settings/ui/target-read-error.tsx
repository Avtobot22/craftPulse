import { ApiError } from '@/shared/api'
import { ErrorState } from '@/shared/ui'

export function TargetReadError({
  error,
  reason,
  retry,
  retrying,
  updatedAt,
}: {
  error: unknown
  reason: string
  retry: () => void
  retrying: boolean
  updatedAt: number
}) {
  const correlationId =
    error instanceof ApiError ? error.problem?.correlation_id : undefined
  return (
    <ErrorState
      reason={reason}
      onRetry={retry}
      retrying={retrying}
      {...(correlationId ? { correlationId } : {})}
      {...(updatedAt ? { lastSuccess: new Date(updatedAt).toISOString() } : {})}
    />
  )
}
