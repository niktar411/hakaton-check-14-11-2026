import { afterEach, expect, test } from 'bun:test';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve, delimiter } from 'node:path';

const project = resolve(import.meta.dir, '..');
const gitBin = Bun.which('git');
if (!gitBin) throw new Error('Git required for hook integration tests');
const git = gitBin;
const dirs: string[] = [];
afterEach(() => {
  for (const dir of dirs.splice(0)) {
    if (dirname(resolve(dir)) !== resolve(tmpdir()) || !dir.includes('hackathon-hook-test-')) throw new Error('Unexpected test directory');
    rmSync(dir, { recursive: true, force: true });
  }
});
function fixture(init = true) {
  const dir = mkdtempSync(join(tmpdir(), 'hackathon-hook-test-'));
  dirs.push(dir);
  if (init) command(dir, ['init', '-q']);
  return dir;
}
function command(dir: string, args: string[]) {
  const result = Bun.spawnSync([git, ...args], { cwd: dir, stdout: 'pipe', stderr: 'pipe' });
  if (result.exitCode !== 0) throw new Error(result.stderr.toString());
}
function file(dir: string, path: string, content: string) {
  mkdirSync(dirname(join(dir, path)), { recursive: true });
  writeFileSync(join(dir, path), content);
}
function commit(dir: string) {
  command(dir, ['add', '.']);
  command(dir, ['-c', 'user.name=Hook Test', '-c', 'user.email=hook@localhost', '-c', 'core.hooksPath=/dev/null', 'commit', '-qm', 'fixture']);
}
function hook(dir: string, name: string, input: Record<string, unknown>, env: Record<string, string> = {}) {
  const result = Bun.spawnSync([process.execPath, join(project, '.claude/hooks', `${name}.ts`)], {
    cwd: dir, stdin: Buffer.from(JSON.stringify(input)), stdout: 'pipe', stderr: 'pipe',
    env: { ...process.env, CLAUDE_PROJECT_DIR: dir, ...env },
  });
  expect(result.exitCode).toBe(0);
  const out = result.stdout.toString().trim();
  return out ? JSON.parse(out) : null;
}
function edit(path: string) {
  return { tool_name: 'Edit', tool_input: { file_path: path } };
}
test('guard allows an untracked and staged new migration, including unborn HEAD', () => {
  const dir = fixture();
  const path = 'server/migrations/001.sql';
  file(dir, path, 'select 1;');
  expect(hook(dir, 'guard', edit(path))).toBeNull();
  command(dir, ['add', '.']);
  expect(hook(dir, 'guard', edit(path))).toBeNull();
  commit(dir);
  const result = hook(dir, 'guard', edit(join(dir, path)));
  expect(result.hookSpecificOutput.permissionDecision).toBe('deny');
  expect(result.hookSpecificOutput.permissionDecisionReason).toContain('HEAD');
  file(dir, 'server/migrations/002.sql', 'select 2;');
  command(dir, ['add', '.']);
  expect(hook(dir, 'guard', edit('server/migrations/002.sql'))).toBeNull();
});
test('guard fails closed when migration Git inspection is unavailable', () => {
  const result = hook(fixture(false), 'guard', edit('server/migrations/001.sql'));
  expect(result.hookSpecificOutput.permissionDecision).toBe('deny');
});
test('guard protects generated files and dangerous git commands', () => {
  const dir = fixture();
  expect(hook(dir, 'guard', edit('api/openapi.json')).hookSpecificOutput.permissionDecision).toBe('deny');
  for (const command of ['git push origin main --force', 'git push --no-verify', 'npm install']) {
    expect(hook(dir, 'guard', { tool_name: 'Bash', tool_input: { command } }).hookSpecificOutput.permissionDecision).toBe('deny');
  }
});
test('done-gate scans staged files with Cyrillic and spaces', () => {
  const dir = fixture();
  file(dir, 'client/новый файл.ts', ['const value:', 'any = 1;'].join(' '));
  command(dir, ['add', '.']);
  const result = hook(dir, 'done-gate', { session_id: crypto.randomUUID() });
  expect(result.decision).toBe('block');
  expect(result.reason).toContain('ts-any');
});
test('done-gate catches committed file changes with Cyrillic and spaces', () => {
  const dir = fixture();
  file(dir, 'client/новый файл.ts', 'export const value = 1;');
  commit(dir);
  file(dir, 'client/новый файл.ts', ['export const value:', 'any = 1;'].join(' '));
  expect(hook(dir, 'done-gate', { session_id: crypto.randomUUID() }).decision).toBe('block');
});
test('done-gate reports Git failure instead of passing', () => {
  expect(hook(fixture(false), 'done-gate', { session_id: crypto.randomUUID() }).decision).toBe('block');
});
test('done-gate caps retries, keeps failure visible, resets on a new user turn', () => {
  const dir = fixture();
  file(dir, 'bad.ts', ['const value:', 'any = 1;'].join(' '));
  const session_id = crypto.randomUUID();
  for (let i = 0; i < 3; i++) expect(hook(dir, 'done-gate', { session_id, stop_hook_active: i > 0 }).decision).toBe('block');
  const stopped = hook(dir, 'done-gate', { session_id, stop_hook_active: true });
  expect(stopped.continue).toBe(false);
  expect(stopped.systemMessage).toContain('ПРОВЕРКА НЕ ПРОЙДЕНА');
  expect(hook(dir, 'done-gate', { session_id, stop_hook_active: false }).decision).toBe('block');
  file(dir, 'bad.ts', 'export const value = 1;');
  expect(hook(dir, 'done-gate', { session_id, stop_hook_active: true })).toBeNull();
});
test('typecheck reports missing Go instead of passing', () => {
  const dir = fixture();
  file(dir, 'server/go.mod', 'module hooktest\n\ngo 1.22\n');
  const result = hook(dir, 'typecheck', { session_id: crypto.randomUUID() }, { PATH: [dirname(git), dirname(process.execPath)].join(delimiter) });
  expect(result.decision).toBe('block');
  expect(result.reason).toContain('Go не установлен');
});
test('typecheck reacts to configuration-only changes and propagates command failure', () => {
  const dir = fixture();
  file(dir, 'package.json', JSON.stringify({ scripts: { typecheck: 'exit 1' } }));
  file(dir, 'tsconfig.json', '{}');
  expect(hook(dir, 'typecheck', { session_id: crypto.randomUUID() }).decision).toBe('block');
});
test('typecheck notices a renamed file leaving the server workspace', () => {
  const dir = fixture();
  file(dir, 'server/a.go', 'package a');
  commit(dir);
  command(dir, ['mv', 'server/a.go', 'a.go']);
  expect(hook(dir, 'typecheck', { session_id: crypto.randomUUID() }).reason).toContain('server/go.mod');
});
test('clean tree passes both Stop checks', () => {
  const dir = fixture();
  expect(hook(dir, 'done-gate', {})).toBeNull();
  expect(hook(dir, 'typecheck', {})).toBeNull();
});
