# stock-go

要求 Go 1.23 或以上。

支持行情、搜索、当日分时、K 线、代码列表、批量查询、技术指标、信号、选股回测、筹码分布、板块、资金流、基金深度数据、日历、龙虎榜、大宗交易、两融、市场异动、期货期权和符号解析。

## 命令行

安装到 Go 的可执行目录，之后直接调用：

```sh
go install github.com/Gavin-Gong/stock-go/cmd/stock-go@latest
stock-go quote 600519 00700 AAPL
stock-go kline 600519 --period weekly --limit 10 -f csv
stock-go --help
```

也可以不安装，在任意目录直接运行：

```sh
go run github.com/Gavin-Gong/stock-go/cmd/stock-go@latest quote 600519
go run github.com/Gavin-Gong/stock-go/cmd/stock-go@latest --help
```

两种方式均由 Go 自动下载和编译，无需手动 build。
`go install` 后确保 GOBIN（默认 GOPATH/bin）在 PATH 中；重新执行安装命令即可更新。
安装目录和 PATH 设置见[命令行文档](docs/CLI.md)。

支持 94 个方法、10 个快捷命令和 JSON / table / CSV 输出。
完整参数、退出码及原始调用见[命令行文档](docs/CLI.md)。

`stock-go mcp` 提供 MCP stdio：默认 26 个 core 工具和 4 个提示模板，
full 模式为 93 个工具和 7 个模板。配置见 [MCP 文档](docs/MCP.md)。

公共缓存、时间、响应解析、JSONP/JS 变量获取、财经外链及泛型工具用法见 [辅助工具文档](docs/HELPERS.md)。

## 行情与搜索

```go
import (
    "context"
    "time"
    stockgo "github.com/Gavin-Gong/stock-go"
)

sdk, err := stockgo.New(stockgo.Options{Timeout: 10 * time.Second})
if err != nil { panic(err) }
ctx := context.Background()
quotes, err := sdk.Quotes.CN(ctx, []string{"600519", "sz000001"})
// sdk.Quotes.CNSimple / HK / US / Fund / LargeOrder 同样接受 ctx 和代码切片
// sdk.Quotes.Timeline(ctx, "600519")
// sdk.Search(ctx, "茅台")
```

可运行示例：`go run ./examples/quotes cn 600519`（也支持 `hk`、`us`、`fund`）。

支持注入 `HTTPClient`、自定义请求端点及超时、重试次数、响应大小限制。
SDK 不修改全局 HTTP 客户端。可选数值和无法解析的时间戳以 JSON `null` 表示。
行情数量、成交额等保留原数据源单位；分时成交量统一为股。

## K 线、代码列表与批量

```go
// 日/周/月：CN、HK、US 接口一致。
bars, err := sdk.Kline.CN(ctx, "600519", stockgo.KlineOptions{
    Period: "daily", Adjust: stockgo.AdjustQFQ,
    StartDate: "20240101", EndDate: "20241231",
})
// 不复权请显式使用 AdjustNone；后复权使用 AdjustHFQ。
// bars 每项的 Source 和 Adjust 标识实际数据源及复权状态。

minutes, err := sdk.Kline.USMinute(ctx, "AAPL", stockgo.MinuteOptions{
    Period: "5", StartDate: "2024-07-09 09:30", EndDate: "2024-07-09 16:00",
})
// CNMinute / HKMinute / USMinute：1/5/15/30/60 分钟；1 分钟为分时。
// 美股输入窗口和返回时间均为美东时间，自动处理夏令时。

codes, err := sdk.Codes.CN(ctx, stockgo.CodeOptions{Market: "kc", Simple: true})
all, err := sdk.Batch.CN(ctx, stockgo.BatchOptions{Concurrency: 5})
quotes, err := sdk.Batch.ByCodes(ctx, codes, stockgo.BatchOptions{BatchSize: 200})
sdk.ClearCaches()
```

代码列表支持 CN / HK / US / Fund；缓存按实例隔离，错误和空表不缓存。
批量查询支持 CN / HK / US、ByCodes / HKByCodes / USByCodes / Raw。
A 股 K 线主源失败时按腾讯、新浪回退，可用 `Options{DisableKlineFallback: true}` 关闭。
新浪及分钟备用源仅提供不复权数据；缺失的金额或换手率保留为空，实际复权状态见 `Adjust`。
代码表和 K 线端点可以通过 `Options.CodeURLs` / `Options.KlineURLs` 配置。

## 指标、信号与回测

`indicators` 包提供全部 17 个技术指标函数和组合计算；`signals` / `screener`
提供本地信号识别、链式选股与回测，不引入网络层。

SDK 可使用 `sdk.Kline.WithIndicators(...)` 和 `sdk.Kline.Signals(...)` 获取带预热的结果。
完整用法、数值空值规则和回测成交约定见 [计算模块文档](docs/COMPUTATION.md)。

## 筹码、板块和资金流

支持 `sdk.Chips.CN/HK/US`、`sdk.Board.Industry/Concept` 和 `sdk.FundFlow`。
实时资金流使用 `sdk.Quotes.FundFlow(ctx, codes)`；筹码纯计算使用
`indicators.CalcChipDistribution`。配置、单位及分页规则见[扩展接口文档](docs/EXTENSIONS.md)。

## 基金、日历与北向

`sdk.Fund`（净值、排名、档案、分红、主题）、`sdk.Calendar`、`sdk.Reference`、
`sdk.Northbound` 和通用 `sdk.Datacenter` 报表入口。

用法与当前上游可用性见[参考与基金接口文档](docs/REFERENCE_FUND.md)。
北向持股排名原报表暂不可用，会明确返回上游错误。

## 龙虎榜、大宗交易、两融与异动

`sdk.DragonTiger`、`sdk.BlockTrade`、`sdk.Margin` 和 `sdk.MarketEvent`，
包括六类股池、22 类盘口异动、个股逐日历史、板块异动及监管异动。
已兼容当前东方财富报表名称与字段变化，详见[专题数据文档](docs/TOPIC_REPORTS.md)。

## 期货与期权

`sdk.Futures` 提供国内／全球期货 K 线、全球行情、品种与 COMEX 库存。
`sdk.Options` 提供股指／商品期权 T 型报价、日 K 线、ETF 月份与分钟线、中金所行情和期权龙虎榜。
示例、合约格式、字段及单位见[期货期权文档](docs/DERIVATIVES.md)。

## 请求治理

通过 `Options.Request` 配置按数据源隔离的限流、熔断、重试、请求头、UA 轮换及生命周期回调。
东财同源主机自动回退，等待过程支持 context 取消；可用 `sdk.Requests` 查看主机健康及熔断状态。
默认行为、配置示例和高级请求入口见[请求治理文档](docs/REQUESTS.md)。

## 符号解析

无网络依赖：

```go
package main

import (
    "fmt"
    "github.com/Gavin-Gong/stock-go/symbols"
)

func main() {
    symbol, err := symbols.Normalize("600519.SH")
    if err != nil { panic(err) }
    key, err := symbols.ToTencent(symbol)
    if err != nil { panic(err) }
    fmt.Println(key) // sh600519
}
```

支持市场/交易所/资产提示、腾讯和东方财富代码转换、特殊指数和期货符号。
例如 `symbols.Normalize("700", symbols.Hint{Market: symbols.HK})` 解析为港股 `00700`。
解析错误可通过 `sdkerrors.CodeOf(err)` 或 `errors.As` 检查。

验证：

```sh
go test -race ./...
go vet ./...
go build ./...
# 真实网络冒烟（默认测试不访问外网）
STOCK_GO_LIVE=1 go test -run '^TestLive' -v ./...
```


## 致谢与许可

感谢 [chengzuopeng](https://github.com/chengzuopeng) 创建并开源
[stock-sdk](https://github.com/chengzuopeng/stock-sdk)。本项目基于其 v2.4.6 固定基线，
以 Go 重写并维护 SDK、CLI、MCP 和辅助工具。

遵循 [ISC 许可](LICENSE)，保留原作者的版权与许可声明。
来源说明见 [ATTRIBUTION.md](ATTRIBUTION.md)。
