package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/database"
)

type knowledgeFactStore interface {
	ListKnowledgeFacts(context.Context, catalog.KnowledgeQuery) (catalog.KnowledgeFactPage, error)
}

type knowledgeRelationStore interface {
	ListKnowledgeRelations(context.Context, catalog.KnowledgeQuery) (catalog.KnowledgeRelationPage, error)
}

type knowledgeExtractionStore interface {
	ExtractKnowledge(context.Context, int) (catalog.KnowledgeExtractionResult, error)
}

type knowledgeConversationStore interface {
	EnsureKnowledgeConversation(context.Context, string, string) (catalog.KnowledgeConversation, error)
	AddKnowledgeMessage(context.Context, catalog.KnowledgeMessage) (catalog.KnowledgeMessage, error)
	ListKnowledgeMessages(context.Context, string, int) ([]catalog.KnowledgeMessage, error)
}

type knowledgeReadOnlySQLStore interface {
	ReadOnlySearchQuery(context.Context, string, int) (database.ReadOnlyQueryResult, error)
}

type knowledgeMediaResult struct {
	Asset       catalog.Asset `json:"asset"`
	Matched     []string      `json:"matched"`
	Explanation string        `json:"explanation"`
}

type knowledgeChatResponse struct {
	ConversationID string                         `json:"conversation_id,omitempty"`
	Answer         string                         `json:"answer"`
	Planner        searchPlan                     `json:"planner"`
	ToolCalls      []map[string]any               `json:"tool_calls"`
	Media          []knowledgeMediaResult         `json:"media"`
	Facts          []catalog.KnowledgeFact        `json:"facts"`
	Relations      []catalog.KnowledgeRelation    `json:"relations"`
	SQLResults     []database.ReadOnlyQueryResult `json:"sql_results,omitempty"`
	Messages       []catalog.KnowledgeMessage     `json:"messages,omitempty"`
	Limit          int                            `json:"limit"`
	LLMStatus      string                         `json:"llm_status"`
	Note           string                         `json:"note"`
}

type knowledgeToolRequest struct {
	Tool      string `json:"tool"`
	Query     string `json:"query,omitempty"`
	Predicate string `json:"predicate,omitempty"`
	Relation  string `json:"relation,omitempty"`
	SQL       string `json:"sql,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type knowledgeLLMStatus struct {
	Mode       string   `json:"mode"`
	Provider   string   `json:"provider"`
	Endpoint   string   `json:"endpoint"`
	Model      string   `json:"model"`
	Configured bool     `json:"configured"`
	Reachable  bool     `json:"reachable"`
	Models     []string `json:"models,omitempty"`
	Error      string   `json:"error,omitempty"`
	Note       string   `json:"note"`
}

func (s *Server) handleKnowledgeFacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	store, ok := s.deps.Store.(knowledgeFactStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("knowledge facts require the PostgreSQL store"))
		return
	}
	query := knowledgeQueryFromRequest(r)
	page, err := store.ListKnowledgeFacts(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleKnowledgeRelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	store, ok := s.deps.Store.(knowledgeRelationStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("knowledge graph requires the PostgreSQL store"))
		return
	}
	query := knowledgeQueryFromRequest(r)
	page, err := store.ListKnowledgeRelations(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleKnowledgeExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "knowledge:extract") {
		return
	}
	limit := intQuery(r.URL.Query(), "limit", 1000, 1, 5000)
	var payload struct {
		Limit int `json:"limit"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	if payload.Limit > 0 {
		limit = payload.Limit
		if limit > 5000 {
			limit = 5000
		}
	}
	store, ok := s.deps.Store.(knowledgeExtractionStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("knowledge extraction requires the PostgreSQL store"))
		return
	}
	result, err := store.ExtractKnowledge(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleKnowledgeChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "knowledge:chat") {
		return
	}
	var payload struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
		Limit          int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload.Message = strings.TrimSpace(payload.Message)
	if payload.Message == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("message is required"))
		return
	}
	if payload.Limit <= 0 || payload.Limit > 100 {
		payload.Limit = 25
	}
	factStore, factsOK := s.deps.Store.(knowledgeFactStore)
	relationStore, relationsOK := s.deps.Store.(knowledgeRelationStore)
	if !factsOK || !relationsOK {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("knowledge chat requires the PostgreSQL store"))
		return
	}
	convStore, convOK := s.deps.Store.(knowledgeConversationStore)
	conversationID := payload.ConversationID
	if convOK {
		conversation, err := convStore.EnsureKnowledgeConversation(r.Context(), conversationID, chatTitle(payload.Message))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		conversationID = conversation.ID
		_, _ = convStore.AddKnowledgeMessage(r.Context(), catalog.KnowledgeMessage{
			ConversationID: conversationID,
			Role:           "user",
			Content:        payload.Message,
		})
	}
	response, err := s.runKnowledgeChatTools(r.Context(), payload.Message, payload.Limit, factStore, relationStore)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response.ConversationID = conversationID
	if convOK {
		assistantMessage, err := convStore.AddKnowledgeMessage(r.Context(), catalog.KnowledgeMessage{
			ConversationID: conversationID,
			Role:           "assistant",
			Content:        response.Answer,
			ToolCalls:      response.ToolCalls,
		})
		if err == nil {
			response.Messages, _ = convStore.ListKnowledgeMessages(r.Context(), conversationID, 20)
			if len(response.Messages) == 0 {
				response.Messages = []catalog.KnowledgeMessage{assistantMessage}
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleKnowledgeLLMStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	status := s.localLLMStatus(r.Context())
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) runKnowledgeChatTools(ctx context.Context, message string, limit int, factStore knowledgeFactStore, relationStore knowledgeRelationStore) (knowledgeChatResponse, error) {
	plan := s.buildNaturalLanguageSearchPlan(message)
	terms := knowledgeChatTerms(message, plan)
	facts := []catalog.KnowledgeFact{}
	relations := []catalog.KnowledgeRelation{}
	media := []knowledgeMediaResult{}
	sqlResults := []database.ReadOnlyQueryResult{}
	seenFacts := map[string]struct{}{}
	seenRelations := map[string]struct{}{}
	seenMedia := map[string]struct{}{}
	toolCalls := []map[string]any{
		{"tool": "search_plan", "query": message, "executable_query": plan.Executable, "tokens": plan.Tokens},
	}
	addFacts := func(items []catalog.KnowledgeFact) {
		for _, fact := range items {
			if _, ok := seenFacts[fact.ID]; ok {
				continue
			}
			facts = append(facts, fact)
			seenFacts[fact.ID] = struct{}{}
			if len(facts) >= limit {
				break
			}
		}
	}
	addRelations := func(items []catalog.KnowledgeRelation) {
		for _, relation := range items {
			if _, ok := seenRelations[relation.ID]; ok {
				continue
			}
			relations = append(relations, relation)
			seenRelations[relation.ID] = struct{}{}
			if len(relations) >= limit {
				break
			}
		}
	}
	addMedia := func(items []knowledgeMediaResult) {
		for _, item := range items {
			if _, ok := seenMedia[item.Asset.ID]; ok {
				continue
			}
			media = append(media, item)
			seenMedia[item.Asset.ID] = struct{}{}
			if len(media) >= limit {
				break
			}
		}
	}
	perTermLimit := limit
	if len(terms) > 1 {
		perTermLimit = max(5, limit/len(terms))
	}
	if strings.TrimSpace(plan.Executable) != "" {
		mediaResults, total, err := s.searchKnowledgeMedia(ctx, plan, min(limit, 25))
		if err != nil {
			toolCalls = append(toolCalls, map[string]any{"tool": "search_media", "query": plan.Executable, "error": err.Error()})
		} else {
			addMedia(mediaResults)
			toolCalls = append(toolCalls, map[string]any{"tool": "search_media", "query": plan.Executable, "returned": len(mediaResults), "total": total})
		}
	}
	for _, term := range terms {
		factPage, err := factStore.ListKnowledgeFacts(ctx, catalog.KnowledgeQuery{Q: term, Limit: perTermLimit})
		if err != nil {
			return knowledgeChatResponse{}, err
		}
		relationPage, err := relationStore.ListKnowledgeRelations(ctx, catalog.KnowledgeQuery{Q: term, Limit: perTermLimit})
		if err != nil {
			return knowledgeChatResponse{}, err
		}
		toolCalls = append(toolCalls,
			map[string]any{"tool": "knowledge_fact_search", "query": term, "returned": len(factPage.Facts), "total": factPage.Page.Total},
			map[string]any{"tool": "knowledge_relation_search", "query": term, "returned": len(relationPage.Relations), "total": relationPage.Page.Total},
		)
		addFacts(factPage.Facts)
		addRelations(relationPage.Relations)
	}
	llmPlanningStatus := "not_used"
	if runtimeStringSetting("knowledge.runner_mode", "deterministic") == "local_llm" {
		requests, status, err := s.planKnowledgeToolsWithLocalLLM(ctx, message, plan)
		llmPlanningStatus = status
		toolCalls = append(toolCalls, map[string]any{"tool": "local_llm_tool_planner", "status": status, "requests": requests, "error": errorString(err)})
		if err == nil {
			for _, request := range requests {
				switch request.Tool {
				case "search_media":
					queryPlan := s.buildNaturalLanguageSearchPlan(firstNonEmpty(request.Query, message))
					results, total, err := s.searchKnowledgeMedia(ctx, queryPlan, clampKnowledgeLimit(request.Limit, min(limit, 25)))
					if err != nil {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "error": err.Error()})
						continue
					}
					addMedia(results)
					toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "returned": len(results), "total": total})
				case "search_facts":
					page, err := factStore.ListKnowledgeFacts(ctx, catalog.KnowledgeQuery{Q: request.Query, Predicate: request.Predicate, Limit: clampKnowledgeLimit(request.Limit, perTermLimit)})
					if err != nil {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "error": err.Error()})
						continue
					}
					addFacts(page.Facts)
					toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "predicate": request.Predicate, "returned": len(page.Facts), "total": page.Page.Total})
				case "search_relations":
					page, err := relationStore.ListKnowledgeRelations(ctx, catalog.KnowledgeQuery{Q: request.Query, Relation: request.Relation, Limit: clampKnowledgeLimit(request.Limit, perTermLimit)})
					if err != nil {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "error": err.Error()})
						continue
					}
					addRelations(page.Relations)
					toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "relation": request.Relation, "returned": len(page.Relations), "total": page.Page.Total})
				case "readonly_sql":
					sqlStore, ok := s.deps.Store.(knowledgeReadOnlySQLStore)
					if !ok {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "sql_sha256": sqlDigest(request.SQL), "error": "read-only SQL is available only with PostgreSQL"})
						continue
					}
					result, err := sqlStore.ReadOnlySearchQuery(ctx, request.SQL, clampKnowledgeLimit(request.Limit, 50))
					if err != nil {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "sql_sha256": sqlDigest(request.SQL), "error": err.Error()})
						continue
					}
					sqlResults = append(sqlResults, result)
					toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "sql_sha256": sqlDigest(result.SQL), "views": result.Views, "returned": result.Count})
				}
			}
		}
	}
	answer := buildKnowledgeAnswer(message, media, facts, relations, sqlResults)
	llmStatus := "not_used"
	note := "Local deterministic tool runner. Configure a local LLM endpoint to synthesize richer answers; remote LLM APIs are not used by default."
	if runtimeStringSetting("knowledge.runner_mode", "deterministic") == "local_llm" {
		llmAnswer, status, llmNote, err := s.synthesizeKnowledgeWithLocalLLM(ctx, message, plan, media, facts, relations, sqlResults)
		llmStatus = status
		note = llmNote
		if llmPlanningStatus != "not_used" {
			note = note + " Tool planner status: " + llmPlanningStatus + "."
		}
		toolCalls = append(toolCalls, map[string]any{"tool": "local_llm_synthesis", "status": status, "error": errorString(err)})
		if err == nil && strings.TrimSpace(llmAnswer) != "" {
			answer = llmAnswer
		} else if err != nil {
			note = note + " Deterministic answer is shown because local LLM synthesis failed: " + err.Error()
		}
	}
	return knowledgeChatResponse{
		Answer:     answer,
		Planner:    plan,
		ToolCalls:  toolCalls,
		Media:      media,
		Facts:      facts,
		Relations:  relations,
		SQLResults: sqlResults,
		Limit:      limit,
		LLMStatus:  llmStatus,
		Note:       note,
	}, nil
}

func (s *Server) synthesizeKnowledgeWithLocalLLM(ctx context.Context, message string, plan searchPlan, media []knowledgeMediaResult, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation, sqlResults []database.ReadOnlyQueryResult) (string, string, string, error) {
	provider := strings.ToLower(strings.TrimSpace(runtimeStringSetting("knowledge.llm_provider", "ollama")))
	endpoint := strings.TrimRight(strings.TrimSpace(runtimeStringSetting("knowledge.llm_endpoint", "http://127.0.0.1:11434")), "/")
	model := strings.TrimSpace(runtimeStringSetting("knowledge.llm_model", ""))
	if model == "" {
		return "", "local_llm_missing_model", "Set knowledge.llm_model in Settings before using local LLM mode.", fmt.Errorf("knowledge.llm_model is empty")
	}
	if endpoint == "" {
		return "", "local_llm_missing_endpoint", "Set knowledge.llm_endpoint in Settings before using local LLM mode.", fmt.Errorf("knowledge.llm_endpoint is empty")
	}
	timeoutSeconds := runtimeIntSetting("knowledge.llm_timeout_seconds", 60)
	if timeoutSeconds < 5 {
		timeoutSeconds = 5
	}
	if timeoutSeconds > 300 {
		timeoutSeconds = 300
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	prompt := knowledgeLLMPrompt(message, plan, media, facts, relations, sqlResults)
	switch provider {
	case "ollama":
		answer, err := postOllamaChat(ctx, endpoint, model, prompt)
		if err != nil {
			return "", "local_llm_error", "Ollama-compatible local LLM failed; deterministic answer is still available.", err
		}
		return answer, "local_llm_ollama", "Answer synthesized by local Ollama-compatible endpoint from read-only tool results.", nil
	case "openai_compatible", "vllm", "openai-compatible":
		answer, err := postOpenAICompatibleChat(ctx, endpoint, model, prompt)
		if err != nil {
			return "", "local_llm_error", "OpenAI-compatible local LLM failed; deterministic answer is still available.", err
		}
		return answer, "local_llm_openai_compatible", "Answer synthesized by local OpenAI-compatible endpoint from read-only tool results.", nil
	default:
		return "", "local_llm_bad_provider", "Unsupported local LLM provider. Use ollama or openai_compatible.", fmt.Errorf("unsupported local LLM provider %q", provider)
	}
}

func (s *Server) planKnowledgeToolsWithLocalLLM(ctx context.Context, message string, plan searchPlan) ([]knowledgeToolRequest, string, error) {
	provider := strings.ToLower(strings.TrimSpace(runtimeStringSetting("knowledge.llm_provider", "ollama")))
	endpoint := strings.TrimRight(strings.TrimSpace(runtimeStringSetting("knowledge.llm_endpoint", "http://127.0.0.1:11434")), "/")
	model := strings.TrimSpace(runtimeStringSetting("knowledge.llm_model", ""))
	if model == "" {
		return nil, "local_llm_missing_model", fmt.Errorf("knowledge.llm_model is empty")
	}
	if endpoint == "" {
		return nil, "local_llm_missing_endpoint", fmt.Errorf("knowledge.llm_endpoint is empty")
	}
	timeoutSeconds := runtimeIntSetting("knowledge.llm_timeout_seconds", 60)
	if timeoutSeconds < 5 {
		timeoutSeconds = 5
	}
	if timeoutSeconds > 60 {
		timeoutSeconds = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	prompt := knowledgeToolPlanPrompt(message, plan)
	var answer string
	var err error
	switch provider {
	case "ollama":
		answer, err = postOllamaChat(ctx, endpoint, model, prompt)
	case "openai_compatible", "vllm", "openai-compatible":
		answer, err = postOpenAICompatibleChat(ctx, endpoint, model, prompt)
	default:
		return nil, "local_llm_bad_provider", fmt.Errorf("unsupported local LLM provider %q", provider)
	}
	if err != nil {
		return nil, "local_llm_planner_error", err
	}
	requests, err := parseKnowledgeToolRequests(answer)
	if err != nil {
		return nil, "local_llm_planner_bad_json", err
	}
	if len(requests) == 0 {
		return nil, "local_llm_planner_empty", nil
	}
	return requests, "local_llm_planner_ok", nil
}

func knowledgeLLMPrompt(message string, plan searchPlan, media []knowledgeMediaResult, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation, sqlResults []database.ReadOnlyQueryResult) string {
	maxItems := runtimeIntSetting("knowledge.llm_max_context_items", 24)
	if maxItems < 4 {
		maxItems = 4
	}
	if maxItems > 80 {
		maxItems = 80
	}
	var builder strings.Builder
	builder.WriteString("You are Cartolensia, a local read-only multimedia archive assistant. ")
	builder.WriteString("Answer only from the provided facts and relations. If the evidence is insufficient, say so. ")
	builder.WriteString("Mention relevant asset names or IDs as clickable candidates for the UI. Do not invent external facts.\n\n")
	builder.WriteString("User request:\n")
	builder.WriteString(message)
	builder.WriteString("\n\nParsed safe query:\n")
	builder.WriteString(searchPlanPreview(plan))
	builder.WriteString("\n\nMedia search results:\n")
	for i, result := range media {
		if i >= maxItems {
			builder.WriteString(fmt.Sprintf("- plus %d more media results not shown to the model\n", len(media)-i))
			break
		}
		builder.WriteString(fmt.Sprintf("- asset_id=%s name=%s kind=%s taken_at=%s matched=%s\n",
			result.Asset.ID, compactFactObject(result.Asset.DisplayName), result.Asset.MediaKind, assetTakenAtString(result.Asset), strings.Join(result.Matched, ",")))
	}
	builder.WriteString("\n\nFacts:\n")
	for i, fact := range facts {
		if i >= maxItems {
			builder.WriteString(fmt.Sprintf("- plus %d more facts not shown to the model\n", len(facts)-i))
			break
		}
		builder.WriteString(fmt.Sprintf("- asset=%s subject=%s predicate=%s object=%s evidence=%s\n",
			firstNonEmpty(fact.AssetID, "(none)"), compactFactObject(fact.Subject), fact.Predicate, compactFactObject(fact.Object), compactFactObject(fact.Evidence)))
	}
	builder.WriteString("\nRelations:\n")
	for i, relation := range relations {
		if i >= maxItems {
			builder.WriteString(fmt.Sprintf("- plus %d more relations not shown to the model\n", len(relations)-i))
			break
		}
		builder.WriteString(fmt.Sprintf("- from_asset=%s from=%s relation=%s to_asset=%s to=%s evidence=%s\n",
			firstNonEmpty(relation.FromAssetID, "(none)"), compactFactObject(relation.FromEntity), relation.Relation,
			firstNonEmpty(relation.ToAssetID, "(none)"), compactFactObject(relation.ToEntity), compactFactObject(relation.Evidence)))
	}
	builder.WriteString("\nRead-only SQL tool results:\n")
	for i, result := range sqlResults {
		if i >= 3 {
			builder.WriteString(fmt.Sprintf("- plus %d more SQL result sets not shown to the model\n", len(sqlResults)-i))
			break
		}
		builder.WriteString(fmt.Sprintf("- views=%s rows=%d columns=%s\n", strings.Join(result.Views, ","), result.Count, strings.Join(result.Columns, ",")))
		for rowIdx, row := range result.Rows {
			if rowIdx >= min(8, maxItems) {
				builder.WriteString(fmt.Sprintf("  - plus %d more rows not shown\n", len(result.Rows)-rowIdx))
				break
			}
			rowJSON, _ := json.Marshal(row)
			builder.WriteString("  - ")
			builder.WriteString(compactFactObject(string(rowJSON)))
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func knowledgeToolPlanPrompt(message string, plan searchPlan) string {
	var builder strings.Builder
	builder.WriteString("You are a local Cartolensia archive query planner. Return only compact JSON, no markdown. ")
	builder.WriteString("Allowed tools: search_media, search_facts, search_relations, readonly_sql. ")
	builder.WriteString("Every tool is read-only and bounded. Do not request writes or raw tables. ")
	builder.WriteString("readonly_sql may SELECT only from cartolensia_search_* views.\n")
	builder.WriteString(`JSON shape: {"tools":[{"tool":"search_media","query":"kind:photo 2025-05..2025-08 поезд","limit":12},{"tool":"search_facts","query":"поезд","limit":12},{"tool":"search_relations","query":"поезд","limit":12},{"tool":"readonly_sql","sql":"select asset_id, display_name, media_kind from cartolensia_search_assets where media_kind='photo' limit 20","limit":20}]}`)
	builder.WriteString("\nUser request:\n")
	builder.WriteString(message)
	builder.WriteString("\nDeterministic parsed query:\n")
	builder.WriteString(plan.Executable)
	return builder.String()
}

func parseKnowledgeToolRequests(raw string) ([]knowledgeToolRequest, error) {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("local LLM did not return a JSON object")
	}
	raw = raw[start : end+1]
	var payload struct {
		Tools []knowledgeToolRequest `json:"tools"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	out := make([]knowledgeToolRequest, 0, min(len(payload.Tools), 8))
	for _, request := range payload.Tools {
		request.Tool = strings.TrimSpace(strings.ToLower(request.Tool))
		request.Query = strings.TrimSpace(request.Query)
		request.Predicate = strings.TrimSpace(request.Predicate)
		request.Relation = strings.TrimSpace(request.Relation)
		request.SQL = strings.TrimSpace(request.SQL)
		request.Limit = clampKnowledgeLimit(request.Limit, 12)
		switch request.Tool {
		case "search_media":
			if request.Query == "" {
				continue
			}
		case "search_facts", "search_relations":
			if request.Query == "" && request.Predicate == "" && request.Relation == "" {
				continue
			}
		case "readonly_sql":
			if request.SQL == "" {
				continue
			}
		default:
			continue
		}
		out = append(out, request)
		if len(out) >= 8 {
			break
		}
	}
	return out, nil
}

func (s *Server) searchKnowledgeMedia(ctx context.Context, plan searchPlan, limit int) ([]knowledgeMediaResult, int, error) {
	limit = clampKnowledgeLimit(limit, 12)
	query := assetQueryFromSearchPlan(plan, limit)
	page, err := s.deps.Store.QueryAssets(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	results := make([]knowledgeMediaResult, 0, len(page.Assets))
	searchCtx := assetSearchContext{}
	for _, asset := range page.Assets {
		matched := assetSearchMatches(asset, plan.Tokens, searchCtx)
		if len(matched) == 0 {
			matched = []string{"bounded media search"}
		}
		results = append(results, knowledgeMediaResult{Asset: asset, Matched: matched, Explanation: searchExplanation(matched)})
	}
	return results, page.Page.Total, nil
}

func assetQueryFromSearchPlan(plan searchPlan, limit int) catalog.AssetQuery {
	query := catalog.AssetQuery{Limit: limit, Sort: "taken_at"}
	freeText := []string{}
	for _, token := range plan.Tokens {
		prefix, plain := splitSearchToken(token)
		plain = strings.TrimSpace(strings.Trim(plain, `"`))
		switch prefix {
		case "kind", "type", "media", "media_kind":
			query.MediaKind = plain
		case "ext", "extension":
			query.Extension = strings.TrimPrefix(plain, ".")
		case "filename", "path":
			freeText = append(freeText, wildcardToLikeText(plain))
		case "":
			if start, end, ok := searchDateRangeBounds(token); ok {
				if !start.IsZero() {
					query.TakenFrom = &start
				}
				if !end.IsZero() {
					query.TakenTo = &end
				}
			} else {
				freeText = append(freeText, wildcardToLikeText(plain))
			}
		}
	}
	if len(freeText) == 0 && strings.TrimSpace(plan.RawQuery) != "" && len(plan.Tokens) == 0 {
		freeText = append(freeText, wildcardToLikeText(plan.RawQuery))
	}
	query.Q = strings.TrimSpace(strings.Join(uniqueStrings(freeText), " "))
	return query
}

func searchDateRangeBounds(token string) (time.Time, time.Time, bool) {
	if strings.Contains(token, "..") {
		startRaw, endRaw, _ := strings.Cut(token, "..")
		start, startOK := parseSearchDateBound(startRaw, false)
		end, endOK := parseSearchDateBound(endRaw, true)
		return start, end, startOK || endOK
	}
	start, startOK := parseSearchDateBound(token, false)
	end, endOK := parseSearchDateBound(token, true)
	return start, end, startOK || endOK
}

func assetTakenAtString(asset catalog.Asset) string {
	if asset.TakenAt == nil {
		return ""
	}
	return asset.TakenAt.Format(time.RFC3339)
}

func clampKnowledgeLimit(value, fallback int) int {
	if fallback <= 0 {
		fallback = 12
	}
	if value <= 0 {
		value = fallback
	}
	if value < 1 {
		value = 1
	}
	if value > 50 {
		value = 50
	}
	return value
}

func sqlDigest(sql string) string {
	sum := sha256.Sum256([]byte(sql))
	return fmt.Sprintf("%x", sum[:8])
}

func postOllamaChat(ctx context.Context, endpoint, model, prompt string) (string, error) {
	keepAlive := runtimeIntSetting("knowledge.llm_idle_unload_minutes", 5)
	if keepAlive < 0 {
		keepAlive = 0
	}
	payload := map[string]any{
		"model":      model,
		"stream":     false,
		"keep_alive": fmt.Sprintf("%dm", keepAlive),
		"options": map[string]any{
			"temperature": 0.1,
			"top_p":       0.8,
		},
		"messages": []map[string]string{
			{"role": "system", "content": "You answer archive questions using only provided local evidence."},
			{"role": "user", "content": prompt},
		},
	}
	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := postLocalLLMJSON(ctx, localLLMURL(endpoint, "/api/chat"), payload, &response); err != nil {
		return "", err
	}
	if response.Error != "" {
		return "", fmt.Errorf("%s", response.Error)
	}
	return strings.TrimSpace(response.Message.Content), nil
}

func postOpenAICompatibleChat(ctx context.Context, endpoint, model, prompt string) (string, error) {
	payload := map[string]any{
		"model":       model,
		"temperature": 0.1,
		"max_tokens":  1200,
		"messages": []map[string]string{
			{"role": "system", "content": "You answer archive questions using only provided local evidence."},
			{"role": "user", "content": prompt},
		},
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := postLocalLLMJSON(ctx, localLLMURL(endpoint, "/v1/chat/completions"), payload, &response); err != nil {
		return "", err
	}
	if response.Error != nil {
		return "", fmt.Errorf("local LLM returned error: %v", response.Error)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("local LLM returned no choices")
	}
	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

func localLLMURL(endpoint, path string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return path
	}
	if strings.HasPrefix(path, "/v1/") && strings.HasSuffix(endpoint, "/v1") {
		return endpoint + strings.TrimPrefix(path, "/v1")
	}
	return endpoint + path
}

func (s *Server) localLLMStatus(ctx context.Context) knowledgeLLMStatus {
	mode := runtimeStringSetting("knowledge.runner_mode", "deterministic")
	provider := strings.ToLower(strings.TrimSpace(runtimeStringSetting("knowledge.llm_provider", "ollama")))
	endpoint := strings.TrimRight(strings.TrimSpace(runtimeStringSetting("knowledge.llm_endpoint", "http://127.0.0.1:11434")), "/")
	model := strings.TrimSpace(runtimeStringSetting("knowledge.llm_model", ""))
	status := knowledgeLLMStatus{
		Mode:       mode,
		Provider:   provider,
		Endpoint:   endpoint,
		Model:      model,
		Configured: mode == "local_llm" && endpoint != "" && model != "",
		Note:       "Local LLM mode uses only bounded read-only tool results. Remote LLM APIs are not used by default.",
	}
	if endpoint == "" {
		status.Error = "knowledge.llm_endpoint is empty"
		return status
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	switch provider {
	case "ollama":
		models, err := getOllamaModels(ctx, endpoint)
		status.Models = models
		status.Reachable = err == nil
		if err != nil {
			status.Error = err.Error()
		}
	case "openai_compatible", "vllm", "openai-compatible":
		models, err := getOpenAICompatibleModels(ctx, endpoint)
		status.Models = models
		status.Reachable = err == nil
		if err != nil {
			status.Error = err.Error()
		}
	default:
		status.Error = fmt.Sprintf("unsupported provider %q", provider)
	}
	return status
}

func getOllamaModels(ctx context.Context, endpoint string) ([]string, error) {
	var response struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := getLocalLLMJSON(ctx, localLLMURL(endpoint, "/api/tags"), &response); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(response.Models))
	for _, model := range response.Models {
		if model.Name != "" {
			models = append(models, model.Name)
		}
	}
	return models, nil
}

func getOpenAICompatibleModels(ctx context.Context, endpoint string) ([]string, error) {
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getLocalLLMJSON(ctx, localLLMURL(endpoint, "/v1/models"), &response); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(response.Data))
	for _, model := range response.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	return models, nil
}

func getLocalLLMJSON(ctx context.Context, rawURL string, out any) error {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("local LLM HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return json.Unmarshal(responseBody, out)
}

func postLocalLLMJSON(ctx context.Context, url string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("local LLM HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return err
	}
	return nil
}

func knowledgeQueryFromRequest(r *http.Request) catalog.KnowledgeQuery {
	query := r.URL.Query()
	return catalog.KnowledgeQuery{
		Q:          strings.TrimSpace(query.Get("q")),
		AssetID:    strings.TrimSpace(query.Get("asset_id")),
		SourceKind: strings.TrimSpace(query.Get("source_kind")),
		Predicate:  strings.TrimSpace(query.Get("predicate")),
		Relation:   strings.TrimSpace(query.Get("relation")),
		Limit:      intQuery(query, "limit", 50, 1, 500),
		Offset:     intQuery(query, "offset", 0, 0, 1_000_000),
	}
}

func knowledgeChatTerms(message string, plan searchPlan) []string {
	terms := []string{}
	add := func(value string) {
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if len([]rune(value)) < 2 {
			return
		}
		lower := strings.ToLower(value)
		switch lower {
		case "kind", "photo", "photos", "video", "audio", "track", "find", "show", "покажи", "найди", "фото", "видео":
			return
		}
		for _, existing := range terms {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		terms = append(terms, value)
	}
	for _, clause := range plan.Clauses {
		if clause.Field == "kind" || clause.Field == "ext" || clause.Field == "safety" || clause.Field == "private" {
			continue
		}
		add(clause.Value)
	}
	for _, word := range significantSearchWords(message) {
		add(word)
		if len(terms) >= 5 {
			break
		}
	}
	if len(terms) == 0 {
		add(message)
	}
	if len(terms) > 5 {
		terms = terms[:5]
	}
	return terms
}

func buildKnowledgeAnswer(message string, media []knowledgeMediaResult, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation, sqlResults []database.ReadOnlyQueryResult) string {
	var builder strings.Builder
	builder.WriteString("I searched the local Knowledge Base and Knowledge Graph for: ")
	builder.WriteString(message)
	builder.WriteString("\n\n")
	if len(media) == 0 && len(facts) == 0 && len(relations) == 0 && len(sqlResults) == 0 {
		builder.WriteString("No mined facts or relations matched yet. Run knowledge extraction after OCR, transcripts, captions, and metadata jobs have produced more local metadata.")
		return builder.String()
	}
	if len(media) > 0 {
		builder.WriteString("Relevant media:\n")
		for i, result := range media {
			if i >= 8 {
				builder.WriteString(fmt.Sprintf("- plus %d more media results\n", len(media)-i))
				break
			}
			builder.WriteString(fmt.Sprintf("- %s (%s, %s) — %s\n", result.Asset.DisplayName, result.Asset.MediaKind, assetTakenAtString(result.Asset), result.Explanation))
		}
	}
	if len(facts) > 0 {
		builder.WriteString("Relevant facts:\n")
		for i, fact := range facts {
			if i >= 8 {
				builder.WriteString(fmt.Sprintf("- plus %d more facts\n", len(facts)-i))
				break
			}
			builder.WriteString(fmt.Sprintf("- %s %s %s", fact.Subject, fact.Predicate, compactFactObject(fact.Object)))
			if fact.Evidence != "" {
				builder.WriteString(" (")
				builder.WriteString(compactFactObject(fact.Evidence))
				builder.WriteString(")")
			}
			builder.WriteString("\n")
		}
	}
	if len(relations) > 0 {
		builder.WriteString("\nRelevant relations:\n")
		for i, relation := range relations {
			if i >= 8 {
				builder.WriteString(fmt.Sprintf("- plus %d more relations\n", len(relations)-i))
				break
			}
			left := firstNonEmpty(relation.FromEntity, relation.FromAssetID)
			right := firstNonEmpty(relation.ToEntity, relation.ToAssetID)
			builder.WriteString(fmt.Sprintf("- %s %s %s", left, relation.Relation, right))
			if relation.Evidence != "" {
				builder.WriteString(" (")
				builder.WriteString(compactFactObject(relation.Evidence))
				builder.WriteString(")")
			}
			builder.WriteString("\n")
		}
	}
	if len(sqlResults) > 0 {
		builder.WriteString("\nRead-only query summaries:\n")
		for _, result := range sqlResults {
			builder.WriteString(fmt.Sprintf("- %d rows from %s\n", result.Count, strings.Join(result.Views, ",")))
		}
	}
	builder.WriteString("\nThis answer is tool-grounded only; extracted facts can contain OCR/AI errors and should remain reviewable.")
	return builder.String()
}

func compactFactObject(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return value
}

func chatTitle(message string) string {
	message = compactFactObject(message)
	runes := []rune(message)
	if len(runes) > 80 {
		return string(runes[:80])
	}
	return message
}
