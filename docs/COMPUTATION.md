# 技术指标、信号、选股与回测

这些模块只做本地计算，不导入 HTTP/provider 层；可以单独用于已有数据。

## 指标

`indicators` 包提供全部 17 个指标函数：

`CalcSMA`、`CalcEMA`、`CalcWMA`、`CalcMA`、`CalcMACD`、`CalcBOLL`、`CalcKDJ`、
`CalcRSI`、`CalcWR`、`CalcBIAS`、`CalcCCI`、`CalcATR`、`CalcOBV`、`CalcROC`、
`CalcDMI`、`CalcSAR`、`CalcKC`。

- 收盘价序列使用 `[]*float64`；OHLCV 使用 `[]indicators.OHLCV`。nil 表示缺失。
- 每个函数返回 `(结果, error)`；结果与输入长度、下标一致。预热数据不足时输出 nil。
- 除 SMA/EMA/WMA 的必填 period 外，各 Options 的零值表示使用原版默认参数。
  配置 `Periods: []int{}` 表示不计算任何周期；nil 切片表示使用默认周期列表。
- 需要表示显式零精度或零倍数时，用 `indicators.Int(0)` / `indicators.Float(0)`。
- 默认保留三位小数；OBV、ROC、DMI、SAR、KC 按原版保留其原始精度规则。
- NaN/Infinity 输入作为缺失值处理；非法周期/选项返回 INVALID_ARGUMENT。
- ARM64 上用显式 float64 转换固定乘法的舍入时点，防止融合运算改变 JS 的半步舍入结果。

示例：

```go
series := []*float64{
    indicators.Float(10), indicators.Float(12), nil,
    indicators.Float(13), indicators.Float(15),
}
ema, err := indicators.CalcEMA(series, 3, 2)
rsi, err := indicators.CalcRSI(series, indicators.RSIOptions{Periods: []int{2, 3}})
```

`Compute` 一次返回各 bar 的 `indicators.Result`。Options 内各字段为指针，nil 不启用，
如 `MACD: &indicators.MACDOptions{}` 表示启用默认 MACD。`AddIndicators` 将计算结果
附到 OHLCV 副本。两者不会在计算时修改输入。

## K 线指标与预热

```go
bars, err := sdk.Kline.WithIndicators(ctx, "600519", stockgo.KlineIndicatorOptions{
    KlineOptions: stockgo.KlineOptions{StartDate: "20240101", EndDate: "20241231"},
    Indicators: indicators.Options{
        MA: &indicators.MAOptions{Periods: []int{5, 20}},
        MACD: &indicators.MACDOptions{},
    },
})
```

自动识别 CN/HK/US，也可显式传 Market。日/周/月线分别估算预热窗口；A 股优先使用
缓存交易日历，日历不可用时按自然日估算。短序列在可能有更早数据时会重新取完整历史。
递推指标保留稳定预热，OBV 等累计指标在重新取完整历史后保留从原点累计的语义。
指标在预热序列上计算，再裁剪返回日期；LeadingBars 可额外保留窗口前的 bar。
交易日历可通过 Options.TradingCalendarURL 替换，ClearCaches 同时清除该缓存。

## 信号

`signals.CalcSignals` 接收 `[]signals.Bar`（时间戳、收盘价及指标 Result），支持：

- MA / MACD / KDJ 金叉死叉；
- KDJ / RSI 超买超卖（可覆盖阈值）；
- BOLL 收盘突破；SAR 趋势反转。

没有时间戳的当前 bar 不生成事件。MA/RSI 周期必须已在输入结果中计算，否则报错，
不会用“零事件”掩盖周期不匹配。

`sdk.Kline.Signals(ctx, code, stockgo.KlineSignalOptions{...})` 负责拉取数据、计算默认指标、
识别信号，并返回日期/时间戳/收盘价。设置起始日时保留一根前置 bar 用于跨窗口交叉识别，
最终事件仍裁剪在请求日期范围内。

## 链式选股

```go
picks, err := screener.Screen(quotes).
    Where(func(q stockgo.FullQuote) bool { return q.PE != nil && *q.PE < 20 }).
    SortBy(func(q stockgo.FullQuote) *float64 { return &q.Amount }).
    Top(20)
```

默认降序，可传 `screener.Ascending`。排序稳定；nil 和非有限值在两个方向均沉底。
`SortByValue(func(T) any)` 接受原始 JSON 中的数字字符串，严格按整串数值转换，
不会将 `"1.5%"` 当作有效数。Builder 保存错误，通过 Err / Top / ToArray 返回。
入口和返回切片均为副本。

## 回测

```go
report, err := screener.Backtest(screener.BacktestOptions[stockgo.KlineWithIndicators]{
    Klines: bars,
    Fee: screener.Fee{Buy: 0.0003, Sell: 0.0013},
    GetClose: func(b stockgo.KlineWithIndicators) *float64 { return b.Close },
    GetDate: func(b stockgo.KlineWithIndicators) any { return b.Date },
    Strategy: func(b stockgo.KlineWithIndicators, i int, all []stockgo.KlineWithIndicators) screener.StrategySignal {
        return screener.Hold
    },
})
```

Go 使用显式价格/日期访问器替代 JS 的动态属性；GetClose 和 Strategy 必填。
初始资金默认 100000、动用现金比例默认 1；用指针字段设置其他值。
Fee 显式区分买卖费用，`screener.FeeRate(rate)` 可设置双边相同费率。

成交语义与原版一致：同根收盘价执行、仅做多、支持小数股；无效价格上的买卖信号挂起到
下一根有效价格，最新非 hold 意图优先。nil bar 不调用策略。尾部仍持仓时按最后有效价
结算，并将出场记录定位到该 bar；尾部挂起的 sell 视为策略平仓，不标为强制平仓。
手续费采用与现金路径一致的双边复合计算，最大回撤以初始资金为起始峰值。

策略可读取完整 all（包括未来数据），调用方应只访问当前及过去下标；该引擎是与原项目
一致的简化回测，不模拟次根成交、滑点、整手或最低佣金。
