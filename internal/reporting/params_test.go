package reporting

import (
	"strings"
	"testing"
)

func TestDailyReportParamsValues(t *testing.T) {
	t.Parallel()
	campaignID := int64(12)
	playbookID := int64(0)
	values, err := (DailyReportParams{
		StartDate: "2026-01-01", EndDate: "2026-12-31",
		GroupBy: "campaign,playbook", Metrics: "spend,views,cpm",
		CampaignID: &campaignID, PlaybookID: &playbookID,
	}).Values()
	if err != nil {
		t.Fatalf("Values() error = %v", err)
	}
	want := "campaign_id=12&end_date=2026-12-31&group_by=campaign%2Cplaybook&metrics=spend%2Cviews%2Ccpm&playbook_id=0&start_date=2026-01-01"
	if values.Encode() != want {
		t.Fatalf("query = %q, want %q", values.Encode(), want)
	}
}

func TestDailyReportParamsRejectInvalidInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params DailyReportParams
		match  string
	}{
		{name: "invalid start", params: DailyReportParams{StartDate: "2026-02-30", EndDate: "2026-03-01"}, match: "valid date"},
		{name: "reverse range", params: DailyReportParams{StartDate: "2026-03-02", EndDate: "2026-03-01"}, match: "on or after"},
		{name: "too long", params: DailyReportParams{StartDate: "2024-01-01", EndDate: "2025-01-01"}, match: "366 inclusive"},
		{name: "bad group", params: DailyReportParams{StartDate: "2026-01-01", EndDate: "2026-01-02", GroupBy: "account"}, match: "invalid group-by"},
		{name: "duplicate metric", params: DailyReportParams{StartDate: "2026-01-01", EndDate: "2026-01-02", Metrics: "views,views"}, match: "duplicate metrics"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.params.Values()
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("Values() error = %v, want match %q", err, test.match)
			}
		})
	}
}

func TestPostsParamsValuesAndPagination(t *testing.T) {
	t.Parallel()
	values, err := (PostsParams{
		StartDate: "2026-01-01", EndDate: "2026-01-31", Mode: "active",
		CampaignIDs: "12,15,0", PlaybookIDs: "3401,0", Page: 3, PageSize: 100,
		Sort: "range_views", Order: "asc",
	}).Values()
	if err != nil {
		t.Fatalf("Values() error = %v", err)
	}
	if values.Get("page") != "3" || values.Get("page_size") != "100" || values.Get("campaign_id") != "12,15,0" {
		t.Fatalf("query = %q", values.Encode())
	}
}

func TestPostsParamsRejectInvalidInputs(t *testing.T) {
	t.Parallel()
	base := PostsParams{
		StartDate: "2026-01-01", EndDate: "2026-01-31", Mode: "created",
		Page: 1, PageSize: 25, Sort: "posted", Order: "desc",
	}
	tests := []struct {
		name   string
		mutate func(*PostsParams)
		match  string
	}{
		{name: "mode", mutate: func(params *PostsParams) { params.Mode = "all" }, match: "mode"},
		{name: "page", mutate: func(params *PostsParams) { params.Page = 0 }, match: "page must"},
		{name: "page size", mutate: func(params *PostsParams) { params.PageSize = 101 }, match: "page-size"},
		{name: "sort", mutate: func(params *PostsParams) { params.Sort = "unknown" }, match: "sort"},
		{name: "order", mutate: func(params *PostsParams) { params.Order = "sideways" }, match: "order"},
		{name: "negative ID", mutate: func(params *PostsParams) { params.CampaignIDs = "1,-2" }, match: "non-negative"},
		{name: "duplicate ID", mutate: func(params *PostsParams) { params.PlaybookIDs = "0,0" }, match: "duplicate"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			params := base
			test.mutate(&params)
			_, err := params.Values()
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("Values() error = %v, want match %q", err, test.match)
			}
		})
	}
}
