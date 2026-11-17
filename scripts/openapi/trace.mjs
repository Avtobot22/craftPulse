// Разбор комплекта ТЗ: каталог функций, UC, карточки FR, реестр и карточки UI, контракт.
import {readFileSync} from 'node:fs';
import {operations, root, srs} from './contract.mjs';

export const featuresUrl = new URL('docs/requirements/features.md', root);
export const useCasesUrl = new URL('docs/requirements/use-cases.md', root);
export const features = readFileSync(featuresUrl, 'utf8');
export const useCases = readFileSync(useCasesUrl, 'utf8');

const pad = (value, width) => String(value).padStart(width, '0');

// Разворачивает записи вида FR-001/013, F-02/F-08, UI-49-UI-52 и FR-001-FR-010.
export function ids(text, prefix) {
  const out = new Set();
  const re = new RegExp(`(?<![A-Za-zА-Яа-яЁё-])${prefix}-(\\d+)((?:-${prefix}-\\d+)|(?:/(?:${prefix}-)?\\d+(?![\\d-]))*)`, 'g');
  for (const match of text.matchAll(re)) {
    const width = match[1].length;
    out.add(`${prefix}-${match[1]}`);
    const tail = match[2];
    if (tail.startsWith('-')) {
      const end = Number(tail.slice(prefix.length + 2));
      for (let n = Number(match[1]) + 1; n <= end; n++) out.add(`${prefix}-${pad(n, width)}`);
    } else if (tail) {
      for (const part of tail.split('/').slice(1)) out.add(`${prefix}-${pad(part.replace(`${prefix}-`, ''), width)}`);
    }
  }
  return [...out].sort();
}

export const cells = row => row.trim().replace(/^\|/, '').replace(/\|$/, '').split(/(?<!\\)\|/).map(cell => cell.trim());

// Тело раздела от заголовка до следующего заголовка того же или более высокого уровня.
export function section(text, heading) {
  const start = text.indexOf(heading);
  if (start < 0) throw new Error(`Не найден раздел: ${heading}`);
  const level = heading.match(/^#+/)[0].length;
  const rest = text.slice(start + heading.length);
  const next = rest.search(new RegExp(`\\n#{1,${level}} `));
  return next < 0 ? rest : rest.slice(0, next);
}

export function cards(text, prefix) {
  const out = new Map();
  const re = new RegExp(`^##### (${prefix}-\\d+)\\. (.+)$`, 'gm');
  const heads = [...text.matchAll(re)];
  heads.forEach((head, index) => {
    const end = text.slice(head.index + head[0].length).search(/\n#{2,5} /);
    const body = end < 0 ? text.slice(head.index) : text.slice(head.index, head.index + head[0].length + end);
    const fields = {};
    for (const line of body.split('\n')) {
      if (!line.startsWith('| ') || line.startsWith('| Поле') || line.startsWith('|---')) continue;
      const [name, ...value] = cells(line);
      fields[name] = value.join(' | ');
    }
    out.set(head[1], {id: head[1], title: head[2], fields, line: text.slice(0, head.index).split('\n').length});
  });
  return out;
}

const tableRows = (text, first) => text.split('\n').filter(line => first.test(line)).map(cells);

// Каталог функций продукта и ТЗ 3.1.
export const productFeatures = new Set(ids(features.split('\n').filter(line => /^\| F-\d\d/.test(line)).join('\n'), 'F'));
const catalog = section(srs, '#### 3.1. Каталог функций');
export const featureRows = new Map(tableRows(catalog, /^\| F-\d\d \|/).map(([id, name, actor, result, uc, delivery, mvp]) =>
  [id, {id, name, actor, result, uc: ids(uc, 'UC'), delivery, mvp}]));
export const sharedScenarios = new Map(tableRows(catalog, /^\| UC-\d\d \|/).map(([id, goal, result, delivery, basis]) =>
  [id, {id, goal, result, delivery, basis}]));

// Каталог UC.
export const ucCatalog = new Map();
for (const match of useCases.matchAll(/^## (UC-\d\d): (.+)$/gm)) {
  const start = match.index;
  const end = useCases.indexOf('\n## UC-', start + 1);
  const body = useCases.slice(start, end < 0 ? undefined : end);
  ucCatalog.set(match[1], {
    id: match[1], title: match[2], body, start,
    alternatives: [...body.matchAll(/^#### (A\d+)\. /gm)].map(item => item[1]),
    shared: ids(body, 'ОВ'),
    header: body.split('\n').find(line => line.startsWith('**Поставка:**')) ?? '',
  });
}

// ТЗ 3.3: шаги и ветки UC → FR.
export const ucIndex = new Map(tableRows(section(srs, '#### 3.3. Алгоритмы UC'), /^\| \[UC-\d\d\]/).map(row => {
  const [link, goal, alternatives, f, result, main, branches] = row;
  const id = link.match(/UC-\d\d/)[0];
  const branchMap = new Map();
  for (const part of branches.split(/;\s*/)) {
    const branch = part.match(/^(A\d+|ОВ-\d)(?:\s*\([^)]*\))?:\s*(.*)$/);
    if (branch) branchMap.set(branch[1], ids(branch[2], 'FR'));
  }
  return [id, {id, goal, alternatives: alternatives.match(/\bA\d+\b/g) ?? [], f: ids(f, 'F'), result,
    main: ids(main, 'FR'), branches: branchMap, branchText: branches}];
}));

// ТЗ 3.2: карточки FR.
export const frCards = new Map([...cards(srs, 'FR')].map(([id, card]) => {
  const links = card.fields['Связи'] ?? '';
  return [id, {...card,
    f: ids(card.fields['Основание'] ?? '', 'F'),
    uc: ids(links, 'UC'), ui: ids(links, 'UI'), dr: ids(links, 'DR'),
    rules: card.fields['Правила и права'] ?? '',
    ac: (card.fields['Критерий приёмки'] ?? '').match(/AC-FR-\d{3}/)?.[0],
    delivery: card.fields['Поставка и приоритет'] ?? ''}];
}));

// ТЗ 6.1: реестр UI; 6.2: карточки UI.
const uiMap = section(srs, '#### 6.1. Карта интерфейса');
export const uiRegistry = new Map(tableRows(uiMap, /^\| UI-\d\d \|/).filter(row => row.length === 7).map(([id, name, actor, goal, trace, flow, delivery]) =>
  [id, {id, name, actor, goal, trace, fr: ids(trace, 'FR'), f: ids(trace, 'F'), uc: ids(trace, 'UC'), flow, delivery}]));
export const uiCards = cards(section(srs, '#### 6.2. Подробные карточки UI'), 'UI');

export const operationIds = new Set(operations.map(operation => operation.operationId));
export const operationTrace = new Map(operations.map(operation => {
  const trace = operation['x-craftpulse-trace'] ?? [];
  return [operation.operationId, {...operation,
    fr: trace.filter(id => id.startsWith('FR-')), uc: trace.filter(id => id.startsWith('UC-')), ui: trace.filter(id => id.startsWith('UI-'))}];
}));
export const backticked = text => [...text.matchAll(/`([a-z][A-Za-z]+)`/g)].map(match => match[1]);
