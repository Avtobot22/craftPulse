import type { paths } from './generated/schema'
import { ApiError, classifyProblem, readProblem } from './problem'
import { InvalidResponse } from './response'

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
  query?: Readonly<Record<string, string | number | undefined>>
  pathParams?: Readonly<Record<string, string>>
}
export interface ApiResponse<T> {
  data: T
  etag: string | null
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
    options: RequestOptions = {},
  ): Promise<ApiResponse<T>> {
    return this.request(method, path, body, decode, options, true)
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
      if (value !== undefined) query.set(key, String(value))
    }
    const search = query.toString()
    return '/api/v1' + resolved + (search ? '?' + search : '')
  }

  private async request<T>(
    method: string,
    path: ApiPath,
    body: unknown,
    decode: Decoder<T>,
    options: RequestOptions,
    versioned: boolean,
  ): Promise<ApiResponse<T>> {
    if (this.ended) throw new ApiError('session-ended')
    const epoch = this.epoch
    const url = this.url(path, options)
    if (method === 'GET') this.versions.delete(url)
    const headers = new Headers({ Accept: 'application/json' })
    if (body !== undefined) headers.set('Content-Type', 'application/json')
    if (versioned) {
      const etag = this.versions.get(url)
      if (!etag) throw new ApiError('missing-version')
      headers.set('If-Match', etag)
      this.versions.delete(url)
    }
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
      if (etag && !etag.startsWith('W/') && /^"[^"\r\n]*"$/u.test(etag))
        this.versions.set(url, etag)
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
