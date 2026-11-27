import type { ApiClient } from './client'
import type { components } from './generated/schema'
import { readArray, readEnum, readRecord, readString } from './response'

type SourceLinkSchema = components['schemas']['SourceLink']
type SourceLink = Pick<SourceLinkSchema, 'connection_id' | 'display_name'> & {
  state: SourceLinkSchema['state'] | 'unknown'
}

function readSourceLink(value: unknown): SourceLink {
  const raw = readRecord(value)
  return {
    connection_id: readString(raw.connection_id),
    display_name: readString(raw.display_name),
    state: readEnum<SourceLinkSchema['state']>(raw.state, [
      'not_linked',
      'linked',
      'expired',
      'revoked',
    ]),
  }
}

export async function listSourceLinks(api: ApiClient, signal: AbortSignal) {
  const { data } = await api.read(
    '/me/source-links',
    (value) => readArray(readRecord(value).data, readSourceLink),
    { signal },
  )
  return data
}
