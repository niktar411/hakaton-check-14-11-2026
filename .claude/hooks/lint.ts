// PostToolUse (Edit/Write/MultiEdit): линтер по изменённому файлу сразу после правки.
// TS/TSX/CSS/JSON — Biome; Go — gofmt и, если установлен, golangci-lint по пакету.
import { dirname } from 'node:path';
import { block, firstLines, firstTime, readInput, rel, root, run, str, tellUser } from './lib.ts';

const input = await readInput();
const path = str(input.tool_input?.file_path);
const file = rel(path);
if (!path || file.startsWith('..')) process.exit(0);

if (/\.(tsx?|css|json|jsonc)$/.test(file)) {
  const r = run(['bun', 'x', 'biome', 'lint', '--colors=off', '--max-diagnostics=20', file]);
  if (!r.ok && !/No files were processed/.test(r.out)) {
    block(`Biome нашёл ошибки в ${file} — исправь:\n${firstLines(r.out, 60)}`);
  }
}

if (file.endsWith('.go')) {
  if (!Bun.which('gofmt')) {
    if (firstTime(input.session_id ?? 'none', 'no-go')) tellUser('lint: Go не установлен — Go-файлы проверит только CI.');
    process.exit(0);
  }
  const fmt = run(['gofmt', '-l', path]);
  if (fmt.out) block(`${file} не отформатирован: запусти gofmt -w ${file}.`);

  if (Bun.which('golangci-lint')) {
    const pkg = `./${rel(dirname(path)).replace(/^server\//, '')}/...`;
    const lint = run(['golangci-lint', 'run', '--fast-only', pkg], `${root}/server`);
    if (!lint.ok) block(`golangci-lint по ${pkg}:\n${firstLines(lint.out, 60)}`);
  }
}
