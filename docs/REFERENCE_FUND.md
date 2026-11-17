# 交易日历、参考数据、北向与基金深度数据

## 日历与市场状态

```go
calendar, err := sdk.Reference.TradingCalendar(ctx)
isTrading, err := sdk.Calendar.IsTradingDay(ctx, "20240102")
next, err := sdk.Calendar.NextTradingDay(ctx, "2024-01-02")
prev, err := sdk.Calendar.PrevTradingDay(ctx, "2024-01-02")
status, err := sdk.Calendar.MarketStatus(symbols.US, time.Now())
```

日历支持省略日期（上海时区今天），接受 YYYYMMDD、YYYY-MM-DD、RFC3339。
Next / Prev 严格排除输入当天，超出日历覆盖范围返回 INVALID_ARGUMENT。
日历缓存 12 小时、按实例隔离；返回切片为副本，ClearCaches 清空该缓存。

MarketStatus 使用市场时区和常规周一至周五时段，返回 pre_market / open /
lunch_break / after_hours / closed，包含美东夏令时。与原版一致，它不检查节假日、
半日市或临时休市；判断 A 股具体日期是否交易应使用 IsTradingDay。

## 分红参考数据与通用数据中心

```go
dividends, err := sdk.Reference.DividendDetail(ctx, "600519.SH")
result, err := sdk.Datacenter.Query(ctx, stockgo.DatacenterQuery{
    ReportName: "RPT_SHAREBONUS_DET",
    Filter: `(SECURITY_CODE="600519")`,
    SortColumns: "REPORT_DATE", SortTypes: "-1",
})
```

股票分红返回报告日期、送转比例、每股派息、收益率、登记/除息/派息日期及财务参考值。
未知日期或缺失数值保留 nil，不从其他字段推算。

Datacenter.Query 是高级报表入口，Data 保留原始字段名与 JSON 值。默认每页 500、起始页 1、
并发波次 3、最多 1000 页，保持页序；FetchAllPages: stockgo.Bool(false) 只请求起始页。
支持列选择、筛选、排序、行情列聚合及 ExtraParams。
达到 maxPages 且未完整获取时返回错误；上游明确 success:false 返回 UPSTREAM_ERROR，
不会把“报表不存在/服务器繁忙”伪装为空数据。无结果对象且未报告错误时仍返回空数组。
可用 Options.DatacenterURL 替换端点。

## 北向与南向

```go
south, err := sdk.Northbound.Minute(ctx, stockgo.South)
summary, err := sdk.Northbound.Summary(ctx)
history, err := sdk.Northbound.History(ctx, stockgo.NorthboundHistoryOptions{
    Direction: stockgo.North, StartDate: "20240102", EndDate: "20240105",
})
rank, err := sdk.Northbound.HoldingRank(ctx, stockgo.NorthboundHoldingRankOptions{
    Market: "shanghai", Period: "5day", Date: "2024-01-05",
})
individual, err := sdk.Northbound.Individual(ctx, "600519", stockgo.NorthboundHistoryOptions{
    StartDate: "20260101",
})
```

分时金额沿用源接口万元口径；数据中心报表金额保留源字段原值。
缺失的成交净买额等字段为 nil，不填零，也不从成交额反推。

接口兼容以下字段与数据范围：

- 旧历史筛选 BOARD_TYPE 被移除时，自动使用 MUTUAL_TYPE=005（北向合计）或 006（南向合计）。
  不将 001/003、002/004 的分通道记录与合计重复相加；支持旧格式，兼容状态按实例记录。
- 个股持仓比例支持旧字段及当前 FREE_SHARES_RATIO / TOTAL_SHARES_RATIO。
- 当前 600519 个股报表只返回最近季度记录；请求 2024 年日度区间可能报告无数据，
  SDK 不捏造缺失历史或改用不同时间范围。
- **持股排名 RPT_MUTUAL_STOCK_NORTHSTA 当前返回“服务器繁忙”。**
  调用时保留 UPSTREAM_ERROR，不使用其他统计口径替代。

分时端点通过 Options.NorthboundMinuteURL 配置。

## 基金净值、排名与档案

```go
nav, err := sdk.Fund.NavHistory(ctx, "110011")
rankHistory, err := sdk.Fund.RankHistory(ctx, "110011")
profile, err := sdk.Fund.Profile(ctx, "110011")
dividends, err := sdk.Fund.DividendList(ctx, stockgo.FundDividendOptions{
    Year: 2024, AllPages: true, Code: "110011",
})
```

净值与累计净值、排名与百分位按原始时间戳对齐；无效时间戳行丢弃，不生成“今天”的幽灵记录。
数值整串解析，`"1.2x"`、`"--"` 等返回 nil。时间戳转换使用上海日期，并保留原始毫秒值。

Profile 包含股票/债券重仓、资产配置、仓位、基金经理及评分、持有人结构、规模变化、
申购赎回、阶段收益与同类基金。单个字段缺失沿用原版各项默认值；未知日期为 nil。

基金分红按年分页，Code 在客户端过滤。要完整获取某基金某年的记录，应设置 AllPages:true；
当前页默认 1，所有页汇总的 CurrentPage 为 -1。默认 MaxPages=1000，超限显式报错。

基金脚本仅扫描顶层 var / let / const 的 JSON 字面量，不执行远程 JavaScript；
注释、字符串中的伪声明、函数体声明及可执行表达式不会成为数据。未找到任何请求变量时
返回 UPSTREAM_EMPTY，而不是“成功但只有空档案”。没有 Node.js 或 JS 运行时依赖。

## 主题基金

```go
themes, err := sdk.Fund.Theme.GetThemeList(ctx, stockgo.ThemeListOptions{
    Page: 1, PageSize: 20, Category: "2",
})
funds, err := sdk.Fund.Theme.GetThemeFunds(ctx, "BK0475", stockgo.ThemeFundsOptions{
    Page: 1, PageSize: 20, SortColumn: "SYL_1N", Sort: "desc",
})
```

保留原版 Data / pageinfo / fundinfo 格式，同时支持当前 Datas、INDEXCODE/INDEXNAME、
FCODE/SHORTNAME、RZDF/DWJZ 和 TotalCount 格式。

当前主题目录不含涨幅和区间回报，相关数值返回 nil，结果标记 CatalogOnly=true；
此模式按目录顺序在客户端筛选行业/概念并分页，不宣称按缺失回报字段排序。
主题下基金则使用接口返回的排名、净值及回报，并保留总数。
原 SDK 已下线的 HotThemes 未重新暴露为可用方法。

基金端点通过 Options.FundURLs（PingzhongBase / Dividend / ThemeBase）配置。
