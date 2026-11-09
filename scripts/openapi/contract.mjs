import {readFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import YAML from 'yaml';

export const root = new URL('../../', import.meta.url);
export const contractUrl = new URL('docs/technical-specification/openapi.yaml', root);
export const srsUrl = new URL('docs/technical-specification/technical-specification.md', root);
export const contractPath = fileURLToPath(contractUrl);
export const contract = YAML.parse(readFileSync(contractUrl, 'utf8'));
export const srs = readFileSync(srsUrl, 'utf8');
export const operations = Object.entries(contract.paths).flatMap(([path, item]) =>
  Object.entries(item).filter(([, operation]) => operation?.operationId)
    .map(([method, operation]) => ({path, method, ...operation})));
export function dereference(value) {
  if (!value?.$ref) return value;
  return value.$ref.slice(2).split('/').reduce((node, key) =>
    node[key.replaceAll('~1', '/').replaceAll('~0', '~')], contract);
}
