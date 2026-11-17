// Package signals detects stock-sdk's indicator events without network access.
package signals

import (
	"github.com/Gavin-Gong/stock-go/indicators"
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"strconv"
)

type Bar struct {
	Timestamp *int64   `json:"timestamp"`
	Close     *float64 `json:"close"`
	indicators.Result
}
type MACross struct{ Fast, Slow int }
type Thresholds struct{ Overbought, Oversold *float64 }
type RSIOptions struct {
	Period int
	Thresholds
}
type Options struct {
	MA        *MACross
	MACD      bool
	KDJ       *Thresholds
	RSI       *RSIOptions
	BOLL, SAR bool
}
type Signal struct {
	Type   string             `json:"type"`
	At     int64              `json:"at"`
	Index  int                `json:"index"`
	Detail map[string]float64 `json:"detail,omitempty"`
}

func invalid(message string) error {
	return &sdkerrors.Error{Code: sdkerrors.InvalidArgument, Message: message}
}
func cross(pf, ps, cf, cs *float64) int {
	if pf == nil || ps == nil || cf == nil || cs == nil {
		return 0
	}
	if *pf <= *ps && *cf > *cs {
		return 1
	}
	if *pf >= *ps && *cf < *cs {
		return -1
	}
	return 0
}
func CalcSignals(bars []Bar, o Options) ([]Signal, error) {
	fast, slow := "", ""
	period := 6
	if o.MA != nil {
		if o.MA.Fast <= 0 || o.MA.Slow <= o.MA.Fast {
			return nil, invalid("MA fast must be positive and less than slow")
		}
		fast = "ma" + strconv.Itoa(o.MA.Fast)
		slow = "ma" + strconv.Itoa(o.MA.Slow)
		found := len(bars) == 0
		for _, b := range bars {
			_, f := b.MA[fast]
			_, s := b.MA[slow]
			found = found || (f && s)
		}
		if !found {
			return nil, invalid("requested MA periods were not computed")
		}
	}
	if o.RSI != nil {
		if o.RSI.Period != 0 {
			period = o.RSI.Period
		}
		if period < 1 {
			return nil, invalid("RSI period must be positive")
		}
		found := len(bars) == 0
		for _, b := range bars {
			_, ok := b.RSI["rsi"+strconv.Itoa(period)]
			found = found || ok
		}
		if !found {
			return nil, invalid("requested RSI period was not computed")
		}
	}
	out := []Signal{}
	for i := 1; i < len(bars); i++ {
		cur, prev := bars[i], bars[i-1]
		if cur.Timestamp == nil {
			continue
		}
		push := func(kind string, detail map[string]float64) {
			out = append(out, Signal{kind, *cur.Timestamp, i, detail})
		}
		emitCross := func(kind string, n int, detail map[string]float64) {
			if n == 1 {
				push(kind+"_golden_cross", detail)
			} else if n == -1 {
				push(kind+"_death_cross", detail)
			}
		}
		if o.MA != nil {
			emitCross("ma", cross(prev.MA[fast], prev.MA[slow], cur.MA[fast], cur.MA[slow]), map[string]float64{"fast": float64(o.MA.Fast), "slow": float64(o.MA.Slow)})
		}
		if o.MACD && cur.MACD != nil && prev.MACD != nil {
			emitCross("macd", cross(prev.MACD.DIF, prev.MACD.DEA, cur.MACD.DIF, cur.MACD.DEA), nil)
		}
		if o.KDJ != nil && cur.KDJ != nil {
			if prev.KDJ != nil {
				emitCross("kdj", cross(prev.KDJ.K, prev.KDJ.D, cur.KDJ.K, cur.KDJ.D), nil)
			}
			bought, sold := 80.0, 20.0
			if o.KDJ.Overbought != nil {
				bought = *o.KDJ.Overbought
			}
			if o.KDJ.Oversold != nil {
				sold = *o.KDJ.Oversold
			}
			if k := cur.KDJ.K; k != nil {
				if *k > bought {
					push("kdj_overbought", map[string]float64{"k": *k})
				} else if *k < sold {
					push("kdj_oversold", map[string]float64{"k": *k})
				}
			}
		}
		if o.RSI != nil {
			bought, sold := 70.0, 30.0
			if o.RSI.Overbought != nil {
				bought = *o.RSI.Overbought
			}
			if o.RSI.Oversold != nil {
				sold = *o.RSI.Oversold
			}
			if r := cur.RSI["rsi"+strconv.Itoa(period)]; r != nil {
				if *r > bought {
					push("rsi_overbought", map[string]float64{"rsi": *r})
				} else if *r < sold {
					push("rsi_oversold", map[string]float64{"rsi": *r})
				}
			}
		}
		if o.BOLL && cur.BOLL != nil && cur.Close != nil {
			if cur.BOLL.Upper != nil && *cur.Close > *cur.BOLL.Upper {
				push("boll_break_upper", nil)
			} else if cur.BOLL.Lower != nil && *cur.Close < *cur.BOLL.Lower {
				push("boll_break_lower", nil)
			}
		}
		if o.SAR && cur.SAR != nil && prev.SAR != nil && cur.SAR.Trend != nil && prev.SAR.Trend != nil && *cur.SAR.Trend != *prev.SAR.Trend {
			if *cur.SAR.Trend == 1 {
				push("sar_reversal_up", nil)
			} else {
				push("sar_reversal_down", nil)
			}
		}
	}
	return out, nil
}
