import {writeFileSync} from 'node:fs';
import {contract, operations, srs, srsUrl} from './contract.mjs';

const credentialNames = {cookieSession: 'сессия', apiKey: 'ключ API', uploadToken: 'токен загрузки', surveyInvite: 'приглашение опроса'};
const escapeCell = value => value.replaceAll('|', '\\|').replaceAll('\n', ' ');
const lines = ['<!-- Карта генерируется: npm --prefix scripts/openapi run map -->', ''];
for (const tag of contract.tags) {
  const entries = operations.filter(operation => operation.tags.includes(tag.name));
  if (!entries.length) continue;
  lines.push(`**${tag.name}** - ${tag.description}`, '',
    '| operationId | Метод и путь | Назначение / FR / UC; UI | Право и вход | Поставка | Ссылка в OpenAPI |',
    '|---|---|---|---|---|---|');
  for (const operation of entries) {
    const trace = operation['x-craftpulse-trace'];
    const purpose = trace.filter(id => !id.startsWith('UI-')).join(', ');
    const ui = trace.filter(id => id.startsWith('UI-')).join(', ');
    const credentials = (operation.security ?? contract.security).flatMap(item => Object.keys(item).map(key => credentialNames[key]));
    const server = (operation.servers ?? contract.paths[operation.path].servers ?? contract.servers)[0].url;
    const prefix = new URL(server).pathname.replace(/\/$/, '');
    const pointer = '#/paths/' + operation.path.replaceAll('~', '~0').replaceAll('/', '~1') + '/' + operation.method;
    lines.push(`| \`${operation.operationId}\` | \`${operation.method.toUpperCase()} ${prefix}${operation.path}\` | ${escapeCell(operation.summary)}. ${purpose}${ui ? '; ' + ui : ''} | \`${operation['x-craftpulse-action']}\`; ${credentials.length ? [...new Set(credentials)].join(', ') : 'без входа'} | ${operation['x-craftpulse-delivery']} | [контракт](openapi.yaml${pointer}) |`);
  }
  lines.push('');
}
const start = '<!-- OPERATION-MAP:START -->';
const end = '<!-- OPERATION-MAP:END -->';
const block = start + '\n' + lines.join('\n') + '\n' + end;
const sectionStart = srs.indexOf('#### 5.4. Карта операций');
const sectionEnd = srs.indexOf('#### 5.5.', sectionStart);
if (sectionStart < 0 || sectionEnd < 0) throw new Error('SRS section 5.4 not found');
let expected;
if (srs.includes(start) && srs.includes(end)) {
  expected = srs.slice(0, srs.indexOf(start)) + block + srs.slice(srs.indexOf(end) + end.length);
} else {
  const tableStart = srs.indexOf('**session**', sectionStart);
  if (tableStart < 0 || tableStart > sectionEnd) throw new Error('SRS operation map not found');
  expected = srs.slice(0, tableStart) + block + '\n\n' + srs.slice(sectionEnd);
}
if (process.argv.includes('--write')) {
  writeFileSync(srsUrl, expected);
  console.log(`Updated SRS map: ${operations.length} operations.`);
} else if (srs !== expected) {
  throw new Error('SRS operation map is stale. Run npm --prefix scripts/openapi run map.');
} else {
  console.log(`SRS map matches ${operations.length} operations.`);
}
