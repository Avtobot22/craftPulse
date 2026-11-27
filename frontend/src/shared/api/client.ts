import type { components, operations, paths } from './generated/schema'
import { ApiError, classifyProblem, readProblem } from './problem'
import { readLink } from './read-fields'
import { InvalidResponse, readRecord, readString } from './response'

export type ApiPath = Exclude<keyof paths, `/o/${string}`>
type ReadPath = {
  [P in ApiPath]: [NonNullable<paths[P]['get']>] extends [never] ? never : P
}[ApiPath]
export type MutationMethod = 'PUT' | 'PATCH' | 'POST' | 'DELETE'
type MutationPath<M extends MutationMethod> = {
  [P in ApiPath]: [NonNullable<paths[P][Lowercase<M>]>] extends [never]
    ? never
    : NonNullable<paths[P][Lowercase<M>]> extends {
          parameters: { header: { 'If-Match': string } }
        }
      ? P
      : never
}[ApiPath]
type MutationBody<P extends ApiPath, M extends MutationMethod> =
  NonNullable<paths[P][Lowercase<M>]> extends {
    requestBody: { content: { 'application/json': infer Body } }
  }
    ? Body
    : undefined
export type Decoder<T> = (value: unknown) => T
export interface RequestOptions {
  signal?: AbortSignal
  query?: Readonly<
    Record<string, string | number | boolean | readonly string[] | undefined>
  >
  pathParams?: Readonly<Record<string, string>>
}
export interface VersionedRequestOptions extends RequestOptions {
  /** The ETag shown in the confirmation, even if a newer read updated the cache. */
  ifMatch?: string
  idempotencyKey?: string
}
export interface ApiResponse<T> {
  data: T
  etag: string | null
}

export function isStrongETag(
  value: string | null | undefined,
): value is string {
  return (
    typeof value === 'string' &&
    /^"[\x21\x23-\x7e\u0080-\u00ff]*"$/u.test(value)
  )
}

export class ApiClient {
  private epoch = 0
  private ended = false
  private versions = new Map<string, string>()
  private authListeners = new Set<() => void>()

  private readonly fetcher: typeof fetch

  constructor(fetcher: typeof fetch = globalThis.fetch.bind(globalThis)) {
    this.fetcher = fetcher
  }

  onAuthRequired(listener: () => void): () => void {
    this.authListeners.add(listener)
    return () => {
      this.authListeners.delete(listener)
    }
  }

  endSession(): void {
    this.epoch += 1
    this.ended = true
    this.versions.clear()
  }

  read<T>(
    path: ReadPath,
    decode: Decoder<T>,
    options: RequestOptions = {},
  ): Promise<ApiResponse<T>> {
    return this.request('GET', path, undefined, decode, options, false)
  }

  update<M extends MutationMethod, P extends MutationPath<M>, T>(
    method: M,
    path: P,
    body: MutationBody<P, M>,
    decode: Decoder<T>,
    options: VersionedRequestOptions = {},
  ): Promise<ApiResponse<T>> {
    return this.request(method, path, body, decode, options, true)
  }

  deleteConnection(connectionId: string, signal: AbortSignal) {
    return this.request(
      'DELETE',
      '/connections/{connection_id}',
      undefined,
      (value) => {
        if (value !== undefined) throw new InvalidResponse()
      },
      { pathParams: { connection_id: connectionId }, signal },
      false,
    )
  }

  requestSyncRun<T>(
    connectionId: string,
    body: components['schemas']['SyncRunRequest'],
    decode: Decoder<T>,
    signal: AbortSignal,
  ) {
    return this.request(
      'POST',
      '/connections/{connection_id}/sync-runs',
      body,
      decode,
      { pathParams: { connection_id: connectionId }, signal },
      false,
    )
  }

  replaceUserRoles<T>(
    userId: string,
    body: {
      data: (Omit<components['schemas']['RoleAssignmentInput'], 'role'> & {
        role: string
      })[]
    },
    decode: Decoder<T>,
    options: { ifMatch: string; signal: AbortSignal },
  ): Promise<ApiResponse<T>> {
    // The editor can resend role codes introduced by a newer server.
    return this.request(
      'PUT',
      '/users/{user_id}/roles',
      body,
      decode,
      {
        ...options,
        pathParams: { user_id: userId },
      },
      true,
    )
  }

  changeUserAccess(
    userId: string,
    action: 'block' | 'unblock' | 'sessions',
    signal: AbortSignal,
  ) {
    const path = {
      block: '/users/{user_id}/block',
      unblock: '/users/{user_id}/unblock',
      sessions: '/users/{user_id}/sessions',
    } as const
    return this.request(
      action === 'sessions' ? 'DELETE' : 'POST',
      path[action],
      undefined,
      (value) => {
        if (value !== undefined) throw new InvalidResponse()
      },
      { pathParams: { user_id: userId }, signal },
      false,
    )
  }

  startSourceLink(
    connectionId: operations['startSourceLink']['parameters']['path']['connection_id'],
    returnTo: string,
    signal: AbortSignal,
  ) {
    return this.request(
      'POST',
      '/me/source-links/{connection_id}/authorization',
      undefined,
      (value): components['schemas']['AuthorizationRedirect'] => {
        const url = new URL(readString(readRecord(value).authorization_url))
        if (
          !['https:', 'http:'].includes(url.protocol) ||
          url.username ||
          url.password
        ) {
          throw new InvalidResponse()
        }
        return { authorization_url: url.href }
      },
      {
        pathParams: { connection_id: connectionId },
        query: { return_to: returnTo },
        signal,
      },
      false,
    )
  }

  resolveIngestConflict<T>(
    conflictId: operations['resolveIngestConflict']['parameters']['path']['conflict_id'],
    body: MutationBody<'/ingest-conflicts/{conflict_id}/resolution', 'POST'>,
    decode: Decoder<T>,
    signal: AbortSignal,
  ): Promise<ApiResponse<T>> {
    return this.request(
      'POST',
      '/ingest-conflicts/{conflict_id}/resolution',
      body,
      decode,
      { pathParams: { conflict_id: conflictId }, signal },
      false,
    )
  }

  createConnection<T>(
    body: MutationBody<'/connections', 'POST'>,
    decode: Decoder<T>,
    signal: AbortSignal,
  ): Promise<ApiResponse<T>> {
    return this.request('POST', '/connections', body, decode, { signal }, false)
  }

  startConnectionProbe<T>(
    connectionId: operations['startConnectionProbe']['parameters']['path']['connection_id'],
    decode: Decoder<T>,
    signal: AbortSignal,
  ): Promise<ApiResponse<T>> {
    return this.request(
      'POST',
      '/connections/{connection_id}/probes',
      undefined,
      decode,
      { pathParams: { connection_id: connectionId }, signal },
      false,
    )
  }

  updateConnection<T>(
    connectionId: operations['updateConnection']['parameters']['path']['connection_id'],
    body: operations['updateConnection']['requestBody']['content']['application/merge-patch+json'],
    decode: Decoder<T>,
    ifMatch: string,
    signal: AbortSignal,
  ): Promise<ApiResponse<T>> {
    return this.request(
      'PATCH',
      '/connections/{connection_id}',
      body,
      decode,
      { pathParams: { connection_id: connectionId }, ifMatch, signal },
      true,
      'application/merge-patch+json',
    )
  }

  createException<T>(
    body: MutationBody<'/exceptions', 'POST'>,
    decode: Decoder<T>,
    options: { idempotencyKey: string; signal: AbortSignal },
  ): Promise<ApiResponse<T>> {
    return this.request('POST', '/exceptions', body, decode, options, false)
  }

  createUploadToken<T>(
    body: MutationBody<'/upload-tokens', 'POST'>,
    decode: Decoder<T>,
    options: { idempotencyKey: string; signal: AbortSignal },
  ): Promise<ApiResponse<T>> {
    return this.request('POST', '/upload-tokens', body, decode, options, false)
  }

  revokeUploadToken(tokenId: string, signal: AbortSignal) {
    return this.request(
      'DELETE',
      '/upload-tokens/{upload_token_id}',
      undefined,
      (value) => {
        if (value !== undefined) throw new InvalidResponse()
      },
      { pathParams: { upload_token_id: tokenId }, signal },
      false,
    )
  }

  decideLink(
    linkId: operations['decideLink']['parameters']['path']['link_id'],
    body: operations['decideLink']['requestBody']['content']['application/json'],
    idempotencyKey: operations['decideLink']['parameters']['header']['Idempotency-Key'],
    signal: AbortSignal,
  ) {
    return this.request(
      'POST',
      '/links/{link_id}/decisions',
      body,
      readLink,
      { pathParams: { link_id: linkId }, idempotencyKey, signal },
      false,
    )
  }

  previewMapping<T>(
    teamId: string,
    body: NonNullable<
      paths['/teams/{team_id}/mapping-previews']['post']
    >['requestBody']['content']['application/json'],
    decode: Decoder<T>,
    signal: AbortSignal,
  ): Promise<ApiResponse<T>> {
    return this.request(
      'POST',
      '/teams/{team_id}/mapping-previews',
      body,
      decode,
      { pathParams: { team_id: teamId }, signal },
      false,
    )
  }

  requestDocumentationCheck<T>(
    serviceId: string,
    body: MutationBody<'/services/{service_id}/documentation-checks', 'POST'>,
    decode: Decoder<T>,
  ): Promise<ApiResponse<T>> {
    return this.request(
      'POST',
      '/services/{service_id}/documentation-checks',
      body,
      decode,
      {
        pathParams: { service_id: serviceId },
        signal: AbortSignal.timeout(30_000),
      },
      false,
    )
  }

  startSeriesRefresh<T>(
    seriesId: string,
    decode: Decoder<T>,
  ): Promise<ApiResponse<T>> {
    return this.request(
      'POST',
      '/series/{series_id}/refreshes',
      undefined,
      decode,
      { pathParams: { series_id: seriesId } },
      false,
    )
  }

  createSavedView<T>(
    body: MutationBody<'/saved-views', 'POST'>,
    decode: Decoder<T>,
  ): Promise<ApiResponse<T>> {
    return this.request('POST', '/saved-views', body, decode, {}, false)
  }

  deleteSavedView(id: string): Promise<ApiResponse<void>> {
    return this.request(
      'DELETE',
      '/saved-views/{saved_view_id}',
      undefined,
      (value) => {
        if (value !== undefined) throw new InvalidResponse()
        return undefined
      },
      { pathParams: { saved_view_id: id } },
      false,
    )
  }

  logout(): Promise<ApiResponse<void>> {
    return this.request(
      'POST',
      '/session/logout',
      undefined,
      (value) => {
        if (value !== undefined) throw new InvalidResponse()
        return undefined
      },
      {},
      false,
    )
  }

  private url(path: ApiPath, options: RequestOptions): string {
    const resolved = path.replace(
      /\{([^}]+)\}/gu,
      (_match: string, key: string) => {
        const value = options.pathParams?.[key]
        if (value === undefined) throw new ApiError('invalid-response')
        return encodeURIComponent(value)
      },
    )
    const query = new URLSearchParams()
    for (const [key, value] of Object.entries(options.query ?? {})) {
      if (typeof value === 'object')
        for (const item of value) query.append(key, item)
      else if (value !== undefined) query.set(key, String(value))
    }
    const search = query.toString()
    return '/api/v1' + resolved + (search ? '?' + search : '')
  }

  private async request<T>(
    method: string,
    path: ApiPath,
    body: unknown,
    decode: Decoder<T>,
    options: VersionedRequestOptions,
    versioned: boolean,
    mediaType = 'application/json',
  ): Promise<ApiResponse<T>> {
    if (this.ended) throw new ApiError('session-ended')
    const epoch = this.epoch
    const url = this.url(path, options)
    if (method === 'GET') this.versions.delete(url)
    const headers = new Headers({ Accept: 'application/json' })
    if (body !== undefined) headers.set('Content-Type', mediaType)
    if (versioned) {
      const etag = options.ifMatch ?? this.versions.get(url)
      if (!isStrongETag(etag)) throw new ApiError('missing-version')
      headers.set('If-Match', etag)
      this.versions.delete(url)
    }
    if (options.idempotencyKey !== undefined)
      headers.set('Idempotency-Key', options.idempotencyKey)
    let response: Response
    try {
      response = await this.fetcher(url, {
        method,
        headers,
        credentials: 'same-origin',
        cache: 'no-store',
        ...(options.signal === undefined ? {} : { signal: options.signal }),
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      })
    } catch (error) {
      this.assertSession(epoch)
      if (options.signal?.aborted) throw error
      throw new ApiError('network')
    }
    this.assertSession(epoch)
    if (!response.ok) {
      let error: ApiError
      try {
        if (
          response.headers.get('Content-Type')?.split(';')[0]?.trim() !==
          'application/problem+json'
        ) {
          throw new InvalidResponse()
        }
        const raw: unknown = await response.json()
        this.assertSession(epoch)
        error = classifyProblem(readProblem(raw, response.status))
      } catch (cause) {
        this.assertSession(epoch)
        if (options.signal?.aborted) throw cause
        error = new ApiError(
          response.status === 401 ? 'auth-required' : 'invalid-response',
        )
      }
      if (response.status === 401) {
        this.endSession()
        for (const listener of this.authListeners) listener()
      }
      throw error
    }
    try {
      let raw: unknown
      if (response.status !== 204) {
        if (
          response.headers.get('Content-Type')?.split(';')[0]?.trim() !==
          'application/json'
        ) {
          throw new InvalidResponse()
        }
        raw = await response.json()
      }
      this.assertSession(epoch)
      options.signal?.throwIfAborted()
      const data = decode(raw)
      const etag = response.headers.get('ETag')
      if (isStrongETag(etag)) this.versions.set(url, etag)
      else this.versions.delete(url)
      return { data, etag }
    } catch (error) {
      if (error instanceof ApiError) throw error
      this.assertSession(epoch)
      if (options.signal?.aborted) throw error
      throw new ApiError('invalid-response')
    }
  }

  private assertSession(epoch: number): void {
    if (epoch !== this.epoch || this.ended) throw new ApiError('session-ended')
  }
}
