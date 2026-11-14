package operations

import (
	"encoding/xml"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var titlePattern = regexp.MustCompile(`<title>[^<]*</title>`)
var robotsPattern = regexp.MustCompile(`<meta name="robots" content="[^"]*"/?>`)

type publicPage struct {
	Path        string
	Title       string
	Description string
	Private     bool
}

func (s *Service) publicPages() []publicPage {
	return []publicPage{
		{Path: "/", Title: "Хакатон: продукт команды", Description: "Публичная информация о проекте, ответы DeepSeek и реальный план разработки команды."},
		{Path: "/questions", Title: "Вопросы о проекте", Description: "Ответы DeepSeek по проверяемому контексту проекта и ссылки только на страницы сайта."},
		{Path: "/planner", Title: "План команды", Description: "Публичный план работ с реальным прогрессом Merge Request и CI GitLab."},
		{Path: "/sam", Title: "SAM: обзор", Description: "Сводка по программным активам, лицензиям, инвентаризации и рискам."},
		{Path: "/sam/catalog", Title: "SAM: каталог", Description: "Каталог программных продуктов, лицензий и обнаруженных установок."},
		{Path: "/sam/licenses", Title: "SAM: лицензии", Description: "Сроки действия лицензий и занятые места."},
		{Path: "/sam/inventory", Title: "SAM: инвентаризация", Description: "Устройства, владельцы и обнаруженное программное обеспечение."},
		{Path: "/sam/compliance", Title: "SAM: комплаенс", Description: "Нарушения правил использования программного обеспечения."},
		{Path: "/sam/matching", Title: "SAM: сопоставление", Description: "Предложения сопоставить установки с каталогом продуктов."},
		{Path: "/sam/assistant", Title: "SAM: ассистент", Description: "Рекомендации по текущим данным SAM без изменения записей."},
		{Path: "/coding", Title: "Разработка с DeepSeek", Description: "Закрытая зона участников: локальный вход и разработка с обязательным уточнением каждой задачи.", Private: true},
	}
}

func writePublic(r *http.Request, w http.ResponseWriter, value string) bool {
	if _, err := io.WriteString(w, value); err != nil {
		slog.WarnContext(r.Context(), "write public response failed", "error", err)
		return false
	}
	return true
}

func (s *Service) sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if !writePublic(r, w, `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`) {
		return
	}
	for _, page := range s.publicPages() {
		if page.Private {
			continue
		}
		base := s.config.PublicURL
		if page.Path == "/planner" {
			base = s.config.PlannerURL
		}
		location := base + page.Path
		if page.Path == "/" {
			location = base
		}
		if !writePublic(r, w, "<url><loc>"+xmlEscape(location)+"</loc></url>") {
			return
		}
	}
	writePublic(r, w, "</urlset>")
}

func xmlEscape(value string) string {
	var output strings.Builder
	if err := xml.EscapeText(&output, []byte(value)); err != nil {
		return ""
	}
	return output.String()
}

func (s *Service) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writePublic(r, w, "User-agent: *\nAllow: /\nDisallow: /admin\nDisallow: /coding\nDisallow: /api/\nSitemap: "+s.config.PublicURL+"/sitemap.xml\n")
}

func (s *Service) llms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	text := "# Хакатон\n\n> Публичный сайт проекта и проверяемый план команды.\n\n" +
		"Основная информация, вопросы и план доступны без регистрации. Администрирование и изменения кода требуют локального входа.\n\n" +
		"## Проект\n- [Главная](" + s.config.PublicURL + "): описание проекта и возможностей.\n" +
		"- [Вопросы](" + s.config.PublicURL + "/questions): ответы по редактируемому проверенному контексту.\n" +
		"- [План](" + s.config.PlannerURL + "/planner): этапы работ и время обновления.\n" +
		"- [SAM](" + s.config.PublicURL + "/sam): сводка по активам, лицензиям и рискам.\n" +
		"Каталог, лицензии, инвентаризация, комплаенс, сопоставление и рекомендации SAM доступны в разделах /sam/catalog, /sam/licenses, /sam/inventory, /sam/compliance, /sam/matching и /sam/assistant.\n" +
		"Зона разработки с DeepSeek доступна только участникам команды после локального входа.\n"
	writePublic(r, w, text)
}

func (s *Service) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(s.config.StaticDir, "index.html"))
	if err != nil {
		http.Error(w, "site.unavailable", http.StatusServiceUnavailable)
		return
	}
	page := s.publicPages()[0]
	for _, candidate := range s.publicPages() {
		if candidate.Path == r.URL.Path {
			page = candidate
			break
		}
	}
	content := string(data)
	if strings.HasPrefix(r.URL.Path, "/admin") || r.URL.Path == "/coding" {
		content = robotsPattern.ReplaceAllString(content, `<meta name="robots" content="noindex,nofollow"/>`)
	} else {
		content = robotsPattern.ReplaceAllString(content, `<meta name="robots" content="index,follow"/>`)
	}
	content = titlePattern.ReplaceAllString(content, "<title>"+html.EscapeString(page.Title)+"</title>")
	canonical := s.config.PublicURL + page.Path
	if page.Path == "/planner" {
		canonical = s.config.PlannerURL + page.Path
	}
	metadata := `<meta name="description" content="` + html.EscapeString(page.Description) + `"/><link rel="canonical" href="` + html.EscapeString(canonical) + `"/>`
	content = strings.Replace(content, "</head>", metadata+"</head>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	writePublic(r, w, content)
}
