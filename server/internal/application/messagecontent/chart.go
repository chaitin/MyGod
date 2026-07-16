package messagecontent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
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

	maxChartBodyBytes         = 64 * 1024
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
)

type chartBody struct {
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

type chartHandler struct{}

func (chartHandler) Type() string { return TypeChart }

func (h chartHandler) Validate(raw json.RawMessage) error {
	_, err := h.decodeAndNormalize(raw)
	return err
}

func (h chartHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	body, err := h.decodeAndNormalize(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(body)
}

func (chartHandler) Summary(raw json.RawMessage) (string, error) {
	var body chartBody
	if decodeStrictChartJSON(raw, &body) != nil {
		return "", errors.New("图表消息格式错误")
	}
	return "[图表] " + strings.TrimSpace(body.Title), nil
}

func (h chartHandler) decodeAndNormalize(raw json.RawMessage) (chartBody, error) {
	if len(raw) > maxChartBodyBytes {
		return chartBody{}, errors.New("图表消息不能超过 64 KiB")
	}
	var body chartBody
	if decodeStrictChartJSON(raw, &body) != nil {
		return chartBody{}, errors.New("图表消息格式错误")
	}
	if strings.TrimSpace(body.Type) != h.Type() {
		return chartBody{}, errors.New("消息类型错误")
	}
	body.Type = h.Type()
	body.ChartType = strings.TrimSpace(body.ChartType)
	body.Title = strings.TrimSpace(body.Title)
	body.Description = strings.TrimSpace(body.Description)
	if body.Title == "" {
		return chartBody{}, errors.New("图表标题不能为空")
	}
	if len([]rune(body.Title)) > maxChartTitleLength {
		return chartBody{}, errors.New("图表标题不能超过 16 个字符")
	}
	if body.Description == "" {
		return chartBody{}, errors.New("图表描述不能为空")
	}
	if len([]rune(body.Description)) > maxChartDescriptionLength {
		return chartBody{}, errors.New("图表描述不能超过 128 个字符")
	}
	if len(body.Data) == 0 || string(body.Data) == "null" {
		return chartBody{}, errors.New("图表数据不能为空")
	}
	normalized, err := normalizeChartData(body.ChartType, body.Data)
	if err != nil {
		return chartBody{}, err
	}
	body.Data = normalized
	return body, nil
}

func normalizeChartData(chartType string, raw json.RawMessage) (json.RawMessage, error) {
	switch chartType {
	case chartTypeLine:
		var data chartCartesianData
		if decodeStrictChartJSON(raw, &data) != nil {
			return nil, errors.New("折线图数据格式错误")
		}
		if err := normalizeCartesianData(&data.Labels, data.Series, 2); err != nil {
			return nil, err
		}
		return json.Marshal(data)
	case chartTypeBar:
		var data chartBarData
		if decodeStrictChartJSON(raw, &data) != nil {
			return nil, errors.New("条形图数据格式错误")
		}
		data.Direction = strings.TrimSpace(data.Direction)
		if data.Direction != chartBarDirectionHorizontal && data.Direction != chartBarDirectionVertical {
			return nil, errors.New("条形图方向必须是 horizontal 或 vertical")
		}
		data.Mode = strings.TrimSpace(data.Mode)
		if data.Mode != chartBarModeGrouped && data.Mode != chartBarModeStacked {
			return nil, errors.New("条形图排列方式必须是 grouped 或 stacked")
		}
		if err := normalizeCartesianData(&data.Labels, data.Series, 1); err != nil {
			return nil, err
		}
		if data.Mode == chartBarModeStacked {
			if err := validateStackedTotals(data.Series, len(data.Labels)); err != nil {
				return nil, err
			}
		}
		return json.Marshal(data)
	case chartTypePie:
		var data chartPieData
		if decodeStrictChartJSON(raw, &data) != nil {
			return nil, errors.New("饼图数据格式错误")
		}
		if len(data.Items) < 2 || len(data.Items) > maxChartPieItems {
			return nil, fmt.Errorf("饼图项目数量必须在 2 到 %d 之间", maxChartPieItems)
		}
		seen := map[string]struct{}{}
		total := 0.0
		for index := range data.Items {
			item := &data.Items[index]
			item.Name = strings.TrimSpace(item.Name)
			if err := validateChartName(item.Name, "饼图项目名称"); err != nil {
				return nil, err
			}
			if _, ok := seen[item.Name]; ok {
				return nil, errors.New("饼图项目名称不能重复")
			}
			seen[item.Name] = struct{}{}
			if !finiteNumber(item.Value) || item.Value <= 0 || item.Value > maxChartValue {
				return nil, fmt.Errorf("饼图数值必须大于 0 且不能超过 %.0f", float64(maxChartValue))
			}
			total += item.Value
		}
		if !finiteNumber(total) {
			return nil, errors.New("饼图数值总和必须是有限数字")
		}
		return json.Marshal(data)
	case chartTypeRadar:
		var data chartRadarData
		if decodeStrictChartJSON(raw, &data) != nil {
			return nil, errors.New("雷达图数据格式错误")
		}
		if len(data.Axes) < minChartRadarAxes || len(data.Axes) > maxChartRadarAxes {
			return nil, fmt.Errorf("雷达图维度数量必须在 %d 到 %d 之间", minChartRadarAxes, maxChartRadarAxes)
		}
		seen := map[string]struct{}{}
		for index := range data.Axes {
			axis := &data.Axes[index]
			axis.Name = strings.TrimSpace(axis.Name)
			if err := validateChartName(axis.Name, "雷达图维度名称"); err != nil {
				return nil, err
			}
			if _, ok := seen[axis.Name]; ok {
				return nil, errors.New("雷达图维度名称不能重复")
			}
			seen[axis.Name] = struct{}{}
			if !finiteNumber(axis.Max) || axis.Max <= 0 || axis.Max > maxChartValue {
				return nil, fmt.Errorf("雷达图维度最大值必须大于 0 且不能超过 %.0f", float64(maxChartValue))
			}
		}
		if err := normalizeChartSeries(data.Series, len(data.Axes), false); err != nil {
			return nil, err
		}
		for _, series := range data.Series {
			for index, value := range series.Values {
				if value == nil {
					return nil, errors.New("雷达图数值不能为空")
				}
				if *value < 0 || *value > data.Axes[index].Max {
					return nil, errors.New("雷达图数值必须在 0 和对应维度最大值之间")
				}
			}
		}
		return json.Marshal(data)
	default:
		return nil, errors.New("图表类型必须是 line、bar、pie 或 radar")
	}
}

func normalizeCartesianData(labels *[]string, series []chartSeries, minLabels int) error {
	if len(*labels) < minLabels || len(*labels) > maxChartLabels {
		return fmt.Errorf("图表标签数量必须在 %d 到 %d 之间", minLabels, maxChartLabels)
	}
	for index := range *labels {
		label := strings.TrimSpace((*labels)[index])
		if err := validateChartName(label, "图表标签"); err != nil {
			return err
		}
		(*labels)[index] = label
	}
	if len(*labels)*len(series) > maxChartCartesianPoints {
		return fmt.Errorf("图表数据点不能超过 %d 个", maxChartCartesianPoints)
	}
	return normalizeChartSeries(series, len(*labels), true)
}

func normalizeChartSeries(series []chartSeries, valueCount int, allowNull bool) error {
	if len(series) < 1 || len(series) > maxChartSeries {
		return fmt.Errorf("图表系列数量必须在 1 到 %d 之间", maxChartSeries)
	}
	seen := map[string]struct{}{}
	for index := range series {
		current := &series[index]
		current.Name = strings.TrimSpace(current.Name)
		if err := validateChartName(current.Name, "图表系列名称"); err != nil {
			return err
		}
		if _, ok := seen[current.Name]; ok {
			return errors.New("图表系列名称不能重复")
		}
		seen[current.Name] = struct{}{}
		if len(current.Values) != valueCount {
			return errors.New("图表系列数值数量必须与标签或维度数量一致")
		}
		hasValue := false
		for _, value := range current.Values {
			if value == nil {
				if allowNull {
					continue
				}
				return errors.New("图表系列数值不能为空")
			}
			if !finiteNumber(*value) || math.Abs(*value) > maxChartValue {
				return fmt.Errorf("图表系列数值绝对值不能超过 %.0f", float64(maxChartValue))
			}
			hasValue = true
		}
		if !hasValue {
			return errors.New("图表系列至少需要一个有效数值")
		}
	}
	return nil
}

func validateStackedTotals(series []chartSeries, valueCount int) error {
	for valueIndex := 0; valueIndex < valueCount; valueIndex++ {
		positiveTotal, negativeTotal := 0.0, 0.0
		for _, current := range series {
			value := current.Values[valueIndex]
			if value == nil {
				continue
			}
			if *value >= 0 {
				positiveTotal += *value
			} else {
				negativeTotal += *value
			}
		}
		if !finiteNumber(positiveTotal) || !finiteNumber(negativeTotal) {
			return errors.New("堆叠条形图数值总和必须是有限数字")
		}
	}
	return nil
}

func validateChartName(value, field string) error {
	if value == "" {
		return errors.New(field + "不能为空")
	}
	if len([]rune(value)) > maxChartLabelLength {
		return fmt.Errorf("%s不能超过 %d 个字符", field, maxChartLabelLength)
	}
	return nil
}

func finiteNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func decodeStrictChartJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("消息体包含多余内容")
	}
	return nil
}
