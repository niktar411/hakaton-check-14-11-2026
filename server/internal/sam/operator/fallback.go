package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var seatPattern = regexp.MustCompile(`(\d+)`)

var productStopWords = map[string]bool{
	"а": true, "без": true, "в": true, "все": true, "всех": true, "вы": true,
	"где": true, "да": true, "для": true, "до": true, "его": true, "ее": true,
	"если": true, "есть": true, "и": true, "из": true, "или": true, "к": true,
	"как": true, "какие": true, "ко": true, "когда": true, "ли": true, "на": true,
	"над": true, "не": true, "но": true, "о": true, "об": true, "от": true,
	"по": true, "под": true, "при": true, "про": true, "с": true, "со": true,
	"так": true, "то": true, "у": true, "что": true, "это": true,
	"сопоставь": true, "сопоставить": true, "сопоставление": true, "привяжи": true,
	"привязать": true, "match": true, "alias": true, "aliases": true,
	"архивируй": true, "архивировать": true, "archive": true,
	"смени": true, "сменить": true, "переведи": true, "перевести": true,
	"статус": true, "статусом": true, "status": true,
	"неактивный": true, "неактивную": true, "неактив": true,
	"активный": true, "активную": true, "актив": true,
	"архивный": true, "архивную": true, "архив": true,
	"категория": true, "категорию": true, "category": true,
	"создай": true, "добавь": true, "создать": true, "добавить": true,
	"create": true, "add": true, "продукт": true, "продукта": true, "продуктов": true,
	"product": true, "products": true, "название": true, "имя": true,
	"лицензия": true, "лицензии": true, "лицензию": true, "license": true,
	"мест": true, "места": true, "мест.": true, "seats": true,
	"продли": true, "продлить": true, "измени": true, "изменить": true,
	"поставь": true, "поставить": true, "сделай": true,
	"изменение": true, "изменения": true, "пожалуйста": true, "нужно": true,
	"надо": true, "этот": true, "эту": true, "этого": true, "ещё": true,
	"еще": true, "хочу": true, "можешь": true, "можете": true,
}

func fallbackProductTerm(prompt string) string {
	normalized := normalizeText(prompt)
	fields := strings.Fields(normalized)
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" || productStopWords[field] || isDigits(field) {
			continue
		}
		parts = append(parts, field)
	}
	return strings.Join(parts, " ")
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func afterKeyword(prompt, keyword string) string {
	lower := strings.ToLower(prompt)
	index := strings.Index(lower, keyword)
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(prompt[index+len(keyword):])
	value = strings.TrimLeft(value, " :«»\"'")
	return strings.TrimSpace(value)
}

func fallbackStatus(prompt string) string {
	switch {
	case strings.Contains(prompt, "неактив"):
		return "inactive"
	case strings.Contains(prompt, "архив"):
		return "archived"
	default:
		return "active"
	}
}

func proposalAnswer(action SamOperatorAction) string {
	answer := fmt.Sprintf("Подготовлено предложение #%d: %s Затронуто строк: %d. Изменение не применено — проверьте preview и примените его отдельным подтверждением администратора.", action.ID, action.Preview.Summary, action.Affected)
	if action.Preview.RequiresPhrase {
		answer += fmt.Sprintf(" Для применения нужна контрольная фраза: %s.", action.Preview.ConfirmationPhrase)
	}
	return answer
}

func (s *Service) fallback(ctx context.Context, actor string, conversationID int64, prompt string) (string, []int64, error) {
	normalized := normalizeText(prompt)
	switch {
	case strings.Contains(normalized, "сопостав") || strings.Contains(normalized, "привяж") || strings.Contains(normalized, "match"):
		product := "Google Chrome"
		if !strings.Contains(normalized, "хром") && !strings.Contains(normalized, "chrome") {
			if index := strings.LastIndex(normalized, " с "); index >= 0 {
				product = strings.TrimSpace(normalized[index+3:])
			} else {
				product = fallbackProductTerm(prompt)
			}
		}
		all := strings.Contains(normalized, "все") || strings.Contains(normalized, "всех") || strings.Contains(normalized, "all")
		arguments, err := json.Marshal(matchAliasesArgs{Product: product, All: all})
		if err != nil {
			return "", nil, err
		}
		action, err := s.Propose(ctx, conversationID, actor, "match_aliases", string(arguments))
		if err != nil {
			return fmt.Sprintf("Не удалось подготовить сопоставление для «%s»: %s. Уточните название продукта или исходные имена.", product, err.Error()), nil, nil
		}
		return proposalAnswer(action), []int64{action.ID}, nil
	case strings.Contains(normalized, "архив"):
		all := strings.Contains(normalized, "все") || strings.Contains(normalized, "всех")
		query := fallbackProductTerm(prompt)
		arguments, err := json.Marshal(archiveProductArgs{Query: query, All: all})
		if err != nil {
			return "", nil, err
		}
		action, err := s.Propose(ctx, conversationID, actor, "archive_product", string(arguments))
		if err != nil {
			return fmt.Sprintf("Не удалось подготовить архивирование: %s. Уточните продукт.", err.Error()), nil, nil
		}
		return proposalAnswer(action), []int64{action.ID}, nil
	case strings.Contains(normalized, "категор"):
		category := afterKeyword(prompt, "категорию")
		if category == "" {
			category = afterKeyword(prompt, "категорию")
		}
		if category == "" {
			category = afterKeyword(prompt, "категория")
		}
		query := fallbackProductTerm(prompt)
		all := strings.Contains(normalized, "все") || strings.Contains(normalized, "всех")
		arguments, err := json.Marshal(setCategoryArgs{Query: query, All: all, Category: category})
		if err != nil {
			return "", nil, err
		}
		action, err := s.Propose(ctx, conversationID, actor, "set_category", string(arguments))
		if err != nil {
			return fmt.Sprintf("Не удалось подготовить смену категории: %s. Укажите продукт и категорию.", err.Error()), nil, nil
		}
		return proposalAnswer(action), []int64{action.ID}, nil
	case strings.Contains(normalized, "созда") && strings.Contains(normalized, "продукт"):
		name := afterKeyword(prompt, "продукт")
		if name == "" {
			name = afterKeyword(prompt, "product")
		}
		arguments, err := json.Marshal(createProductArgs{Name: name, Status: "active"})
		if err != nil {
			return "", nil, err
		}
		action, err := s.Propose(ctx, conversationID, actor, "create_product", string(arguments))
		if err != nil {
			return fmt.Sprintf("Не удалось подготовить создание продукта: %s.", err.Error()), nil, nil
		}
		return proposalAnswer(action), []int64{action.ID}, nil
	case strings.Contains(normalized, "статус") || strings.Contains(normalized, "переведи") || strings.Contains(normalized, "смени"):
		status := fallbackStatus(normalized)
		query := fallbackProductTerm(prompt)
		all := strings.Contains(normalized, "все") || strings.Contains(normalized, "всех")
		arguments, err := json.Marshal(setStatusArgs{Query: query, All: all, Status: status})
		if err != nil {
			return "", nil, err
		}
		action, err := s.Propose(ctx, conversationID, actor, "set_status", string(arguments))
		if err != nil {
			return fmt.Sprintf("Не удалось подготовить смену статуса: %s. Уточните продукт.", err.Error()), nil, nil
		}
		return proposalAnswer(action), []int64{action.ID}, nil
	case strings.Contains(normalized, "лицензи"):
		query := fallbackProductTerm(prompt)
		var seats *int64
		if match := seatPattern.FindStringSubmatch(normalized); len(match) == 2 {
			var value int64
			if _, err := fmt.Sscanf(match[1], "%d", &value); err == nil {
				seats = &value
			}
		}
		if seats == nil {
			return "Укажите количество мест для изменения лицензии, например: «поставь 25 мест для Google Chrome».", nil, nil
		}
		arguments, err := json.Marshal(updateLicenseArgs{Product: query, Seats: seats})
		if err != nil {
			return "", nil, err
		}
		action, err := s.Propose(ctx, conversationID, actor, "update_license", string(arguments))
		if err != nil {
			return fmt.Sprintf("Не удалось подготовить изменение лицензии: %s. Уточните продукт.", err.Error()), nil, nil
		}
		return proposalAnswer(action), []int64{action.ID}, nil
	default:
		return "Не понял, какое изменение нужно. Примеры: «сопоставь все хромы с Google Chrome», «архивируй Notepad++», «переведи Slack в неактивный статус», «поставь 25 мест для Google Chrome».", nil, nil
	}
}
