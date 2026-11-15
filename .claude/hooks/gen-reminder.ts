// PostToolUse (Edit/Write/MultiEdit): правка источника, из которого что-то генерируется,
// → напоминание перегенерировать. Один раз за сессию на каждый вид, чтобы не шуметь.
import { addContext, firstTime, readInput, rel, str } from './lib.ts';

const REMINDERS: { kind: string; re: RegExp; text: string }[] = [
  {
    kind: 'api',
    re: /^server\/internal\/[^/]+\/(types|handler)\.go$/,
    text: 'Изменён контракт API. Перед концом задачи: `bun run gen:api` — иначе клиент работает со старыми типами, а CI упадёт на проверке контракта.',
  },
  {
    kind: 'sqlc',
    re: /^server\/queries\/.+\.sql$/,
    text: 'Изменены SQL-запросы. Запусти `sqlc generate` в server/ и используй сгенерированные методы, не пиши SQL в Go-коде.',
  },
  {
    kind: 'migration',
    re: /^server\/migrations\/.+\.sql$/,
    text: 'Новая миграция. Примени на dev (`goose up`), затем `sqlc generate`; если меняется контракт — `bun run gen:api`.',
  },
  {
    kind: 'i18n',
    re: /^client\/(messages\/ru\.json|src\/paraglide\/messages\.ts)$/,
    text: 'Новые строки интерфейса. Запусти `bun run i18n:sync` и сразу замени английскую заглушку переводом.',
  },
  {
    kind: 'nginx',
    re: /^client\/nginx(\.prod)?\.conf$/,
    text: 'Конфиги nginx dev и prod меняются только вместе — проверь второй файл.',
  },
];

const input = await readInput();
const file = rel(str(input.tool_input?.file_path));
const hit = REMINDERS.find((r) => r.re.test(file));
if (hit && firstTime(input.session_id ?? 'none', hit.kind)) addContext('PostToolUse', hit.text);
