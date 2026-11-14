import { expect, test } from "bun:test";
import { parseTeamSection } from "../client/src/features/team/readme-team";

const readme = await Bun.file(new URL("../README.md", import.meta.url)).text();

test("README team block powers the public page", () => {
	const team = parseTeamSection(readme);
	expect(team.name?.trim().length).toBeGreaterThan(0);
	expect(team.tagline?.trim().length).toBeGreaterThan(0);
	expect(team.summary?.trim().length).toBeGreaterThan(0);
	expect(team.members.length).toBeGreaterThanOrEqual(5);
	for (const member of team.members) {
		expect(member.name.length).toBeGreaterThan(0);
		expect(member.role.length).toBeGreaterThan(0);
		expect(member.telegram).toMatch(/^https:\/\/t\.me\//);
	}
});

test("a missing team block is an empty section", () => {
	expect(parseTeamSection("# Проект\n\nБез состава.").members).toEqual([]);
});
