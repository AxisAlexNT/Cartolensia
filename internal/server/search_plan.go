package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/AxisAlexNT/Cartolensia/internal/database"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type searchPlanClause struct {
	Boolean  string `json:"boolean"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Token    string `json:"token"`
}

type searchPlan struct {
	RawQuery       string             `json:"raw_query"`
	Executable     string             `json:"executable_query"`
	Tokens         []string           `json:"tokens"`
	Clauses        []searchPlanClause `json:"clauses"`
	Backend        string             `json:"backend"`
	BackendMode    string             `json:"backend_mode"`
	Planner        string             `json:"planner"`
	LLMStatus      string             `json:"llm_status"`
	Warnings       []string           `json:"warnings"`
	Notes          []string           `json:"notes"`
	SafeStructured bool               `json:"safe_structured"`
}

var sqlLikeClauseRE = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_-]*)\s*(=|:|like|contains|~)\s*("[^"]*"|'[^']*'|[^\s,()]+)`)

func (s *Server) handleSearchParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.buildSearchPlan(r.URL.Query().Get("q")))
}

func (s *Server) handleSearchPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	description := strings.TrimSpace(r.URL.Query().Get("q"))
	if r.Method == http.MethodPost {
		var payload struct {
			Description string `json:"description"`
			Query       string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		description = firstNonEmpty(payload.Description, payload.Query, description)
	}
	plan := s.buildNaturalLanguageSearchPlan(description)
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleSearchSQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		SQL   string `json:"sql"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	runner, ok := s.deps.Store.(interface {
		ReadOnlySearchQuery(context.Context, string, int) (database.ReadOnlyQueryResult, error)
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("read-only SQL search is available only with the PostgreSQL store"))
		return
	}
	result, err := runner.ReadOnlySearchQuery(r.Context(), payload.SQL, payload.Limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) buildSearchPlan(raw string) searchPlan {
	raw = strings.TrimSpace(raw)
	backend := s.searchBackend()
	plan := searchPlan{
		RawQuery:       raw,
		Executable:     raw,
		Backend:        backend.ID(),
		BackendMode:    backend.Mode(),
		Planner:        "safe_parser",
		LLMStatus:      "not_used",
		SafeStructured: true,
	}
	if raw == "" {
		plan.Tokens = nil
		plan.Notes = append(plan.Notes, "empty query")
		return plan
	}
	if executable, clauses, ok := parseSQLLikeSearch(raw); ok {
		plan.Executable = executable
		plan.Clauses = clauses
		plan.Tokens = searchTokens(executable)
		plan.Notes = append(plan.Notes, "SQL-like input was translated to Cartolensia search tokens; arbitrary SQL is never executed.")
	} else {
		plan.Tokens = searchTokens(raw)
		plan.Clauses = clausesFromTokens(plan.Tokens)
	}
	plan.Warnings = searchWarnings(plan.Tokens)
	return plan
}

func parseSQLLikeSearch(raw string) (string, []searchPlanClause, bool) {
	matches := sqlLikeClauseRE.FindAllStringSubmatchIndex(raw, -1)
	if len(matches) == 0 {
		return "", nil, false
	}
	clauses := make([]searchPlanClause, 0, len(matches))
	tokens := make([]string, 0, len(matches))
	for idx, match := range matches {
		field := raw[match[2]:match[3]]
		op := raw[match[4]:match[5]]
		value := raw[match[6]:match[7]]
		field = canonicalSearchField(field)
		op = canonicalSearchOperator(op)
		value = trimSearchValue(value)
		if field == "" || value == "" {
			continue
		}
		boolean := "and"
		if idx > 0 {
			between := strings.ToLower(raw[matches[idx-1][1]:match[0]])
			if strings.Contains(between, " or ") || strings.Contains(between, ",") {
				boolean = "or"
			}
		}
		token := field + ":" + value
		if strings.ContainsAny(value, " \t\n") {
			token = field + `:"` + strings.ReplaceAll(value, `"`, "") + `"`
		}
		if field == "text" {
			token = value
		}
		clauses = append(clauses, searchPlanClause{Boolean: boolean, Field: field, Operator: op, Value: value, Token: token})
		tokens = append(tokens, token)
	}
	if len(tokens) == 0 {
		return "", nil, false
	}
	return strings.Join(tokens, " "), clauses, true
}

func clausesFromTokens(tokens []string) []searchPlanClause {
	clauses := make([]searchPlanClause, 0, len(tokens))
	for _, token := range tokens {
		field, value := splitSearchToken(token)
		if field == "" {
			field = "text"
		}
		clauses = append(clauses, searchPlanClause{Boolean: "and", Field: canonicalSearchField(field), Operator: "contains", Value: value, Token: token})
	}
	return clauses
}

func canonicalSearchField(field string) string {
	field = strings.Trim(strings.ToLower(field), " `")
	switch field {
	case "extension":
		return "ext"
	case "type", "media", "media_kind":
		return "kind"
	case "name", "file", "basename":
		return "filename"
	case "text", "content", "any", "all":
		return "text"
	case "video_caption":
		return "video-caption"
	default:
		return field
	}
}

func canonicalSearchOperator(op string) string {
	op = strings.TrimSpace(strings.ToLower(op))
	switch op {
	case ":", "~", "like":
		return "contains"
	default:
		return op
	}
}

func trimSearchValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}

func (s *Server) buildNaturalLanguageSearchPlan(description string) searchPlan {
	description = strings.TrimSpace(description)
	plan := s.buildSearchPlan(description)
	plan.Planner = "rule_based_local"
	plan.LLMStatus = runtimeStringSetting("search.llm_status", "not_configured")
	if description == "" {
		return plan
	}
	lower := strings.ToLower(description)
	var tokens []string
	add := func(token string) {
		token = strings.TrimSpace(strings.ToLower(token))
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	if containsAnyWord(lower, "photo", "photos", "image", "images", "picture", "pictures", "фото", "фотографии", "снимки", "изображения") {
		add("kind:photo")
	}
	if containsAnyWord(lower, "video", "videos", "movie", "movies", "видео", "ролик", "ролики") {
		add("kind:video")
	}
	if containsAnyWord(lower, "audio", "sound", "recording", "voice", "speech", "аудио", "звук", "запись", "голос", "речь") {
		add("kind:audio")
	}
	if containsAnyWord(lower, "track", "gps", "gpx", "kml", "трек", "маршрут", "gps") {
		add("kind:track")
	}
	for _, ext := range storageLikeExtensionsFromText(lower) {
		add("ext:" + ext)
	}
	for _, phrase := range quotedPhrases(description) {
		add(phrase)
	}
	for _, word := range significantSearchWords(description) {
		add(word)
	}
	if len(tokens) == 0 {
		add(description)
	}
	plan.Executable = strings.Join(uniqueStrings(tokens), " ")
	plan.Tokens = searchTokens(plan.Executable)
	plan.Clauses = clausesFromTokens(plan.Tokens)
	plan.Notes = append(plan.Notes,
		"Natural-language planning used a deterministic local English/Russian fallback.",
		"Configure a local LLM endpoint to replace the fallback; remote APIs are not used by default.",
	)
	plan.Warnings = searchWarnings(plan.Tokens)
	return plan
}

func containsAnyWord(text string, words ...string) bool {
	for _, word := range words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word != "" && strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func storageLikeExtensionsFromText(text string) []string {
	var out []string
	for _, ext := range storage.SupportedExtensions() {
		if ext != "" && (strings.Contains(text, "."+ext) || containsAnyWord(" "+text+" ", " "+ext+" ")) {
			out = append(out, ext)
		}
	}
	return uniqueStrings(out)
}

func quotedPhrases(text string) []string {
	var out []string
	var b strings.Builder
	inQuote := false
	for _, r := range text {
		switch r {
		case '"', '“', '”':
			if inQuote && b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			inQuote = !inQuote
		default:
			if inQuote {
				b.WriteRune(r)
			}
		}
	}
	return out
}

func significantSearchWords(text string) []string {
	stop := map[string]struct{}{
		"find": {}, "show": {}, "me": {}, "with": {}, "from": {}, "and": {}, "or": {}, "the": {}, "a": {}, "an": {}, "in": {}, "on": {}, "near": {},
		"найди": {}, "покажи": {}, "мне": {}, "с": {}, "со": {}, "и": {}, "или": {}, "в": {}, "на": {}, "около": {}, "рядом": {}, "где": {},
	}
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == ';' || r == ':' || r == '(' || r == ')' || r == '[' || r == ']'
	})
	var out []string
	for _, field := range fields {
		word := strings.Trim(strings.ToLower(field), `"'!?./\`)
		if len([]rune(word)) < 3 {
			continue
		}
		if _, ok := stop[word]; ok {
			continue
		}
		switch word {
		case "photo", "photos", "image", "images", "video", "videos", "audio", "track", "tracks", "фото", "видео", "аудио", "трек", "маршрут":
			continue
		}
		out = append(out, word)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return uniqueStrings(out)
}

func searchPlanPreview(plan searchPlan) string {
	if len(plan.Clauses) == 0 {
		return plan.Executable
	}
	parts := make([]string, 0, len(plan.Clauses))
	for _, clause := range plan.Clauses {
		parts = append(parts, fmt.Sprintf("%s %s %q", clause.Field, clause.Operator, clause.Value))
	}
	return strings.Join(parts, " AND ")
}
