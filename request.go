package stockgo

import "github.com/Gavin-Gong/stock-go/internal/transport"

// RequestOptions provides instance-wide and provider-specific HTTP governance.
type RequestOptions = transport.Config
type RequestClient = transport.Client
type ProviderPolicy = transport.ProviderPolicy
type RetryOptions = transport.RetryOptions
type RateLimitOptions = transport.RateLimitOptions
type CircuitBreakerOptions = transport.CircuitBreakerOptions
type CircuitState = transport.CircuitState
type CircuitStats = transport.CircuitStats
type HostHealthStats = transport.HostHealthStats
type RequestHooks = transport.Hooks
type RequestContext = transport.RequestContext
type ResponseMeta = transport.ResponseMeta
type RequestGetOptions = transport.GetOptions

const (
	CircuitClosed   = transport.Closed
	CircuitOpen     = transport.Open
	CircuitHalfOpen = transport.HalfOpen
)

func NewRequestClient(options RequestOptions) (*RequestClient, error) { return transport.New(options) }
func InferProvider(endpoint string) string                            { return transport.InferProvider(endpoint, "") }
