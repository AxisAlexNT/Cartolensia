package database

import "testing"

func TestValidateReadOnlySearchSQLAllowsSearchViews(t *testing.T) {
	query, views, err := validateReadOnlySearchSQL(`select asset_id, display_name from cartolensia_search_assets where extension = 'mp4' order by taken_at desc`)
	if err != nil {
		t.Fatalf("expected query to validate: %v", err)
	}
	if query == "" || len(views) != 1 || views[0] != "cartolensia_search_assets" {
		t.Fatalf("unexpected query=%q views=%v", query, views)
	}

	query, views, err = validateReadOnlySearchSQL(`select subject, predicate, object from cartolensia_search_knowledge_facts where predicate = 'caption'`)
	if err != nil {
		t.Fatalf("expected knowledge query to validate: %v", err)
	}
	if query == "" || len(views) != 1 || views[0] != "cartolensia_search_knowledge_facts" {
		t.Fatalf("unexpected knowledge query=%q views=%v", query, views)
	}
}

func TestValidateReadOnlySearchSQLRejectsUnsafeStatements(t *testing.T) {
	cases := []string{
		`update assets set display_name = 'bad'`,
		`select * from assets`,
		`select * from cartolensia_search_assets; drop table assets`,
		`select * from cartolensia_search_assets -- comment`,
		`select * from cartolensia_search_assets join ai_predictions on true`,
	}
	for _, query := range cases {
		if _, _, err := validateReadOnlySearchSQL(query); err == nil {
			t.Fatalf("expected unsafe query to fail: %s", query)
		}
	}
}
