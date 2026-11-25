import { Text } from '@gravity-ui/uikit'
import type { ReviewWait } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'

function duration(seconds: number): string {
  if (seconds > 0 && seconds < 3600) return '< 1 ч'
  const hours = Math.floor(seconds / 3600)
  const days = Math.floor(hours / 24)
  return days > 0
    ? `${String(days)} д ${String(hours % 24)} ч`
    : `${String(hours)} ч`
}

export function ReviewWaitDetails({ wait }: { wait: ReviewWait }) {
  if (!wait) return <Text>Данные об ожидании ревью недоступны</Text>
  return (
    <Text as="div">
      {contractLabel(contractLabels.reviewWait, wait.kind)}
      {' · '}
      {wait.wait_seconds >= 0
        ? duration(wait.wait_seconds)
        : 'Длительность неизвестна'}
      {wait.fallback_open && ' (от открытия)'}
    </Text>
  )
}
