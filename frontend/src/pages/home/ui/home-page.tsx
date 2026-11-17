import { Text } from '@gravity-ui/uikit'
import { Link, useOutlet } from 'react-router'
import { routePaths } from '@/shared/config'

export function HomePage() {
  const outlet = useOutlet()
  return (
    <main>
      <header className="app-header">
        <Link to={routePaths.home}>craftPulse</Link>
      </header>
      {outlet ?? (
        <Text as="h1" variant="header-1">
          craftPulse
        </Text>
      )}
    </main>
  )
}
