import type { ReactNode } from 'react'
import { Link } from '@gravity-ui/uikit'

export function SourceLink({
  url,
  sourceName,
  children,
}: {
  url: string | null
  sourceName?: string | undefined
  children?: ReactNode
}) {
  if (!url) return children ?? null
  const parsed = URL.parse(url)
  if (
    !parsed ||
    (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') ||
    parsed.username ||
    parsed.password
  )
    return children ?? null

  return (
    <Link href={parsed.href}>
      {children ?? `Открыть в ${sourceName ?? parsed.hostname}`}
    </Link>
  )
}
