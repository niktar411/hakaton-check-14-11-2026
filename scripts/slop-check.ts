// Проверка на «мусор нейросети»: заглушки, пропуски кода, моки в рабочем коде,
// any, проглоченные ошибки, сырые цвета, русский текст в JSX — то, чего не видит
// Biome (классы Tailwind проверяют его правила noTailwind*). Один движок для трёх мест — хук slop-guard (до записи на диск),
// хук done-gate (перед концом хода), pre-push и CI (для тех, кто пишет без Claude).
//
// Проверяются только ДОБАВЛЕННЫЕ строки: старый код не блокирует работу.
//
//   bun scripts/slop-check.ts                    рабочее дерево против HEAD + новые файлы
//   bun scripts/slop-check.ts --diff A...B       диапазон коммитов (CI, pre-push)
//   bun scripts/slop-check.ts путь/к/файлу …     файлы целиком
import { existsSync, readFileSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';

export type Finding = { file: string; line: number; text: string; rule: string; message: string };

type Rule = {
  id: string;
  re: RegExp;
  files: RegExp;
  except?: RegExp;
  message: string;
  /** Правило ищется по всему тексту, а не построчно (пустой catch на нескольких строках). */
  multiline?: boolean;
  /** Убрать комментарии перед проверкой строки. */
  stripComments?: boolean;
};

const CODE = /\.(tsx?|go|css|sql)$/;
const TS = /\.tsx?$/;
const TSX = /\.tsx$/;
const GO = /\.go$/;
const STYLE = /\.css$/;
const TESTS = /(_test\.go|\.test\.tsx?|\.spec\.tsx?)$|(^|\/)(seed|fixtures|testdata)\//;
/** Файлы, где шаблоны мусора — это данные: сам движок, его тесты и шаблоны генераторов. */
const SELF = /^(scripts\/slop-check(\.test)?\.ts|scripts\/templates\/)/;
const GENERATED = /(\.gen\.ts|\.d\.ts|\/sqlc\/|\/paraglide\/|openapi\.json)$|\/sqlc\/|\/paraglide\//;

const HEX = '#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})';

export const RULES: Rule[] = [
  {
    id: 'stub-marker',
    re: /\b(TODO|FIXME|XXX|HACK)\b/,
    files: CODE,
    message: 'Маркер недоделки. Доделай сейчас или прямо скажи пользователю, что не сделано.',
  },
  {
    id: 'elision',
    re: /(\/\/|--)\s*\.{3}|\/\*\s*\.{3}\s*\*\/|(\/\/|\/\*|--)\s*(rest of|the rest|remaining|остальн|implement (this|here|me|later)|your (code|logic) here|add (your )?logic|здесь будет|тут будет|допиши|дописать позже)/i,
    files: CODE,
    message: 'Пропуск кода комментарием. Напиши код полностью.',
  },
  {
    id: 'not-implemented',
    re: /not[ _-]?implemented|не реализован/i,
    files: /\.(tsx?|go)$/,
    except: TESTS,
    message: 'Заглушка «not implemented». Реализуй или убери вызов.',
  },
  {
    id: 'placeholder-data',
    re: /lorem ipsum|\bexample\.(com|org)\b|\b(john|jane) doe\b|иван иванов|test@test|foo@bar|\b(mock|fake|dummy|sample)(Data|Users?|Items?|List|Response|Rows?)\b|\bdummy\b/i,
    files: /\.(tsx?|go)$/,
    except: TESTS,
    message: 'Выдуманные данные в рабочем коде. Данные — из API; демо — в сиды (server/seed/).',
  },
  {
    id: 'fake-delay',
    re: /new Promise\(\s*\(?\s*\w+\s*\)?\s*=>\s*setTimeout\(/,
    files: TS,
    except: TESTS,
    message: 'Фейковая задержка. Настоящие данные — createResource/query к API.',
  },
  {
    id: 'ts-any',
    re: /\bas any\b|:\s*any\b(?!-)|<any>|\bany\[\]|@ts-(ignore|nocheck)/,
    files: TS,
    message: 'any / @ts-ignore. Возьми настоящий тип (API — из shared/api/schema.d.ts).',
    stripComments: false,
  },
  {
    id: 'ts-expect-no-reason',
    re: /@ts-expect-error\s*(\*\/)?\s*$/,
    files: TS,
    message: '@ts-expect-error без причины. Формат: // @ts-expect-error — <почему>.',
  },
  {
    id: 'biome-ignore-no-reason',
    re: /biome-ignore(-all|-start|-end)?\s+[^\s:]+\s*(\*\/\s*\}?)?\s*$/,
    files: /\.(tsx?|css|json)$/,
    message: 'biome-ignore без причины. Формат: // biome-ignore lint/<группа>/<правило>: <причина>.',
  },
  {
    id: 'nolint-no-reason',
    re: /\/\/\s*nolint(:[\w,-]+)?\s*$/,
    files: GO,
    message: '//nolint без причины. Формат: //nolint:<линтер> // <причина>.',
  },
  {
    id: 'empty-catch',
    re: /catch\s*(\([^)]*\))?\s*\{\s*\}|\.catch\(\s*\(?\s*\w*\s*\)?\s*=>\s*(\{\s*\}|undefined|null)\s*\)/,
    files: TS,
    multiline: true,
    message: 'Ошибка проглочена. Покажи её (toast) или пробрось дальше.',
  },
  {
    id: 'go-swallowed-err',
    re: /^\s*_\s*=\s*err\b|if err != nil \{\s*return( nil)?\s*\}/m,
    files: GO,
    multiline: true,
    message: 'Ошибка потеряна. return fmt.Errorf("<что делали>: %w", err).',
  },
  {
    id: 'go-print',
    re: /\bfmt\.Print(f|ln)?\(|\blog\.(Print|Fatal|Panic)(f|ln)?\(|^\s*println\(/,
    files: GO,
    except: TESTS,
    message: 'Печать в stdout. Логи — slog.InfoContext(ctx, "msg", "key", v).',
  },
  {
    id: 'go-background-ctx',
    re: /\bcontext\.(Background|TODO)\(\)/,
    files: GO,
    except: /(_test\.go$|^server\/cmd\/)/,
    message: 'context.Background() вне cmd/. Передай ctx из аргумента.',
  },
  {
    id: 'raw-color',
    re: new RegExp(`(?<!(?:href|to|hash|id)\\s*=\\s*\\{?["'\`]?)(?<![\\w&/-])${HEX}\\b(?![\\w-])|\\b(rgba?|hsla?|oklch|oklab|lch|hwb)\\(`),
    files: /\.(tsx|css)$/,
    except: /(^|\/)tokens\.css$/,
    message: 'Сырой цвет. Только токены: var(--…) в CSS или класс темы (bg-surface, text-muted).',
    stripComments: true,
  },
  // Произвольные значения и палитра Tailwind (bg-[#f00], bg-red-500) — правила Biome
  // noTailwindArbitraryValue / noTailwindRawColors; обход — biome-ignore с причиной.
  {
    id: 'inline-style-literal',
    re: /style=\{\{[^}]*["'`](?:#|\d+(?:\.\d+)?(?:px|rem|em)\b|rgb|hsl)/,
    files: TSX,
    message: 'Захардкоженное значение в style. Класс или токен; style — только для вычисляемых значений.',
  },
  {
    id: 'font-family',
    re: /font-family["']?\s*:(?!\s*var\(--font\))/,
    files: /\.(tsx|css)$/,
    message: 'Шрифт один — токен --font. Новых font-family не бывает.',
  },
  {
    id: 'css-apply',
    re: /@apply\b/,
    files: STYLE,
    message: '@apply прячет копипасту. Повторяющийся набор классов — компонент в shared/ui.',
  },
  {
    id: 'italic-class',
    re: /(?:^|[\s"'`])italic(?:$|[\s"'`])/,
    files: TSX,
    message: 'Курсива в проекте нет. Выделяй размером и text-muted.',
    stripComments: true,
  },
  {
    id: 'cyrillic-jsx',
    re: /[А-Яа-яЁё]/,
    files: TSX,
    except: TESTS,
    message: 'Русский текст в компоненте. Строки — m.key() из @/paraglide/messages.',
    stripComments: true,
  },
];

/** Убирает комментарии из строки, чтобы русские пояснения и примеры не считались нарушением. */
function stripLineComments(line: string): string {
  const trimmed = line.trim();
  if (trimmed.startsWith('*') || trimmed.startsWith('/*') || trimmed.startsWith('//')) return '';
  return line
    .replace(/\{\s*\/\*.*?\*\/\s*\}/g, '')
    .replace(/\/\*.*?\*\//g, '')
    .replace(/(^|[\s;,{}()])\/\/.*$/, '$1')
    .replace(/^\s*import\s.*$/, '');
}

function toPosix(file: string): string {
  return file.replaceAll('\\', '/').replace(/^\.\//, '');
}

function applies(rule: Rule, file: string): boolean {
  return rule.files.test(file) && !rule.except?.test(file);
}

/**
 * Проверка содержимого файла. `added` — номера добавленных строк (с 1);
 * без него проверяется весь файл.
 */
export function checkContent(path: string, content: string, added?: Set<number>): Finding[] {
  const file = toPosix(path);
  if (SELF.test(file) || GENERATED.test(file)) return [];
  const lines = content.split('\n');
  const findings: Finding[] = [];

  for (const rule of RULES) {
    if (!applies(rule, file)) continue;

    if (rule.multiline) {
      const re = new RegExp(rule.re.source, rule.re.flags.includes('g') ? rule.re.flags : `${rule.re.flags}g`);
      for (const m of content.matchAll(re)) {
        const line = content.slice(0, m.index).split('\n').length;
        if (added && !added.has(line)) continue;
        findings.push({ file, line, text: (lines[line - 1] ?? '').trim(), rule: rule.id, message: rule.message });
      }
      continue;
    }

    lines.forEach((raw, i) => {
      if (added && !added.has(i + 1)) return;
      const text = rule.stripComments ? stripLineComments(raw) : raw;
      if (!rule.re.test(text)) return;
      findings.push({ file, line: i + 1, text: raw.trim(), rule: rule.id, message: rule.message });
    });
  }
  return findings;
}

const DUPLICATE_NAME = [/^(?:My|Custom|New|Simple|Base|Better|Fancy|Another)([A-Z]\w*)\.tsx$/, /^([A-Z]\w*?)(?:\d+|V\d+|New|Copy|Alt)\.tsx$/];

/** Новый файл в shared/ui, похожий на дубль существующего компонента (Button2, CustomSelect). */
export function checkNewPath(path: string, root: string): Finding[] {
  const file = toPosix(path);
  if (!/(^|\/)client\/src\/shared\/ui\/[^/]+\.tsx$/.test(file)) return [];
  const name = basename(file);
  for (const re of DUPLICATE_NAME) {
    const core = re.exec(name)?.[1];
    if (core && existsSync(join(root, dirname(file), `${core}.tsx`))) {
      return [
        {
          file,
          line: 1,
          text: name,
          rule: 'duplicate-component',
          message: `Уже есть ${core}.tsx. Расширь его (вариант, проп), а не создавай копию.`,
        },
      ];
    }
  }
  return [];
}

/** Номера строк `after`, которых не было в `before` (с учётом повторов). */
export function addedLines(before: string, after: string): Set<number> {
  const pool = new Map<string, number>();
  for (const l of before.split('\n')) pool.set(l.trim(), (pool.get(l.trim()) ?? 0) + 1);
  const added = new Set<number>();
  after.split('\n').forEach((l, i) => {
    const left = pool.get(l.trim()) ?? 0;
    if (left > 0) pool.set(l.trim(), left - 1);
    else added.add(i + 1);
  });
  return added;
}

function git(root: string, args: string[]): { ok: boolean; out: string } {
  const r = Bun.spawnSync(['git', ...args], { cwd: root, stdout: 'pipe', stderr: 'pipe' });
  return { ok: r.exitCode === 0, out: r.stdout.toString() };
}

/** Разбор `git diff -U0`: файл → номера добавленных строк. */
function parseDiff(diff: string): Map<string, Set<number>> {
  const result = new Map<string, Set<number>>();
  let current: Set<number> | null = null;
  for (const line of diff.split('\n')) {
    if (line.startsWith('+++ ')) {
      const path = line.slice(4).replace(/^b\//, '');
      current = path === '/dev/null' ? null : new Set<number>();
      if (current) result.set(path, current);
      continue;
    }
    const hunk = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@/.exec(line);
    if (hunk && current) {
      const start = Number(hunk[1]);
      const count = hunk[2] === undefined ? 1 : Number(hunk[2]);
      for (let n = start; n < start + count; n++) current.add(n);
    }
  }
  return result;
}

function readSafe(path: string): string | null {
  try {
    return readFileSync(path, 'utf8');
  } catch {
    return null;
  }
}

/** Рабочее дерево против HEAD плюс неотслеживаемые файлы. */
export function scanWorkingTree(root: string): Finding[] {
  const findings: Finding[] = [];
  if (!git(root, ['status', '--porcelain']).ok) throw new Error('Git status не выполнен.');
  const hasHead = git(root, ['rev-parse', '--verify', 'HEAD']).ok;
  const diff = hasHead ? git(root, ['-c', 'core.quotePath=false', 'diff', '-U0', '--no-color', 'HEAD']) : { ok: true, out: '' };
  const listed = git(root, ['ls-files', '-z', '--others', '--exclude-standard', ...(hasHead ? [] : ['--cached'])]);
  if (!diff.ok || !listed.ok) throw new Error('Не удалось прочитать изменения Git.');
  const changed = parseDiff(diff.out);
  const untracked = listed.out;

  for (const [file, added] of changed) {
    const content = readSafe(join(root, file));
    if (content === null) throw new Error(`Не удалось прочитать изменённый файл: ${file}`);
    findings.push(...checkContent(file, content, added));
  }
  for (const file of untracked.split('\0').filter(Boolean)) {
    const content = readSafe(join(root, file));
    if (content === null) throw new Error(`Не удалось прочитать новый файл: ${file}`);
    findings.push(...checkContent(file, content), ...checkNewPath(file, root));
  }
  return findings;
}

/** Диапазон коммитов `A...B` или `A..B`: содержимое берётся из правого коммита. */
export function scanRange(root: string, range: string): Finding[] {
  const right = range.split(/\.{2,3}/)[1] || 'HEAD';
  const findings: Finding[] = [];
  for (const [file, added] of parseDiff(git(root, ['diff', '-U0', '--no-color', '--diff-filter=AM', range]).out)) {
    const shown = git(root, ['show', `${right}:${file}`]);
    if (shown.ok) findings.push(...checkContent(file, shown.out, added));
  }
  return findings;
}

export function formatFindings(findings: Finding[]): string {
  return findings.map((f) => `${f.file}:${f.line}  [${f.rule}] ${f.message}\n    > ${f.text.slice(0, 160)}`).join('\n');
}

if (import.meta.main) {
  const root = git(process.cwd(), ['rev-parse', '--show-toplevel']).out.trim() || process.cwd();
  const args = process.argv.slice(2);
  const diffAt = args.indexOf('--diff');
  let findings: Finding[];
  if (diffAt !== -1) {
    const range = args[diffAt + 1];
    if (!range) {
      console.error('--diff требует диапазон, например origin/main...HEAD');
      process.exit(2);
    }
    findings = scanRange(root, range);
  } else if (args.length > 0) {
    findings = args.flatMap((f) => {
      const content = readSafe(f);
      return content === null ? [] : checkContent(f, content);
    });
  } else {
    findings = scanWorkingTree(root);
  }

  if (findings.length > 0) {
    console.error(`${formatFindings(findings)}\n\n✖ slop-check: ${findings.length} нарушений`);
    process.exit(1);
  }
  console.log('✔ slop-check: чисто');
}
