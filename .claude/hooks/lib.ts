// Общее для хуков: чтение входа, ответы в формате Claude Code, путь проекта,
// содержимое файла после правки, счётчик блокировок на сессию.
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative, resolve } from 'node:path';

export type HookInput = {
  session_id?: string;
  hook_event_name?: string;
  tool_name?: string;
  tool_input?: Record<string, unknown>;
  prompt?: string;
  stop_hook_active?: boolean;
};

export const root = process.env.CLAUDE_PROJECT_DIR ?? process.cwd();

export async function readInput(): Promise<HookInput> {
  const text = await Bun.stdin.text();
  return text ? (JSON.parse(text) as HookInput) : {};
}

export function rel(path: string): string {
  return relative(root, resolve(root, path)).replaceAll('\\', '/');
}

function emit(payload: unknown): never {
  console.log(JSON.stringify(payload));
  process.exit(0);
}

/** PreToolUse: запретить вызов; причина уходит модели, и она переделывает. */
export function deny(reason: string): never {
  emit({ hookSpecificOutput: { hookEventName: 'PreToolUse', permissionDecision: 'deny', permissionDecisionReason: reason } });
}

/** PostToolUse / Stop: вернуть модели замечание, которое она обязана учесть. */
export function block(reason: string): never {
  emit({ decision: 'block', reason });
}

/** Добавить текст в контекст модели (SessionStart, UserPromptSubmit, PostToolUse). */
export function addContext(event: string, text: string): never {
  emit({ hookSpecificOutput: { hookEventName: event, additionalContext: text } });
}

/** Показать сообщение человеку, не мешая модели. */
export function tellUser(text: string): never {
  emit({ systemMessage: text });
}

export function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

/** Содержимое файла до и после правки Edit / MultiEdit / Write. */
export function projected(input: HookInput): { path: string; before: string; after: string; isNew: boolean } | null {
  const args = input.tool_input ?? {};
  const path = str(args.file_path);
  if (!path) return null;
  const isNew = !existsSync(path);
  const before = isNew ? '' : readFileSync(path, 'utf8');

  switch (input.tool_name) {
    case 'Write':
      return { path, before, after: str(args.content), isNew };
    case 'Edit': {
      const from = str(args.old_string);
      const to = str(args.new_string);
      const after = args.replace_all === true ? before.replaceAll(from, to) : before.replace(from, to);
      return { path, before, after, isNew };
    }
    case 'MultiEdit': {
      const edits = Array.isArray(args.edits) ? (args.edits as Record<string, unknown>[]) : [];
      let after = before;
      for (const e of edits) {
        after = e.replace_all === true ? after.replaceAll(str(e.old_string), str(e.new_string)) : after.replace(str(e.old_string), str(e.new_string));
      }
      return { path, before, after, isNew };
    }
    default:
      return null;
  }
}

const stateDir = join(tmpdir(), 'claude-hackathon-hooks');

function stateFile(session: string, name: string): string {
  mkdirSync(stateDir, { recursive: true });
  return join(stateDir, `${createHash('sha256').update(`${resolve(root)}\0${session}`).digest('hex')}-${name}`);
}

/** Сколько раз хук уже блокировал в этой сессии; чтобы не зациклить модель. */
export function blocksUsed(session: string, name: string): number {
  const f = stateFile(session, name);
  return existsSync(f) ? Number(readFileSync(f, 'utf8')) || 0 : 0;
}

export function setBlocks(session: string, name: string, n: number): void {
  writeFileSync(stateFile(session, name), String(n));
}

/** true — напоминание этого вида в сессии ещё не показывалось (и отмечает его показанным). */
export function firstTime(session: string, kind: string): boolean {
  const f = stateFile(session, `once-${kind}`);
  if (existsSync(f)) return false;
  writeFileSync(f, '1');
  return true;
}

export function run(cmd: string[], cwd = root): { ok: boolean; out: string } {
  try {
    const r = Bun.spawnSync(cmd, { cwd, stdout: 'pipe', stderr: 'pipe', timeout: 30_000 });
    return { ok: r.exitCode === 0, out: `${r.stdout.toString()}${r.stderr.toString()}`.trim() };
  } catch (error) {
    return { ok: false, out: error instanceof Error ? error.message : String(error) };
  }
}

/** NUL-разделители сохраняют пробелы, кириллицу и оба пути переименования. */
export function changedFiles(): string[] {
  const r = Bun.spawnSync(['git', 'status', '--porcelain=v1', '-z', '--untracked-files=all'], { cwd: root, stdout: 'pipe', stderr: 'pipe', timeout: 10_000 });
  if (r.exitCode !== 0) throw new Error(`Не удалось получить изменения Git: ${r.stderr.toString()}`);
  const entries = r.stdout.toString().split('\0');
  const files: string[] = [];
  for (let i = 0; i < entries.length; i++) {
    const entry = entries[i];
    if (!entry) continue;
    files.push(entry.slice(3));
    if (/[RC]/.test(entry.slice(0, 2))) {
      const old = entries[++i];
      if (old) files.push(old);
    }
  }
  return [...new Set(files)];
}

/** После трёх попыток работа остановлена для человека, а не признана готовой. */
export function failStop(input: HookInput, name: string, reason: string): never {
  const session = input.session_id ?? 'none';
  const used = input.stop_hook_active ? blocksUsed(session, name) : 0;
  setBlocks(session, name, used + 1);
  if (used >= 3) {
    const message = `${name}: ПРОВЕРКА НЕ ПРОЙДЕНА. Нужна проверка человеком после трёх попыток исправления.\n${reason}`;
    emit({ continue: false, stopReason: message, systemMessage: message });
  }
  block(`${name}: проверка не пройдена (попытка ${used + 1}/3).\n${reason}\nИсправь причину; не удаляй нужную функциональность ради прохождения проверки.`);
}

export function firstLines(text: string, n: number): string {
  const lines = text.split('\n');
  return lines.length > n ? `${lines.slice(0, n).join('\n')}\n… ещё ${lines.length - n} строк` : text;
}
