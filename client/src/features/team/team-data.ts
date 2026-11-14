// biome-ignore lint/style/noRestrictedImports: README лежит вне client и остаётся единственным источником состава команды.
import readme from "../../../../README.md?raw";
import { parseTeamSection } from "./readme-team";

/**
 * Team facts for the public page come from the README block so the team can
 * edit names, roles and contacts without touching components.
 */
export const team = parseTeamSection(readme);
