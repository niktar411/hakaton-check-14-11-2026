// Stop: затронутые исходники, конфигурация и зависимости; пропуск не равен успеху.
import { existsSync } from 'node:fs';
import { changedFiles, failStop, firstLines, readInput, root, run, setBlocks } from './lib.ts';

const input = await readInput();
try {
  const files = changedFiles();
  const errors: string[] = [];
  const common = files.some((f) => /^(package\.json|bun\.lock|tsconfig[^/]*\.json)$/.test(f));
  const scripts = common || files.some((f) => /^(scripts|\.claude)\/.+\.ts$/.test(f) && !f.startsWith('scripts/templates/'));
  const client = files.some((f) => /^client\/(src\/|.*(?:package\.json|tsconfig[^/]*\.json|bun\.lock)$)/.test(f)) || (common && existsSync(`${root}/client/package.json`));
  const server = files.some((f) => /^server\/(?:.*\.go$|go\.(mod|sum)$|go\.work(?:\.sum)?$)/.test(f));
  const check = (label: string, cmd: string[], cwd = root) => {
    const result = run(cmd, cwd);
    if (!result.ok) errors.push(`${label}:\n${firstLines(result.out, 40)}`);
  };
  if (scripts) {
    if (!existsSync(`${root}/tsconfig.json`)) errors.push('Скрипты: нет tsconfig.json, проверка не выполнена.');
    else check('tsc (scripts, .claude)', [process.execPath, 'run', 'typecheck']);
  }
  if (client) {
    if (!existsSync(`${root}/client/tsconfig.json`)) errors.push('Клиент: нет client/tsconfig.json, проверка не выполнена.');
    else check('tsc (client)', [process.execPath, 'x', '--no-install', 'tsc', '--noEmit', '-p', 'client']);
  }
  if (server) {
    if (!existsSync(`${root}/server/go.mod`)) errors.push('Сервер: нет server/go.mod, проверка не выполнена.');
    else if (!Bun.which('go')) errors.push('Сервер: Go не установлен. Сборка и vet не выполнены; установи Go или передай проверку человеку.');
    else {
      check('go build', ['go', 'build', './...'], `${root}/server`);
      check('go vet', ['go', 'vet', './...'], `${root}/server`);
    }
  }
  if (errors.length) failStop(input, 'typecheck', errors.join('\n\n'));
  setBlocks(input.session_id ?? 'none', 'typecheck', 0);
} catch (error) {
  failStop(input, 'typecheck', `Проверка не выполнена: ${error instanceof Error ? error.message : String(error)}`);
}
