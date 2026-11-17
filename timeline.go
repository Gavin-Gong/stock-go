package stockgo

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strings"

	"github.com/Gavin-Gong/stock-go/internal/markettime"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"github.com/Gavin-Gong/stock-go/symbols"
)

type TimelinePoint struct {
	Time      string  `json:"time"`
	Timestamp *int64  `json:"timestamp"`
	TZ        string  `json:"tz"`
	Price     float64 `json:"price"`
	Volume    float64 `json:"volume"` // Cumulative shares, normalized from source lots when needed.
	Amount    float64 `json:"amount"`
	AvgPrice  float64 `json:"avgPrice"`
}
type Timeline struct {
	Code      string          `json:"code"`
	Date      string          `json:"date"`
	Timestamp *int64          `json:"timestamp"`
	TZ        string          `json:"tz"`
	PreClose  float64         `json:"preClose"`
	Data      []TimelinePoint `json:"data"`
}

// Timeline returns today's A-share intraday points, with cumulative volume in shares.
func (s *QuoteService) Timeline(ctx context.Context, code string) (*Timeline, error) {
	symbol, err := symbols.Normalize(code, symbols.Hint{Market: symbols.CN})
	if err != nil {
		return nil, err
	}
	key, err := symbols.ToTencent(symbol)
	if err != nil {
		return nil, err
	}
	endpoint := endpointQuery(s.timelineURL, url.Values{"code": {key}})
	body, err := s.client.Get(ctx, "tencent", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
		Data map[string]*struct {
			Data struct {
				Data []string `json:"data"`
				Date string   `json:"date"`
			} `json:"data"`
			QT map[string][]string `json:"qt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid timeline response", Provider: "tencent", URL: endpoint, Cause: err}
	}
	if response.Code == nil || *response.Code != 0 {
		return nil, &sdkerrors.Error{Code: sdkerrors.UpstreamError, Message: response.Msg, Provider: "tencent", URL: endpoint}
	}
	out := &Timeline{Code: key, TZ: markettime.CN, Data: []TimelinePoint{}}
	stock := response.Data[key]
	if stock == nil {
		return out, nil
	}
	out.Date = stock.Data.Date
	out.Timestamp = markettime.Parse(out.Date, markettime.CN)
	out.PreClose = num(at(stock.QT[key], 4))
	inLots := false
	for _, line := range stock.Data.Data {
		parts := strings.Fields(line)
		price, volume, amount := num(at(parts, 1)), num(at(parts, 2)), num(at(parts, 3))
		if price > 0 && volume > 0 {
			inLots = amount/volume > price*50
			break
		}
	}
	for _, line := range stock.Data.Data {
		parts := strings.Fields(line)
		clock := at(parts, 0)
		if len(clock) != 4 || !digitsOnly(clock) {
			return nil, &sdkerrors.Error{Code: sdkerrors.Parse, Message: "invalid timeline clock", Provider: "tencent", URL: endpoint}
		}
		clock = clock[:2] + ":" + clock[2:]
		volume, amount := num(at(parts, 2)), num(at(parts, 3))
		if inLots {
			volume *= 100
		}
		avg := 0.0
		if volume > 0 {
			avg = math.Round(amount/volume*100) / 100
		}
		out.Data = append(out.Data, TimelinePoint{Time: clock, Timestamp: markettime.DateAndTime(out.Date, clock, markettime.CN), TZ: markettime.CN, Price: num(at(parts, 1)), Volume: volume, Amount: amount, AvgPrice: avg})
	}
	return out, nil
}
