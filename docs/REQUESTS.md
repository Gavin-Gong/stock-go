# 请求治理

行情、K 线、基金、专题数据、期货期权等服务共用实例请求层。
限流、熔断按 provider 分开，备用主机共享同一 provider 的限制。
K 线的快速主源探测与普通请求共享治理状态，探测本身不消耗完整重试预算。

## 配置

```go
retries := 2
sdk, err := stockgo.New(stockgo.Options{
    Request: &stockgo.RequestOptions{
        Timeout: 10 * time.Second,
        Headers: http.Header{"X-Client": {"my-app"}},
        UserAgent: "my-stock-app/1.0",
        Retry: &stockgo.RetryOptions{
            MaxRetries: &retries,
            BaseDelay: 300 * time.Millisecond,
            MaxDelay: 5 * time.Second,
            BackoffMultiplier: 2,
            Jitter: 100 * time.Millisecond,
        },
        RateLimit: &stockgo.RateLimitOptions{
            RequestsPerSecond: 5, MaxBurst: 5,
        },
        CircuitBreaker: &stockgo.CircuitBreakerOptions{
            FailureThreshold: 5, ResetTimeout: 30 * time.Second,
            HalfOpenRequests: 1,
        },
        ProviderPolicies: map[string]stockgo.ProviderPolicy{
            "eastmoney": {
                Timeout: 8 * time.Second,
                RateLimit: &stockgo.RateLimitOptions{RequestsPerSecond: 3},
            },
            "linkdiary": {DisableRateLimit: true},
        },
    },
})
```

示例需导入 context、net/http、time 和 stockgo。
Options 原有 HTTPClient / Timeout / MaxRetries / BaseDelay / MaxBodyBytes 仍有效；
当它们非零时，覆盖 Request 中对应的配置。需要明确关闭重试可使用 Retry.MaxRetries 的零值指针。

支持 tencent / eastmoney / sina / linkdiary / unknown，也可显式使用自定义名称。
SDK 的代码表和交易日历归属 linkdiary；通用请求未显式传 provider 时按域名推断。
配置在创建时复制，请求头、状态码切片和数值指针后续修改不会污染客户端。
底层 HTTPClient 以及回调捕获的共享变量仍由调用者负责并发安全。

## 默认值与继承

| 配置 | Go 默认值 |
| --- | --- |
| 每次 HTTP 尝试超时 | 15 秒 |
| 最大重试次数 | 0 |
| 初始 / 最大退避 | 500 毫秒 / 30 秒 |
| 退避倍数 / 随机抖动 | 2 / 0 |
| 最大响应体 | 32 MiB |
| 限流、熔断、UA 轮换 | 默认不启用 |
| 同源备用主机 | 对已知东财 push2 / push2his 主机启用 |

可通过全局选项或 provider 策略覆盖这些默认值。

provider 的 Timeout 和 UserAgent 覆盖全局值；Headers 按不区分大小写的名称合并。
显式 User-Agent 请求头优先于 UserAgent 选项；启用 RotateUserAgent 时轮换值最终优先。
UA 池与原版一致，按实例计数轮换，无跨实例全局计数。

Retry 的字段逐项继承，MaxRetries、RetryOnNetworkError、RetryOnTimeout 使用指针区分未设置和零/false。
RetryableStatusCodes 为 nil 时继承，显式空切片禁用 HTTP 状态码重试。
RateLimit / CircuitBreaker 的嵌套字段逐项继承；零值选择继承或默认值。
使用 DisableRateLimit / DisableCircuitBreaker 明确关闭某个 provider 的全局治理配置。

## 重试、取消与限流

默认可重试状态码为 408 / 429 / 500 / 502 / 503 / 504，
网络错误与单次尝试超时可重试；可通过 RetryOnNetworkError / RetryOnTimeout 关闭。
Parse、参数错误和主动取消不重试。普通 4xx 默认不重试，也不切换备用主机。
HTTP 501 默认不在重试列表中，但可尝试备用主机。

退避按倍数增长，最多 MaxDelay，再增加小于 Jitter 的随机抖动。
HTTP Retry-After 支持秒数和 HTTP 日期，实际重试等待采用退避与 Retry-After 的较大值，
其中 Retry-After 最多按 MaxDelay 等待。OnRetry 的 attempt 从 1 开始，表示下一次重试序号。

限流为令牌桶，默认启用参数为每秒 5、突发 5；也可使用小数速率。
MaxBurst 必须至少 1，未设置时为 max(1, requestsPerSecond)。
并发获取不会将令牌扣为负数；重试及备用主机的每一次 HTTP 尝试都要获取令牌。
context 取消会立即退出排队和退避，不占住后续请求。
令牌等待不计入单次 HTTP Timeout，但受整个调用的 context 截止时间约束。

RequestOptions.ClientContext 为实例生命周期取消入口，每个调用仍接受自己的 context。
主动取消归类 ABORTED；调用 context 的截止时间或单次尝试超时归类 TIMEOUT。
外部取消及调用截止时间耗尽不计入主机故障或熔断失败。
自定义 HTTPClient/Doer 应遵守 Request.Context() 并关闭响应体；
Go 无法强行终止完全无视 context 的自定义阻塞代码，本层不会为其不断创建无法回收的工作协程。

## 熔断

一整次逻辑请求的重试和备用主机都失败后才累计一次失败，不按尝试次数累计。
失败阈值默认 5，冷却 30 秒，半开默认只放行一个探测请求。
探测成功后恢复 CLOSED，失败重新进入 OPEN；主动取消只释放该探测自己的名额。

ProbeRecycleTimeout 默认 max(ResetTimeout × 4, 120 秒)，只回收失联探测。
Go 使用探测标识和状态代际核对，迟到或已回收的旧结果不会释放其他探测、重开新代际熔断器。
OnStateChange 回调在锁外执行，可读取状态；回调 panic 被隔离。

```go
stats, enabled := sdk.Requests.CircuitStats("eastmoney")
health := sdk.Requests.HostHealth("eastmoney")
sdk.Requests.ResetCircuit("eastmoney")
```

CircuitStats 读取不会预占探测名额，包含状态、失败次数、最近失败毫秒时间、半开成功数和在途探测数。
HostHealth 返回副本，包含失败/成功计数、冷却截止、最后故障时间和错误码。
时间字段为 Unix 毫秒。HostHealth("") 返回全部已访问主机，按主机名排序。

## 同源主机回退

东财 push2his 的备用池为未编号 / 7 / 33 / 63 / 91；push2 为 17 / 29 / 79 / 91 / push2delay。
优先健康主机，故障主机冷却 30 秒后恢复优先级；全部冷却时仍允许按顺序探测。
保留请求路径及原始 query（包含异动类型需要的裸逗号）。

只有首个候选主机使用完整重试预算，后续每台仅一次。
网络、单次超时、JSON/响应解析错误以及 HTTP 408 / 429 / 5xx 可以切换主机。
这与 A 股 K 线的“东财→腾讯→新浪”跨数据源回退是两层独立机制。
DisableKlineFallback 只关闭跨数据源回退；Request.DisableHostFallback 关闭同源主机回退。

对默认未编号 push2、非池内编号主机，Go 保留配置的原主机作为健康时的首选。
只对真实的已知东财域名启用该机制；自定义端口、含用户信息的 URL、代理及相似域名不会改写主机。

## 回调与高级请求

```go
hooks := &stockgo.RequestHooks{
    OnRequest: func(rc stockgo.RequestContext) {
        // rc.Provider / URL / Timeout / Attempt / ResponseType
    },
    OnResponse: func(rc stockgo.RequestContext, meta stockgo.ResponseMeta) {
        // meta.Status / Duration（收到响应头所用时间）
    },
    OnError: func(rc stockgo.RequestContext, err error) {},
    OnRetry: func(rc stockgo.RequestContext, err error, delay time.Duration) {},
    Trace: func(event string, rc stockgo.RequestContext) {},
}
// 创建 SDK 时设置 Request.Hooks = hooks。

noRetries := 0
var payload map[string]any
err := sdk.Requests.GetJSONWithOptions(ctx, "https://example.com/api", &payload,
    stockgo.RequestGetOptions{
        Provider: "unknown",
        Retry: &stockgo.RetryOptions{MaxRetries: &noRetries},
        HostFallback: stockgo.Bool(false),
    })
```

也可使用 NewRequestClient(RequestOptions{...}) 单独创建请求层；Timeout() 返回全局默认超时。
Get(ctx, provider, url, headers) / GetWithOptions 返回字节，
GetJSON / GetJSONWithOptions 在生命周期内解析 JSON，GetParsed 支持字节校验器。
单次请求可覆盖 headers、retry、HTTPClient 和 hostFallback。
JSON 目标仅在完整解析成功后更新，失败主机的部分内容不会污染最终结果。
新浪 JSONP 也在生命周期内完成包裹校验，不执行脚本。

Trace 事件为 request / response / error / retry / fallback；Attempt 从 0 开始，备用主机重置为 0。
成功顺序为 request→response；HTTP 失败会先产生 response，再产生 error；
解析失败不会在同一主机重复请求，但可产生 fallback。
每个显式生命周期回调之后调用 Trace，两者分别隔离 panic，互不影响。
回调同步执行，不在请求锁内；不同并发请求的回调可能同时执行，应保持简短并同步共享状态。

通用熔断统计 HTTP/网络/解析失败；已成功取得响应后的业务层校验（如报表 success:false）
保留业务错误，不伪装为网络错误。缓存命中的服务调用不触发 HTTP 生命周期事件。
