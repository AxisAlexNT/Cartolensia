package server

import (
	"strings"
	"testing"
)

func TestSearchPlanTranslatesSQLLikeInput(t *testing.T) {
	query, clauses, ok := parseSQLLikeSearch(`kind = "video" and ext = mp4 and caption contains "train station"`)
	if !ok {
		t.Fatal("expected SQL-like query to parse")
	}
	if query != `kind:video ext:mp4 caption:"train station"` {
		t.Fatalf("unexpected executable query %q", query)
	}
	if len(clauses) != 3 || clauses[2].Field != "caption" || clauses[2].Value != "train station" {
		t.Fatalf("unexpected clauses %#v", clauses)
	}
}

func TestNaturalLanguageSearchPlanRussianFallback(t *testing.T) {
	plan := (&Server{}).buildNaturalLanguageSearchPlan("покажи видео с поездом и станцией")
	if plan.Planner != "rule_based_local" {
		t.Fatalf("unexpected planner %q", plan.Planner)
	}
	if !containsString(plan.Tokens, "kind:video") {
		t.Fatalf("expected video token in %#v", plan.Tokens)
	}
	if !containsString(plan.Tokens, "поезд") && !containsString(plan.Tokens, "станци") {
		t.Fatalf("expected Russian content terms in %#v", plan.Tokens)
	}
}

func TestNaturalLanguageSearchPlanRussianMonthRange(t *testing.T) {
	plan := (&Server{}).buildNaturalLanguageSearchPlan("найди фотографии май-август 2025 года с поездом")
	if !containsString(plan.Tokens, "kind:photo") {
		t.Fatalf("expected photo token in %#v", plan.Tokens)
	}
	if !containsString(plan.Tokens, "2025-05..2025-08") {
		t.Fatalf("expected date range token in %#v", plan.Tokens)
	}
	if containsString(plan.Tokens, "май-август") || containsString(plan.Tokens, "2025") {
		t.Fatalf("month range should not be emitted as plain text terms: %#v", plan.Tokens)
	}
}

func TestNaturalLanguageSearchPlanEnglishTrainPhotoMonth(t *testing.T) {
	plan := (&Server{}).buildNaturalLanguageSearchPlan("Please, find and count all photos with trains, made in May 2025.")
	if !containsString(plan.Tokens, "kind:photo") {
		t.Fatalf("expected photo token in %#v", plan.Tokens)
	}
	if !containsString(plan.Tokens, "2025-05") {
		t.Fatalf("expected May 2025 token in %#v", plan.Tokens)
	}
	if !containsString(plan.Tokens, "train") {
		t.Fatalf("expected canonical train token in %#v", plan.Tokens)
	}
	for _, unexpected := range []string{"please", "find", "count", "all", "trains", "made"} {
		if containsString(plan.Tokens, unexpected) {
			t.Fatalf("unexpected filler token %q in %#v", unexpected, plan.Tokens)
		}
	}
}

func TestKnowledgeMediaEvidenceSQLUsesSearchViews(t *testing.T) {
	plan := (&Server{}).buildNaturalLanguageSearchPlan("videos with trains from May 2025")
	sql, ok := knowledgeMediaEvidenceSQL(plan)
	if !ok {
		t.Fatal("expected evidence SQL")
	}
	for _, view := range []string{
		"cartolensia_search_video_captions",
		"cartolensia_search_ai_predictions",
		"cartolensia_search_tags",
		"cartolensia_search_knowledge_facts",
	} {
		if !strings.Contains(sql, view) {
			t.Fatalf("expected %s in evidence SQL:\n%s", view, sql)
		}
	}
	for _, fragment := range []string{"media_kind", "2025-05-01", "%train%"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected %q in evidence SQL:\n%s", fragment, sql)
		}
	}
}

func TestKnowledgeChatTermsIgnoreMediaKindTokens(t *testing.T) {
	plan := (&Server{}).buildNaturalLanguageSearchPlan("покажи видео с поездом и станцией")
	terms := knowledgeChatTerms("покажи видео с поездом и станцией", plan)
	if containsString(terms, "kind:video") || containsString(terms, "video") {
		t.Fatalf("expected media kind token to be ignored in %#v", terms)
	}
	if !containsString(terms, "поезд") && !containsString(terms, "станци") {
		t.Fatalf("expected Russian search terms in %#v", terms)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
