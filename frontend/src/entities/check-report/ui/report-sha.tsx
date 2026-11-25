import { ClipboardButton } from '@gravity-ui/uikit'
import { useState } from 'react'
import type { components } from '@/shared/api'

type GitSha = components['schemas']['GitSha']
type LastGood = NonNullable<
  components['schemas']['MergeRequestSummary']['last_good']
>

export function CommitSha({ sha }: { sha: GitSha }) {
  const [copy, setCopy] = useState<{ sha: GitSha; ok: boolean }>()
  return (
    <span>
      <code title={sha} aria-label={sha}>
        {sha.slice(0, 12)}
      </code>{' '}
      <ClipboardButton
        text={sha}
        size="s"
        aria-label={`Скопировать SHA ${sha}`}
        tooltipInitialText="Скопировать SHA"
        tooltipSuccessText="Скопировано"
        onCopy={(value, ok) => {
          setCopy({ sha: value, ok })
        }}
      >
        Копировать
      </ClipboardButton>
      <span role="status">
        {copy?.sha === sha &&
          (copy.ok ? 'Скопировано' : 'Не удалось скопировать SHA')}
      </span>
    </span>
  )
}

export function ReportSha({
  sha,
  expectedSha,
}: {
  sha: GitSha
  expectedSha?: GitSha | null
}) {
  return (
    <span>
      <CommitSha sha={sha} />
      {expectedSha &&
        sha.toLowerCase() !== expectedSha.toLowerCase() &&
        ' · Отчёт другого SHA'}
    </span>
  )
}

export function LastGoodResult({
  result,
}: {
  result: Pick<LastGood, 'head_sha' | 'built_at'>
}) {
  return (
    <p>
      Последний хороший результат от{' '}
      <time dateTime={result.built_at}>{result.built_at}</time>{' '}
      <CommitSha sha={result.head_sha} />
    </p>
  )
}
