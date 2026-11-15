// PreToolUse (Edit/Write/MultiEdit): не даёт записать на диск заглушки и мусор.
// Проверяются только добавляемые строки — правила в scripts/slop-check.ts.
import { addedLines, checkContent, checkNewPath, formatFindings } from '../../scripts/slop-check.ts';
import { deny, projected, readInput, rel, root } from './lib.ts';

const input = await readInput();
const edit = projected(input);
if (!edit) process.exit(0);

const file = rel(edit.path);
const findings = [
  ...checkContent(file, edit.after, edit.isNew ? undefined : addedLines(edit.before, edit.after)),
  ...(edit.isNew ? checkNewPath(file, root) : []),
];

if (findings.length > 0) {
  deny(
    `slop-guard: правка не записана — в ней ${findings.length} нарушений правил проекта.\n\n${formatFindings(findings)}\n\n` +
      'Перепиши эти места по подсказкам и повтори правку. Если без нарушения правда нельзя — ' +
      'остановись и объясни пользователю почему, не обходи проверку.',
  );
}
