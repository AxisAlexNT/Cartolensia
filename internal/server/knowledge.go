package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
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

type knowledgeChatResponse struct {
	ConversationID string                      `json:"conversation_id,omitempty"`
	Answer         string                      `json:"answer"`
	Planner        searchPlan                  `json:"planner"`
	ToolCalls      []map[string]any            `json:"tool_calls"`
	Facts          []catalog.KnowledgeFact     `json:"facts"`
	Relations      []catalog.KnowledgeRelation `json:"relations"`
	Messages       []catalog.KnowledgeMessage  `json:"messages,omitempty"`
	Limit          int                         `json:"limit"`
	LLMStatus      string                      `json:"llm_status"`
	Note           string                      `json:"note"`
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

func (s *Server) runKnowledgeChatTools(ctx context.Context, message string, limit int, factStore knowledgeFactStore, relationStore knowledgeRelationStore) (knowledgeChatResponse, error) {
	plan := s.buildNaturalLanguageSearchPlan(message)
	terms := knowledgeChatTerms(message, plan)
	facts := []catalog.KnowledgeFact{}
	relations := []catalog.KnowledgeRelation{}
	seenFacts := map[string]struct{}{}
	seenRelations := map[string]struct{}{}
	toolCalls := []map[string]any{
		{"tool": "search_plan", "query": message, "executable_query": plan.Executable, "tokens": plan.Tokens},
	}
	perTermLimit := limit
	if len(terms) > 1 {
		perTermLimit = max(5, limit/len(terms))
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
		for _, fact := range factPage.Facts {
			if _, ok := seenFacts[fact.ID]; ok {
				continue
			}
			facts = append(facts, fact)
			seenFacts[fact.ID] = struct{}{}
			if len(facts) >= limit {
				break
			}
		}
		for _, relation := range relationPage.Relations {
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
	answer := buildKnowledgeAnswer(message, facts, relations)
	llmStatus := "not_used"
	note := "Local deterministic tool runner. Configure a local LLM endpoint to synthesize richer answers; remote LLM APIs are not used by default."
	if runtimeStringSetting("knowledge.runner_mode", "deterministic") == "local_llm" {
		llmAnswer, status, llmNote, err := s.synthesizeKnowledgeWithLocalLLM(ctx, message, plan, facts, relations)
		llmStatus = status
		note = llmNote
		toolCalls = append(toolCalls, map[string]any{"tool": "local_llm_synthesis", "status": status, "error": errorString(err)})
		if err == nil && strings.TrimSpace(llmAnswer) != "" {
			answer = llmAnswer
		} else if err != nil {
			note = note + " Deterministic answer is shown because local LLM synthesis failed: " + err.Error()
		}
	}
	return knowledgeChatResponse{
		Answer:    answer,
		Planner:   plan,
		ToolCalls: toolCalls,
		Facts:     facts,
		Relations: relations,
		Limit:     limit,
		LLMStatus: llmStatus,
		Note:      note,
	}, nil
}

func (s *Server) synthesizeKnowledgeWithLocalLLM(ctx context.Context, message string, plan searchPlan, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation) (string, string, string, error) {
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
	prompt := knowledgeLLMPrompt(message, plan, facts, relations)
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

func knowledgeLLMPrompt(message string, plan searchPlan, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation) string {
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
	return builder.String()
}

func postOllamaChat(ctx context.Context, endpoint, model, prompt string) (string, error) {
	payload := map[string]any{
		"model":  model,
		"stream": false,
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
	if err := postLocalLLMJSON(ctx, endpoint+"/api/chat", payload, &response); err != nil {
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
	if err := postLocalLLMJSON(ctx, endpoint+"/v1/chat/completions", payload, &response); err != nil {
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

func buildKnowledgeAnswer(message string, facts []catalog.KnowledgeFact, relations []catalog.KnowledgeRelation) string {
	var builder strings.Builder
	builder.WriteString("I searched the local Knowledge Base and Knowledge Graph for: ")
	builder.WriteString(message)
	builder.WriteString("\n\n")
	if len(facts) == 0 && len(relations) == 0 {
		builder.WriteString("No mined facts or relations matched yet. Run knowledge extraction after OCR, transcripts, captions, and metadata jobs have produced more local metadata.")
		return builder.String()
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
