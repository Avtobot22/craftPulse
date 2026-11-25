import { Link } from '@gravity-ui/uikit'

export function SourceLink({
  url,
  sourceName,
}: {
  url: string | null
  sourceName?: string
}) {
  if (!url) return null
  const parsed = URL.parse(url)
  if (
    !parsed ||
    (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') ||
    parsed.username ||
    parsed.password
  )
    return null

  return (
    <Link href={parsed.href}>Открыть в {sourceName ?? parsed.hostname}</Link>
  )
}
