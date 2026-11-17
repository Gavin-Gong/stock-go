# MCP stdio 服务

MCP 服务提供 **93 个工具、7 个提示模板**，复用 Go SDK 的类型绑定与请求治理。
默认暴露 26 个 core 工具、4 个 core 提示模板；full 模式提供全部内容。
工具参数采用 JSON Schema 校验，无 Node.js 运行时依赖。

## 启动

```sh
go install github.com/Gavin-Gong/stock-go/cmd/stock-go@latest
stock-go mcp

# 全量工具与模板
STOCK_GO_MCP_TOOLS=full STOCK_GO_MCP_PROMPTS=full stock-go mcp

# 精确选择工具；名称取自 tools/list
STOCK_GO_MCP_TOOLS=search,get_a_share_quotes stock-go mcp
```

也可以不安装，直接运行：

```sh
go run github.com/Gavin-Gong/stock-go/cmd/stock-go@latest mcp
```

安装目录与 PATH 设置见 [命令行文档](CLI.md)。

在支持 MCP stdio 的客户端中配置可执行文件的绝对路径与参数 ["mcp"]。
例如通用配置片段：

```json
{
  "mcpServers": {
    "stock-go": {
      "command": "/absolute/path/to/stock-go",
      "args": ["mcp"],
      "env": {
        "STOCK_GO_MCP_TOOLS": "full",
        "STOCK_GO_MCP_PROMPTS": "full"
      }
    }
  }
}
```

stdout 仅输出一行一个的 JSON-RPC 消息；就绪日志、未知名称和配置提示写 stderr。
`stock-go --version` 与 MCP initialize 返回的版本使用同一构建参数 main.version，默认 dev。
serverInfo.name 为 stock-go。

## 工具集、模板集与配置

- STOCK_GO_MCP_TOOLS：core（默认）/ full / 逗号分隔的工具名称。
- STOCK_GO_MCP_PROMPTS：core（默认）/ full / 逗号分隔的模板名称。
- STOCK_GO_MCP_TIMEOUT：SDK 每次 HTTP 尝试超时，正数毫秒。

兼容原版的 STOCK_SDK_MCP_TOOLS / PROMPTS / TIMEOUT，非空的 Go 前缀变量优先。
工具与模板独立过滤，未知名称写日志提示；模板引用未启用的工具也会提示。
core / full 大小写容错，空值和显式空名称数组选择 core。
模板本身只返回分析任务说明，不在服务端执行其工具编排。

编程式使用可传入现有 SDK，复用缓存、限流、熔断及 HTTP 客户端：

```go
sdk, err := stockgo.New(stockgo.Options{
    Request: &stockgo.RequestOptions{
        RateLimit: &stockgo.RateLimitOptions{RequestsPerSecond: 5, MaxBurst: 3},
    },
})
server, err := mcp.New(mcp.Options{
    SDK: sdk,
    ToolSet: "full", PromptSet: "full",
    MaxConcurrent: 8, MaxPending: 32,
})
err = server.Serve(ctx, os.Stdin, os.Stdout, os.Stderr)
```

使用 `mcp` 包，也可直接调用 mcp.Start(ctx, in, out, log, options)。
SDK > SDKOptions > 超时环境变量 > 默认值；工具/模板的显式配置优先于环境变量。
Server.Dispatch(ctx, line) 可直接处理一条 JSON 消息，便于嵌入应用和测试。

## 协议与请求

支持固定原版协议版本 2025-11-25、2025-06-18；请求其他版本时协商为 2025-11-25。
提供 initialize、ping、tools/list、tools/call、prompts/list、prompts/get。
notifications/initialized 不响应；notifications/cancelled 会按 requestId 取消已排队或执行中的请求。
所有合法通知均无响应，字符串、数字和 null 请求 ID 按协议保留；超大数字 ID 不经过 float64 转换。
每行必须是单个请求对象，不接受 JSON-RPC 批量数组。

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_a_share_quotes","arguments":{"codes":["600519"]}}}
{"jsonrpc":"2.0","id":3,"method":"prompts/get","params":{"name":"analyze_stock","arguments":{"symbol":"600519"}}}
```

tools/list 的 inputSchema 包含必填项、基本类型与枚举，不注入展示用 default。
参数校验还检查数组元素、对象结构和 14 个指标配置键；错误参数不会触发 SDK 查询。
带指标工具接受嵌套 indicators，支持 true / false / 配置对象，与 CLI 扁平参数共用原生 Go 计算层。

RPC 格式、未知方法或工具名称等错误返回标准 JSON-RPC error。
参数不合 schema 和 SDK 查询错误返回 isError:true 的工具结果，包含结构化错误码及可用的来源信息。
有效工具数据为 content[0].text 中的 JSON；HTTP/上游错误不会被伪装为无数据。

## 结果与资源限制

- 顶层数组超过 200 项：返回 total / truncated / note / sample（前 200 项）。
- 序列化文本超过 80000 个 UTF-16 单元：省略数据体，返回 oversized 提示和 _meta 大小信息。
- 对象内的大数组（如基金全历史净值）不会按项裁剪，而是应用整体文本限制。
- 消息输入默认上限 1000000 字节；超长行丢弃到换行后只报一次错，随后继续接收。
- 默认最多 16 个并行请求、64 个等待请求；队列满返回明确的 Server busy 错误，不无限启动任务。

MaxLineBytes / MaxConcurrent / MaxPending 可在 Options 中调整。
提示模板保留原版语言、只读操作和数据缺失处理要求；参数替换为一次性插值，不重复解释参数里的模板标记。
返回顺序可以与请求顺序不同，客户端应按 ID 对账；重复在途 ID 会被拒绝。

stdin EOF 会等待已接受的请求完成并输出；最后一条无换行的完整 JSON 也会处理。
上层 context 取消、信号或输出 EPIPE 会取消在途查询，关闭支持 io.Closer 的流，结束阻塞读写。
输出 EPIPE 按正常断开退出。自定义不支持关闭的 Reader/Writer 应保证其 I/O 能自行返回。

## 调用与运行约定

保持工具/模板/参数和限制的主契约，并补齐实际取消、并发与消息边界处理：
合法通知不生成应答；错误 envelope、非法 ID 或批量数组明确拒绝；
超长输入按字节检查，分片和整行输入使用相同规则；数组元素和指标配置在查询前校验。
工具和提示清单返回独立副本，调用方无法通过修改清单改变服务端校验规则。
MCP 的默认请求超时与重试沿用 Go SDK 配置，见 REQUESTS.md。
