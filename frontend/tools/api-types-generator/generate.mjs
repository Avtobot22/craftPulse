import { mkdir, readFile, writeFile } from 'node:fs/promises'
import openapiTS, { astToString } from 'openapi-typescript'

const schema = new URL(
  '../../../docs/technical-specification/openapi.yaml',
  import.meta.url,
)
const output = new URL(
  '../../src/shared/api/generated/schema.d.ts',
  import.meta.url,
)
const ast = await openapiTS(schema)
const source =
  '// Generated from openapi.yaml. Run npm run generate:api.\n' +
  astToString(ast)

if (process.argv.includes('--check')) {
  if ((await readFile(output, 'utf8')) !== source) {
    throw new Error('API types are stale. Run npm run generate:api.')
  }
} else {
  await mkdir(new URL('.', output), { recursive: true })
  await writeFile(output, source)
}
