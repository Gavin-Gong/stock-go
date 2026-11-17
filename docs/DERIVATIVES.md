# 期货与期权

所有方法接受 context.Context，使用 SDK 的超时、重试、取消和响应大小限制。
没有浏览器、JSONP script 注入或 Node.js 运行时依赖。

## 国内与全球期货

```go
bars, err := sdk.Futures.Kline(ctx, "RBM", stockgo.FuturesKlineOptions{
    Period: "daily", StartDate: "20261101", EndDate: "20261117",
})
spot, err := sdk.Futures.GlobalSpot(ctx, stockgo.GlobalFuturesSpotOptions{
    PageSize: 20,
})
gold, err := sdk.Futures.GlobalKline(ctx, "GC00Y",
    stockgo.GlobalFuturesKlineOptions{StartDate: "20261101"})
```

Kline / GlobalKline 支持 daily / weekly / monthly，默认起止日期 19700101 / 20500101。
日期接受 YYYYMMDD 和 YYYY-MM-DD，输出 Date 为上游交易日；期货不复权。
返回价格、成交量、金额、振幅、涨跌幅、涨跌额、换手率及 CSV 第 13 列持仓量。
单位与上游品种一致，不把不同合约的手数、吨数或报价币种强制混用。
全球期货跨时区且包含夜盘，保留原交易日期，不臆造统一时区的午夜时间戳。

国内支持固定原版全部 73 个内置品种，覆盖 SHFE / DCE / CZCE / INE / CFFEX / GFEX，
品种查表允许大小写容错，支持 RBM / IFM / TAM 等主连标记。请求保留输入合约的大小写。
`FuturesMarketCode("rb")` 可获取东方财富市场代码。

GlobalKline 默认识别 COMEX / NYMEX / CBOT / LME 等内置品种；
未内置品种可用 GlobalFuturesKlineOptions.MarketCode（*int）指定市场代码。
GlobalSpot 顺序分页取全，默认每页 20、最多 1000 页；支持 PageSize / MaxPages。
非空短页继续读取，达到已知总数或空页结束；安全上限耗尽仍不完整时返回错误。

## 库存

```go
varieties, err := sdk.Futures.InventorySymbols(ctx)
copper, err := sdk.Futures.Inventory(ctx, "cu", stockgo.FuturesInventoryOptions{
    StartDate: "20261101", PageSize: 500,
})
goldStock, err := sdk.Futures.ComexInventory(ctx, "gold")
silverStock, err := sdk.Futures.ComexInventory(ctx, "silver",
    stockgo.ComexInventoryOptions{PageSize: 500})
```

InventorySymbols 返回品种 code / name / marketCode，与原版一致只请求首批 500 项。
Inventory 使用返回的品种代码，筛选时转大写，默认开始日期 2020-10-28；
返回库存及变化值，数量单位由品种决定，不能把全部品种当作同一单位。
ComexInventory 支持 gold / silver，返回吨与盎司两个字段，不自行换算或补空值。
库存历史复用数据中心有序分页，默认每页 500，PageSize 上限 5000。

## 股指及商品期权

```go
tQuote, err := sdk.Options.IndexSpot(ctx, stockgo.OptionIO, "io2612")
indexBars, err := sdk.Options.IndexKline(ctx, "io2612C4000")
commodity, err := sdk.Options.CommoditySpot(ctx, "au", "au2612")
commodityBars, err := sdk.Options.CommodityKline(ctx, "au2612C1000")
cffex, err := sdk.Options.CFFEXQuotes(ctx)
```

IndexSpot 产品为 OptionHO / OptionIO / OptionMO，对应上证50 / 沪深300 / 中证1000。
CommoditySpot 保留原版全部 30 个品种与大小写约定，例如 au、m、SR、TA。
合约需要使用新浪格式：如 io2612C4000；不能把东财 IO2612-C-4000 直接当作相同输入。

T 型报价分别返回 Calls / Puts，包含买卖价、量、持仓、涨跌和代码；
按原版 put 行格式，其 StrikePrice 为 nil，保留该字段缺失约定。
CFFEXQuotes 返回东方财富合约列表，包含行权价、剩余天数、日增仓、前结算等。
该方法保留原版单次请求语义，默认最多 20000 项；
传 CFFEXOptionQuotesOptions{PageSize: 100} 只取首批 100 项，不声称已取全部。

## ETF 期权月份、到期及分钟数据

```go
months, err := sdk.Options.ETFMonths(ctx, stockgo.ETF50)
expiry, err := sdk.Options.ETFExpireDay(ctx, stockgo.ETF50, "2026-12")
minutes, err := sdk.Options.ETFMinute(ctx, "10011430")
daily, err := sdk.Options.ETFDailyKline(ctx, "10011430")
fiveDays, err := sdk.Options.ETF5DayMinute(ctx, "10011430")
```

类别可选 ETF50 / ETF300 / ETF500 / ETFStar50（科创50）/ ETFStarBoard50（科创板50）。
月份 YYYY-MM，纯数字期权代码为 8 位；接口自动添加 CON_OP_ 前缀。
ETFMonths 在月份数组多于一项时删除首个提示项，沿用原版。
到期接口的 remainderDays 为数值且小于 0 时，自动用 XD+类别重试一次；
字符串形式的负数不触发 XD 重试，返回时转为 Go 的数值字段。

分钟数据在同一天内继承首条记录的日期；五日数据逐日重置日期，再按源顺序展平。
不把上一日日期带入下一组缺失日期的数据，也不从本机“今天”填充历史。
过期合约可能只有日 K 线而没有当日或五日分钟数据；示例代码会随合约到期失效。

## 期权龙虎榜

```go
lhb, err := sdk.Options.LHB(ctx, "510050", "20240102")
```

支持六位 CN 标的代码及符号别名，日期自动转 ISO 格式。
保留原接口首批 200 条的语义，包含成交、持仓、净额、变化及占比字段。
上游明确 success:false 时返回 UPSTREAM_ERROR，不将错误当作空数据。

## 请求与兼容

端点可通过 Options.FuturesURLs / OptionURLs 替换，便于代理与离线测试。
所有新浪数据也经过可注入的 HTTPClient，沿用实例重试、超时和取消。
JSONP 只接受 JSON 或单一回调包裹的 JSON，允许前导注释；
不执行表达式、变量赋值或附加脚本。返回的明确状态错误保留为 UPSTREAM_ERROR。
无数据返回非 nil 的空切片，可选数值为 nil，序列化为 null。
