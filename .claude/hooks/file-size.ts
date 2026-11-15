// PreToolUse (Edit/Write/MultiEdit): файл длиннее 1000 строк не пишем.
// В прошлом проекте один CSS дорос до 8 679 строк, и его дважды перекраивали.
import { deny, projected, readInput, rel } from './lib.ts';

const LIMIT = 1000;
const EXEMPT = /(\.gen\.ts|\.d\.ts|\.lock|\.sum|openapi\.json|\.svg)$|\/(sqlc|paraglide|locales|messages)\//;

const input = await readInput();
const edit = projected(input);
if (!edit) process.exit(0);

const file = rel(edit.path);
if (EXEMPT.test(file)) process.exit(0);

const lines = edit.after.split('\n').length;
const before = edit.before ? edit.before.split('\n').length : 0;
if (lines > LIMIT && lines > before) {
  deny(
    `${file} станет ${lines} строк (лимит ${LIMIT}). Разбей по смыслу: компонент — на подкомпоненты в той же папке, ` +
      'сервис — по сценариям, CSS — в модули компонентов. Не переноси код в «utils2» и не сжимай строки.',
  );
}
