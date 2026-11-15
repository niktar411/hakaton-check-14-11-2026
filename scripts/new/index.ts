// Генераторы скелетов: модель (и человек) дописывает логику в готовый файл по
// шаблону, а не изобретает структуру с нуля. Шаблоны — scripts/templates/<вид>/.
//
//   bun run new:page <домен> --title "<Заголовок>" [--path /url] [--name ИмяPascal]
//   bun run new:component <ИмяPascal> [--dir client/src/features/<домен>]
//   bun run new:route <ресурс>
//   bun run new:migration <описание_snake_case>
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { checkNewPath } from '../slop-check.ts';

const root = join(import.meta.dir, '..', '..');
const templates = join(import.meta.dir, '..', 'templates');

type Vars = Record<string, string>;

function fail(message: string): never {
  console.error(`✖ ${message}`);
  process.exit(1);
}

function flag(args: string[], name: string): string | undefined {
  const i = args.indexOf(`--${name}`);
  return i === -1 ? undefined : args[i + 1];
}

export const pascal = (s: string) => s.replace(/(^|[-_\s]+)(\w)/g, (_, __, c: string) => c.toUpperCase());
export const camel = (s: string) => pascal(s).replace(/^\w/, (c) => c.toLowerCase());
export const kebab = (s: string) =>
  s
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/[_\s]+/g, '-')
    .toLowerCase();

/** jobs → job, categories → category, address → address. */
export function singular(word: string): string {
  if (/ies$/.test(word)) return word.replace(/ies$/, 'y');
  if (/(ss|us)$/.test(word)) return word;
  if (/(ch|sh|x)es$/.test(word)) return word.replace(/es$/, '');
  return word.replace(/s$/, '');
}

function render(template: string, vars: Vars): string {
  const text = readFileSync(join(templates, template), 'utf8');
  return text.replace(/\{\{(\w+)\}\}/g, (whole, key: string) => {
    const value = vars[key];
    if (value === undefined) fail(`шаблон ${template}: нет значения для ${whole}`);
    return value;
  });
}

function create(path: string, content: string): void {
  const full = join(root, path);
  if (existsSync(full)) fail(`${path} уже существует — генератор не перезаписывает файлы.`);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, content);
  console.log(`  + ${path}`);
}

/** Вставка строки перед меткой; false — метки нет. */
function insertBefore(path: string, marker: string, line: string): boolean {
  const full = join(root, path);
  if (!existsSync(full)) return false;
  const text = readFileSync(full, 'utf8');
  const at = text.split('\n').findIndex((l) => l.includes(marker));
  if (at === -1) return false;
  const lines = text.split('\n');
  const indent = /^\s*/.exec(lines[at] ?? '')?.[0] ?? '';
  lines.splice(at, 0, `${indent}${line}`);
  writeFileSync(full, lines.join('\n'));
  console.log(`  ~ ${path}`);
  return true;
}

function newPage(args: string[]): string[] {
  const domain = args[0];
  const title = flag(args, 'title');
  if (!domain || !/^[a-z][a-z0-9-]*$/.test(domain)) fail('Домен — kebab-case латиницей: bun run new:page profile --title "Профиль"');
  if (!title) fail('Нужен --title "<Заголовок по-русски>" — он станет текстом в словаре, без заглушек.');
  const name = flag(args, 'name') ?? pascal(domain);
  const urlPath = flag(args, 'path') ?? `/${domain}`;
  const titleKey = `${camel(name)}PageTitle`;
  const dir = `client/src/features/${domain}`;
  const vars = { Name: name, titleKey };

  create(`${dir}/${name}Page.tsx`, render('page/Page.tsx.tpl', vars));
  create(`${dir}/${name}Page.module.css`, render('page/Page.module.css.tpl', vars));

  const todo: string[] = [];
  const messages = join(root, 'client/messages/ru.json');
  if (existsSync(messages)) {
    const dict = JSON.parse(readFileSync(messages, 'utf8')) as Record<string, string>;
    if (dict[titleKey]) fail(`ключ ${titleKey} уже есть в client/messages/ru.json`);
    dict[titleKey] = title;
    writeFileSync(messages, `${JSON.stringify(dict, null, 2)}\n`);
    console.log('  ~ client/messages/ru.json');
    todo.push('bun run i18n:sync и перевод на английский');
  } else {
    todo.push(`добавить ключ "${titleKey}": "${title}" в словарь (client/messages/ru.json ещё нет)`);
  }

  const route = `<Route path="${urlPath}" component={lazy(() => import('@/features/${domain}/${name}Page'))} />`;
  if (!insertBefore('client/src/app/App.tsx', '@new-page', route)) {
    todo.push(`зарегистрировать маршрут в client/src/app/App.tsx: ${route}`);
  }
  return todo;
}

function newComponent(args: string[]): string[] {
  const name = args[0];
  if (!name || !/^[A-Z][A-Za-z0-9]*$/.test(name)) fail('Имя — PascalCase: bun run new:component Avatar');
  const dir = flag(args, 'dir') ?? 'client/src/shared/ui';
  const path = `${dir}/${name}.tsx`;
  const duplicate = checkNewPath(path, root)[0];
  if (duplicate) fail(duplicate.message);

  create(path, render('component/Component.tsx.tpl', { Name: name }));
  create(`${dir}/${name}.module.css`, render('component/Component.module.css.tpl', { Name: name }));
  return dir === 'client/src/shared/ui' ? ['показать компонент во всех вариантах на витрине /ui'] : [];
}

function newRoute(args: string[]): string[] {
  const resource = args[0];
  if (!resource || !/^[a-z][a-z0-9-]*$/.test(resource)) fail('Ресурс — kebab-case во множественном числе: bun run new:route jobs');
  const one = singular(resource);
  const pkg = resource.replaceAll('-', '');
  const vars: Vars = {
    pkg,
    route: resource,
    kebab: one,
    Type: pascal(one),
    type: camel(one),
  };
  const dir = `server/internal/${pkg}`;
  for (const file of ['types.go', 'service.go', 'handler.go', 'handler_test.go']) {
    create(`${dir}/${file}`, render(`route/${file}.tpl`, vars));
  }

  const register = `${pkg}.Register(api, ${pkg}.NewService(store))`;
  const todo = [
    `таблица и запрос: bun run new:migration create_${resource.replaceAll('-', '_')}, затем server/queries/${resource}.sql с "-- name: Get${vars.Type} :one" и sqlc generate`,
  ];
  if (!insertBefore('server/internal/http/router.go', '@new-route', register)) {
    todo.push(`зарегистрировать в server/internal/http/router.go: ${register} (+ импорт пакета)`);
  }
  todo.push('bun run gen:api — контракт для клиента');
  return todo;
}

function newMigration(args: string[]): string[] {
  const desc = args[0];
  if (!desc || !/^[a-z][a-z0-9_]*$/.test(desc)) fail('Описание — snake_case латиницей: bun run new:migration add_jobs_city_index');
  const dir = join(root, 'server/migrations');
  const numbers = existsSync(dir) ? readdirSync(dir).map((f) => Number(/^(\d+)_/.exec(f)?.[1] ?? 0)) : [];
  const next = String(Math.max(0, ...numbers) + 1).padStart(5, '0');
  create(`server/migrations/${next}_${desc}.sql`, render('migration/migration.sql.tpl', {}));
  return ['заполнить обе секции: Up и Down (Down откатывает Up)', 'goose up на dev, затем sqlc generate'];
}

const GENERATORS: Record<string, (args: string[]) => string[]> = {
  page: newPage,
  component: newComponent,
  route: newRoute,
  migration: newMigration,
};

if (import.meta.main) {
  const [kind = '', ...args] = process.argv.slice(2);
  const generator = GENERATORS[kind];
  if (!generator) fail(`Неизвестный генератор «${kind}». Есть: ${Object.keys(GENERATORS).join(', ')}.`);
  const todo = generator(args);
  if (todo.length > 0) console.log(`\nОсталось:\n${todo.map((t) => `  - ${t}`).join('\n')}`);
}
