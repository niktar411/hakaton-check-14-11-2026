package questions

import "hackathon/internal/projectfacts"

// builtInProjectFacts is part of the trusted system prompt. It carries the
// verified team composition, the product decisions and the stack rationale the
// public assistant may explain. The administrator grounding is appended
// separately, so this block cannot be lost when the FAQ context is edited.
const builtInProjectFacts = projectfacts.TeamFacts + "\n\n" + projectfacts.StackFacts
