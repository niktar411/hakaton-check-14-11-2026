// Конвертер исходных xlsx кейса SAM в CSV UTF-8.
//
//   bun scripts/sam/xlsx-to-csv.ts --kind inventory --in demo/sam/source/INSTALLED_SOFTWARE.xlsx --out demo/sam/source/installed_software.csv
//   bun scripts/sam/xlsx-to-csv.ts --kind software   --in demo/sam/source/SOFTWARE.xlsx --out demo/sam/source/software.csv
//   bun scripts/sam/xlsx-to-csv.ts --kind licenses   --in demo/sam/source/LICENSES.xlsx --out demo/sam/source/licenses.csv
//
// Требует системный `unzip` (только чтение zip-контейнера xlsx). Новых npm-зависимостей нет.
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

type Kind = "inventory" | "software" | "licenses";

function fail(message: string): never {
  throw new Error(message);
}

function argValue(args: readonly string[], name: string): string {
  const index = args.indexOf(`--${name}`);
  if (index === -1) fail(`нет обязательного флага --${name}`);
  const value = args[index + 1];
  if (value === undefined || value.startsWith("--")) fail(`флаг --${name} требует значения`);
  return value;
}

function unzipEntry(path: string, entry: string): string {
  const result = spawnSync("unzip", ["-p", path, entry], { encoding: "utf8", maxBuffer: 32 * 1024 * 1024 });
  if (result.status !== 0) {
    fail(`unzip ${entry}: ${result.stderr.trim() || `код ${result.status}`}`);
  }
  return result.stdout;
}

function decodeXml(text: string): string {
  return text
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&quot;", '"')
    .replaceAll("&apos;", "'")
    .replaceAll("&#39;", "'")
    .replaceAll("&amp;", "&");
}

function sharedStrings(xml: string): string[] {
  const values: string[] = [];
  const itemRe = /<si>([\s\S]*?)<\/si>/g;
  for (const match of xml.matchAll(itemRe)) {
    const text = [...(match[1] ?? "").matchAll(/<t[^>]*>([\s\S]*?)<\/t>/g)]
      .map((part) => decodeXml(part[1] ?? ""))
      .join("");
    values.push(text.trim());
  }
  return values;
}

function columnNumber(column: string): number {
  let value = 0;
  for (const char of column) value = value * 26 + (char.charCodeAt(0) - 64);
  return value;
}

function sheetPath(workbookXml: string, relsXml: string, sheetName?: string): string {
  const rels = new Map<string, string>();
  for (const match of relsXml.matchAll(/<Relationship[^>]*Id="([^"]+)"[^>]*Target="([^"]+)"/g)) {
    rels.set(match[1] ?? "", decodeXml(match[2] ?? ""));
  }
  const sheets = [...workbookXml.matchAll(/<sheet[^>]*name="([^"]+)"[^>]*r:id="([^"]+)"/g)];
  const selected = sheetName
    ? sheets.find((match) => decodeXml(match[1] ?? "") === sheetName)
    : sheets[0];
  if (!selected) fail(`лист не найден: ${sheetName ?? "<первый>"}`);
  const target = rels.get(selected[2] ?? "");
  if (!target) fail("не найден файл листа");
  return target.startsWith("/") ? target.slice(1) : target.startsWith("xl/") ? target : `xl/${target}`;
}

function readRows(path: string, sheetName?: string): string[][] {
  const shared = sharedStrings(unzipEntry(path, "xl/sharedStrings.xml"));
  const workbook = unzipEntry(path, "xl/workbook.xml");
  const rels = unzipEntry(path, "xl/_rels/workbook.xml.rels");
  const sheet = unzipEntry(path, sheetPath(workbook, rels, sheetName));
  const rows: string[][] = [];
  for (const rowMatch of sheet.matchAll(/<row[^>]*>([\s\S]*?)<\/row>/g)) {
    const cells = new Map<number, string>();
    for (const cellMatch of (rowMatch[1] ?? "").matchAll(/<c[^>]*r="([A-Z]+)\d+"[^>]*?(?:\/>|>([\s\S]*?)<\/c>)/g)) {
      const column = columnNumber(cellMatch[1] ?? "A");
      const body = cellMatch[2] ?? "";
      const typeMatch = /<c[^>]*t="([^"]+)"/.exec(cellMatch[0] ?? "");
      const type = typeMatch?.[1] ?? "n";
      const valueMatch = /<v>([\s\S]*?)<\/v>/.exec(body);
      const inline = [...body.matchAll(/<t[^>]*>([\s\S]*?)<\/t>/g)].map((part) => decodeXml(part[1] ?? "")).join("");
      let value = "";
      if (type === "s" && valueMatch) value = shared[Number(valueMatch[1])] ?? "";
      else if (type === "inlineStr") value = inline;
      else if (valueMatch) value = decodeXml(valueMatch[1] ?? "");
      cells.set(column, value.trim());
    }
    if (cells.size === 0) continue;
    const width = Math.max(...cells.keys());
    rows.push(Array.from({ length: width }, (_, index) => cells.get(index + 1) ?? ""));
  }
  return rows;
}

function csvField(value: string): string {
  return /[",\n\r]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value;
}

function csvLine(values: readonly string[]): string {
  return values.map(csvField).join(",");
}

function normalizeHeader(value: string): string {
  return value.toLowerCase().replaceAll(/[^a-zа-я0-9]+/g, "");
}

function inventoryCsv(rows: string[][]): string {
  const headerIndex = rows.findIndex((row) => row.some((cell) => normalizeHeader(cell).includes("serial")));
  if (headerIndex === -1) fail("в INSTALLED_SOFTWARE нет заголовка с серийником");
  const header = rows[headerIndex] ?? [];
  const indexOf = (needles: readonly string[]): number => {
    for (let i = 0; i < header.length; i++) {
      const normalized = normalizeHeader(header[i] ?? "");
      if (needles.some((needle) => normalized.includes(needle))) return i;
    }
    return -1;
  };
  const serial = indexOf(["serial", "serialnumber", "серийник"]);
  const name = indexOf(["displayname", "software", "name", "название", "по"]);
  const publisher = indexOf(["publisher", "издатель", "vendor"]);
  const version = indexOf(["version", "версия"]);
  if (serial === -1 || name === -1) fail("не найдены колонки серийника и названия ПО");
  const lines = [csvLine(["serial", "name", "publisher", "version", "install_date", "source"])];
  for (const row of rows.slice(headerIndex + 1)) {
    if (!row.some((cell) => cell !== "")) continue;
    lines.push(
      csvLine([
        row[serial] ?? "",
        row[name] ?? "",
        publisher === -1 ? "" : (row[publisher] ?? ""),
        version === -1 ? "" : (row[version] ?? ""),
        "",
        "xlsx",
      ]),
    );
  }
  return `${lines.join("\n")}\n`;
}

function softwareCsv(rows: string[][]): string {
  const lines = [csvLine(["name", "version", "category", "license_agreement"])];
  for (const row of rows) {
    const name = (row[0] ?? "").trim();
    if (name === "" || name === "Наименование БПО" || /^\d+$/.test(name)) continue;
    const version = (row[1] ?? "").trim();
    const section = (row[3] ?? "").trim();
    const klass = (row[4] ?? "").trim();
    const license = (row[5] ?? "").trim().replaceAll(/\s+/g, " ");
    const category = [section, klass].filter(Boolean).join(" / ");
    lines.push(csvLine([name, version, category, license]));
  }
  return `${lines.join("\n")}\n`;
}

function licensesCsv(rows: string[][]): string {
  const lines = [csvLine(["license_name"])];
  for (const row of rows) {
    const name = (row[0] ?? "").trim();
    if (name === "" || name === "Наименование лицензионного соглашения") continue;
    lines.push(csvLine([name]));
  }
  return `${lines.join("\n")}\n`;
}

function main(): void {
  const args = process.argv.slice(2);
  const kind = argValue(args, "kind") as Kind;
  if (kind !== "inventory" && kind !== "software" && kind !== "licenses") fail("--kind: inventory | software | licenses");
  const input = resolve(argValue(args, "in"));
  const output = resolve(argValue(args, "out"));
  const sheet = args.includes("--sheet") ? argValue(args, "sheet") : undefined;
  const rows = readRows(input, sheet);
  const csv = kind === "inventory" ? inventoryCsv(rows) : kind === "software" ? softwareCsv(rows) : licensesCsv(rows);
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, csv, "utf8");
  console.log(`${kind}: ${input} -> ${output} (${csv.split("\n").filter(Boolean).length - 1} строк)`);
}

if (import.meta.main) main();
