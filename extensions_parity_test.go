package stockgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func extensionTestSDK(t *testing.T, handler http.HandlerFunc) (*SDK, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	boards := BoardEndpoints{List: srv.URL + "/list", Spot: srv.URL + "/spot", Constituents: srv.URL + "/constituents", Kline: srv.URL + "/kline", Trends: srv.URL + "/trends"}
	sdk, err := New(Options{IndustryURLs: boards, ConceptURLs: boards, FundFlowURLs: FundFlowEndpoints{History: srv.URL + "/flow", Rank: srv.URL + "/rank", Realtime: srv.URL + "/realtime"}, KlineURLs: KlineEndpoints{CN: srv.URL + "/kline", HK: srv.URL + "/hk", US: srv.URL + "/us", Tencent: srv.URL + "/tx"}, DisableKlineFallback: false})
	if err != nil {
		t.Fatal(err)
	}
	return sdk, srv
}
func TestExtensionUpstreamParity(t *testing.T) {
	body, err := os.ReadFile("testdata/extension-parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Kind, Operation    string
		Options            FundFlowRankOptions
		Response, Expected json.RawMessage
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Kind+"/"+f.Operation+"/"+f.Options.Indicator, func(t *testing.T) {
			sdk, _ := extensionTestSDK(t, func(w http.ResponseWriter, r *http.Request) { w.Write(f.Response) })
			ctx := context.Background()
			var actual any
			var err error
			if f.Kind == "flow" {
				switch f.Operation {
				case "individual":
					actual, err = sdk.FundFlow.Individual(ctx, "600519")
				case "market":
					actual, err = sdk.FundFlow.Market(ctx)
				case "sectorHistory":
					actual, err = sdk.FundFlow.SectorHistory(ctx, "BK0475")
				case "realtime":
					actual, err = sdk.Quotes.FundFlow(ctx, []string{"600519", "sh600519", "930955"})
				case "rank":
					actual, err = sdk.FundFlow.Rank(ctx, f.Options)
				case "sectorRank":
					actual, err = sdk.FundFlow.SectorRank(ctx, f.Options)
				}
			} else {
				provider := sdk.Board.Industry
				if f.Kind == "concept" {
					provider = sdk.Board.Concept
				}
				switch f.Operation {
				case "List":
					actual, err = provider.List(ctx)
				case "Spot":
					actual, err = provider.Spot(ctx, "BK0475")
				case "Constituents":
					actual, err = provider.Constituents(ctx, "BK0475")
				case "Kline":
					actual, err = provider.Kline(ctx, "BK0475")
				case "Minute1":
					actual, err = provider.MinuteKline(ctx, "BK0475", "1")
				case "Minute5":
					actual, err = provider.MinuteKline(ctx, "BK0475", "5")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			json.Unmarshal(encoded, &got)
			json.Unmarshal(f.Expected, &want)
			compareSourceFields(t, got, want)
		})
	}
}
