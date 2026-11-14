import { describe, expect, test } from 'bun:test';
import { addedLines, checkContent } from './slop-check.ts';

const rules = (file: string, content: string) => checkContent(file, content).map((f) => f.rule);

describe('ловит мусор', () => {
  const cases: [string, string, string][] = [
    ['stub-marker', 'client/src/features/a/A.tsx', '// TODO: доделать'],
    ['stub-marker', 'server/internal/jobs/service.go', '// FIXME later'],
    ['elision', 'client/src/features/a/a.ts', '  // ... остальной код'],
    ['elision', 'server/internal/jobs/service.go', '\t// rest of the logic'],
    ['elision', 'server/queries/jobs.sql', '-- ...'],
    ['not-implemented', 'server/internal/jobs/service.go', 'return errors.New("not implemented")'],
    ['not-implemented', 'client/src/shared/lib/x.ts', "throw new Error('Not implemented')"],
    ['placeholder-data', 'client/src/features/jobs/list.ts', 'const mockData = [1, 2]'],
    ['placeholder-data', 'client/src/features/jobs/list.ts', "email: 'test@test.ru'"],
    ['placeholder-data', 'server/internal/users/service.go', 'name := "John Doe"'],
    ['fake-delay', 'client/src/features/a/a.ts', 'await new Promise((r) => setTimeout(r, 500))'],
    ['ts-any', 'client/src/features/a/a.ts', 'const x = y as any'],
    ['ts-any', 'client/src/features/a/a.ts', 'function f(a: any) {}'],
    ['ts-any', 'client/src/features/a/a.ts', '// @ts-ignore'],
    ['ts-expect-no-reason', 'client/src/features/a/a.ts', '// @ts-expect-error'],
    ['biome-ignore-no-reason', 'client/src/features/a/a.ts', '// biome-ignore lint/style/noNonNullAssertion'],
    ['nolint-no-reason', 'server/internal/a/a.go', 'x := 1 //nolint:gosec'],
    ['empty-catch', 'client/src/features/a/a.ts', 'try { go() } catch {\n}'],
    ['empty-catch', 'client/src/features/a/a.ts', 'load().catch(() => {})'],
    ['go-swallowed-err', 'server/internal/a/a.go', '\t_ = err'],
    ['go-swallowed-err', 'server/internal/a/a.go', 'if err != nil {\n\t\treturn nil\n\t}'],
    ['go-print', 'server/internal/a/a.go', 'fmt.Println("hi")'],
    ['go-print', 'server/internal/a/a.go', 'log.Fatal(err)'],
    ['go-background-ctx', 'server/internal/a/a.go', 'ctx := context.Background()'],
    ['raw-color', 'client/src/shared/ui/Button.module.css', '.btn { color: #ff0000; }'],
    ['raw-color', 'client/src/shared/ui/Button.module.css', '.btn { background: rgba(0, 0, 0, .5); }'],
    ['raw-color', 'client/src/features/a/A.tsx', "<div style={{ color: '#fff' }} />"],
    ['inline-style-literal', 'client/src/features/a/A.tsx', "<p style={{ 'font-size': '13px' }} />"],
    ['font-family', 'client/src/shared/ui/X.module.css', '.x { font-family: Arial; }'],
    ['css-apply', 'client/src/shared/ui/X.module.css', '.x { @apply p-2; }'],
    ['italic-class', 'client/src/features/a/A.tsx', '<span class="text-sm italic" />'],
    ['cyrillic-jsx', 'client/src/features/a/A.tsx', '<h1>Профиль</h1>'],
    ['cyrillic-jsx', 'client/src/features/a/A.tsx', '<input placeholder="Имя" />'],
    ['mojibake-run', 'client/messages/ru.json', '"appTitle": "???????",'],
    ['mojibake-run', 'client/index.html', '<title>??????????????</title>'],
    ['mojibake-run', 'client/src/features/a/A.module.css', '/* ?????? ?? ???????? */'],
  ];

  for (const [rule, file, content] of cases) {
    test(`${rule}: ${content.split('\n')[0]}`, () => {
      expect(rules(file, content)).toContain(rule);
    });
  }
});

describe('не трогает нормальный код', () => {
  const clean: [string, string][] = [
    ['client/src/features/a/A.tsx', '<a href="#add">{m.add()}</a>'],
    ['client/src/features/a/A.tsx', '// Профиль пользователя: шапка и вкладки'],
    ['client/src/features/a/A.tsx', '{/* Пояснение по-русски в JSX-комментарии */}'],
    ['client/src/features/a/A.tsx', '<div class="flex items-center gap-2 bg-surface text-muted rounded-sm" />'],
    [
      'client/src/features/a/A.tsx',
      '{/* biome-ignore lint/nursery/noTailwindArbitraryValue: сетка карточки из макета */}\n<div class="grid-cols-[200px_1fr]" />',
    ],
    // biome-ignore lint/suspicious/noTemplateCurlyInString: это текст проверяемого JSX, а не шаблон
    ['client/src/features/a/A.tsx', '<div style={{ width: `${progress()}%` }} />'],
    ['client/src/features/a/A.tsx', 'const [local, rest] = splitProps(props, ["class"]);'],
    ['client/src/features/a/a.ts', 'const phone = "7XXXXXXXXXX";'],
    ['client/src/features/a/a.ts', 'items.map((x) => ({ ...x, done: true }))'],
    ['client/src/features/a/a.ts', '// biome-ignore lint/style/noNonNullAssertion: элемент рендерится выше'],
    ['client/src/features/a/a.ts', '} catch (err) {\n  toast.error(m.saveFailed());\n}'],
    ['client/src/shared/styles/tokens.css', ':root { --bg: #ffffff; --overlay: rgba(0, 0, 0, .45); }'],
    ['client/src/shared/styles/base.css', 'body { font-family: var(--font); }'],
    ['server/internal/a/a.go', 'if err != nil {\n\t\treturn fmt.Errorf("load job: %w", err)\n\t}'],
    ['server/internal/a/a.go', 'func Do(args ...string) {}'],
    ['server/internal/a/a.go', 'x := 1 //nolint:gosec // значение из конфига, не от пользователя'],
    ['server/cmd/api/main.go', 'ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)'],
    ['server/internal/a/a_test.go', 'fmt.Println(mockData)'],
    ['server/seed/users.go', 'name := "Иван Иванов"'],
    ['client/src/shared/api/schema.d.ts', 'export type X = any;'],
    ['docs/playbook.md', 'TODO в документации не проверяется'],
    ['client/src/features/a/a.ts', 'const mode = search() ?? "planner";'],
    ['client/src/features/a/a.ts', 'value === "?" ? "да" : "нет"'],
    ['client/src/shared/api/client.ts', 'const href = "/x?" + params.toString();'],
  ];

  for (const [file, content] of clean) {
    test(`${file}: ${content.split('\n')[0]}`, () => {
      expect(rules(file, content)).toEqual([]);
    });
  }
});

describe('только добавленные строки', () => {
  test('старый TODO не мешает, новый ловится', () => {
    const before = 'const a = 1;\n// TODO старое\n';
    const after = 'const a = 1;\n// TODO старое\n// TODO новое\n';
    const findings = checkContent('client/src/features/a/a.ts', after, addedLines(before, after));
    expect(findings.map((f) => f.line)).toEqual([3]);
  });
});
