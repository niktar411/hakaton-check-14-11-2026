// SessionStart: короткая сводка окружения в контекст модели и предупреждения человеку.
import { existsSync } from 'node:fs';
import { addContext, changedFiles, readInput, root, run } from './lib.ts';

await readInput();

const branch = run(['git', 'branch', '--show-current']).out || '(нет коммитов)';
const dirty = changedFiles().length;
const hooksPath = run(['git', 'config', 'core.hooksPath']).out;

const lines = [
  `Ветка: ${branch}; изменённых файлов: ${dirty}.`,
  `Клиент: ${existsSync(`${root}/client/package.json`) ? 'есть' : 'ещё не создан'}; сервер: ${existsSync(`${root}/server/go.mod`) ? 'есть' : 'ещё не создан'}; Go: ${Bun.which('go') ? 'установлен' : 'не установлен (Go проверяет только CI)'}.`,
  'Новое — через генераторы `bun run new:*`; готовность — Definition of Done в CLAUDE.md.',
];
if (hooksPath !== '.githooks') lines.push('pre-push не включён: предложи пользователю `bun run hooks:install`.');

addContext('SessionStart', lines.join('\n'));
