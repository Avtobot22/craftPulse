// Сквозная трассировка ТЗ: проверка связей и генерация раздела 9.4 и связей в каталоге UC.
// npm --prefix scripts/openapi run trace - обновить; без --write - проверить.
import {readFileSync, writeFileSync} from 'node:fs';
import {root, srs, srsUrl} from './contract.mjs';
import {
  backticked, featureRows, frCards, ids, operationIds, operationTrace, productFeatures,
  sharedScenarios, ucCatalog, ucIndex, uiCards, uiRegistry, useCases, useCasesUrl,
} from './trace.mjs';

const errors = [];
const fail = message => errors.push(message);
const byNumber = (a, b) => a.localeCompare(b, 'ru', {numeric: true});
const list = values => [...new Set(values)].sort(byNumber);
const code = values => values.map(value => `\`${value}\``).join(', ');
const general = id => Number(id.slice(3)) <= 10;
const stage = text => text.match(/\bM[012]\b/)?.[0] ?? '';

// 1. Функции и UC.
for (const id of productFeatures) if (!featureRows.has(id)) fail(`${id} нет в разделе 3.1`);
for (const id of featureRows.keys()) if (!productFeatures.has(id)) fail(`${id} из раздела 3.1 нет в каталоге функций`);
const featureByUc = new Map();
for (const row of featureRows.values()) for (const uc of row.uc) {
  if (!ucCatalog.has(uc)) fail(`${row.id}: нет ${uc}`);
  featureByUc.set(uc, [...(featureByUc.get(uc) ?? []), row.id]);
}
for (const uc of ucCatalog.keys()) if (!featureByUc.has(uc) && !sharedScenarios.has(uc)) fail(`${uc} не связан ни с F, ни с общими сценариями 3.1`);

for (const [id, uc] of ucCatalog) {
  const row = ucIndex.get(id);
  if (!row) { fail(`${id} нет в таблице 3.3`); continue; }
  const alternatives = uc.alternatives.join(',');
  if (alternatives !== row.alternatives.join(',')) fail(`${id}: альтернативы каталога ${alternatives} и 3.3 ${row.alternatives} различаются`);
  const mapped = [...row.branches.keys()];
  if (alternatives !== mapped.filter(key => key.startsWith('A')).join(',')) fail(`${id}: не у всех альтернатив есть FR в 3.3`);
  if (uc.shared.join(',') !== list(mapped.filter(key => key.startsWith('ОВ'))).join(',')) fail(`${id}: общие ветки каталога ${uc.shared} и 3.3 различаются`);
  if (!row.main.length) fail(`${id}: основной ход без FR`);
  for (const [branch, frs] of row.branches) if (!frs.length) fail(`${id} ${branch}: нет FR`);
}

// 2. FR.
const frUc = new Map();
for (const row of ucIndex.values()) for (const fr of [...row.main, ...[...row.branches.values()].flat()]) {
  if (!frCards.has(fr)) fail(`${row.id}: нет карточки ${fr}`);
  frUc.set(fr, new Set([...(frUc.get(fr) ?? []), row.id]));
}
for (const card of frCards.values()) {
  if (!frUc.has(card.id)) fail(`${card.id} не связан ни с одним шагом или веткой в 3.3`);
  if (card.ac !== `AC-${card.id}`) fail(`${card.id}: критерий приёмки ${card.ac}`);
  if (!card.f.length) fail(`${card.id}: нет F в основании`);
  for (const f of card.f) if (!featureRows.has(f)) fail(`${card.id}: нет ${f}`);
  for (const uc of card.uc) if (!frUc.get(card.id)?.has(uc)) fail(`${card.id}: ${uc} из карточки нет в 3.3`);
  if (!card.ui.length && !/отдельного экрана нет/.test(card.fields['Связи'])) fail(`${card.id}: нет UI и причины его отсутствия`);
  for (const ui of card.ui) {
    if (!uiRegistry.get(ui)?.fr.includes(card.id)) fail(`${card.id} → ${ui}: реестр 6.1 не связывает экран с FR`);
  }
}

// 3. UI.
for (const [id, row] of uiRegistry) {
  if (!uiCards.has(id)) fail(`${id}: нет карточки 6.2`);
  for (const fr of row.fr) if (!frCards.get(fr)?.ui.includes(id)) fail(`${id} → ${fr}: карточка FR не называет экран`);
  for (const uc of row.uc) if (!ucCatalog.has(uc)) fail(`${id}: нет ${uc}`);
}
for (const [id, card] of uiCards) {
  if (!uiRegistry.has(id)) fail(`${id}: карточки нет в реестре 6.1`);
  if (!/https:\/\/www\.figma\.com\/design\//.test(card.fields['Figma'] ?? '')) fail(`${id}: нет ссылки на макет`);
  const links = card.fields['Связи'] ?? '';
  const registry = uiRegistry.get(id);
  if (registry) {
    if (ids(links, 'UC').join() !== registry.uc.join()) fail(`${id}: UC карточки ${ids(links, 'UC')} и реестра ${registry.uc} различаются`);
    for (const fr of registry.fr) if (!ids(links, 'FR').includes(fr)) fail(`${id}: ${fr} из реестра не указан в карточке`);
  }
  const named = backticked(links);
  for (const name of named) {
    if (!operationIds.has(name)) fail(`${id}: операции ${name} нет в контракте`);
    else if (!operationTrace.get(name).ui.includes(id)) fail(`${id} → ${name}: x-craftpulse-trace не называет экран`);
  }
}

// 4. Операции.
for (const [id, operation] of operationTrace) {
  if (!operation.fr.length) fail(`${id}: нет FR в x-craftpulse-trace`);
  if (!operation.uc.length) fail(`${id}: нет UC в x-craftpulse-trace`);
  for (const fr of operation.fr) if (!frCards.has(fr)) fail(`${id}: нет ${fr}`);
  for (const uc of operation.uc) if (!ucCatalog.has(uc)) fail(`${id}: нет ${uc}`);
  for (const ui of operation.ui) if (!backticked(uiCards.get(ui)?.fields['Связи'] ?? '').includes(id)) fail(`${id} → ${ui}: карточка UI не называет операцию`);
}

// 5. Определения идентификаторов: каждая ссылка ведёт на строку или карточку с этим ID.
const mvp = readFileSync(new URL('docs/technical-specification/mvp-scope.md', root), 'utf8');
const defined = new Set([
  ...[...srs.matchAll(/^\| ((?:BR|DR|SEC|NFR|DS|Q|V|EV|ST|S)-\d+)(?: \||;| и|\. )/gm)].map(match => match[1]),
  ...[...srs.matchAll(/^##### ((?:FR|UI)-\d+)\./gm)].map(match => match[1]),
  ...[...srs.matchAll(/^\| Набор (V-[\d/V-]+) \|/gm)].flatMap(match => ids(match[1], 'V')),
  ...[...mvp.matchAll(/^\| ((?:AC|NFR)-\d+)[ ;]/gm)].map(match => match[1]),
]);
for (const prefix of ['BR', 'DR', 'SEC', 'NFR', 'DS', 'Q', 'EV', 'ST', 'AC', 'V', 'S']) {
  for (const id of ids(srs, prefix)) if (!defined.has(id)) fail(`${id} упоминается, но не определён`);
}

// 6. UI по веткам UC из реестра 6.1: «UC-03: 5-7/A3, UC-12 A2», «UC-03, ОВ-1/ОВ-7».
const uiByBranch = new Map();
const uiKey = (uc, branch) => `${uc} ${branch}`;
const altIds = spec => [...spec.matchAll(/(?<![A-Za-zА-Яа-я])A(\d+)(?:-A(\d+))?/g)].flatMap(match =>
  Array.from({length: Number(match[2] ?? match[1]) - Number(match[1]) + 1}, (_, n) => `A${Number(match[1]) + n}`));
const putUi = (uc, branch, ui) => uiByBranch.set(uiKey(uc, branch), list([...(uiByBranch.get(uiKey(uc, branch)) ?? []), ui]));
for (const row of uiRegistry.values()) {
  const shared = ids(row.trace, 'ОВ');
  const segments = row.trace.split(/;\s*/).flatMap(part => part.split(/,\s*(?=UC-)/)).filter(part => /^UC-/.test(part));
  for (const segment of segments) {
    const ucs = ids(segment.match(/^UC-[\d/]+(?:-UC-\d+)?/)[0], 'UC');
    const spec = segment.replace(/^UC-[\d/]+(?:-UC-\d+)?:?\s*/, '');
    for (const uc of ucs) {
      if (!ucCatalog.has(uc)) continue;
      if (!altIds(spec).length || /(^|[\s/,])(\d|предуслов)/.test(spec)) putUi(uc, 'main', row.id);
      for (const branch of altIds(spec)) putUi(uc, branch, row.id);
      for (const branch of shared) putUi(uc, branch, row.id);
    }
  }
}

// 7. Матрица по веткам.
const branchTitle = (uc, branch) => {
  if (branch === 'main') return 'основной ход';
  if (branch.startsWith('ОВ')) return useCases.match(new RegExp(`\\| (${branch}\\. [^|]+) \\|`))?.[1] ?? branch;
  return ucCatalog.get(uc).body.match(new RegExp(`^#### (${branch}\\. .+)$`, 'm'))[1];
};
const ucUi = uc => list([...uiRegistry.values()].filter(row => row.uc.includes(uc)).map(row => row.id));
const ucStage = uc => stage(ucCatalog.get(uc).header);
const rows = [];
for (const [uc, index] of ucIndex) {
  const branches = [['main', index.main], ...index.branches];
  for (const [branch, frs] of branches) {
    const title = branchTitle(uc, branch);
    const branchStage = title.match(/\((M[012])\)/)?.[1] ?? ucStage(uc);
    const cardsOf = frs.map(fr => frCards.get(fr));
    let ui = uiByBranch.get(uiKey(uc, branch)) ?? [];
    let uiNote = '';
    if (!ui.length) {
      const own = cardsOf.flatMap(card => card.ui).filter(id => uiRegistry.get(id).uc.includes(uc));
      if (own.length) ui = list(own);
      // Общие ветки ОВ покрываются правилами 6.1.6 в экранах сценария.
      else if (frs.every(general) || branch.startsWith('ОВ')) uiNote = 'общие состояния и подтверждения 6.1.6 в экранах сценария';
      else fail(`${uc} ${branch}: ни реестр 6.1, ни карточки FR не называют экран`);
    }
    const ops = list([...operationTrace.values()].filter(op => op.uc.includes(uc) && op.fr.some(fr => frs.includes(fr))).map(op => op.operationId));
    const actions = list(ops.map(op => operationTrace.get(op)['x-craftpulse-action']));
    // Операция расширения следующей поставки помечается своим этапом.
    const opNames = ops.map(op => {
      const delivery = operationTrace.get(op)['x-craftpulse-delivery'];
      return `\`${op}\`` + (delivery > branchStage ? ` (${delivery})` : '');
    });
    let opText = ops.length ? `${opNames.join(', ')}; право ${code(actions)}` : '';
    if (!ops.length) {
      const sameUc = [...operationTrace.values()].filter(op => op.uc.includes(uc));
      opText = sameUc.length
        ? 'N/A: ветка меняет поведение операций основного хода, отдельного вызова нет'
        : 'N/A: сценарий выполняется вне HTTP API craftPulse';
      if (frs.every(general)) opText = 'N/A: общее правило применяется к операциям сценария';
    }
    const dr = list(cardsOf.flatMap(card => [...ids(card.rules, 'BR'), ...ids(card.rules, 'SEC'), ...ids(card.rules, 'NFR'), ...card.dr]));
    const ac = frs.map(fr => `AC-${fr}`);
    const mvpAc = branch === 'main' ? (ucCatalog.get(uc).header.match(/\*\*Приёмка MVP:\*\* ([^*]+?)\.?(?: \*\*|$)/)?.[1] ?? '') : '';
    rows.push({uc, branch, title, stage: branchStage, frs, ui, uiNote, opText, dr, ac, mvpAc});
  }
}

// FR функции - это FR её UC по таблице 3.3; общие FR-001-FR-010 вынесены отдельно.
const frText = ucs => {
  const frs = list(ucs.flatMap(uc => [ucIndex.get(uc).main, ...ucIndex.get(uc).branches.values()].flat()));
  const own = frs.filter(fr => !general(fr)).map(fr => `${fr} (AC-${fr})`).join(', ');
  const shared = frs.filter(general);
  return [own, shared.length && `общие ${shared.join(', ')}`].filter(Boolean).join('; ');
};
const uiText = row => [row.ui.join(', '), row.uiNote].filter(Boolean).join('; ');
const matrix = ['<!-- TRACE-MATRIX:START -->', '<!-- Матрица генерируется: npm --prefix scripts/openapi run trace -->', '',
  '**От функции к сценариям и приёмке**', '',
  '| Функция | UC | FR и критерии приёмки | UI | Поставка |', '|---|---|---|---|---|'];
for (const row of featureRows.values()) {
  const ui = list([...uiRegistry.values()].filter(item => item.f.includes(row.id) || item.uc.some(uc => row.uc.includes(uc))).map(item => item.id));
  matrix.push(`| ${row.id}. ${row.name} | ${row.uc.join(', ')} | ${frText(row.uc)} | ${ui.join(', ')} | ${row.delivery.replaceAll('|', '\\|')} |`);
}
for (const row of sharedScenarios.values()) {
  if (featureByUc.has(row.id)) continue;
  const goal = row.goal.replace(/^[^:]+:\s*/, '');
  matrix.push(`| Без строки F: ${goal} | ${row.id} | ${frText([row.id])} | ${ucUi(row.id).join(', ')} | ${row.delivery} |`);
}
matrix.push('', '**От ветки сценария к интерфейсу, операции, данным и приёмке**', '',
  '| UC и ветка | FR | UI / внешний интерфейс | operationId и право | Правила и данные | Приёмка | Поставка |', '|---|---|---|---|---|---|---|');
for (const row of rows) {
  const name = row.branch === 'main' ? `**${row.uc}**, ${row.title}` : `${row.uc} ${row.title}`;
  const accept = [row.ac.join(', '), row.mvpAc && `MVP ${row.mvpAc}`].filter(Boolean).join('; ');
  matrix.push(`| ${name} | ${row.frs.join(', ')} | ${uiText(row)} | ${row.opText} | ${row.dr.join(', ')} | ${accept} | ${row.stage} |`);
}
matrix.push('<!-- TRACE-MATRIX:END -->');

// 8. Связи в каталоге UC: FR и UI перед блоком сценария.
let catalog = useCases;
for (const [id, uc] of ucCatalog) {
  const index = ucIndex.get(id);
  const frs = list([index.main, ...index.branches.values()].flat());
  const own = frs.filter(fr => !general(fr));
  const shared = frs.filter(general);
  const links = ` **Требования ТЗ:** ${[own.join(', '), shared.length && `общие ${shared.join(', ')}`].filter(Boolean).join('; ')}; приёмка по AC тех же номеров. **Интерфейс:** ${ucUi(id).join(', ')}.`;
  const header = uc.header.replace(/ \*\*Требования ТЗ:\*\*.*$/, '');
  catalog = catalog.replace(uc.header, header + links);
}

const start = '<!-- TRACE-MATRIX:START -->';
const end = '<!-- TRACE-MATRIX:END -->';
const block = matrix.join('\n');
let expected;
if (srs.includes(start)) expected = srs.slice(0, srs.indexOf(start)) + block + srs.slice(srs.indexOf(end) + end.length);
else fail('В разделе 9.4 нет маркеров матрицы');

if (errors.length) {
  console.error(errors.map(error => '- ' + error).join('\n'));
  throw new Error(`Трассировка ТЗ: ${errors.length} нарушений`);
}
if (process.argv.includes('--write')) {
  writeFileSync(srsUrl, expected);
  writeFileSync(useCasesUrl, catalog);
  console.log(`Updated trace matrix: ${featureRows.size} F, ${rows.length} branches.`);
} else if (srs !== expected || useCases !== catalog) {
  throw new Error('Матрица 9.4 или связи каталога UC устарели. Run npm --prefix scripts/openapi run trace.');
} else {
  console.log(`Trace matrix matches: ${featureRows.size} F, ${ucCatalog.size} UC, ${rows.length} branches, ${frCards.size} FR, ${uiCards.size} UI.`);
}
