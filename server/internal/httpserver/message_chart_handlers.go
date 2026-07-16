package httpserver

import (
	"context"
	"encoding/json"

	messagecontentapp "app/internal/application/messagecontent"
)

const (
	chartTypeLine  = "line"
	chartTypeBar   = "bar"
	chartTypePie   = "pie"
	chartTypeRadar = "radar"

	chartBarDirectionHorizontal = "horizontal"
	chartBarDirectionVertical   = "vertical"
	chartBarModeGrouped         = "grouped"
	chartBarModeStacked         = "stacked"

	maxChartMessageBodyBytes  = 64 * 1024
	maxChartTitleLength       = 16
	maxChartDescriptionLength = 128
	maxChartLabelLength       = 64
	maxChartLabels            = 100
	maxChartSeries            = 5
	maxChartValue             = 1_000_000_000_000_000
	maxChartCartesianPoints   = maxChartLabels * maxChartSeries
	maxChartPieItems          = 5
	minChartRadarAxes         = 3
	maxChartRadarAxes         = 12
	messageTypeChart          = "chart"
)

type chartMessageBody struct {
	Type        string          `json:"type"`
	ChartType   string          `json:"chart_type"`
	Title       string          `json:"title"`
	Data        json.RawMessage `json:"data"`
	Description string          `json:"description"`
}

type chartCartesianData struct {
	Labels []string      `json:"labels"`
	Series []chartSeries `json:"series"`
}

type chartBarData struct {
	Direction string        `json:"direction"`
	Mode      string        `json:"mode"`
	Labels    []string      `json:"labels"`
	Series    []chartSeries `json:"series"`
}

type chartSeries struct {
	Name   string     `json:"name"`
	Values []*float64 `json:"values"`
}

type chartPieData struct {
	Items []chartPieItem `json:"items"`
}

type chartPieItem struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type chartRadarData struct {
	Axes   []chartRadarAxis `json:"axes"`
	Series []chartSeries    `json:"series"`
}

type chartRadarAxis struct {
	Name string  `json:"name"`
	Max  float64 `json:"max"`
}

// chartMessageBodyHandler remains as a compatibility adapter for the legacy
// package-level tests. Production message handling uses messagecontent.Service.
type chartMessageBodyHandler struct{}

func (chartMessageBodyHandler) Type() string {
	return messageTypeChart
}

func (chartMessageBodyHandler) Validate(raw json.RawMessage) error {
	_, err := messagecontentapp.NewService(messagecontentapp.Dependencies{}).Normalize(context.Background(), raw)
	return err
}

func (chartMessageBodyHandler) Normalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	return messagecontentapp.NewService(messagecontentapp.Dependencies{}).Normalize(ctx, raw)
}

func (chartMessageBodyHandler) Summary(raw json.RawMessage) (string, error) {
	_, summary, err := messagecontentapp.NewService(messagecontentapp.Dependencies{}).
		Finalize(context.Background(), raw)
	return summary, err
}
