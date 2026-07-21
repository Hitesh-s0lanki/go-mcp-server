package gsc

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	searchconsole "google.golang.org/api/searchconsole/v1"
)

// registerAnalyticsTools wires the Search Analytics (search-traffic) tools.
func registerAnalyticsTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_search_analytics",
		Description: "Search-traffic report for a property over the last N days (default 28): top rows by " +
			"dimension(s) with clicks, impressions, CTR and average position. dimensions is a comma-separated " +
			"list of query,page,country,device,date,searchAppearance. Use gsc_advanced_search_analytics for " +
			"explicit dates, filters, pagination or a non-web search type.",
	}, c.searchAnalytics)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_advanced_search_analytics",
		Description: "Full-featured Search Analytics query: explicit start_date/end_date (YYYY-MM-DD), " +
			"multi-dimension grouping, a single dimension filter (filter_dimension/filter_operator/" +
			"filter_expression), search_type (WEB/IMAGE/VIDEO/NEWS/DISCOVER/GOOGLE_NEWS), sorting, and " +
			"pagination (row_limit up to 25000, start_row). Defaults to the last 28 days when dates are omitted.",
	}, c.advancedSearchAnalytics)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_performance_overview",
		Description: "High-level performance summary for a property over the last N days (default 28): total " +
			"clicks/impressions, overall CTR and average position, plus a per-day trend.",
	}, c.performanceOverview)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_compare_periods",
		Description: "Compare Search Analytics between two explicit date ranges (period1_* vs period2_*) grouped " +
			"by a dimension, returning per-key clicks/impressions/CTR/position for each period and the deltas.",
	}, c.comparePeriods)

	mcp.AddTool(s, &mcp.Tool{
		Name: "gsc_search_by_page_query",
		Description: "For a single page URL, the search queries that drove impressions/clicks to it over the last " +
			"N days (default 28), plus that page's totals. Equivalent to grouping by query with a page==URL filter.",
	}, c.searchByPageQuery)
}

// analyticsRow is one Search Analytics result row with its dimension values
// resolved to their dimension names.
type analyticsRow struct {
	Keys        map[string]string `json:"keys,omitempty"`
	Clicks      float64           `json:"clicks"`
	Impressions float64           `json:"impressions"`
	CTR         float64           `json:"ctr"`
	Position    float64           `json:"position"`
}

func buildRows(dims []string, rows []*searchconsole.ApiDataRow) []analyticsRow {
	out := make([]analyticsRow, 0, len(rows))
	for _, r := range rows {
		ar := analyticsRow{
			Clicks:      r.Clicks,
			Impressions: r.Impressions,
			CTR:         round2(r.Ctr * 100), // fraction → percentage
			Position:    round2(r.Position),
		}
		if len(r.Keys) > 0 {
			ar.Keys = make(map[string]string, len(r.Keys))
			for i, k := range r.Keys {
				name := "key"
				if i < len(dims) {
					name = dims[i]
				}
				ar.Keys[name] = k
			}
		}
		out = append(out, ar)
	}
	return out
}

// parseDimensions splits a comma-separated dimension list; defaults to ["query"].
func parseDimensions(s string) []string {
	dims := make([]string, 0, 4)
	for _, d := range strings.Split(s, ",") {
		if t := strings.TrimSpace(d); t != "" {
			dims = append(dims, t)
		}
	}
	if len(dims) == 0 {
		return []string{"query"}
	}
	return dims
}

// today returns the current UTC date. Search Console reports in PST but a UTC
// "today" is a safe upper bound for the requested end date.
func today() time.Time { return time.Now().UTC() }

func dateStr(t time.Time) string { return t.Format("2006-01-02") }

// defaultRange returns [today-days, today] as YYYY-MM-DD strings.
func defaultRange(days int) (start, end string) {
	if days <= 0 {
		days = 28
	}
	now := today()
	return dateStr(now.AddDate(0, 0, -days)), dateStr(now)
}

// dataStateEnum normalizes a data-state string ("all"/"final") to the API enum.
func dataStateEnum(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "ALL":
		return "ALL"
	case "FINAL":
		return "FINAL"
	default:
		return strings.ToUpper(strings.TrimSpace(s))
	}
}

// runQuery executes a Search Analytics query and returns the raw rows.
func (c *Client) runQuery(ctx context.Context, siteURL string, req *searchconsole.SearchAnalyticsQueryRequest) ([]*searchconsole.ApiDataRow, error) {
	if req.DataState == "" {
		req.DataState = dataStateEnum(c.dataState)
	}
	resp, err := c.svc.Searchanalytics.Query(siteURL, req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return resp.Rows, nil
}

// --- gsc_search_analytics ---

type searchAnalyticsInput struct {
	SiteURL    string `json:"site_url" jsonschema:"the Search Console property, e.g. https://example.com/ or sc-domain:example.com"`
	Days       int    `json:"days,omitempty" jsonschema:"look-back window in days ending today; default 28"`
	Dimensions string `json:"dimensions,omitempty" jsonschema:"comma-separated dimensions to group by (query,page,country,device,date,searchAppearance); default query"`
	RowLimit   int    `json:"row_limit,omitempty" jsonschema:"max rows to return; default 20"`
}

type searchAnalyticsOutput struct {
	SiteURL    string         `json:"site_url"`
	StartDate  string         `json:"start_date"`
	EndDate    string         `json:"end_date"`
	Dimensions []string       `json:"dimensions"`
	RowCount   int            `json:"row_count"`
	Rows       []analyticsRow `json:"rows"`
}

func (c *Client) searchAnalytics(ctx context.Context, _ *mcp.CallToolRequest, in searchAnalyticsInput) (*mcp.CallToolResult, searchAnalyticsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[searchAnalyticsOutput]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[searchAnalyticsOutput]("site_url is required")
	}
	days := in.Days
	if days <= 0 {
		days = 28
	}
	limit := in.RowLimit
	if limit <= 0 {
		limit = 20
	}
	dims := parseDimensions(in.Dimensions)
	start, end := defaultRange(days)

	rows, err := c.runQuery(ctx, in.SiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, Dimensions: dims, RowLimit: int64(limit),
	})
	if err != nil {
		return toolErr[searchAnalyticsOutput]("search analytics query: %v", err)
	}
	built := buildRows(dims, rows)
	return jsonResult(searchAnalyticsOutput{
		SiteURL: in.SiteURL, StartDate: start, EndDate: end,
		Dimensions: dims, RowCount: len(built), Rows: built,
	})
}

// --- gsc_advanced_search_analytics ---

type advancedInput struct {
	SiteURL          string `json:"site_url" jsonschema:"the Search Console property"`
	StartDate        string `json:"start_date,omitempty" jsonschema:"YYYY-MM-DD; defaults to 28 days before end_date"`
	EndDate          string `json:"end_date,omitempty" jsonschema:"YYYY-MM-DD; defaults to today"`
	Dimensions       string `json:"dimensions,omitempty" jsonschema:"comma-separated dimensions; default query"`
	SearchType       string `json:"search_type,omitempty" jsonschema:"WEB (default), IMAGE, VIDEO, NEWS, DISCOVER or GOOGLE_NEWS"`
	RowLimit         int    `json:"row_limit,omitempty" jsonschema:"max rows, up to 25000; default 1000"`
	StartRow         int    `json:"start_row,omitempty" jsonschema:"zero-based offset for pagination; default 0"`
	SortBy           string `json:"sort_by,omitempty" jsonschema:"metric to sort rows by client-side: clicks (default), impressions, ctr or position"`
	SortDirection    string `json:"sort_direction,omitempty" jsonschema:"ascending or descending (default)"`
	FilterDimension  string `json:"filter_dimension,omitempty" jsonschema:"dimension to filter on, e.g. query, page, country, device"`
	FilterOperator   string `json:"filter_operator,omitempty" jsonschema:"contains (default), equals, notContains, notEquals, includingRegex, excludingRegex"`
	FilterExpression string `json:"filter_expression,omitempty" jsonschema:"value the filter compares against; required to apply a filter"`
	DataState        string `json:"data_state,omitempty" jsonschema:"ALL (default, includes fresh data) or FINAL (finalized only)"`
}

type advancedOutput struct {
	SiteURL        string          `json:"site_url"`
	StartDate      string          `json:"start_date"`
	EndDate        string          `json:"end_date"`
	Dimensions     []string        `json:"dimensions"`
	SearchType     string          `json:"search_type"`
	DataState      string          `json:"data_state"`
	FiltersApplied []appliedFilter `json:"filters_applied,omitempty"`
	StartRow       int             `json:"start_row"`
	RowLimit       int             `json:"row_limit"`
	RowCount       int             `json:"row_count"`
	Rows           []analyticsRow  `json:"rows"`
}

type appliedFilter struct {
	Dimension  string `json:"dimension"`
	Operator   string `json:"operator"`
	Expression string `json:"expression"`
}

func (c *Client) advancedSearchAnalytics(ctx context.Context, _ *mcp.CallToolRequest, in advancedInput) (*mcp.CallToolResult, advancedOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[advancedOutput]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[advancedOutput]("site_url is required")
	}

	end := in.EndDate
	if end == "" {
		end = dateStr(today())
	}
	start := in.StartDate
	if start == "" {
		// 28 days before the end date.
		if t, err := time.Parse("2006-01-02", end); err == nil {
			start = dateStr(t.AddDate(0, 0, -28))
		} else {
			start, _ = defaultRange(28)
		}
	}

	dims := parseDimensions(in.Dimensions)
	searchType := strings.ToUpper(firstNonEmpty(in.SearchType, "WEB"))
	limit := in.RowLimit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 25000 {
		limit = 25000
	}
	dataState := dataStateEnum(firstNonEmpty(in.DataState, c.dataState))

	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: dims,
		SearchType: searchType,
		RowLimit:   int64(limit),
		StartRow:   int64(in.StartRow),
		DataState:  dataState,
	}

	var applied []appliedFilter
	if in.FilterDimension != "" && in.FilterExpression != "" {
		op := firstNonEmpty(in.FilterOperator, "contains")
		req.DimensionFilterGroups = []*searchconsole.ApiDimensionFilterGroup{{
			GroupType: "and",
			Filters: []*searchconsole.ApiDimensionFilter{{
				Dimension:  in.FilterDimension,
				Operator:   op,
				Expression: in.FilterExpression,
			}},
		}}
		applied = append(applied, appliedFilter{Dimension: in.FilterDimension, Operator: op, Expression: in.FilterExpression})
	}

	rows, err := c.runQuery(ctx, in.SiteURL, req)
	if err != nil {
		return toolErr[advancedOutput]("advanced search analytics query: %v", err)
	}
	built := buildRows(dims, rows)
	sortRows(built, in.SortBy, in.SortDirection)

	return jsonResult(advancedOutput{
		SiteURL: in.SiteURL, StartDate: start, EndDate: end,
		Dimensions: dims, SearchType: searchType, DataState: dataState,
		FiltersApplied: applied, StartRow: in.StartRow, RowLimit: limit,
		RowCount: len(built), Rows: built,
	})
}

// sortRows orders rows client-side. The API already sorts by clicks desc, but a
// caller can re-sort by any metric.
func sortRows(rows []analyticsRow, by, dir string) {
	by = strings.ToLower(strings.TrimSpace(by))
	if by == "" {
		return
	}
	asc := strings.HasPrefix(strings.ToLower(strings.TrimSpace(dir)), "asc")
	metric := func(r analyticsRow) float64 {
		switch by {
		case "impressions":
			return r.Impressions
		case "ctr":
			return r.CTR
		case "position":
			return r.Position
		default:
			return r.Clicks
		}
	}
	// simple insertion sort keeps it dependency-free and stable for small N,
	// but use sort.SliceStable for larger result sets.
	sortSliceStable(rows, func(a, b analyticsRow) bool {
		if asc {
			return metric(a) < metric(b)
		}
		return metric(a) > metric(b)
	})
}

// --- gsc_performance_overview ---

type overviewInput struct {
	SiteURL string `json:"site_url" jsonschema:"the Search Console property"`
	Days    int    `json:"days,omitempty" jsonschema:"look-back window in days; default 28"`
}

type overviewOutput struct {
	SiteURL    string        `json:"site_url"`
	StartDate  string        `json:"start_date"`
	EndDate    string        `json:"end_date"`
	Totals     metricTotals  `json:"totals"`
	DailyTrend []dailyMetric `json:"daily_trend"`
}

type metricTotals struct {
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type dailyMetric struct {
	Date        string  `json:"date"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

func (c *Client) performanceOverview(ctx context.Context, _ *mcp.CallToolRequest, in overviewInput) (*mcp.CallToolResult, overviewOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[overviewOutput]("%v", err)
	}
	if in.SiteURL == "" {
		return toolErr[overviewOutput]("site_url is required")
	}
	days := in.Days
	if days <= 0 {
		days = 28
	}
	start, end := defaultRange(days)

	rows, err := c.runQuery(ctx, in.SiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, Dimensions: []string{"date"}, RowLimit: 1000,
	})
	if err != nil {
		return toolErr[overviewOutput]("performance overview query: %v", err)
	}

	out := overviewOutput{SiteURL: in.SiteURL, StartDate: start, EndDate: end, DailyTrend: make([]dailyMetric, 0, len(rows))}
	var totalClicks, totalImpr, weightedPos float64
	for _, r := range rows {
		date := ""
		if len(r.Keys) > 0 {
			date = r.Keys[0]
		}
		out.DailyTrend = append(out.DailyTrend, dailyMetric{
			Date: date, Clicks: r.Clicks, Impressions: r.Impressions,
			CTR: round2(r.Ctr * 100), Position: round2(r.Position),
		})
		totalClicks += r.Clicks
		totalImpr += r.Impressions
		weightedPos += r.Position * r.Impressions // impression-weighted average position
	}
	out.Totals = metricTotals{
		Clicks:      totalClicks,
		Impressions: totalImpr,
		CTR:         round2(pct(totalClicks, totalImpr)),
		Position:    round2(safeDiv(weightedPos, totalImpr)),
	}
	return jsonResult(out)
}

// --- gsc_compare_periods ---

type compareInput struct {
	SiteURL      string `json:"site_url" jsonschema:"the Search Console property"`
	Period1Start string `json:"period1_start" jsonschema:"YYYY-MM-DD start of the first (baseline) period"`
	Period1End   string `json:"period1_end" jsonschema:"YYYY-MM-DD end of the first period"`
	Period2Start string `json:"period2_start" jsonschema:"YYYY-MM-DD start of the second (comparison) period"`
	Period2End   string `json:"period2_end" jsonschema:"YYYY-MM-DD end of the second period"`
	Dimensions   string `json:"dimensions,omitempty" jsonschema:"comma-separated dimensions to group by; default query"`
	Limit        int    `json:"limit,omitempty" jsonschema:"max keys to compare (ranked by period-2 clicks); default 10"`
}

type comparisonRow struct {
	Keys             map[string]string `json:"keys,omitempty"`
	Period1          metricTotals      `json:"period1"`
	Period2          metricTotals      `json:"period2"`
	ClicksDelta      float64           `json:"clicks_delta"`
	ClicksPctChange  float64           `json:"clicks_pct_change"`
	ImpressionsDelta float64           `json:"impressions_delta"`
	PositionDelta    float64           `json:"position_delta"`
}

type compareOutput struct {
	SiteURL     string          `json:"site_url"`
	Period1     [2]string       `json:"period1"`
	Period2     [2]string       `json:"period2"`
	Dimensions  []string        `json:"dimensions"`
	Comparisons []comparisonRow `json:"comparisons"`
}

func (c *Client) comparePeriods(ctx context.Context, _ *mcp.CallToolRequest, in compareInput) (*mcp.CallToolResult, compareOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[compareOutput]("%v", err)
	}
	if in.SiteURL == "" || in.Period1Start == "" || in.Period1End == "" || in.Period2Start == "" || in.Period2End == "" {
		return toolErr[compareOutput]("site_url and all four period date bounds are required")
	}
	dims := parseDimensions(in.Dimensions)
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}

	rows1, err := c.runQuery(ctx, in.SiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: in.Period1Start, EndDate: in.Period1End, Dimensions: dims, RowLimit: 25000,
	})
	if err != nil {
		return toolErr[compareOutput]("period 1 query: %v", err)
	}
	rows2, err := c.runQuery(ctx, in.SiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: in.Period2Start, EndDate: in.Period2End, Dimensions: dims, RowLimit: 25000,
	})
	if err != nil {
		return toolErr[compareOutput]("period 2 query: %v", err)
	}

	type agg struct {
		keys map[string]string
		p1   *searchconsole.ApiDataRow
		p2   *searchconsole.ApiDataRow
	}
	byKey := map[string]*agg{}
	keyOf := func(r *searchconsole.ApiDataRow) string { return strings.Join(r.Keys, "\x1f") }
	for _, r := range rows1 {
		k := keyOf(r)
		byKey[k] = &agg{keys: keysMap(dims, r.Keys), p1: r}
	}
	for _, r := range rows2 {
		k := keyOf(r)
		if a, ok := byKey[k]; ok {
			a.p2 = r
		} else {
			byKey[k] = &agg{keys: keysMap(dims, r.Keys), p2: r}
		}
	}

	comps := make([]comparisonRow, 0, len(byKey))
	for _, a := range byKey {
		p1 := rowTotals(a.p1)
		p2 := rowTotals(a.p2)
		comps = append(comps, comparisonRow{
			Keys:             a.keys,
			Period1:          p1,
			Period2:          p2,
			ClicksDelta:      round2(p2.Clicks - p1.Clicks),
			ClicksPctChange:  round2(pctChange(p1.Clicks, p2.Clicks)),
			ImpressionsDelta: round2(p2.Impressions - p1.Impressions),
			PositionDelta:    round2(p2.Position - p1.Position),
		})
	}
	// Rank by period-2 clicks, then trim.
	sortSliceStable(comps, func(a, b comparisonRow) bool { return a.Period2.Clicks > b.Period2.Clicks })
	if len(comps) > limit {
		comps = comps[:limit]
	}

	return jsonResult(compareOutput{
		SiteURL:     in.SiteURL,
		Period1:     [2]string{in.Period1Start, in.Period1End},
		Period2:     [2]string{in.Period2Start, in.Period2End},
		Dimensions:  dims,
		Comparisons: comps,
	})
}

// --- gsc_search_by_page_query ---

type pageQueryInput struct {
	SiteURL  string `json:"site_url" jsonschema:"the Search Console property"`
	PageURL  string `json:"page_url" jsonschema:"the exact page URL to break down by query"`
	Days     int    `json:"days,omitempty" jsonschema:"look-back window in days; default 28"`
	RowLimit int    `json:"row_limit,omitempty" jsonschema:"max queries to return; default 20"`
}

type pageQueryOutput struct {
	SiteURL   string         `json:"site_url"`
	PageURL   string         `json:"page_url"`
	StartDate string         `json:"start_date"`
	EndDate   string         `json:"end_date"`
	Totals    metricTotals   `json:"totals"`
	RowCount  int            `json:"row_count"`
	Rows      []analyticsRow `json:"rows"`
}

func (c *Client) searchByPageQuery(ctx context.Context, _ *mcp.CallToolRequest, in pageQueryInput) (*mcp.CallToolResult, pageQueryOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[pageQueryOutput]("%v", err)
	}
	if in.SiteURL == "" || in.PageURL == "" {
		return toolErr[pageQueryOutput]("site_url and page_url are required")
	}
	days := in.Days
	if days <= 0 {
		days = 28
	}
	limit := in.RowLimit
	if limit <= 0 {
		limit = 20
	}
	start, end := defaultRange(days)

	pageFilter := []*searchconsole.ApiDimensionFilterGroup{{
		GroupType: "and",
		Filters:   []*searchconsole.ApiDimensionFilter{{Dimension: "page", Operator: "equals", Expression: in.PageURL}},
	}}

	rows, err := c.runQuery(ctx, in.SiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, Dimensions: []string{"query"},
		DimensionFilterGroups: pageFilter, RowLimit: int64(limit),
	})
	if err != nil {
		return toolErr[pageQueryOutput]("page query breakdown: %v", err)
	}

	built := buildRows([]string{"query"}, rows)
	// Page totals: same filter, no dimensions.
	totalRows, err := c.runQuery(ctx, in.SiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, DimensionFilterGroups: pageFilter, RowLimit: 1,
	})
	if err != nil {
		return toolErr[pageQueryOutput]("page totals: %v", err)
	}
	var totals metricTotals
	if len(totalRows) > 0 {
		totals = rowTotals(totalRows[0])
	}

	return jsonResult(pageQueryOutput{
		SiteURL: in.SiteURL, PageURL: in.PageURL, StartDate: start, EndDate: end,
		Totals: totals, RowCount: len(built), Rows: built,
	})
}

// --- shared numeric/aggregation helpers ---

func keysMap(dims, keys []string) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	m := make(map[string]string, len(keys))
	for i, k := range keys {
		name := "key"
		if i < len(dims) {
			name = dims[i]
		}
		m[name] = k
	}
	return m
}

func rowTotals(r *searchconsole.ApiDataRow) metricTotals {
	if r == nil {
		return metricTotals{}
	}
	return metricTotals{
		Clicks:      r.Clicks,
		Impressions: r.Impressions,
		CTR:         round2(r.Ctr * 100),
		Position:    round2(r.Position),
	}
}

func pct(part, whole float64) float64 {
	if whole == 0 {
		return 0
	}
	return part / whole * 100
}

func pctChange(from, to float64) float64 {
	if from == 0 {
		if to == 0 {
			return 0
		}
		return 100
	}
	return (to - from) / from * 100
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
