package {{pkg}}

import "time"

// {{Type}} — ответ API. Поля и теги здесь — единственный источник контракта:
// клиент получает их через OpenAPI (`bun run gen:api`).
type {{Type}} struct {
	ID        int64     `json:"id" doc:"Идентификатор"`
	CreatedAt time.Time `json:"createdAt" doc:"Когда создан"`
}

type get{{Type}}Input struct {
	ID int64 `path:"id" minimum:"1" doc:"Идентификатор"`
}

type {{type}}Output struct {
	Body {{Type}}
}
