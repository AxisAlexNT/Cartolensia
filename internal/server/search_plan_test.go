package server

import "testing"

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
	if !containsString(plan.Tokens, "поездом") && !containsString(plan.Tokens, "станцией") {
		t.Fatalf("expected Russian content terms in %#v", plan.Tokens)
	}
}

func TestKnowledgeChatTermsIgnoreMediaKindTokens(t *testing.T) {
	plan := (&Server{}).buildNaturalLanguageSearchPlan("покажи видео с поездом и станцией")
	terms := knowledgeChatTerms("покажи видео с поездом и станцией", plan)
	if containsString(terms, "kind:video") || containsString(terms, "video") {
		t.Fatalf("expected media kind token to be ignored in %#v", terms)
	}
	if !containsString(terms, "поездом") && !containsString(terms, "станцией") {
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
