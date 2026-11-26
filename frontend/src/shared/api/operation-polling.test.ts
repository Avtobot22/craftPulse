import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { probeExample, receiptExample } from '../../../test/admin-fixtures'
import { jsonResponse, problemResponse } from '../../../test/fixtures'
import { ApiClient, getConnectionProbe, getReceipt } from './index'

const quarantined = {
  ...receiptExample,
  status: 'quarantined',
  retry_after_seconds: 3,
}

const probeOptions = (api: ApiClient) =>
  getConnectionProbe(api, 'connection-1', probeExample.id)

let client: QueryClient
let dispose: (() => void)[]

beforeEach(() => {
  vi.useFakeTimers()
  client = new QueryClient()
  dispose = []
})

afterEach(() => {
  for (const unsubscribe of dispose) unsubscribe()
  client.clear()
  vi.useRealTimers()
})

it('waits for each probe response delay and stops with a terminal status', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(
      jsonResponse({
        ...probeExample,
        status: 'pending',
        retry_after_seconds: 3,
      }),
    )
    .mockResolvedValueOnce(
      jsonResponse({
        ...probeExample,
        status: 'running',
        retry_after_seconds: 5,
      }),
    )
    .mockResolvedValueOnce(jsonResponse(probeExample))
  const api = new ApiClient(fetcher)
  const observer = new QueryObserver(client, probeOptions(api))
  dispose.push(observer.subscribe(() => undefined))

  await vi.advanceTimersByTimeAsync(2999)
  expect(fetcher).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(1)
  expect(observer.getCurrentResult().data?.status).toBe('running')
  await vi.advanceTimersByTimeAsync(4999)
  expect(fetcher).toHaveBeenCalledTimes(2)
  await vi.advanceTimersByTimeAsync(1)
  expect(observer.getCurrentResult().data?.status).toBe(probeExample.status)
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetcher).toHaveBeenCalledTimes(3)
})

it('polls a quarantined receipt and stops after it is processed', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(jsonResponse(quarantined))
    .mockResolvedValueOnce(jsonResponse(receiptExample))
  const api = new ApiClient(fetcher)
  const observer = new QueryObserver(client, getReceipt(api, receiptExample.id))
  dispose.push(observer.subscribe(() => undefined))

  await vi.advanceTimersByTimeAsync(2999)
  expect(fetcher).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(1)
  expect(observer.getCurrentResult().data?.status).toBe('partial')
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetcher).toHaveBeenCalledTimes(2)
})

it('shares a slow poll between observers and cancels it when they leave', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(jsonResponse(quarantined))
    .mockImplementation(() => new Promise(() => undefined))
  const api = new ApiClient(fetcher)
  const first = new QueryObserver(client, getReceipt(api, receiptExample.id))
  const second = new QueryObserver(client, getReceipt(api, receiptExample.id))
  const unsubscribeFirst = first.subscribe(() => undefined)
  dispose.push(unsubscribeFirst)
  await vi.advanceTimersByTimeAsync(1000)
  const unsubscribeSecond = second.subscribe(() => undefined)
  dispose.push(unsubscribeSecond)
  expect(fetcher).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(2000)
  await vi.advanceTimersByTimeAsync(9000)
  expect(fetcher).toHaveBeenCalledTimes(2)
  const signal = fetcher.mock.calls[1]?.[1]?.signal
  expect(signal?.aborted).toBe(false)

  unsubscribeFirst()
  expect(signal?.aborted).toBe(false)
  unsubscribeSecond()
  expect(signal?.aborted).toBe(true)
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetcher).toHaveBeenCalledTimes(2)
})

it('stops a receipt poll after 401 despite cached quarantined data', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(jsonResponse(quarantined))
    .mockResolvedValueOnce(problemResponse('auth_required', 401))
  const api = new ApiClient(fetcher)
  const observer = new QueryObserver(client, getReceipt(api, receiptExample.id))
  dispose.push(observer.subscribe(() => undefined))

  await vi.advanceTimersByTimeAsync(3000)
  expect(observer.getCurrentResult().error).toMatchObject({
    kind: 'auth-required',
  })
  expect(observer.getCurrentResult().data?.status).toBe('quarantined')
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetcher).toHaveBeenCalledTimes(2)
})

it('stops the scheduled poll when the last observer leaves before the delay', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(jsonResponse(quarantined))
  const api = new ApiClient(fetcher)
  const observer = new QueryObserver(client, getReceipt(api, receiptExample.id))
  const unsubscribe = observer.subscribe(() => undefined)
  dispose.push(unsubscribe)

  await vi.advanceTimersByTimeAsync(1000)
  expect(observer.getCurrentResult().data?.status).toBe('quarantined')
  unsubscribe()
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetcher).toHaveBeenCalledTimes(1)
})
