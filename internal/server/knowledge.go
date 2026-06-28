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
	"path/filepath"
	"regexp"
	"sort"
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
	Actions        []knowledgeAction              `json:"actions,omitempty"`
	Media          []knowledgeMediaResult         `json:"media"`
	Facts          []catalog.KnowledgeFact        `json:"facts"`
	Relations      []catalog.KnowledgeRelation    `json:"relations"`
	SQLResults     []database.ReadOnlyQueryResult `json:"sql_results,omitempty"`
	Messages       []catalog.KnowledgeMessage     `json:"messages,omitempty"`
	Limit          int                            `json:"limit"`
	LLMStatus      string                         `json:"llm_status"`
	Note           string                         `json:"note"`
}

type knowledgeAction struct {
	Action  string         `json:"action"`
	Label   string         `json:"label"`
	AssetID string         `json:"asset_id,omitempty"`
	Query   string         `json:"query,omitempty"`
	Method  string         `json:"method,omitempty"`
	URL     string         `json:"url,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
	Note    string         `json:"note,omitempty"`
	Details map[string]any `json:"details,omitempty"`
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
	actions := []knowledgeAction{}
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
	addActions := func(items []knowledgeAction) {
		for _, item := range items {
			actions = append(actions, item)
			if len(actions) >= limit {
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
	if knowledgeMessageLooksLikeTranscode(message) {
		results, transcodeActions, err := s.knowledgeTranscodeRecommendations(ctx, firstNonEmpty(plan.Executable, message), min(limit, 8))
		if err != nil {
			toolCalls = append(toolCalls, map[string]any{"tool": "transcode_recommendations", "query": message, "error": err.Error()})
		} else {
			addMedia(results)
			addActions(transcodeActions)
			toolCalls = append(toolCalls, map[string]any{"tool": "transcode_recommendations", "query": message, "returned": len(transcodeActions)})
		}
	}
	if knowledgeMessageLooksLikeSegmentMerge(message) {
		segmentActions, err := s.knowledgeSegmentedSeriesPlans(ctx, message, min(limit, 12))
		if err != nil {
			toolCalls = append(toolCalls, map[string]any{"tool": "find_segmented_video_series", "query": message, "error": err.Error()})
		} else {
			addActions(segmentActions)
			toolCalls = append(toolCalls, map[string]any{"tool": "find_segmented_video_series", "query": message, "returned": len(segmentActions)})
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
				case "transcode_recommendations":
					results, transcodeActions, err := s.knowledgeTranscodeRecommendations(ctx, firstNonEmpty(request.Query, message), clampKnowledgeLimit(request.Limit, min(limit, 8)))
					if err != nil {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "error": err.Error()})
						continue
					}
					addMedia(results)
					addActions(transcodeActions)
					toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "returned": len(transcodeActions)})
				case "find_segmented_video_series":
					segmentActions, err := s.knowledgeSegmentedSeriesPlans(ctx, firstNonEmpty(request.Query, message), clampKnowledgeLimit(request.Limit, min(limit, 12)))
					if err != nil {
						toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "error": err.Error()})
						continue
					}
					addActions(segmentActions)
					toolCalls = append(toolCalls, map[string]any{"tool": request.Tool, "query": request.Query, "returned": len(segmentActions)})
				}
			}
		}
	}
	answer := buildKnowledgeAnswer(message, media, facts, relations, sqlResults, actions)
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
		Actions:    actions,
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
	builder.WriteString("Allowed tools: search_media, search_facts, search_relations, readonly_sql, transcode_recommendations, find_segmented_video_series. ")
	builder.WriteString("Every tool is read-only and bounded. Do not request writes or raw tables. ")
	builder.WriteString("readonly_sql may SELECT only from cartolensia_search_* views.\n")
	builder.WriteString(`JSON shape: {"tools":[{"tool":"search_media","query":"kind:photo 2025-05..2025-08 поезд","limit":12},{"tool":"search_facts","query":"поезд","limit":12},{"tool":"search_relations","query":"поезд","limit":12},{"tool":"readonly_sql","sql":"select asset_id, display_name, media_kind from cartolensia_search_assets where media_kind='photo' limit 20","limit":20},{"tool":"transcode_recommendations","query":"kind:video hevc 4k","limit":8},{"tool":"find_segmented_video_series","query":"camera segmented mp4 thm","limit":8}]}`)
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
		case "transcode_recommendations", "find_segmented_video_series":
			if request.Query == "" {
				request.Query = request.Tool
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

func knowledgeMessageLooksLikeTranscode(message string) bool {
	lower := strings.ToLower(message)
	needles := []string{
		"transcode", "transcoding", "encode", "encoder", "convert", "compress", "h264", "h.264", "h265", "h.265", "hevc", "av1",
		"транскод", "перекод", "кодиров", "сжать", "сжима", "конверт", "пережат",
	}
	return containsAnySubstring(lower, needles)
}

func knowledgeMessageLooksLikeSegmentMerge(message string) bool {
	lower := strings.ToLower(message)
	needles := []string{
		"merge", "join", "concat", "segment", "sequential", "series", ".thm", "thm",
		"скле", "объедин", "сегмент", "част", "серии", "серия", "последовательн",
	}
	return containsAnySubstring(lower, needles)
}

func containsAnySubstring(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func (s *Server) knowledgeTranscodeRecommendations(ctx context.Context, query string, limit int) ([]knowledgeMediaResult, []knowledgeAction, error) {
	limit = clampKnowledgeLimit(limit, 8)
	query = strings.TrimSpace(query)
	if !strings.Contains(strings.ToLower(query), "kind:") && !strings.Contains(strings.ToLower(query), "media_kind:") {
		query = strings.TrimSpace("kind:video " + query)
	}
	plan := s.buildNaturalLanguageSearchPlan(query)
	results, _, err := s.searchKnowledgeMedia(ctx, plan, limit)
	if err != nil {
		return nil, nil, err
	}
	if !knowledgeHasVideoResults(results) {
		fallbackPlan := s.buildNaturalLanguageSearchPlan("kind:video")
		results, _, err = s.searchKnowledgeMedia(ctx, fallbackPlan, limit)
		if err != nil {
			return nil, nil, err
		}
	}
	actions := make([]knowledgeAction, 0, len(results))
	for _, result := range results {
		if result.Asset.MediaKind != "video" {
			continue
		}
		profile, details := transcodeRecommendationForAsset(result.Asset)
		actions = append(actions, knowledgeAction{
			Action:  "start_transcode_session",
			Label:   "Start cache-only transcode",
			AssetID: result.Asset.ID,
			Method:  http.MethodPost,
			URL:     "/api/v1/media/" + url.PathEscape(result.Asset.ID) + "/transcode-session",
			Payload: map[string]any{"profile": profile},
			Note:    "Starts an on-demand transcode session under the Cartolensia cache. Originals remain read-only and are never overwritten.",
			Details: details,
		})
		if len(actions) >= limit {
			break
		}
	}
	return results, actions, nil
}

func knowledgeHasVideoResults(results []knowledgeMediaResult) bool {
	for _, result := range results {
		if result.Asset.MediaKind == "video" {
			return true
		}
	}
	return false
}

func transcodeRecommendationForAsset(asset catalog.Asset) (string, map[string]any) {
	metadata := asset.Metadata
	codec := strings.ToLower(stringFromAny(metadata["codec"]))
	container := strings.ToLower(stringFromAny(metadata["container"]))
	width := int(floatFromAny(metadata["width"]))
	height := int(floatFromAny(metadata["height"]))
	duration := floatFromAny(metadata["duration_seconds"])
	bitrate := int64(floatFromAny(metadata["bitrate_bps"]))
	profile := "h264_720p_lan"
	reasons := []string{"browser-compatible H.264 preview is the safest default"}
	if width >= 3840 || height >= 2160 {
		profile = "h264_1080p_lan"
		reasons = append(reasons, "source is 4K or larger; 1080p preview balances LAN playback and storage")
	}
	if codec == "av1" || strings.Contains(codec, "hevc") || strings.Contains(codec, "h265") {
		reasons = append(reasons, "source codec may not play everywhere without transcoding")
	}
	if bitrate > 60_000_000 {
		reasons = append(reasons, "high source bitrate can stall browser playback over slower clients")
	}
	return profile, map[string]any{
		"asset_name":       asset.DisplayName,
		"profile":          profile,
		"codec":            codec,
		"container":        container,
		"width":            width,
		"height":           height,
		"duration_seconds": duration,
		"bitrate_bps":      bitrate,
		"reasons":          reasons,
		"output_policy":    "Cartolensia cache/export only; never write to originals",
	}
}

type segmentedSeriesCandidate struct {
	Directory  string
	Prefix     string
	Segments   []segmentedSeriesItem
	Delimiters []segmentedSeriesItem
}

type segmentedSeriesItem struct {
	AssetID      string `json:"asset_id,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	FileName     string `json:"file_name"`
	RelativePath string `json:"relative_path"`
	StorageName  string `json:"storage_name,omitempty"`
	Extension    string `json:"extension"`
	Number       int    `json:"number"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
}

func (s *Server) knowledgeSegmentedSeriesPlans(ctx context.Context, message string, limit int) ([]knowledgeAction, error) {
	sqlStore, ok := s.deps.Store.(knowledgeReadOnlySQLStore)
	if !ok {
		return nil, fmt.Errorf("segmented video series detection requires PostgreSQL search views")
	}
	limit = clampKnowledgeLimit(limit, 8)
	rowLimit := max(500, limit*300)
	if rowLimit > 1000 {
		rowLimit = 1000
	}
	result, err := sqlStore.ReadOnlySearchQuery(ctx, `
		select asset_id, display_name, media_kind, storage_name, relative_path, file_name, extension, size_bytes
		from cartolensia_search_assets
		where lower(extension) in ('mp4','mov','m4v','thm')
		order by storage_name, relative_path, file_name
	`, rowLimit)
	if err != nil {
		return nil, err
	}
	candidates := segmentedSeriesCandidates(result.Rows)
	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].Segments) == len(candidates[j].Segments) {
			return candidates[i].Directory < candidates[j].Directory
		}
		return len(candidates[i].Segments) > len(candidates[j].Segments)
	})
	actions := make([]knowledgeAction, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		if len(candidate.Segments) < 2 {
			continue
		}
		segments := make([]map[string]any, 0, len(candidate.Segments))
		for _, segment := range candidate.Segments {
			segments = append(segments, map[string]any{
				"asset_id":      segment.AssetID,
				"display_name":  segment.DisplayName,
				"relative_path": segment.RelativePath,
				"file_name":     segment.FileName,
				"number":        segment.Number,
				"size_bytes":    segment.SizeBytes,
				"storage_name":  segment.StorageName,
				"extension":     segment.Extension,
			})
		}
		actions = append(actions, knowledgeAction{
			Action: "plan_segmented_video_merge",
			Label:  fmt.Sprintf("Review merge plan: %s (%d segments)", candidate.Prefix, len(candidate.Segments)),
			Query:  message,
			Note:   "Review-only plan. A future merge job must write output under Cartolensia exports/cache first; originals remain read-only and manual cleanup is separate.",
			Details: map[string]any{
				"directory":        candidate.Directory,
				"prefix":           candidate.Prefix,
				"segments":         segments,
				"delimiter_count":  len(candidate.Delimiters),
				"output_policy":    "Cartolensia export/cache only; non-overwriting; originals are never removed",
				"ffmpeg_strategy":  "concat demuxer with stream-copy when compatible, otherwise cache-scoped transcode after validation",
				"source_row_limit": result.Count,
			},
		})
		if len(actions) >= limit {
			break
		}
	}
	return actions, nil
}

var segmentedSeriesNameRE = regexp.MustCompile(`(?i)^(.+?)[_-](\d{1,8})\.(mp4|mov|m4v|thm)$`)

func segmentedSeriesCandidates(rows []map[string]any) []segmentedSeriesCandidate {
	groups := map[string]*segmentedSeriesCandidate{}
	for _, row := range rows {
		fileName := strings.TrimSpace(stringFromAny(row["file_name"]))
		match := segmentedSeriesNameRE.FindStringSubmatch(fileName)
		if len(match) != 4 {
			continue
		}
		extension := strings.ToLower(match[3])
		item := segmentedSeriesItem{
			AssetID:      stringFromAny(row["asset_id"]),
			DisplayName:  stringFromAny(row["display_name"]),
			FileName:     fileName,
			RelativePath: stringFromAny(row["relative_path"]),
			StorageName:  stringFromAny(row["storage_name"]),
			Extension:    extension,
			Number:       int(floatFromAny(match[2])),
			SizeBytes:    int64(floatFromAny(row["size_bytes"])),
		}
		directory := item.StorageName + ":" + filepath.ToSlash(filepath.Dir(item.RelativePath))
		prefix := strings.ToLower(match[1])
		key := directory + ":" + prefix
		group := groups[key]
		if group == nil {
			group = &segmentedSeriesCandidate{Directory: directory, Prefix: match[1]}
			groups[key] = group
		}
		if extension == "thm" {
			group.Delimiters = append(group.Delimiters, item)
		} else {
			group.Segments = append(group.Segments, item)
		}
	}
	candidates := make([]segmentedSeriesCandidate, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group.Segments, func(i, j int) bool { return group.Segments[i].Number < group.Segments[j].Number })
		sort.Slice(group.Delimiters, func(i, j int) bool { return group.Delimiters[i].Number < group.Delimiters[j].Number })
		if len(group.Segments) >= 2 {
			candidates = append(candidates, *group)
		}
	}
	return candidates
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

func buildKnowledgeAnswer(message string, media []knowledgeMediaResult, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation, sqlResults []database.ReadOnlyQueryResult, actions []knowledgeAction) string {
	var builder strings.Builder
	builder.WriteString("I searched the local Knowledge Base and Knowledge Graph for: ")
	builder.WriteString(message)
	builder.WriteString("\n\n")
	if len(media) == 0 && len(facts) == 0 && len(relations) == 0 && len(sqlResults) == 0 && len(actions) == 0 {
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
	if len(actions) > 0 {
		builder.WriteString("\nAvailable safe actions:\n")
		for i, action := range actions {
			if i >= 8 {
				builder.WriteString(fmt.Sprintf("- plus %d more actions\n", len(actions)-i))
				break
			}
			builder.WriteString(fmt.Sprintf("- %s: %s\n", firstNonEmpty(action.Label, action.Action), compactFactObject(action.Note)))
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
