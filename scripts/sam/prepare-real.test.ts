import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  DEMO_DIR,
  DEFAULT_SOURCE,
  assertChecks,
  buildRealDemo,
  parseCsv,
} from "./prepare-real.ts";

const csvText = readFileSync(join(DEMO_DIR, "installed_software.csv"), "utf8");
const catalog = JSON.parse(readFileSync(join(DEMO_DIR, "catalog.json"), "utf8"));
const licenses = JSON.parse(readFileSync(join(DEMO_DIR, "licenses.json"), "utf8"));
const usage = JSON.parse(readFileSync(join(DEMO_DIR, "usage.json"), "utf8"));
const summary = JSON.parse(readFileSync(join(DEMO_DIR, "summary.json"), "utf8"));

const header = parseCsv(csvText)[0] ?? [];
const rows = parseCsv(csvText)
  .slice(1)
  .map((cells) => ({
    row_id: cells[0] ?? "",
    device: cells[1] ?? "",
    product_id: cells[2] ?? "",
    product_name: cells[3] ?? "",
    publisher: cells[4] ?? "",
    version: cells[5] ?? "",
    install_date: cells[6] ?? "",
    is_trackable: cells[7] ?? "",
    is_duplicate: cells[8] ?? "",
    is_demo: cells[9] ?? "",
  }));

describe("реальный демо-пул", () => {
  test("структура и счётчики CSV", () => {
    expect(header).toEqual([
      "row_id",
      "device",
      "product_id",
      "product_name",
      "publisher",
      "version",
      "install_date",
      "is_trackable",
      "is_duplicate",
      "is_demo",
    ]);
    expect(rows).toHaveLength(24_589);
    expect(new Set(rows.map((row) => row.device))).toHaveLength(187);
    expect(rows.filter((row) => row.is_trackable === "false")).toHaveLength(8_590);
    expect(rows.filter((row) => row.version === "")).toHaveLength(68);
    expect(rows.filter((row) => row.is_duplicate === "true")).toHaveLength(21);
    for (const row of rows) expect(row.is_demo).toBe("true");
  });

  test("нет серийников и хостов, только PC-NNNN", () => {
    for (const row of rows) expect(row.device).toMatch(/^PC-\d{4}$/);
    expect(csvText).not.toMatch(/G69ZD5J|B1FG0S2|B1RG0S2/);
    expect(csvText).not.toMatch(/COMPUTERS\.SERIAL_NUMBER/i);
  });

  test("каталог в формате catalog.Load", () => {
    expect(catalog._meta.demo).toBe(true);
    expect(catalog.vendors.length).toBeGreaterThan(20);
    expect(catalog.products.length).toBeGreaterThan(25);
    const names = new Set(catalog.products.map((product: { name: string }) => product.name));
    expect(names.has("Microsoft Office")).toBe(true);
    expect(names.has("Microsoft Project")).toBe(true);
    expect(names.has("Adobe Photoshop")).toBe(true);
    expect(names.has("Radmin")).toBe(true);
    const office = catalog.products.find((product: { name: string }) => product.name === "Microsoft Office");
    expect(office?.licenses[0]?.quantity).toBe(100);
    const project = catalog.products.find((product: { name: string }) => product.name === "Microsoft Project");
    expect(project?.licenses[0]?.quantity).toBe(50);
  });

  test("демо-кейсы и summary", () => {
    expect(summary.checks.filter((check: { ok: boolean }) => !check.ok)).toEqual([]);
    expect(summary.demo_cases["Microsoft Office"].installed_devices).toBe(119);
    expect(summary.demo_cases["Adobe Photoshop"].installed_devices).toBe(2);
    expect(summary.demo_cases.Radmin.installed_devices).toBe(8);
    expect(summary.demo_cases["Microsoft Project"].seats_total).toBe(50);
    expect(summary.demo_cases["Microsoft Project"].active_devices).toBe(10);
    expect(summary.usage.active_pct).toBeGreaterThanOrEqual(69.5);
    expect(summary.usage.active_pct).toBeLessThanOrEqual(70.5);
  });

  test("лицензии и usage", () => {
    const office = licenses.licenses.find((item: { product_id: string }) => item.product_id === "Microsoft Office");
    const project = licenses.licenses.find((item: { product_id: string }) => item.product_id === "Microsoft Project");
    expect(office?.seats_total).toBe(100);
    expect(project?.seats_total).toBe(50);
    expect(project?.seats_active).toBe(10);
    const projectUsage = usage.usage.filter((item: { product_id: string }) => item.product_id === "Microsoft Project");
    expect(projectUsage).toHaveLength(68);
    expect(projectUsage.filter((item: { active: boolean }) => item.active)).toHaveLength(10);
  });
});

describe("воспроизводимость", () => {
  test.skipIf(!existsSync(DEFAULT_SOURCE))("повторная сборка из исходного CSV совпадает", () => {
    const first = buildRealDemo();
    const second = buildRealDemo();
    expect(first.csv).toBe(second.csv);
    expect(JSON.stringify(first.catalog)).toBe(JSON.stringify(second.catalog));
    expect(JSON.stringify(first.usage)).toBe(JSON.stringify(second.usage));
    expect(JSON.stringify(first.summary)).toBe(JSON.stringify(second.summary));
    assertChecks(first.summary);
  });
});
