export type TeamMember = {
	name: string;
	role: string;
	telegram?: string;
	quote?: string;
	bio?: string;
	photo?: string;
};

export type TeamSection = {
	name?: string;
	tagline?: string;
	summary?: string;
	members: TeamMember[];
};

const START_MARKER = "<!-- team:start -->";
const END_MARKER = "<!-- team:end -->";

function telegramUrl(value: string) {
	const trimmed = value.trim();
	if (!trimmed) return undefined;
	if (/^https?:\/\//i.test(trimmed)) return trimmed;
	const handle = trimmed.replace(/^@/, "");
	return handle ? `https://t.me/${handle}` : undefined;
}

/**
 * Reads the editable team block from README.md. The block is deliberately
 * small and marker-based: content editors change README, not the page.
 */
export function parseTeamSection(source: string): TeamSection {
	const start = source.indexOf(START_MARKER);
	const end = source.indexOf(END_MARKER);
	if (start === -1 || end === -1 || end <= start) {
		return { members: [] };
	}

	const result: TeamSection = { members: [] };
	let inTable = false;
	const block = source.slice(start + START_MARKER.length, end);

	for (const raw of block.split(/\r?\n/)) {
		const line = raw.trim();
		if (!line) {
			inTable = false;
			continue;
		}
		if (line.startsWith("|")) {
			if (!inTable) {
				inTable = true;
				continue;
			}
			if (/^\|[\s|:-]+\|?$/.test(line)) continue;
			const cells = line
				.replace(/^\||\|$/g, "")
				.split("|")
				.map((cell) => cell.trim());
			const name = cells[0] ?? "";
			const role = cells[1] ?? "";
			if (name && role) {
				result.members.push({
					name,
					role,
					telegram: telegramUrl(cells[2] ?? ""),
					quote: cells[3]?.trim() || undefined,
					bio: cells[4]?.trim() || undefined,
					photo: cells[5]?.trim() || undefined,
				});
			}
			continue;
		}

		inTable = false;
		const field = /^\*\*([^*]+):\*\*\s*(.+)$/.exec(line);
		if (!field) continue;
		const label = field[1];
		const value = field[2];
		if (label === "Название") result.name = value;
		else if (label === "Девиз") result.tagline = value;
		else if (label === "О команде") result.summary = value;
	}

	return result;
}
