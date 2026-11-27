import { queryOptions } from '@tanstack/react-query'
import { InvalidResponse, readNumber, readRecord } from '@/shared/api'
import type { ApiClient } from '@/shared/api'

export function organizationStructureVersion(
  api: ApiClient,
  organizationId: string,
) {
  return queryOptions({
    queryKey: ['organization-structure-version', organizationId],
    queryFn: async ({ signal }) =>
      (
        await api.read(
          '/organization/structure',
          (value) => {
            const version = readNumber(readRecord(value).version)
            if (!Number.isSafeInteger(version) || version < 0)
              throw new InvalidResponse()
            return version
          },
          { signal },
        )
      ).data,
  })
}
