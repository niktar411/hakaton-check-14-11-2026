package operator

import "hackathon/internal/llm"

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func intSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": 0}
}

func boolSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func intArraySchema(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "integer"}}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "string"}}
}

func tool(name, description string, properties map[string]any, required []string) llm.Tool {
	parameters := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}
	return llm.Tool{Type: "function", Function: llm.Function{Name: name, Description: description, Parameters: parameters}}
}

func toolDefinitions() []llm.Tool {
	return []llm.Tool{
		tool("propose_match_aliases", "Предложить сопоставление несопоставленных установок с продуктом через алиасы. Только preview, без записи в бизнес-таблицы.", map[string]any{
			"product": stringSchema("Существующий продукт SAM, например Google Chrome."),
			"aliases": stringArraySchema("Необязательные исходные имена или их части. Если пусто, сервер сам ищет похожие несопоставленные имена."),
			"all":     boolSchema("Отметить, что пользователь просит обработать все найденные совпадения. Потребует контрольную фразу."),
		}, []string{"product"}),
		tool("propose_set_status", "Предложить смену статуса продуктов SAM. Только preview.", map[string]any{
			"productIds": intArraySchema("ID продуктов, если известны."),
			"query":      stringSchema("Название или часть названия продукта."),
			"all":        boolSchema("Применить ко всем продуктам. Потребует контрольную фразу."),
			"status":     map[string]any{"type": "string", "enum": []string{"active", "inactive", "archived", "banned"}, "description": "Новый статус."},
		}, []string{"status"}),
		tool("propose_set_category", "Предложить смену категории продуктов SAM. Только preview.", map[string]any{
			"productIds": intArraySchema("ID продуктов, если известны."),
			"query":      stringSchema("Название или часть названия продукта."),
			"all":        boolSchema("Применить ко всем продуктам. Потребует контрольную фразу."),
			"category":   stringSchema("Новая категория, например Браузеры."),
		}, []string{"category"}),
		tool("propose_update_license", "Предложить изменение лицензии: тип, места, стоимость, даты или статус. Неуказанные поля сохраняются. Только preview.", map[string]any{
			"licenseIds":  intArraySchema("ID лицензий, если известны."),
			"product":     stringSchema("Название продукта, если меняются все его лицензии."),
			"licenseType": map[string]any{"type": "string", "enum": []string{"subscription", "perpetual", "open_source", "trial"}},
			"seats":       intSchema("Количество мест."),
			"costPerSeat": intSchema("Стоимость места в рублях за год."),
			"currency":    stringSchema("Валюта, обычно RUB."),
			"startsAt":    stringSchema("Дата начала в формате YYYY-MM-DD. Пустая строка очищает дату."),
			"expiresAt":   stringSchema("Дата окончания в формате YYYY-MM-DD. Пустая строка очищает дату."),
			"status":      map[string]any{"type": "string", "enum": []string{"active", "expired", "cancelled"}},
			"all":         boolSchema("Изменить все лицензии выбранного продукта. Потребует контрольную фразу."),
		}, nil),
		tool("propose_archive_product", "Предложить архивирование продуктов SAM. Только preview.", map[string]any{
			"productIds": intArraySchema("ID продуктов, если известны."),
			"query":      stringSchema("Название или часть названия продукта."),
			"all":        boolSchema("Архивировать все продукты. Потребует контрольную фразу."),
		}, nil),
		tool("propose_create_product", "Предложить создание нового продукта SAM и необязательных алиасов. Только preview.", map[string]any{
			"name":     stringSchema("Название нового продукта."),
			"vendor":   stringSchema("Вендор."),
			"version":  stringSchema("Версия."),
			"category": stringSchema("Категория."),
			"status":   map[string]any{"type": "string", "enum": []string{"active", "inactive", "archived", "banned"}},
			"aliases":  stringArraySchema("Необязательные алиасы."),
			"all":      boolSchema("Служебный флаг, обычно false."),
		}, []string{"name"}),
	}
}

func toolIntent(name string) (string, bool) {
	switch name {
	case "propose_match_aliases":
		return "match_aliases", true
	case "propose_set_status":
		return "set_status", true
	case "propose_set_category":
		return "set_category", true
	case "propose_update_license":
		return "update_license", true
	case "propose_archive_product":
		return "archive_product", true
	case "propose_create_product":
		return "create_product", true
	default:
		return "", false
	}
}
