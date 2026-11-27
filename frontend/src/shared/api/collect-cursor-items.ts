export async function collectCursorItems<T extends { id: string }>(
  load: (cursor?: string) => Promise<{ data: T[]; next_cursor: string | null }>,
): Promise<T[]> {
  const items = new Map<string, T>()
  const seen = new Set<string>()
  let cursor: string | undefined
  do {
    const page = await load(cursor)
    for (const item of page.data) items.set(item.id, item)
    cursor = page.next_cursor ?? undefined
    if (cursor && seen.has(cursor)) throw new Error('Повтор курсора списка')
    if (cursor) seen.add(cursor)
  } while (cursor)
  return [...items.values()]
}
