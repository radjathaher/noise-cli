package reporting

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	validGroupBy = map[string]bool{"campaign": true, "playbook": true}
	validMetrics = map[string]bool{
		"spend": true, "views": true, "posts": true, "creators": true,
		"cpm": true, "estimated_installs": true,
	}
	validPostModes = map[string]bool{"created": true, "active": true}
	validPostSorts = map[string]bool{
		"posted": true, "views": true, "range_views": true, "likes": true,
		"comments": true, "bookmarks": true, "shares": true, "spend": true,
		"range_spend": true, "engagement": true, "handle": true,
	}
	validOrders = map[string]bool{"asc": true, "desc": true}
)

type DailyReportParams struct {
	StartDate  string
	EndDate    string
	GroupBy    string
	CampaignID *int64
	PlaybookID *int64
	Metrics    string
}

func (p DailyReportParams) Values() (url.Values, error) {
	if err := validateDateRange(p.StartDate, p.EndDate); err != nil {
		return nil, err
	}
	groupBy, err := validateCSVOptions("group-by", p.GroupBy, validGroupBy, 2)
	if err != nil {
		return nil, err
	}
	metrics, err := validateCSVOptions("metrics", p.Metrics, validMetrics, len(validMetrics))
	if err != nil {
		return nil, err
	}
	if p.CampaignID != nil && *p.CampaignID < 1 {
		return nil, fmt.Errorf("campaign-id must be a positive integer")
	}
	if p.PlaybookID != nil && *p.PlaybookID < 0 {
		return nil, fmt.Errorf("playbook-id must not be negative")
	}

	values := url.Values{"start_date": {p.StartDate}, "end_date": {p.EndDate}}
	setOptional(values, "group_by", groupBy)
	setOptional(values, "metrics", metrics)
	if p.CampaignID != nil {
		values.Set("campaign_id", strconv.FormatInt(*p.CampaignID, 10))
	}
	if p.PlaybookID != nil {
		values.Set("playbook_id", strconv.FormatInt(*p.PlaybookID, 10))
	}
	return values, nil
}

type PostsParams struct {
	StartDate   string
	EndDate     string
	Mode        string
	CampaignIDs string
	PlaybookIDs string
	Page        int
	PageSize    int
	Sort        string
	Order       string
}

func (p PostsParams) Values() (url.Values, error) {
	if err := validateDateRange(p.StartDate, p.EndDate); err != nil {
		return nil, err
	}
	if !validPostModes[p.Mode] {
		return nil, fmt.Errorf("mode must be one of: active, created")
	}
	if !validPostSorts[p.Sort] {
		return nil, fmt.Errorf("invalid sort value %q", p.Sort)
	}
	if !validOrders[p.Order] {
		return nil, fmt.Errorf("order must be one of: asc, desc")
	}
	if p.Page < 1 {
		return nil, fmt.Errorf("page must be at least 1")
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		return nil, fmt.Errorf("page-size must be between 1 and 100")
	}
	campaignIDs, err := validateIDs("campaign-id", p.CampaignIDs)
	if err != nil {
		return nil, err
	}
	playbookIDs, err := validateIDs("playbook-id", p.PlaybookIDs)
	if err != nil {
		return nil, err
	}

	values := url.Values{
		"start_date": {p.StartDate}, "end_date": {p.EndDate}, "mode": {p.Mode},
		"page": {strconv.Itoa(p.Page)}, "page_size": {strconv.Itoa(p.PageSize)},
		"sort": {p.Sort}, "order": {p.Order},
	}
	setOptional(values, "campaign_id", campaignIDs)
	setOptional(values, "playbook_id", playbookIDs)
	return values, nil
}

func validateDateRange(startValue, endValue string) error {
	start, err := parseDate("start-date", startValue)
	if err != nil {
		return err
	}
	end, err := parseDate("end-date", endValue)
	if err != nil {
		return err
	}
	if end.Before(start) {
		return fmt.Errorf("end-date must be on or after start-date")
	}
	if end.Sub(start) > 365*24*time.Hour {
		return fmt.Errorf("date range must not exceed 366 inclusive days")
	}
	return nil
}

func parseDate(name, value string) (time.Time, error) {
	if len(value) != len("2006-01-02") {
		return time.Time{}, fmt.Errorf("%s must use YYYY-MM-DD", name)
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, fmt.Errorf("%s must be a valid date in YYYY-MM-DD", name)
	}
	return parsed, nil
}

func validateCSVOptions(name, value string, allowed map[string]bool, maxItems int) (string, error) {
	if value == "" {
		return "", nil
	}
	items := strings.Split(value, ",")
	if len(items) > maxItems {
		return "", fmt.Errorf("%s accepts at most %d values", name, maxItems)
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item == "" || item != strings.TrimSpace(item) || !allowed[item] {
			return "", fmt.Errorf("invalid %s value %q", name, item)
		}
		if seen[item] {
			return "", fmt.Errorf("duplicate %s value %q", name, item)
		}
		seen[item] = true
	}
	return strings.Join(items, ","), nil
}

func validateIDs(name, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	items := strings.Split(value, ",")
	if len(items) > 100 {
		return "", fmt.Errorf("%s accepts at most 100 values", name)
	}
	seen := make(map[int64]bool, len(items))
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" || item != strings.TrimSpace(item) {
			return "", fmt.Errorf("%s must be comma-separated non-negative integers", name)
		}
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil || id < 0 {
			return "", fmt.Errorf("%s must be comma-separated non-negative integers", name)
		}
		if seen[id] {
			return "", fmt.Errorf("duplicate %s value %d", name, id)
		}
		seen[id] = true
		normalized = append(normalized, strconv.FormatInt(id, 10))
	}
	return strings.Join(normalized, ","), nil
}

func setOptional(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}
