// Подготовка реального демо-пула SAM из обезличенной выгрузки инвентаризации.
//
//   bun run demo:sam
//
// Вход: demo/sam/source/installed_software.csv (получается из xlsx через
// scripts/sam/xlsx-to-csv.ts). Исходные xlsx/csv с серийниками в git не коммитятся.
// Соль и mapping хранятся вне репозитория: ~/.config/sam-demo/.
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";

export const DEMO_DIR = join(import.meta.dir, "..", "..", "demo", "sam");
export const SOURCE_DIR = join(DEMO_DIR, "source");
export const DEFAULT_SOURCE = join(SOURCE_DIR, "installed_software.csv");
export const DEFAULT_SOFTWARE = join(SOURCE_DIR, "software.csv");
export const DEFAULT_LICENSES = join(SOURCE_DIR, "licenses.csv");
export const DATASET_DATE = "2026-11-14";
export const ACTIVE_SHARE = 0.7;
export const DEFAULT_SEED = 20_261_114;

const DEMO_DISCLAIMER =
  "Демонстрационные обезличенные данные. Исходная инвентаризация очищена от серийников; " +
  "устройства заменены на PC-0001…PC-0187. Купленные места, цены и last_used_at синтетические.";

type InstallInput = {
  serial: string;
  name: string;
  publisher: string;
  version: string;
  installDate: string;
  source: string;
};

type CatalogLicense = {
  metric: "per_device" | "per_user";
  quantity: number;
  expiresAt: string;
  notes: string;
};

type CatalogProduct = {
  vendor: string;
  name: string;
  aliases: string[];
  category: string;
  status: string;
  licenses: CatalogLicense[];
  demo?: boolean;
};

type CatalogFile = {
  _meta: Record<string, unknown>;
  vendors: string[];
  products: CatalogProduct[];
  licenses: CatalogLicense[];
};

type ToolRow = {
  rowId: number;
  deviceId: string;
  productId: string;
  productName: string;
  publisher: string;
  version: string;
  installDate: string;
  isTrackable: boolean;
  isDuplicate: boolean;
};

type LicenseEntry = {
  id: string;
  product_id: string;
  product_name: string;
  vendor: string;
  metric: "device" | "user";
  seats_total: number;
  seats_active: number;
  unit_price_rub: number;
  total_price_rub: number;
  purchased_at: string;
  currency: "RUB";
  demo: true;
};

type UsageEntry = {
  device_id: string;
  product_id: string;
  last_used_at: string;
  active: boolean;
  demo: true;
};

type Summary = {
  _meta: { demo: true; seed: number; disclaimer: string };
  rows_total: number;
  devices: number;
  trackable_rows: number;
  untrackable_rows: number;
  empty_publisher: number;
  empty_version: number;
  duplicate_rows: number;
  usage: { total: number; active: number; idle: number; active_pct: number; demo: true };
  demo_cases: Record<string, { installed_devices: number; seats_total: number; active_devices: number; status: string; demo: true }>;
  checks: { name: string; expected: number | string | boolean; actual: number | string | boolean; ok: boolean }[];
  demo: true;
};

type Built = {
  csv: string;
  catalog: CatalogFile;
  licenses: { _meta: Record<string, unknown>; licenses: LicenseEntry[] };
  usage: { _meta: Record<string, unknown>; usage: UsageEntry[] };
  summary: Summary;
};

function fail(message: string): never {
  throw new Error(message);
}

function hash32(...parts: string[]): number {
  return createHash("sha256").update(parts.join("\u0000")).digest().readUInt32BE(0);
}

function normalizeSpace(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

function normalizeName(value: string): string {
  return value
    .toLowerCase()
    .replaceAll("ё", "е")
    .replaceAll(/[^a-zа-я0-9]+/g, " ")
    .trim();
}

function csvField(value: string): string {
  return /[",\n\r]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value;
}

function csvLine(values: readonly (string | number | boolean)[]): string {
  return values.map((value) => csvField(String(value))).join(",");
}

export function parseCsv(text: string): string[][] {
  const source = text.replace(/^\uFEFF/, "");
  const rows: string[][] = [];
  let row: string[] = [];
  let field = "";
  let quoted = false;
  for (let i = 0; i < source.length; i++) {
    const char = source[i] ?? "";
    if (quoted) {
      if (char === '"' && source[i + 1] === '"') {
        field += '"';
        i++;
      } else if (char === '"') quoted = false;
      else field += char;
      continue;
    }
    if (char === '"') quoted = true;
    else if (char === ",") {
      row.push(field);
      field = "";
    } else if (char === "\n") {
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
    } else if (char !== "\r") field += char;
  }
  if (field.length > 0 || row.length > 0) {
    row.push(field);
    rows.push(row);
  }
  return rows;
}

function cell(row: readonly string[], index: number): string {
  if (index < 0) return "";
  return normalizeSpace(row[index] ?? "");
}

function headerIndex(header: readonly string[], aliases: readonly string[]): number {
  const normalized = header.map((value) => normalizeName(value));
  for (const alias of aliases) {
    const index = normalized.findIndex((value) => value === alias || value.includes(alias));
    if (index !== -1) return index;
  }
  return -1;
}

export function isTrackableName(name: string): boolean {
  return !/(^|\s)(update|hotfix)(\s|$)|security\s+update|\bKB\d{4,}/i.test(name);
}

function daysAgo(days: number): string {
  return new Date(Date.UTC(2026, 10, 14) - days * 86_400_000).toISOString().slice(0, 10);
}

function pcId(index: number): string {
  return `PC-${String(index).padStart(4, "0")}`;
}

function configDir(): string {
  return join(homedir(), ".config", "sam-demo");
}

function privateWrite(path: string, content: string): void {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  writeFileSync(path, content, { mode: 0o600 });
  chmodSync(path, 0o600);
}

function loadSalt(): string {
  const fromEnv = process.env.SAM_DEMO_SALT;
  if (fromEnv !== undefined && fromEnv.length > 0) return fromEnv;
  const file = process.env.SAM_DEMO_SALT_FILE ?? join(configDir(), "salt");
  if (existsSync(file)) {
    const value = readFileSync(file, "utf8").trim();
    if (value.length > 0) return value;
  }
  const generated = createHash("sha256").update(`${DEFAULT_SEED}:${Math.random()}`).digest("hex");
  privateWrite(file, `${generated}\n`);
  return generated;
}

function loadMapping(file: string): Record<string, string> {
  if (!existsSync(file)) return {};
  const parsed: unknown = JSON.parse(readFileSync(file, "utf8"));
  if (typeof parsed !== "object" || parsed === null || !("devices" in parsed)) fail(`mapping повреждён: ${file}`);
  const devices = (parsed as { devices: unknown }).devices;
  if (typeof devices !== "object" || devices === null) fail(`mapping повреждён: ${file}`);
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(devices)) {
    if (typeof value === "string") result[key] = value;
  }
  return result;
}

function anonymizeDevices(keys: readonly string[]): Map<string, string> {
  const salt = loadSalt();
  const mappingFile = process.env.SAM_DEMO_MAPPING ?? join(configDir(), "mapping.json");
  const mapping = loadMapping(mappingFile);
  const unique = [...new Set(keys)].filter((key) => key !== "");
  if (unique.length > 187) fail(`устройств ${unique.length}, ожидается не больше 187`);
  const digest = (key: string) => createHash("sha256").update(`${salt}\u0000${key}`).digest("hex");
  unique.sort((a, b) => digest(a).localeCompare(digest(b)));
  const used = new Set(Object.values(mapping));
  let next = 1;
  const allocate = (): string => {
    while (used.has(pcId(next))) next++;
    const value = pcId(next);
    used.add(value);
    return value;
  };
  let changed = false;
  for (const key of unique) {
    if (mapping[key] === undefined) {
      mapping[key] = allocate();
      changed = true;
    }
  }
  if (changed) privateWrite(mappingFile, `${JSON.stringify({ version: 1, devices: mapping }, null, 2)}\n`);
  return new Map(unique.map((key) => [key, mapping[key] ?? ""]));
}

function readInstalls(path: string): InstallInput[] {
  const rows = parseCsv(readFileSync(path, "utf8"));
  const headerAt = rows.findIndex((row) => row.some((value) => value.trim() !== ""));
  if (headerAt === -1) fail("пустой CSV");
  const header = rows[headerAt] ?? [];
  const serial = headerIndex(header, ["serial", "serialnumber", "seriynik"]);
  const name = headerIndex(header, ["name", "software", "displayname", "nazvanie", "po"]);
  const publisher = headerIndex(header, ["publisher", "vendor", "izdatel"]);
  const version = headerIndex(header, ["version", "versiya"]);
  const installDate = headerIndex(header, ["installdate", "date"]);
  const source = headerIndex(header, ["source", "istochnik"]);
  if (serial === -1 || name === -1) fail("нет колонок serial и name");
  return rows.slice(headerAt + 1).map((row, index) => {
    const serialValue = cell(row, serial);
    const nameValue = cell(row, name);
    if (serialValue === "" && nameValue === "") return null;
    if (serialValue === "") fail(`строка ${headerAt + index + 2}: пустой серийник`);
    if (nameValue === "") fail(`строка ${headerAt + index + 2}: пустое название ПО`);
    return {
      serial: serialValue,
      name: nameValue,
      publisher: cell(row, publisher),
      version: cell(row, version),
      installDate: cell(row, installDate),
      source: cell(row, source),
    };
  }).filter((row): row is InstallInput => row !== null);
}

type SoftwareRow = { name: string; version: string; category: string; license: string };

function readSoftware(path: string): SoftwareRow[] {
  const rows = parseCsv(readFileSync(path, "utf8"));
  const result: SoftwareRow[] = [];
  for (const row of rows) {
    const name = cell(row, 0);
    if (name === "" || name === "name") continue;
    result.push({ name, version: cell(row, 1), category: cell(row, 2), license: cell(row, 3) });
  }
  return result;
}

const KNOWN_VENDORS: readonly (readonly [RegExp, string])[] = [
  [/microsoft|офис|office|визIO|visio|проект|project/i, "Microsoft"],
  [/google|chrome|хром/i, "Google"],
  [/adobe|photoshop|акробат|acrobat/i, "Adobe"],
  [/oracle|java|jdk|jre/i, "Oracle"],
  [/kaspersky|каспер/i, "Kaspersky"],
  [/dr\.?web|доктор веб/i, "Dr.Web"],
  [/1с|1c|бухгалтер/i, "1С"],
  [/famatech|radmin/i, "Famatech"],
  [/jetbrains|intellij|idea/i, "JetBrains"],
  [/python|python software/i, "Python Software Foundation"],
  [/gitlab|git/i, "Git"],
  [/node\.?js|nodejs|openjs/i, "OpenJS Foundation"],
  [/nginx/i, "NGINX"],
  [/postgresql|postgres/i, "PostgreSQL"],
  [/apache/i, "Apache Software Foundation"],
  [/mozilla|firefox/i, "Mozilla"],
  [/7-?zip|igor pavlov/i, "Igor Pavlov"],
  [/vim/i, "Vim"],
  [/far manager/i, "Far Manager"],
  [/virtualbox|oracle vm/i, "Oracle"],
  [/inkscape/i, "Inkscape"],
  [/miktex/i, "MiKTeX"],
  [/cppcheck/i, "Cppcheck"],
  [/gimp/i, "GIMP"],
  [/glassfish/i, "GlassFish"],
  [/mediawiki/i, "MediaWiki"],
  [/pdfcreator/i, "PDFCreator"],
  [/tortoise/i, "Tortoise"],
  [/graphviz/i, "Graphviz"],
  [/pgadmin/i, "pgAdmin"],
  [/archi/i, "Archi"],
  [/cmake/i, "CMake"],
  [/pgrouting|postgis/i, "PostGIS"],
  [/dbeaver/i, "DBeaver"],
  [/nvidia|cuda/i, "NVIDIA"],
  [/liberica/i, "BellSoft"],
];

function vendorFor(name: string, publisher: string): string {
  if (publisher !== "") return publisher;
  for (const [pattern, vendor] of KNOWN_VENDORS) {
    if (pattern.test(name)) return vendor;
  }
  return "";
}

function buildCatalog(software: SoftwareRow[], installs: InstallInput[]): { catalog: CatalogFile; products: CatalogProduct[] } {
  const byName = new Map<string, CatalogProduct>();
  const publisherCounts = new Map<string, Map<string, number>>();
  for (const install of installs) {
    const normalized = normalizeName(install.name);
    if (normalized === "") continue;
    const counts = publisherCounts.get(normalized) ?? new Map<string, number>();
    const publisher = install.publisher === "" ? "—" : install.publisher;
    counts.set(publisher, (counts.get(publisher) ?? 0) + 1);
    publisherCounts.set(normalized, counts);
  }
  const bestPublisher = (name: string): string => {
    const counts = publisherCounts.get(normalizeName(name));
    if (!counts) return "";
    const sorted = [...counts.entries()].sort((a, b) => b[1] - a[1]);
    return sorted[0]?.[0] === "—" ? "" : (sorted[0]?.[0] ?? "");
  };

  for (const item of software) {
    const key = normalizeName(item.name);
    if (key === "") continue;
    const existing = byName.get(key);
    const vendor = bestPublisher(item.name) || vendorFor(item.name, "");
    if (existing) {
      if (!existing.aliases.includes(item.version) && item.version !== "") existing.aliases.push(item.version);
      continue;
    }
    byName.set(key, {
      vendor,
      name: item.name,
      aliases: item.version === "" ? [] : [item.version],
      category: item.category,
      status: "active",
      licenses: [],
    });
  }

  const demoProducts: CatalogProduct[] = [
    {
      vendor: "Microsoft",
      name: "Microsoft Office",
      aliases: ["Office", "Microsoft Office", "Office 2010", "Office 2013", "Office 2016", "Office 2019", "Office 2021"],
      category: "05 - Прикладное программное обеспечение",
      status: "active",
      licenses: [{ metric: "per_device", quantity: 100, expiresAt: "2027-12-31", notes: "demo" }],
      demo: true,
    },
    {
      vendor: "Microsoft",
      name: "Microsoft Project",
      aliases: ["Project", "MS Project", "Microsoft Project"],
      category: "05 - Прикладное программное обеспечение",
      status: "active",
      licenses: [{ metric: "per_user", quantity: 50, expiresAt: "2027-12-31", notes: "demo" }],
      demo: true,
    },
    {
      vendor: "Adobe",
      name: "Adobe Photoshop",
      aliases: ["Photoshop", "Adobe Photoshop"],
      category: "05 - Прикладное программное обеспечение",
      status: "active",
      licenses: [],
      demo: true,
    },
    {
      vendor: "Famatech",
      name: "Radmin",
      aliases: ["Radmin", "Radmin Server", "Radmin Viewer"],
      category: "04 - Средства разработки программного обеспечения",
      status: "banned",
      licenses: [],
      demo: true,
    },
  ];
  for (const product of demoProducts) {
    byName.set(normalizeName(product.name), product);
  }

  const products = [...byName.values()].sort((a, b) => a.name.localeCompare(b.name));
  const vendors = [...new Set(products.map((product) => product.vendor).filter((vendor) => vendor !== ""))].sort();
  const licenses = products.flatMap((product) => product.licenses);
  return {
    catalog: {
      _meta: { demo: true, dataset_date: DATASET_DATE, disclaimer: DEMO_DISCLAIMER },
      vendors,
      products,
      licenses,
    },
    products,
  };
}

function productIdFor(name: string, products: CatalogProduct[]): string {
  const normalized = normalizeName(name);
  if (normalized === "") return "";
  let best = "";
  let bestScore = 0;
  for (const product of products) {
    const productName = normalizeName(product.name);
    const aliases = product.aliases.map(normalizeName).filter(Boolean);
    let score = 0;
    if (productName !== "" && normalized === productName) score = 1;
    else if (productName !== "" && normalized.includes(productName)) score = 0.9;
    else if (aliases.some((alias) => alias !== "" && normalized.includes(alias))) score = 0.85;
    else {
      const left = new Set(normalized.split(" "));
      const right = new Set(productName.split(" "));
      const intersection = [...left].filter((token) => right.has(token)).length;
      const union = new Set([...left, ...right]).size;
      const similarity = union === 0 ? 0 : intersection / union;
      if (similarity >= 0.5) score = 0.7;
    }
    if (score > bestScore || (score === bestScore && score > 0 && product.name.length > best.length)) {
      bestScore = score;
      best = product.name;
    }
  }
  return bestScore >= 0.7 ? best : "";
}

function buildRows(installs: InstallInput[], products: CatalogProduct[]): ToolRow[] {
  const mapping = anonymizeDevices(installs.map((row) => row.serial));
  const rows: ToolRow[] = installs.map((row) => {
    const productName = productIdFor(row.name, products);
    return {
      rowId: 0,
      deviceId: mapping.get(row.serial) ?? "",
      productId: productName,
      productName: row.name,
      publisher: row.publisher,
      version: row.version,
      installDate: row.installDate,
      isTrackable: isTrackableName(row.name),
      isDuplicate: false,
    };
  });
  rows.sort(
    (a, b) =>
      a.deviceId.localeCompare(b.deviceId) ||
      a.productName.localeCompare(b.productName) ||
      a.version.localeCompare(b.version) ||
      Number(a.isTrackable) - Number(b.isTrackable),
  );
  const seen = new Set<string>();
  rows.forEach((row, index) => {
    row.rowId = index + 1;
    const key = `${row.deviceId}\u0000${row.productName}\u0000${row.version}`;
    row.isDuplicate = seen.has(key);
    seen.add(key);
  });
  return rows;
}

function buildUsage(rows: ToolRow[], seed: number): { entries: UsageEntry[]; activeByProduct: Map<string, number> } {
  const unique = new Map<string, UsageEntry>();
  for (const row of rows) {
    if (row.productId === "" || (!row.isTrackable && row.productId !== "Microsoft Project")) continue;
    const key = `${row.deviceId}\u0000${row.productId}`;
    if (unique.has(key)) continue;
    unique.set(key, {
      device_id: row.deviceId,
      product_id: row.productId,
      last_used_at: "",
      active: hash32(String(seed), "usage", row.deviceId, row.productId) % 100 < ACTIVE_SHARE * 100,
      demo: true,
    });
  }
  const entries = [...unique.values()].sort(
    (a, b) => a.device_id.localeCompare(b.device_id) || a.product_id.localeCompare(b.product_id),
  );
  const project = entries.filter((entry) => entry.product_id === "Microsoft Project");
  project.forEach((entry, index) => {
    entry.active = index < 10;
  });
  const target = Math.round(entries.length * ACTIVE_SHARE);
  let active = entries.filter((entry) => entry.active).length;
  if (active > target) {
    for (let i = entries.length - 1; i >= 0 && active > target; i--) {
      const entry = entries[i];
      if (entry?.active && entry.product_id !== "Microsoft Project") {
        entry.active = false;
        active--;
      }
    }
  } else if (active < target) {
    for (const entry of entries) {
      if (active >= target) break;
      if (!entry.active && entry.product_id !== "Microsoft Project") {
        entry.active = true;
        active++;
      }
    }
  }
  if (active !== target) fail(`активных ${active}, ожидалось ${target}`);
  for (const entry of entries) {
    const days = entry.active
      ? hash32(String(seed), "last", entry.device_id, entry.product_id) % 30
      : 120 + (hash32(String(seed), "idle", entry.device_id, entry.product_id) % 421);
    entry.last_used_at = daysAgo(days);
  }
  const activeByProduct = new Map<string, number>();
  for (const entry of entries) {
    if (entry.active) activeByProduct.set(entry.product_id, (activeByProduct.get(entry.product_id) ?? 0) + 1);
  }
  return { entries, activeByProduct };
}

export function buildRealDemo(seed = DEFAULT_SEED): Built {
  if (!existsSync(DEFAULT_SOURCE)) fail(`нет исходного CSV: ${DEFAULT_SOURCE}. Сначала запустите scripts/sam/xlsx-to-csv.ts`);
  if (!existsSync(DEFAULT_SOFTWARE)) fail(`нет справочника ПО: ${DEFAULT_SOFTWARE}`);
  const installs = readInstalls(DEFAULT_SOURCE);
  const software = readSoftware(DEFAULT_SOFTWARE);
  const { catalog, products } = buildCatalog(software, installs);
  const rows = buildRows(installs, products);
  const usage = buildUsage(rows, seed);

  const devices = new Set(rows.map((row) => row.deviceId));
  if (devices.size !== 187) fail(`устройств ${devices.size}, ожидалось 187`);
  const trackable = rows.filter((row) => row.isTrackable);
  const untrackable = rows.length - trackable.length;
  const emptyPublisher = rows.filter((row) => row.publisher === "").length;
  const emptyVersion = rows.filter((row) => row.version === "").length;
  const duplicates = rows.filter((row) => row.isDuplicate).length;
  const active = usage.entries.filter((entry) => entry.active).length;

  const devicesByProduct = new Map<string, Set<string>>();
  for (const row of rows) {
    if (row.productId === "") continue;
    const set = devicesByProduct.get(row.productId) ?? new Set<string>();
    set.add(row.deviceId);
    devicesByProduct.set(row.productId, set);
  }
  type DemoCase = {
    installed_devices: number;
    seats_total: number;
    active_devices: number;
    status: string;
    demo: true;
  };
  const caseOf = (productName: string, status: string): DemoCase => {
    const seats = products.find((product) => product.name === productName)?.licenses[0]?.quantity ?? 0;
    return {
      installed_devices: devicesByProduct.get(productName)?.size ?? 0,
      seats_total: seats,
      active_devices: usage.activeByProduct.get(productName) ?? 0,
      status,
      demo: true as const,
    };
  };

  const summary: Summary = {
    _meta: { demo: true, seed, disclaimer: DEMO_DISCLAIMER },
    rows_total: rows.length,
    devices: devices.size,
    trackable_rows: trackable.length,
    untrackable_rows: untrackable,
    empty_publisher: emptyPublisher,
    empty_version: emptyVersion,
    duplicate_rows: duplicates,
    usage: {
      total: usage.entries.length,
      active,
      idle: usage.entries.length - active,
      active_pct: Number(((active / usage.entries.length) * 100).toFixed(1)),
      demo: true,
    },
    demo_cases: {
      "Microsoft Office": caseOf("Microsoft Office", "active"),
      "Adobe Photoshop": caseOf("Adobe Photoshop", "active"),
      Radmin: caseOf("Radmin", "banned"),
      "Microsoft Project": caseOf("Microsoft Project", "active"),
    },
    checks: [],
    demo: true,
  };
  const check = (name: string, expected: number | string | boolean, actual: number | string | boolean) => ({
    name,
    expected,
    actual,
    ok: expected === actual,
  });
  const demoCase = (name: string): DemoCase => {
    const value = summary.demo_cases[name];
    if (!value) fail(`нет демо-кейса ${name}`);
    return value;
  };
  summary.checks = [
    check("rows_total", 24_589, rows.length),
    check("devices", 187, devices.size),
    check("untrackable_rows", 8_590, untrackable),
    check("empty_version", 68, emptyVersion),
    check("duplicate_rows", 21, duplicates),
    check("office_devices", 119, demoCase("Microsoft Office").installed_devices),
    check("photoshop_devices", 2, demoCase("Adobe Photoshop").installed_devices),
    check("radmin_devices", 8, demoCase("Radmin").installed_devices),
    check("project_seats", 50, demoCase("Microsoft Project").seats_total),
    check("project_active", 10, demoCase("Microsoft Project").active_devices),
  ];

  const csvLines = [
    csvLine(["row_id", "device", "product_id", "product_name", "publisher", "version", "install_date", "is_trackable", "is_duplicate", "is_demo"]),
    ...rows.map((row) =>
      csvLine([row.rowId, row.deviceId, row.productId, row.productName, row.publisher, row.version, row.installDate, row.isTrackable, row.isDuplicate, true]),
    ),
  ];
  const csv = `${csvLines.join("\n")}\n`;

  const licenses: LicenseEntry[] = products
    .filter((product) => product.licenses.length > 0)
    .flatMap((product) =>
      product.licenses.map((license) => {
        const activeSeats = usage.activeByProduct.get(product.name) ?? 0;
        const price = product.name === "Microsoft Office" ? 48_000 : product.name === "Microsoft Project" ? 62_000 : 0;
        return {
          id: `lic-${normalizeName(product.name)}`,
          product_id: product.name,
          product_name: product.name,
          vendor: product.vendor,
          metric: license.metric === "per_user" ? "user" : "device",
          seats_total: license.quantity,
          seats_active: activeSeats,
          unit_price_rub: price,
          total_price_rub: license.quantity * price,
          purchased_at: "2025-03-01",
          currency: "RUB",
          demo: true,
        };
      }),
    );

  const meta = { demo: true, dataset_date: DATASET_DATE, disclaimer: DEMO_DISCLAIMER, seed };
  return {
    csv,
    catalog,
    licenses: { _meta: meta, licenses },
    usage: { _meta: meta, usage: usage.entries },
    summary,
  };
}

export function writeDemo(built: Built, outDir = DEMO_DIR): string[] {
  mkdirSync(outDir, { recursive: true });
  const files: [string, string][] = [
    [join(outDir, "installed_software.csv"), built.csv],
    [join(outDir, "catalog.json"), `${JSON.stringify(built.catalog, null, 2)}\n`],
    [join(outDir, "licenses.json"), `${JSON.stringify(built.licenses, null, 2)}\n`],
    [join(outDir, "usage.json"), `${JSON.stringify(built.usage, null, 2)}\n`],
    [join(outDir, "summary.json"), `${JSON.stringify(built.summary, null, 2)}\n`],
  ];
  for (const [path, content] of files) writeFileSync(path, content, "utf8");
  return files.map(([path]) => path);
}

export function assertChecks(summary: Summary, allowMismatch = false): void {
  const failed = summary.checks.filter((check) => !check.ok);
  if (failed.length === 0) return;
  const text = failed.map((check) => `${check.name}: ${check.actual} ≠ ${check.expected}`).join("; ");
  if (allowMismatch) {
    console.warn(`⚠ Расхождения: ${text}`);
    return;
  }
  fail(`Расхождения: ${text}. Принудительно: --allow-count-mismatch`);
}

function flag(args: readonly string[], name: string): string | undefined {
  const index = args.indexOf(`--${name}`);
  if (index === -1) return undefined;
  const value = args[index + 1];
  if (value === undefined || value.startsWith("--")) fail(`флаг --${name} требует значения`);
  return value;
}

function main(): void {
  const args = process.argv.slice(2);
  const outDir = resolve(flag(args, "out") ?? DEMO_DIR);
  const seed = Number(flag(args, "seed") ?? DEFAULT_SEED);
  const allowMismatch = args.includes("--allow-count-mismatch");
  const built = buildRealDemo(seed);
  assertChecks(built.summary, allowMismatch);
  const files = writeDemo(built, outDir);
  const summary = built.summary;
  console.log(`SAM real demo: ${summary.rows_total} строк, ${summary.devices} ПК, обновлений ${summary.untrackable_rows}`);
  console.log(`  пустых version: ${summary.empty_version}, дублей: ${summary.duplicate_rows}`);
  console.log(`  usage: ${summary.usage.active}/${summary.usage.total} (${summary.usage.active_pct}%)`);
  console.log(`  кейсы: ${JSON.stringify(summary.demo_cases)}`);
  console.log(`  файлы: ${files.map((file) => file.replace(`${process.cwd()}/`, "")).join(", ")}`);
}

if (import.meta.main) main();
