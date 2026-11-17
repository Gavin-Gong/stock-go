# 龙虎榜、大宗交易、两融与异动

所有请求接受 context，支持取消，使用 SDK 的实例请求客户端。
报表使用 Options.DatacenterURL，股池/盘口异动使用 Options.TopicBaseURL（默认 push2ex.eastmoney.com）。
数值缺失返回 nil，空列表返回非 nil 的空切片。

## 龙虎榜

```go
rows, err := sdk.DragonTiger.Detail(ctx, stockgo.DragonTigerDateOptions{
    StartDate: "20240102", EndDate: "20240105",
})
stats, err := sdk.DragonTiger.StockStats(ctx, stockgo.DragonMonth)
branches, err := sdk.DragonTiger.BranchRank(ctx, stockgo.DragonYear)
seats, err := sdk.DragonTiger.SeatDetail(ctx, "600519.SH", "20240102")
```

Institution 与 Detail 使用相同日期选项，起止日期必填。
统计周期可省略，默认为一个月；可选 DragonMonth / DragonQuarter / DragonHalfYear / DragonYear。
席位按买入榜、卖出榜分别取全分页，返回买入在前、卖出在后；缺失排名按各榜跨页连续编号。
金额单位元，占比使用百分数，日期返回 YYYY-MM-DD。

## 大宗交易与融资融券

```go
market, err := sdk.BlockTrade.MarketStat(ctx)
detail, err := sdk.BlockTrade.Detail(ctx, stockgo.BlockTradeDateOptions{
    StartDate: "20240102", EndDate: "20240105",
})
daily, err := sdk.BlockTrade.DailyStat(ctx, stockgo.BlockTradeDateOptions{
    StartDate: "20240102", EndDate: "20240105",
})
accounts, err := sdk.Margin.AccountInfo(ctx)
targets, err := sdk.Margin.TargetList(ctx, "20240102")
```

大宗日期范围两端均可省略，返回全部可用分页；较长时间范围请设置 context 截止时间。
TargetList 可省略日期，兼容当前报表时先查最新日期，再取该日全部证券，避免误取全部历史。
大宗金额统一元、成交量统一股、溢价率为百分数。
MarginTargetItem.LoanBalance 是融券余量（股），AccountInfo.LoanBalance 是融券余额（元）。

## 股池及全市场异动

```go
pool, err := sdk.MarketEvent.ZTPool(ctx, stockgo.ZTPoolOptions{
    Type: stockgo.PoolLimitUp, Date: "20240102",
})
changes, err := sdk.MarketEvent.StockChanges(ctx, stockgo.StockChangesOptions{
    Types: []stockgo.StockChangeType{stockgo.AllChanges},
})
page, err := sdk.MarketEvent.StockChanges(ctx, stockgo.StockChangesOptions{
    Types: []stockgo.StockChangeType{stockgo.LargeBuy, stockgo.LargeSell},
    Page: 2, PageSize: 100,
})
boards, err := sdk.MarketEvent.BoardChanges(ctx)
```

股池可选 PoolLimitUp / PoolYesterday / PoolStrong / PoolSubNew / PoolBroken / PoolLimitDown；
省略选项时为涨停池、上海时区今天。价格由上游千分位转换为元，封板时间返回 HH:MM:SS。

StockChanges 未指定 Types（nil）默认大笔买入，显式空切片返回参数错误。
Types 可选择 22 类枚举，单独 AllChanges 表示全部类型；不支持与其他类型混用。
不设 Page/PageSize 时每页 5000、顺序翻页，默认最多 10 页；可通过 MaxPages 调整。
达到安全上限仍未收完时返回 UPSTREAM_ERROR，不静默返回前缀。
任设 Page/PageSize 则只请求一页；页码从 1 开始，默认每页 100，上限 5000。
多类型请求保留裸逗号，以避免上游将编码后的逗号误读为单类型。
响应未知类型保留 TypeCode、ChangeType=unknown 和空标签，包括 constructor 等特殊字符串。
单类型响应缺 t 时用所请求的类型，多类型缺 t 则为 unknown。

## 个股异动与历史覆盖

```go
events, err := sdk.MarketEvent.IndividualChanges(ctx, "sh600519", "20240102")
day, err := sdk.MarketEvent.IndividualDay(ctx, "600519")
history, err := sdk.MarketEvent.IndividualChangesHistory(ctx, "600519", 7)
```

只支持 A 股个股，在请求日历前验证资产类型。IndividualDay 额外返回日期、名称与 Available：
false 表示上游无该日数据；true 且 Changes 为空表示该日存在但没有事件。
单日日期以服务端回显为准，非交易日可能回退至上一交易日。

History 默认最近 7 个自然日，允许 1–60；截止上海时区今天，按交易日历枚举、最多并发 4，结果日期升序。
上游只保留最近数周且存在不连续空洞，Coverage.AvailableFrom 不是连续覆盖保证。
若回显日期与请求日期不一致，将请求日期记为 Available=false，不重复计入回退日的事件。
Stats 使用原始类型码为键，未知码也计数；缺码使用 unknown。
任一请求失败则整次返回错误，无数据标记不会掩盖请求失败。

## 监管异动

```go
unusual, err := sdk.MarketEvent.UnusualFluctuation(ctx,
    stockgo.UnusualFluctuationOptions{
        StartDate: "20261101", EndDate: "20261117", Triggered: stockgo.Bool(true),
    })
```

Date 和 StartDate/EndDate 互斥；Triggered 未设时返回已触发和逼近阈值两类。
日期带上海时区的 UTC 毫秒 Timestamp，偏离值、窗口天数、方向和规则沿用原报表。
当前报表历史覆盖有限，较早日期可能返回上游“返回数据为空”错误。

## 上游字段兼容

固定原版映射与参数保持兼容，只有明确的“报表配置/列不存在”才使用当前模式：

| 调用 | 原版问题 | 当前模式及处理 |
| --- | --- | --- |
| BranchRank | RPT_BILLBOARD_TRADEDETAILS 已删除 | RPT_OPERATEDEPT_LIST_STATISTICS，STATISTICSCYCLE，同周期上榜统计 |
| SeatDetail | BUY_AMT_REAL/SELL_AMT_REAL 排序列不存在 | 同报表 BUY/SELL/NET，TOTAL_BUYRIO/TOTAL_SELLRIO 转为百分数 |
| BlockTrade.Detail | RPT_BLOCK_TRADE_DETAIL 已删除 | RPT_DATA_BLOCKTRADE，买卖方名称新字段，PREMIUM_RATIO 转为百分数 |
| BlockTrade.DailyStat | RPT_BLOCK_TRADE_STA 已删除 | RPT_BLOCKTRADE_STAINCLUDE，万元/万股转为元/股 |
| Margin.TargetList | RPT_MARGIN_TRADE_DETAIL 已删除 | RPTA_WEB_RZRQ_GGMX，DATE/SCODE/RZYE/RQYL 等映射 |

新版大宗日汇总仅提供溢价/折价笔数，没有对应成交额；PremiumAmount/DiscountAmount 保留 nil。
不把笔数充当金额，也不根据平均价反推；可从逐笔明细自行汇总。
兼容请求若失败则返回该错误；超时、取消、服务器繁忙不会触发模式切换。
