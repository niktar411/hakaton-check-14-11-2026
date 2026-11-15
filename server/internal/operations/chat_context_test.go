package operations

import (
	"strings"
	"testing"

	db "hackathon/internal/db/sqlc"
)

func TestWantsSkipClarification(t *testing.T) {
	for _, prompt := range []string{
		"продолжи работу на свое усмотрение",
		"Продолжай работу на своё усмотрение",
		"действуй на свое усмотрение",
		"Сделай задачу и продолжи на свое усмотрение",
	} {
		if !wantsSkipClarification(prompt) {
			t.Fatalf("expected skip phrase in %q", prompt)
		}
	}
	for _, prompt := range []string{
		"продолжи работу над задачей",
		"на свое усмотрение",
		"как лучше сделать?",
	} {
		if wantsSkipClarification(prompt) {
			t.Fatalf("unexpected skip phrase in %q", prompt)
		}
	}
}

func TestClarificationAlreadyAsked(t *testing.T) {
	if clarificationAlreadyAsked(nil) {
		t.Fatal("empty history must not skip clarification")
	}
	if clarificationAlreadyAsked([]db.Message{{Role: "user", Content: "вопрос"}}) {
		t.Fatal("a single user message must not skip clarification")
	}
	if !clarificationAlreadyAsked([]db.Message{
		{Role: "user", Content: "вопрос"},
		{Role: "assistant", Content: "уточняющий вопрос"},
	}) {
		t.Fatal("an assistant answer must skip the next clarification round")
	}
}

func TestComposeChatInstructionIncludesSharedContext(t *testing.T) {
	instruction := composeChatInstruction("clarifying", "AGENTS-CONTENT", "PROJECT-CONTENT", "GENERAL-CONTENT", "PERSONAL-CONTENT")
	for _, expected := range []string{
		"единственный раунд",
		accessInstruction,
		"AGENTS-CONTENT",
		"PROJECT-CONTENT",
		"GENERAL-CONTENT",
		"PERSONAL-CONTENT",
	} {
		if !strings.Contains(instruction, expected) {
			t.Fatalf("clarifying instruction is missing %q: %s", expected, instruction)
		}
	}
	solution := composeChatInstruction("ready", "AGENTS-CONTENT", "PROJECT-CONTENT", "GENERAL-CONTENT", "")
	if !strings.Contains(solution, "перешёл от уточнения к решению") {
		t.Fatalf("solution instruction is missing its phase text: %s", solution)
	}
	if strings.Contains(solution, "единственный раунд") {
		t.Fatal("solution instruction must not contain the clarification text")
	}
}

func TestComposeChatInstructionSkipsEmptySections(t *testing.T) {
	instruction := composeChatInstruction("ready", "  ", "  ", "  ", "  ")
	if strings.Contains(instruction, "AGENTS.md") || strings.Contains(instruction, "Дополнительный общий контекст") {
		t.Fatalf("empty shared sections must be omitted: %s", instruction)
	}
}
