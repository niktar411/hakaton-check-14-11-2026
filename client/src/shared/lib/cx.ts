/**
 * Склейка классов: CSS Module, классы Tailwind и класс от родителя. Пустые пропускаются.
 * Biome знает эту функцию (`tailwind.mergeFunctions` в biome.json) и проверяет
 * строки внутри — поэтому классы склеиваются только через неё, не шаблонной строкой.
 */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(' ');
}
