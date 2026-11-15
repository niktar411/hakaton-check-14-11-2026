// PreToolUse: то, чего в проекте не делают никогда.
//  - правка миграции из HEAD (политика неизменяемости, не проверка состояния БД);
//  - правка сгенерированных файлов (перегенерируются и затрут правку);
//  - npm / pnpm / yarn / npx — менеджер пакетов только Bun;
//  - force-push и обход git-хуков.
import { deny, readInput, rel, run, str } from './lib.ts';

const input = await readInput();
const tool = input.tool_name ?? '';
const args = input.tool_input ?? {};

const GENERATED: [RegExp, string][] = [
  [/^api\/openapi\.json$/, 'Контракт генерируется из Go-типов: поправь server/internal/<ресурс>/types.go и запусти `bun run gen:api`.'],
  [/^client\/src\/shared\/api\/(schema\.d\.ts|.+\.gen\.ts)$/, 'Типы клиента генерируются: поправь Go-типы и запусти `bun run gen:api`.'],
  [/^server\/internal\/db\/sqlc\//, 'Код sqlc генерируется: поправь server/queries/*.sql и запусти `sqlc generate` в server/.'],
];

if (/^(Edit|Write|MultiEdit|NotebookEdit)$/.test(tool)) {
  const path = str(args.file_path ?? args.notebook_path);
  if (!path) deny('Не удалось определить путь изменяемого файла.');
  const file = rel(path);

  if (/^server\/migrations\/.+\.sql$/.test(file)) {
    const repo = run(['git', 'rev-parse', '--is-inside-work-tree']);
    if (!repo.ok || repo.out !== 'true') deny('Нельзя проверить защиту миграции: Git недоступен или каталог не является рабочим деревом.');
    const head = run(['git', 'rev-parse', '--verify', '--quiet', 'HEAD']);
    if (!head.ok) {
      const unborn = run(['git', 'symbolic-ref', '-q', 'HEAD']);
      const refs = run(['git', 'show-ref', '--head']);
      if (!unborn.ok || refs.ok || refs.out) deny('Не удалось проверить HEAD; правка миграции заблокирована до проверки Git.');
    } else {
      const tree = run(['git', 'ls-tree', '-z', '--name-only', 'HEAD', '--', file]);
      if (!tree.ok) deny(`Не удалось проверить миграцию в HEAD: ${tree.out}`);
      if (tree.out) deny(`${file} есть в HEAD. По правилам проекта зафиксированные миграции неизменяемы: создай новую. Состояние БД эта проверка не определяет.`);
    }
  }
  for (const [re, why] of GENERATED) {
    if (re.test(file)) deny(`${file} — сгенерированный файл. ${why}`);
  }
}

if (tool === 'Bash' || tool === 'PowerShell') {
  const cmd = str(args.command);
  // Только в позиции команды, чтобы не срабатывать на слово «npm» в тексте коммита.
  const at = (name: string) => new RegExp(`(^|[;&|(\\n]|&&|\\|\\|)\\s*${name}(\\.cmd|\\.exe)?(\\s|$)`).test(cmd);

  if (['npm', 'pnpm', 'yarn', 'npx'].some(at)) {
    deny('В проекте только Bun: bun install / bun add / bun run / bun x.');
  }
  if (/git\s+push\b[^\n]*(\s--force\b|\s-f\b|\s--force-with-lease\b)/.test(cmd)) {
    deny('Force-push запрещён. Если история правда разошлась — опиши ситуацию пользователю, пусть решит сам.');
  }
  if (/git\s+(commit|push)\b[^\n]*\s--no-verify\b/.test(cmd)) {
    deny('--no-verify обходит проверки. Почини то, на что жалуется хук.');
  }
}
