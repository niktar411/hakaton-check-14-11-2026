package assistant

import (
	"context"
	"fmt"
	"strings"

	db "hackathon/internal/db/sqlc"
)

var wordsPattern = mustWordPattern()

func mustWordPattern() *strings.Replacer {
	replacer := strings.NewReplacer(
		"ё", "е", "Ё", "Е",
		",", " ", ".", " ", "!", " ", "?", " ", ";", " ", ":", " ",
		"(", " ", ")", " ", "\"", " ", "'", " ", "«", " ", "»", " ",
		"—", " ", "–", " ", "-", " ", "\n", " ", "\t", " ",
	)
	return replacer
}

var stopWords = map[string]bool{
	"а": true, "без": true, "бы": true, "в": true, "во": true, "все": true, "всех": true,
	"вы": true, "где": true, "да": true, "для": true, "до": true, "его": true, "ее": true,
	"если": true, "есть": true, "еще": true, "же": true, "за": true, "здесь": true,
	"и": true, "из": true, "или": true, "им": true, "их": true, "к": true, "как": true,
	"какие": true, "каких": true, "ко": true, "когда": true, "кто": true, "ли": true,
	"мне": true, "мы": true, "на": true, "над": true, "надо": true, "наш": true,
	"не": true, "нет": true, "но": true, "ну": true, "о": true, "об": true, "от": true,
	"по": true, "под": true, "покажи": true, "после": true, "при": true, "про": true,
	"с": true, "сколько": true, "со": true, "так": true, "там": true, "те": true,
	"то": true, "только": true, "у": true, "уже": true, "что": true, "чтобы": true,
	"это": true, "я": true, "найди": true, "найти": true,
	"список": true, "всё": true, "общий": true, "общая": true,
	"отчет": true, "отчёт": true, "сводка": true, "данные": true, "данных": true,
	"продукт": true, "продукта": true, "продуктов": true, "продукты": true,
	"лицензия": true, "лицензии": true, "лицензий": true, "лицензию": true,
	"место": true, "места": true, "мест": true, "мест.": true, "свободно": true,
	"свободных": true, "осталось": true, "оплачено": true, "оплаченных": true,
	"установка": true, "установки": true, "установок": true, "установлено": true,
	"установлен": true, "устройство": true, "устройств": true, "устройствах": true,
	"компьютер": true, "компьютерах": true, "развернут": true, "развёрнут": true,
	"риск": true, "риски": true, "рисков": true, "проблема": true, "проблемы": true,
	"проблем": true, "опасность": true, "опасности": true,
	"несопоставленные": true, "несопоставленных": true, "несопоставлено": true,
	"привязанные": true, "привязанных": true, "сырые": true, "имена": true, "имен": true,
	"баланс": true, "стоимость": true, "стоимост": true, "затраты": true, "годовая": true,
	"активные": true, "активных": true, "неактивные": true, "неактивных": true,
	"архивные": true, "архивных": true, "статус": true, "статусом": true,
	"show": true, "list": true, "all": true, "top": true, "risks": true, "risk": true,
	"how": true, "many": true, "remaining": true, "seats": true, "left": true, "free": true,
	"product": true, "products": true, "license": true, "licenses": true, "installed": true,
	"installations": true, "unmatched": true, "summary": true, "overview": true,
}

var synonyms = map[string]string{
	"хром": "chrome", "хромы": "chrome", "хрома": "chrome", "хроме": "chrome", "хрому": "chrome",
	"гугл": "google", "гугла": "google", "гугле": "google",
	"майкрософт": "microsoft", "майкрософта": "microsoft",
	"виндовс": "windows", "виндоус": "windows",
	"офис": "office", "офиса": "office",
	"линукс": "linux", "линукса": "linux",
	"зум": "zoom", "слак": "slack", "слака": "slack",
}

func normalizePrompt(prompt string) string {
	return strings.ToLower(strings.Join(strings.Fields(wordsPattern.Replace(prompt)), " "))
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func extractTerm(prompt string) string {
	normalized := normalizePrompt(prompt)
	fields := strings.Fields(normalized)
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, " ")
		if field == "" || stopWords[field] {
			continue
		}
		if replacement, ok := synonyms[field]; ok {
			field = replacement
		}
		parts = append(parts, field)
	}
	return strings.Join(parts, " ")
}

func statusFilter(prompt string) string {
	normalized := normalizePrompt(prompt)
	switch {
	case containsAny(normalized, "архив"):
		return "archived"
	case containsAny(normalized, "неактив"):
		return "inactive"
	case containsAny(normalized, "актив"):
		return "active"
	default:
		return ""
	}
}

func (s *Service) fallback(ctx context.Context, prompt string) (string, []string, error) {
	normalized := normalizePrompt(prompt)
	switch {
	case containsAny(normalized, "риск", "риски", "risk", "проблем", "опасн"):
		rows, err := s.store.SamTopRisks(ctx, 10)
		if err != nil {
			return "", nil, storageError(ctx, "load SAM top risks", err)
		}
		if len(rows) == 0 {
			return "Рисков не найдено: в SAM нет несопоставленных установок, истёкших лицензий и перерасхода мест.\nИсточник: sam_risk_overview (top_risks).", []string{"sam_risk_overview (top_risks)"}, nil
		}
		var answer strings.Builder
		answer.WriteString("Главные риски SAM:\n")
		for _, row := range rows {
			fmt.Fprintf(&answer, "- %s: %d строк (важность %s).\n", row.Detail, row.Affected, row.Severity)
		}
		answer.WriteString("Источник: sam_risk_overview (top_risks).")
		return answer.String(), []string{"sam_risk_overview (top_risks)"}, nil
	case containsAny(normalized, "несопостав", "unmatched", "без продукт", "не привязан", "сырые имен"):
		rows, err := s.store.SamUnmatchedNames(ctx, 20)
		if err != nil {
			return "", nil, storageError(ctx, "load SAM unmatched names", err)
		}
		if len(rows) == 0 {
			return "Несопоставленных установок нет.\nИсточник: sam_installations (unmatched_names).", []string{"sam_installations (unmatched_names)"}, nil
		}
		var total int64
		var answer strings.Builder
		answer.WriteString("Несопоставленные имена установок:\n")
		for _, row := range rows {
			total += row.Installations
			fmt.Fprintf(&answer, "- %s — %d установок на %d устройствах.\n", row.RawName, row.Installations, row.Devices)
		}
		fmt.Fprintf(&answer, "Всего в выборке: %d установок.\nИсточник: sam_installations (unmatched_names).", total)
		return answer.String(), []string{"sam_installations (unmatched_names)"}, nil
	case containsAny(normalized, "лиценз", "license", "мест", "баланс", "стоимост", "затрат", "истек", "expires"):
		term := extractTerm(prompt)
		if term != "" {
			rows, err := s.store.SamProductBalances(ctx, db.SamProductBalancesParams{Query: term, Limit: 5})
			if err != nil {
				return "", nil, storageError(ctx, "load SAM product balance", err)
			}
			if len(rows) > 0 {
				var answer strings.Builder
				fmt.Fprintf(&answer, "Баланс мест по запросу «%s»:\n", term)
				for _, row := range rows {
					fmt.Fprintf(&answer, "- %s: оплачено %d мест, установлено %d, свободно %d.", row.Name, row.LicensedSeats, row.Installed, row.AvailableSeats)
					if expiry := valueString(row.NearestExpiry); expiry != "" {
						fmt.Fprintf(&answer, " Ближайшее истечение: %s.", expiry)
					}
					answer.WriteString("\n")
				}
				answer.WriteString("Источник: sam_products/sam_licenses/sam_installations (product_balance).")
				return answer.String(), []string{"sam_products/sam_licenses/sam_installations (product_balance)"}, nil
			}
		}
		rows, err := s.store.SamLicenseSummary(ctx)
		if err != nil {
			return "", nil, storageError(ctx, "load SAM license summary", err)
		}
		if len(rows) == 0 {
			return "Действующих лицензий нет.\nИсточник: sam_licenses (license_summary).", []string{"sam_licenses (license_summary)"}, nil
		}
		var licenses, seats, expired, cost int64
		var answer strings.Builder
		answer.WriteString("Сводка действующих лицензий:\n")
		for _, row := range rows {
			licenses += row.Licenses
			seats += row.Seats
			expired += row.Expired
			cost += row.AnnualCost
			fmt.Fprintf(&answer, "- %s: %d лицензий, %d мест, истекло %d, стоимость %d RUB/год.\n", row.LicenseType, row.Licenses, row.Seats, row.Expired, row.AnnualCost)
		}
		fmt.Fprintf(&answer, "Итого: %d лицензий, %d мест, %d истёкших, %d RUB/год.\nИсточник: sam_licenses (license_summary).", licenses, seats, expired, cost)
		return answer.String(), []string{"sam_licenses (license_summary)"}, nil
	case containsAny(normalized, "установ", "installation", "устройств", "компьютер", "развернут", "развёрнут", "где стоит"):
		term := extractTerm(prompt)
		productID := int64(0)
		if term != "" {
			products, err := s.store.SearchSamProducts(ctx, db.SearchSamProductsParams{Query: term, Limit: 10})
			if err != nil {
				return "", nil, storageError(ctx, "resolve SAM product", err)
			}
			if len(products) > 0 {
				productID = products[0].ID
				term = products[0].Name
			}
		}
		rows, err := s.store.SamFindInstallations(ctx, db.SamFindInstallationsParams{ProductID: productID, Query: "", Limit: 20})
		if err != nil {
			return "", nil, storageError(ctx, "find SAM installations", err)
		}
		var answer strings.Builder
		fmt.Fprintf(&answer, "Обезличенные установки%s: %d.\n", productSuffix(term), len(rows))
		for _, row := range rows {
			fmt.Fprintf(&answer, "- %s, версия %s, статус %s.\n", row.RawName, row.Version, row.Status)
		}
		answer.WriteString("Источник: sam_installations (find_installations).")
		return answer.String(), []string{"sam_installations (find_installations)"}, nil
	case containsAny(normalized, "найд", "поиск", "search", "список", "list", "покажи", "какие продукт"):
		status := statusFilter(prompt)
		term := extractTerm(prompt)
		if status != "" || term == "" {
			rows, err := s.store.ListSamProducts(ctx, db.ListSamProductsParams{Status: status, Limit: 20})
			if err != nil {
				return "", nil, storageError(ctx, "list SAM products", err)
			}
			var answer strings.Builder
			fmt.Fprintf(&answer, "Продуктов%s: %d.\n", statusSuffix(status), len(rows))
			for _, row := range rows {
				fmt.Fprintf(&answer, "- %s (%s), статус %s.\n", row.Name, row.Vendor, row.Status)
			}
			answer.WriteString("Источник: sam_products (list_products).")
			return answer.String(), []string{"sam_products (list_products)"}, nil
		}
		rows, err := s.store.SearchSamProducts(ctx, db.SearchSamProductsParams{Query: term, Limit: 20})
		if err != nil {
			return "", nil, storageError(ctx, "search SAM products", err)
		}
		var answer strings.Builder
		fmt.Fprintf(&answer, "Найдено продуктов по запросу «%s»: %d.\n", term, len(rows))
		for _, row := range rows {
			fmt.Fprintf(&answer, "- %s (%s), статус %s.\n", row.Name, row.Vendor, row.Status)
		}
		answer.WriteString("Источник: sam_products/sam_product_aliases (search_products).")
		return answer.String(), []string{"sam_products/sam_product_aliases (search_products)"}, nil
	default:
		row, err := s.store.SamOverview(ctx)
		if err != nil {
			return "", nil, storageError(ctx, "load SAM overview", err)
		}
		answer := fmt.Sprintf(
			"Сводка SAM: продуктов — %d (активных — %d), установок — %d, несопоставленных установок — %d, действующих лицензий — %d на %d мест, истёкших лицензий — %d.\nИсточник: sam_products/sam_installations/sam_licenses (overview).",
			row.Products, row.ActiveProducts, row.Installations, row.UnmatchedInstallations, row.ActiveLicenses, row.LicensedSeats, row.ExpiredLicenses,
		)
		return answer, []string{"sam_products/sam_installations/sam_licenses (overview)"}, nil
	}
}

func productSuffix(term string) string {
	if term == "" {
		return " (все продукты)"
	}
	return " по продукту «" + term + "»"
}

func statusSuffix(status string) string {
	if status == "" {
		return ""
	}
	return " со статусом " + status
}
