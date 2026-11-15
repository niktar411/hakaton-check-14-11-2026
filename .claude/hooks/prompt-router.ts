// UserPromptSubmit: «тупой» промт → подсказка, каким навыком и по каким правилам делать.
// Срабатывает по ключевым словам; Definition of Done напоминает всегда, когда промт —
// задача, а не вопрос.
import { addContext, readInput } from './lib.ts';

const ROUTES: { re: RegExp; hint: string }[] = [
  {
    re: /страниц|экран|раздел|вкладк|форм[уаы]|лендинг|page/i,
    hint: 'Страница/экран → навык /new-page (начни с `bun run new:page`). Компоненты только из shared/ui, вёрстка проверяется на 375px, публичная страница — пререндер + sitemap + llms.txt.',
  },
  {
    re: /компонент|кнопк|модалк|диалог|карточк|виджет/i,
    hint: 'Компонент → сначала проверь client/src/shared/ui (есть — расширь), иначе навык /new-component.',
  },
  {
    re: /эндпоинт|endpoint|ручк|\bapi\b|маршрут|роут|обработчик|handler/i,
    hint: 'API → навык /new-route (`bun run new:route`): Go-типы с тегами валидации, сервис, тест humatest, затем `bun run gen:api`.',
  },
  {
    re: /таблиц|колонк|пол[ея] в баз|миграц|схем[ау] (бд|баз)|индекс/i,
    hint: 'База → навык /migration: новая goose-миграция (старые не трогать), запросы в server/queries, `sqlc generate`.',
  },
  {
    re: /текст|перевод|надпис|локализ|i18n|язык/i,
    hint: 'Строки интерфейса → навык /i18n: ключ в messages, `bun run i18n:sync`, перевод.',
  },
  {
    re: /стил|цвет|красив|дизайн|оформ|тем[ауы]|отступ|шрифт/i,
    hint: 'Оформление → только токены и классы темы (bg-surface, text-muted, rounded-sm), образец — страница /ui и docs/playbook.md п. 4–5. Новых цветов и шрифтов не добавлять.',
  },
];

const DOD =
  'Definition of Done: функция работает полностью (без заглушек, моков и маркеров недоделок); есть загрузка, пусто, ошибка, успех; ' +
  'тексты через i18n, вид через токены и shared/ui. Не сделал часть — скажи прямо.';

const input = await readInput();
const prompt = (input.prompt ?? '').trim();
if (!prompt || prompt.startsWith('/')) process.exit(0);

const isQuestion = /^(почему|зачем|как\b|что\b|где|какой|какая|какие|сколько|объясни|расскажи)|\?\s*$/i.test(prompt);
const hints = ROUTES.filter((r) => r.re.test(prompt)).map((r) => `- ${r.hint}`);
const vague = !isQuestion && prompt.split(/\s+/).length < 8;

const parts: string[] = [];
if (hints.length > 0) parts.push(`Подсказки по задаче:\n${hints.join('\n')}`);
if (vague) parts.push('Промт короткий: выбери ближайший шаблон проекта и в конце ответа перечисли сделанные допущения.');
if (!isQuestion) parts.push(DOD);

if (parts.length > 0) addContext('UserPromptSubmit', parts.join('\n\n'));
