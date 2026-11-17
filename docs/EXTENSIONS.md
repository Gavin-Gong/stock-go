# 筹码分布、板块与资金流

所有网络接口接受 context，支持 HTTP 注入和自定义端点。

## 筹码分布

```go
chips, err := sdk.Chips.CN(ctx, "600519", stockgo.ChipRequestOptions{
    Days: 30,
    Range: indicators.Int(120),
    IncludeHistogram: indicators.HistogramLast,
})
// sdk.Chips.HK(ctx, "00700", ...)
// sdk.Chips.US(ctx, "105.AAPL", ...)
```

Days 零值默认 90；Range 的 nil 默认 120，显式 `indicators.Int(0)` 从完整历史累计。
IncludeHistogram 支持空字符串（不附带）、HistogramLast、HistogramAll。Go 使用枚举替代
原版 boolean/string 联合选项。Decimals 只影响比例，成本价格固定两位小数。

每项返回获利比例、中位数成本（原版“平均成本”口径）、90%/70% 成本区间与集中度。
可选 Histogram 提供 150 个价格档及筹码比例。没有有效 OHLC 或窗口换手均为零时返回
空数值，保持日期对齐。负换手夹为 0，大于 100% 夹为 100%；缺失 OHLC 的 bar 整体跳过。

算法也可独立使用，无网络依赖：

```go
result, err := indicators.CalcChipDistribution(bars, indicators.ChipOptions{
    Range: indicators.Int(120), Tail: indicators.Int(10),
    IncludeHistogram: indicators.HistogramAll,
})
```

这里 bars 是 `[]indicators.ChipBar`，换手率单位为百分数。
默认逐日返回；Tail 为 nil 返回全部，显式 0 或负数返回空结果。
价格两位小数和筹码 12 位有效数字按 JS 十进制格式化的规则舍入，包含半步边界。

SDK 先取暖机段，再输出最近 Days 项；数据不足且可能有更早历史时重新取完整历史。
A 股计算始终只用东财，不会使用缺少换手率的腾讯/新浪备用源，也不修改共享 Kline 配置。

## 行业与概念板块

```go
boards, err := sdk.Board.Industry.List(ctx)
page, err := sdk.Board.Concept.List(ctx, stockgo.PageOptions{Page: 1, PageSize: 20})
spot, err := sdk.Board.Industry.Spot(ctx, "银行")
constituents, err := sdk.Board.Industry.Constituents(ctx, "BK0475")
klines, err := sdk.Board.Industry.Kline(ctx, "BK0475", stockgo.KlineOptions{
    Period: "weekly", StartDate: "20240101", EndDate: "20241231",
})
minutes, err := sdk.Board.Concept.MinuteKline(ctx, "BK0475", "5")
```

Industry 和 Concept 均支持 List、Spot、Constituents、Kline、MinuteKline。
输入可以是 BK 代码、`90.BK...` 或精确板块名称。名称映射按实例与板块类型隔离，
缓存一小时，空列表和错误不缓存；ClearCaches 同时清除名称缓存。

列表按涨跌幅降序稳定排序并重编 rank；Spot 保留原版 10 个中文指标名，并对价格、
涨跌幅等原始整数除以 100。行情缺失返回 nil，不将缺失估值当成零。

板块历史 K 线默认不复权，与个股默认前复权不同。分钟线默认 5 分钟、固定前复权；
1 分钟走分时接口且不复权，其中 Price 对应 CSV 第 8 列。Go 复用 KlineBar 并补时间元信息，
BoardMinuteBar 的 Price 在非分时结果中为 nil。

端点由 Options.IndustryURLs / ConceptURLs 配置。

## 资金流

```go
snapshot, err := sdk.Quotes.FundFlow(ctx, []string{"600519", "sz000001"})
// 等价入口 sdk.FundFlow.Realtime
history, err := sdk.FundFlow.Individual(ctx, "600519", stockgo.FundFlowOptions{Period: "weekly"})
market, err := sdk.FundFlow.Market(ctx)
rank, err := sdk.FundFlow.Rank(ctx, stockgo.FundFlowRankOptions{
    Indicator: "5day", PageOptions: stockgo.PageOptions{Page: 1, PageSize: 20},
})
sector, err := sdk.FundFlow.SectorRank(ctx, stockgo.FundFlowRankOptions{
    Indicator: "today", SectorType: "concept",
    PageOptions: stockgo.PageOptions{Page: 1, PageSize: 20},
})
sectorHistory, err := sdk.FundFlow.SectorHistory(ctx, "BK0475")
```

实时快照使用 push2delay 个股扩展字段，每个 secid 单独请求，最大并发 5。
符号归一后去重；无法映射的代码和上游无数据的项目跳过。金额保持原版万元口径，
净占比按净额/成交额计算，时间戳秒转换为 UTC 毫秒，并使用上海时区生成日期。

历史支持 daily / weekly / monthly，净流入金额为元。排名支持 today / 3day / 5day / 10day，
各周期使用各自的排序及字段映射。SectorRank 支持 industry / concept / region。
特殊指数不能查询个股资金流；返回 INVALID_ARGUMENT。
端点由 Options.FundFlowURLs 配置。

## 分页约定

Page / PageSize 均为零时自动取全量；任意一个非零时只取一页。默认 page=1、pageSize=100，
pageSize 上限 100。自动分页按有界并发波次推进，合并时保留页顺序，空页之后不再启动新波次。
板块成分股若遇服务端暗中缩小每页数量，会在已知页数之后补取直到满足 total 或遇空页。
默认最大 1000 页，可通过 MaxPages 修改；触及上限且结果不完整时显式报错，不冒充完整数据。
Concurrency 默认为 3，所有分页和实时快照均响应取消，失败时取消同批在途请求。
