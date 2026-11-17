# 公开辅助工具

所有运行时实现均为 Go，不依赖 Node.js 或脚本执行器。

## 缓存

`cache` 包提供泛型 TTL/LRU 内存缓存、
可注入 `Store[T]`、并发去重、共享缓存和客户端作用域。

```go
import (
    "context"
    "time"
    "github.com/Gavin-Gong/stock-go/cache"
)

store, err := cache.NewMemoryStore[string](cache.Options{
    DefaultTTL: time.Minute, MaxSize: 1000,
})
if err != nil { panic(err) }
value, err := cache.CacheThrough(context.Background(), store, "quote:600519",
    func(ctx context.Context) (string, error) { return "value", nil })
```

- `cache.New[T]` 返回 `MemoryCache[T]`，提供 Get/Set/Has/Delete/Clear/Len/Cleanup、
  GetOrFetch、Options/MatchesOptions/Configure。Get 返回 `(value, found)`，可以缓存 nil。
- TTL 使用 `time.Duration`；0 永不过期。默认 MaxSize=1000；负 TTL、非正容量重配返回 INVALID_ARGUMENT。
  与原版一致，恰好到达过期时刻仍有效；Len 在访问或 Cleanup 前包含过期条目。
- 默认缓存返回原值，调用方应管理指针、map、slice 的并发访问；不自动深拷贝。
- LRU 采用实际访问顺序，不依赖毫秒时钟打平时间的偶然顺序。
  Configure 缩容立即淘汰；改默认 TTL 只影响后续写入。
- 同一缓存或 store 的同 key 并发读取只发起一次 fetch；首个调用的 context 控制 fetch，
  等待者可以独立取消。fetch、ShouldCache 或自定义 store 的 panic 转为 error，等待者可正常结束。
- MemoryCache / MemoryStore 在 Clear/Delete/Set 后阻止旧 fetch 回写；
  对自定义 Store 的在途写入失效须由该 Store 的版本或事务机制保证。
- `CacheThroughWithOptions` 的 `ShouldCache` 可以排除空表等结果；TTL 指针 nil 使用 store 默认值。
  Store 的 Get/Set/Delete/Clear 都接受 context；通常用指针实现可比较的 store 身份。
  Go 使用 found 区分未命中，不引入 JavaScript undefined。
- `GetShared(namespace, options...)` 首次配置生效；之后使用 ConfigureShared 显式重配。
  ClearShared 保留命名实例但清空内容。共享命名空间由应用控制生命周期。
- `GetClientScoped(sdk.Requests, namespace, options...)` 按客户端隔离。
  `cache.ClearClientScoped(sdk.Requests)`、`sdk.Requests.ClearCaches()`、`sdk.ClearCaches()`
  都清除该客户端公开缓存及 SDK 代码表、日历、美股代码和板块名称缓存。
  `WithoutRetries()` 与原客户端共享作用域；独立 SDK 实例互不影响。
- `CreateKey(parts...)` 用 `:` 连接标量，nil 保留为 `∅`，零参数为空串；分隔符不转义。
  有碰撞隔离需求时应由应用编码各段。

原版的 getSharedCache / configureSharedCache / clearClientScopedCaches / createCacheKey
分别对应上述 GetShared / ConfigureShared / ClearClientScoped / CreateKey。

## 市场时间

根包提供 `MarketTZCN`、`MarketTZHK`、`MarketTZUS`（IANA 字符串）、`TimeMeta` 以及：

```go
meta := stockgo.BuildTimeMeta("2024-07-09 09:30", stockgo.MarketTZUS)
wall := stockgo.FormatInTZ(meta.Timestamp, stockgo.MarketTZCN)
// wall == "2024-07-09 21:30"
```

`ParseMarketTime` 返回 UTC 毫秒指针，格式无效或时区不支持返回 nil。
支持 YYYY-MM-DD、YYYYMMDD、YYYYMMDDHHmmss、日期加空格/T 和 HH:mm[:ss]。
`BuildTimeMetaFromDateAndTime` 单独接收日期和 HH:mm[:ss]；两个分量均可有首尾空白。
`FormatInTZ` 返回 YYYY-MM-DD HH:mm，nil、超出 JS Date 范围或未知时区返回空串。
时区数据随 Go 二进制提供，无需部署机安装 zoneinfo。

美东夏令时的重叠选择较早时刻，春季缺失时段顺延；历史香港/上海 DST 同样使用时区规则。
公开解析保留原版 Date.UTC 的数字日期溢出行为（2024-02-30 → 2024-03-01，
年份 0..99 按 1900..1999 处理）。SDK 数据记录内部继续严格拒绝非法日历日期，
并额外接受部分供应商的斜杠日期。

## 响应与数值解析

- `DecodeGBK([]byte)` 返回字符串和 error，支持 Windows-936 私用字符映射，
  无效字节序列使用 Unicode 替换字符。
- `ParseResponse(text)` 返回 `[]RawQuote`，公开原版宽松的分号/等号/波浪号拆分；
  SDK 行情接口另外验证赋值键及字段数量。
- `SafeNumber` / `SafeNumberOrNull` / `ToNumber` / `ToNumberSafe` 采用 parseFloat 数字前缀语义，
  例如 `"1.2%" → 1.2`，无值为 0 或 nil；允许 ±Infinity。
  Go 的 JSON 编码拒绝 Infinity，需要输出 JSON 时请用 `ToFiniteNumberOrNull` 或预先处理。
  SDK 数据记录自身仍只返回有限数值/空值。
- `ToFiniteNumberOrNull` 要求整串可转换且有限，支持字符串/数值，包括正十六、八、二进制字符串。
  `"1.2%"`、bool、对象、无值、非有限数返回 nil。
- `ExtractJSONFromJSONP(text)` 返回 `json.RawMessage`；支持前置块注释、回调包装或纯 JSON。
  可执行表达式、尾随语句和非法 payload 返回 PARSE_ERROR。
- `ParseJSVars(text, names)` 返回 `map[string]json.RawMessage`。
  支持顶层 var/let/const 的 JSON 值和单引号字符串，不执行脚本；缺失或非法值不出现在 map 中。
  Go 扫描器主动忽略注释、字符串、函数体内伪装的声明，比原版声明正则更严格。

## 获取 JS 变量与 JSONP

```go
vars, err := stockgo.FetchJSVars(ctx, endpoint, []string{"fS_name", "Data_netWorthTrend"},
    stockgo.FetchJSVarsOptions{Client: sdk.Requests, Timeout: 5*time.Second})
raw, err := stockgo.JSONPRequest(ctx, endpoint, stockgo.JSONPOptions{
    Client: sdk.Requests, CallbackParam: "callback", CallbackMode: "query",
})
// CallbackMode:"path" 使用 URL 中的 {callback} 占位符。
```

两种获取入口都支持 Headers、Timeout、Client；HTTP(S) URL 不允许 fragment。
不传 Client 时创建默认 15 秒客户端；传入时沿用 provider、重试、限流、熔断、fallback、hooks。
显式 Timeout 为整个调用增加期限（客户端各次尝试自身仍有超时），context 随时可以取消。
JSONP 校验放在请求生命周期内，解析失败进入错误回调/主机回退，不执行任何 JS。
Go 不使用 DOM/window 全局变量，因此不暴露浏览器脚本互斥锁。

## 外链与泛型并发

`GenerateSearchExternalLinks(SearchResult)` 按固定原版返回东方财富和雪球两个 `ExternalLink`。
处理腾讯市场码、规范符号、港股补零、美股大小写、全球指数别名及查询编码；
未知市场、期货、板块和不匹配符号退回站内搜索。

`ChunkArray[T](items, size)` 返回分块副本，size 必须为正；空输入返回空数组。
更改分块元素不会覆盖输入切片（指针所指对象仍共享）。

`AsyncPool[T](ctx, tasks, concurrency, preserveOrder...)` 有界执行 context 任务。
默认按完成顺序返回；传 true 则保持任务顺序；空任务返回空数组。
失败或 panic 取消剩余任务，等待已运行任务退出后返回 error，无部分结果；
任务必须遵循 context 才能及时取消。相比原 Promise.all，Go 显式等待任务结束以避免遗留工作。
