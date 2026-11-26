import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { FindingRow } from '@/entities/finding'
import { RestrictedResults } from '@/entities/result-access'
import { getFinding, InvalidResponse } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { ErrorState, InspectionBack, LoadingState } from '@/shared/ui'

export function FindingDetailPage({ api }: { api: ApiClient }) {
  const { findingId = '' } = useParams()
  const query = useQuery({
    ...getFinding(api, findingId),
    retry: false,
    select: (finding) => {
      if (finding.id !== findingId) throw new InvalidResponse()
      return finding
    },
  })
  const finding = query.data

  return (
    <article>
      <Text as="h1" variant="header-1">
        Карточка находки
      </Text>
      <InspectionBack />
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить карточку находки"
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {query.isSuccess && finding && (
        <>
          <RestrictedResults redactions={finding.redactions} />
          <FindingRow finding={finding} />
        </>
      )}
    </article>
  )
}
