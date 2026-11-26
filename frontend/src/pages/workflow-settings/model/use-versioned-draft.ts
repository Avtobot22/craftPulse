import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { isStrongETag } from '@/shared/api'
import type { ApiResponse } from '@/shared/api'
import type {
  ChangeAttempt,
  ChangePreview,
  ChangeRecheck,
  ChangeValidation,
  RecheckReason,
} from '@/shared/ui'
import type { VersionApi, VersionPage } from '../api/workflow-api'
import { validationIssues } from './validation'
import type { FieldIssues } from './validation'

interface Version {
  version_no: number
  confirmed_by: string
  confirmed_at: string
}

function includeVersion<V extends Version>(page: VersionPage<V>, version: V) {
  return {
    ...page,
    data: [
      version,
      ...page.data.filter((item) => item.version_no !== version.version_no),
    ],
  }
}

export function useVersionedDraft<D, V extends Version>({
  api,
  initialPage,
  initialDraft,
  fromVersion,
  compare,
  validate,
}: {
  api: VersionApi<D, V>
  initialPage: ApiResponse<VersionPage<V>>
  initialDraft: D
  fromVersion: (version: V) => D
  compare: (current: D | undefined, draft: D) => ChangePreview[]
  validate: (draft: D) => FieldIssues
}) {
  const [snapshot, setSnapshot] = useState(initialPage)
  const [draft, setDraft] = useState(initialDraft)
  const [issues, setIssues] = useState<FieldIssues>(new Map())
  const [saved, setSaved] = useState<V>()
  const [noChanges, setNoChanges] = useState(false)
  const [original, setOriginal] = useState(initialDraft)
  const current = snapshot.data.data[0]
  const changes = compare(current && fromVersion(current), draft)
  const dirty = compare(original, draft).length > 0

  const older = useMutation({
    mutationFn: (cursor: string) =>
      api.list(AbortSignal.timeout(30_000), cursor),
    onSuccess: ({ data }, cursor) => {
      setSnapshot((existing) => {
        if (existing.data.next_cursor !== cursor) return existing
        const seen = new Set(
          existing.data.data.map((version) => version.version_no),
        )
        return {
          ...existing,
          data: {
            ...data,
            data: [
              ...existing.data.data,
              ...data.data.filter((version) => !seen.has(version.version_no)),
            ],
          },
        }
      })
    },
  })

  function edit(next: D) {
    setDraft(next)
    setIssues(new Map())
    setSaved(undefined)
    setNoChanges(false)
  }

  function check() {
    const errors = validate(draft)
    setIssues(errors)
    return errors.size === 0
  }

  function proposal() {
    if (!check()) return undefined
    if (!changes.length) {
      setNoChanges(true)
      return undefined
    }
    return { etag: snapshot.etag ?? '', draft, changes }
  }

  async function recheck(
    attempt: ChangeAttempt<D>,
    reason: RecheckReason,
    signal: AbortSignal,
  ): Promise<ChangeRecheck<D, ApiResponse<V>>> {
    const latest = await api.list(signal)
    signal.throwIfAborted()
    if (!isStrongETag(latest.etag)) return { kind: 'unknown' }
    if (latest.etag === attempt.etag) {
      // These version endpoints support replay with the original key.
      return reason === 'uncertain'
        ? { kind: 'retryable' }
        : { kind: 'unknown' }
    }
    const version = latest.data.data[0]
    const fresh = compare(version && fromVersion(version), attempt.draft)
    // A lost response can be reconciled by the observed effect. A 412 always
    // presents the newly loaded baseline and requires an explicit decision.
    setSnapshot(latest)
    if (reason === 'uncertain' && version && !fresh.length) {
      return {
        kind: 'applied',
        result: { data: version, etag: latest.etag },
      }
    }
    return {
      kind: 'not-applied',
      proposal: { etag: latest.etag, changes: fresh, draft: attempt.draft },
    }
  }

  function accept({ data, etag }: ApiResponse<V>) {
    const next = fromVersion(data)
    setSnapshot((existing) => ({
      data: includeVersion(existing.data, data),
      etag,
    }))
    setOriginal(next)
    if (compare(next, draft).length) setDraft(next)
    setIssues(new Map())
    setNoChanges(current?.version_no === data.version_no)
    setSaved(data)
  }

  function onValidation({ fields }: ChangeValidation) {
    setIssues(validationIssues(fields))
  }

  return {
    current,
    versions: snapshot.data.data,
    older,
    nextCursor: snapshot.data.next_cursor,
    draft,
    dirty,
    issues,
    saved,
    noChanges,
    edit,
    check,
    proposal,
    recheck,
    accept,
    onValidation,
  }
}
