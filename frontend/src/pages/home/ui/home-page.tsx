import { Box, Container, Flex, Grid, Link, Text } from '@gravity-ui/uikit'
import { useLinkClickHandler, useOutlet } from 'react-router'
import { contentGridProps, layoutSizes, routePaths } from '@/shared/config'

export function HomePage() {
  const outlet = useOutlet()
  const onHomeClick = useLinkClickHandler(routePaths.home)
  return (
    <Container
      as="main"
      maxWidth={layoutSizes.content}
      paddingBlock={{ xs: 'spacing-4', m: 'spacing-6', xl: 'spacing-8' }}
      style={{ boxSizing: 'border-box' }}
    >
      <Flex
        as="header"
        alignItems="center"
        minHeight={{ xs: '56px', m: '64px' }}
        marginBlockEnd="spacing-8"
      >
        <Link href={routePaths.home} onClick={onHomeClick}>
          craftPulse
        </Link>
      </Flex>
      <Grid {...contentGridProps}>
        <Box gridColumn="1 / -1">
          {outlet ?? (
            <Text as="h1" variant="header-1">
              craftPulse
            </Text>
          )}
        </Box>
      </Grid>
    </Container>
  )
}
