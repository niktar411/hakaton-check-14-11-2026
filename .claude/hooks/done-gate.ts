// Stop: проверка добавленного кода; сбой проверки не считается успехом.
import { formatFindings, scanWorkingTree } from '../../scripts/slop-check.ts';
import { changedFiles, failStop, firstLines, readInput, root, setBlocks } from './lib.ts';

const input = await readInput();
try {
  changedFiles(); // Проверить доступность Git до сканирования.
  const findings = scanWorkingTree(root);
  if (findings.length > 0) failStop(input, 'done-gate', firstLines(formatFindings(findings), 80));
  setBlocks(input.session_id ?? 'none', 'done-gate', 0);
} catch (error) {
  failStop(input, 'done-gate', `Сканирование не выполнено: ${error instanceof Error ? error.message : String(error)}`);
}
