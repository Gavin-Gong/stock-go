# stock-go 命令行

命令行提供 **94 个方法、10 个快捷命令**。
要求 Go 1.23 或以上，无需 Node.js 运行时。

## 安装与运行

### 安装后直接调用

在任意目录执行：

```sh
go install github.com/Gavin-Gong/stock-go/cmd/stock-go@latest
stock-go --help
stock-go quote 600519 00700 AAPL
```

Go 自动下载、编译并安装命令，无需单独执行 build。
可执行文件安装到 GOBIN；未设置 GOBIN 时，默认使用 GOPATH/bin（通常为 ~/go/bin）。
确保安装目录在 PATH 中；若使用默认安装目录，可在 shell 配置文件中加入：

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

重新执行同一条 `go install ...@latest` 命令即可更新已安装的版本。

### 不安装，直接运行

```sh
go run github.com/Gavin-Gong/stock-go/cmd/stock-go@latest --help
go run github.com/Gavin-Gong/stock-go/cmd/stock-go@latest quote 600519
go run github.com/Gavin-Gong/stock-go/cmd/stock-go@latest kline 600519 --limit 10 -f csv
```

Go 自动下载和编译，并复用本地缓存；命令不会安装到 GOBIN。
`@latest` 使用远程模块版本，不读取当前目录中的源码改动。

### 运行本地源码

在项目根目录执行 `go run ./cmd/stock-go ...` 使用当前源码；
也可运行 `go install ./cmd/stock-go` 将当前源码安装为 stock-go 命令。

`--version` 默认输出 dev，可用 `go install -ldflags '-X main.version=0.1.0' ./cmd/stock-go` 设置本地安装的版本号。

## 常用查询

```sh
# 默认简版 A 股行情；自动分组查询 A/HK/US
stock-go quote 600519 00700 AAPL --pretty
stock-go quote 600519 --full
stock-go quote 110011 --market fund

# 日/周/月 K 线和分钟线；--limit 仅裁剪输出，不减少上游请求
stock-go kline 600519 --period weekly --adjust hfq --limit 30
stock-go kline AAPL --start 20261101 --limit 10 -f csv
stock-go minute 00700 --period 1 --ndays 5 --limit 20
stock-go indicators 600519 --ma 5,10,20 --macd --rsi --limit 10

# 搜索、分时、代码列表、市场状态、股池
stock-go search 茅台 --limit 5
stock-go timeline 600519
stock-go codes a --simple --board kc --limit 10
stock-go status hk
stock-go ztpool strong --date 20261117 --limit 10
```

`quote` 按 A 股、港股、美股、基金顺序合并结果，不保证保留跨市场输入顺序。
各市场并发请求，部分失败时返回有效结果并写 stderr 提示；全部失败返回错误。
基金需显式 `--market fund`，明确的市场冲突会报错。
有歧义的英文代码保持 SDK 符号规则，例如恒指请写 hkHSI，避免与美股代码 HSI 混淆。
腾讯不覆盖的特殊指数会提示使用 K 线入口。

## 命名空间入口

命名空间路径保持原版大小写和名称，例如 cnSimple / getThemeList：

```sh
stock-go quotes cnSimple 600519 000001
stock-go kline us AAPL --period daily --limit 10
stock-go board industry list --limit 5 --format table
stock-go board concept minuteKline BK0475 --period 5
stock-go fundFlow rank --indicator 5day --pageSize 10
stock-go northbound history south --start 20240102 --end 20240105
stock-go marketEvent stockChanges all --page 1 --pageSize 100
stock-go marketEvent unusualFluctuation --triggered=false --start 20261101
stock-go dragonTiger detail --start 20240102 --end 20240102
stock-go blockTrade detail --start 20240102 --end 20240102
stock-go margin targetList 20240102 --limit 10
stock-go fund profile 110011
stock-go fund dividendList --year 2024 --page all --code 110011
stock-go fund theme getThemeList --category 2 --pageSize 5
stock-go futures kline RBM --start 20261101 --limit 5
stock-go futures globalKline GC00Y --limit 5
stock-go options index spot io io2612
stock-go options etf months 50ETF
stock-go options etf expireDay 50ETF 2026-12
stock-go reference tradingCalendar --limit 5
```

所有方法都可加 `--help` 查看位置参数、类型、枚举和必填选项。
日期/合约为示例，会随真实数据覆盖或合约到期影响可用性。
期货始终不复权；原版期货 CLI 的 `--adjust` 仅作为兼容参数接受，不改变期货数据。

清单由固定原版 spec 生成，Go 调用使用明确的类型绑定，不查找任意对象成员。
原版只允许字符串透传的隐藏选项在 Go 中也会列入帮助并转换为对应类型，
例如 --pageSize、--marketCode、基金 --year/--page。

## call 原始实参

```sh
stock-go call search --args '["茅台"]'
stock-go call quotes.cn --args '[["600519","000001"]]'
stock-go call kline.cn --args '["600519",{"period":"daily","adjust":"","startDate":"20261101"}]'
stock-go call marketEvent.stockChanges --args '["all",{"page":1,"pageSize":20}]'
stock-go call fund.dividendList --args '[{"year":"2024","page":"all","code":"110011"}]'
```

`--args` 是 JSON 实参数组；传一个非数组 JSON 值时会将其包装为一个参数。
只允许原版清单中的 94 个路径，包括顶层 search；内部客户端、原型成员等不可达。
返回值保留 Go SDK 的 JSON 结构和 Source / Adjust 等实际数据元信息。

桥接层兼容原 JS 参数：A→CN、显式 adjust:""→Go AdjustNone、
indicators 的 true/false→对应指标选项，以及基金 page:"all"→AllPages。
个股异动日期/天数、北向方向、股池类型等也按原版签名转换。
`call calendar.marketStatus` 额外接受第二个 RFC3339 时间字符串，便于可复现查询。

## 输出和参数

- 默认输出一行 JSON；`--pretty` 缩进，stdout 只放数据。
- `--format/-f table` 按中文显示宽度对齐，数字列右对齐。
- `--format/-f csv` 收集全部字段，按首次出现的列顺序输出，正确转义逗号、引号和换行。
- 嵌套对象/数组在表格或 CSV 中作为紧凑 JSON 单元格；空值为空单元格。
- `--limit N` 统一裁剪数组输出的前 N 项，不裁剪嵌套对象；0/负数不限制。
- `--quiet/-q` 关闭“无数据”提示，错误和部分市场失败仍写 stderr。
- `--timeout` 接受正数毫秒（兼容原版），也接受 2s / 500ms 等 Go duration。
- 支持 --key value、--key=value、短名、负数值、重复 flag 和 -- 终止选项解析。
- 非数组选项重复时取最后一个值；指标周期数组可重复指定并合并。
- 布尔值可写 --macd 或 --macd=true；关闭使用 --macd=false。不要把 true/false 另放一个位置参数。

相较原版的宽松透传，Go 统一拒绝未知选项、非法数值、缺值、枚举错误和多余位置参数，
在发送请求前报告用法错误；不支持任意 JavaScript 表达式或回调参数。
空结果仍是成功，默认同时在 stderr 写“无数据”；JSON 输出 [] 或 null。
下游提前关闭管道（EPIPE，如 `| head`）按成功退出，不打印堆栈。
Ctrl-C/SIGTERM 会取消进行中的网络请求和排队等待。

## 退出码

| 退出码 | 含义 |
| --- | --- |
| 0 | 成功、空数据或下游关闭管道 |
| 1 | 程序、输出或一般错误 |
| 2 | 用法、参数、符号或方法不存在 |
| 3 | 网络、超时或取消 |
| 4 | 上游、HTTP 或解析错误 |
| 5 | 限流或熔断 |

JSON 模式的错误写入 stderr，结构为 `{"error":{"code":"...","message":"..."}}`；
表格/CSV 模式使用可读文本。参数与查询失败时 stdout 为空。

`stock-go mcp` 启动 MCP stdio 服务，工具集、提示模板和资源限制见 [MCP 文档](MCP.md)。
