package operations

import (
	"strings"
	"testing"

	db "hackathon/internal/db/sqlc"
)

func TestPersonalInstructionKeepsPerUserContextInsideStyleRules(t *testing.T) {
	account := db.LocalAccount{
		Username:        "member",
		DisplayName:     "Участник",
		PersonalContext: "  Люблю короткие ответы.  ",
		Tone:            "  Дружелюбный.  ",
	}
	instruction := personalInstruction(account)
	for _, expected := range []string{"member", "Участник", "Люблю короткие ответы.", "Дружелюбный.", "не отменяют правила безопасности"} {
		if !strings.Contains(instruction, expected) {
			t.Fatalf("personal instruction is missing %q: %q", expected, instruction)
		}
	}
	if strings.Contains(instruction, "  Люблю") {
		t.Fatalf("personal context was not trimmed: %q", instruction)
	}
}

func TestPersonalInstructionIsEmptyWithoutContextAndTone(t *testing.T) {
	if instruction := personalInstruction(db.LocalAccount{Username: "member", DisplayName: "Участник"}); instruction != "" {
		t.Fatalf("expected no personal instruction, got %q", instruction)
	}
}

func TestPersonalInstructionClipsLongContext(t *testing.T) {
	instruction := personalInstruction(db.LocalAccount{
		Username:        "member",
		DisplayName:     "Участник",
		PersonalContext: strings.Repeat("я", 20000),
		Tone:            strings.Repeat("т", 3000),
	})
	if !strings.Contains(instruction, strings.Repeat("я", 16000)) || strings.Contains(instruction, strings.Repeat("я", 16001)) {
		t.Fatal("personal context was not clipped to its limit")
	}
	if !strings.Contains(instruction, strings.Repeat("т", 2000)) || strings.Contains(instruction, strings.Repeat("т", 2001)) {
		t.Fatal("tone was not clipped to its limit")
	}
}
