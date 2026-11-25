interface OperationState {
  data: { status: string; retry_after_seconds: number | null } | undefined
  status: string
  fetchStatus: string
}

// Poll only from mounted observers. A query's key shares its request across observers.
// Focus and reconnect refetches or retries could bypass the server delay.
export function operationPolling(...activeStatuses: string[]) {
  function delay(data: OperationState['data']) {
    const seconds = data?.retry_after_seconds
    return data !== undefined &&
      activeStatuses.includes(data.status) &&
      typeof seconds === 'number' &&
      Number.isInteger(seconds) &&
      seconds > 0 &&
      seconds * 1000 <= 2_147_483_647
      ? seconds * 1000
      : false
  }

  return {
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    // Another observer mounting before the next poll can use the current response.
    staleTime: ({ state }: { state: OperationState }) => delay(state.data) || 0,
    refetchInterval: ({ state }: { state: OperationState }) =>
      state.status === 'success' && state.fetchStatus === 'idle'
        ? delay(state.data)
        : false,
  } as const
}
