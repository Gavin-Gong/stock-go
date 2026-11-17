package symbols

import (
	"github.com/Gavin-Gong/stock-go/sdkerrors"
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		input          string
		hint           Hint
		market         Market
		exchange, code string
		asset          AssetType
		em, tx         string
	}{
		{"600519", Hint{}, CN, "SSE", "600519", Stock, "1.600519", "sh600519"},
		{" sh600519 ", Hint{}, CN, "SSE", "600519", Stock, "1.600519", "sh600519"},
		{"600519.SH", Hint{}, CN, "SSE", "600519", Stock, "1.600519", "sh600519"},
		{"sh600519.SH", Hint{}, CN, "SSE", "600519", Stock, "1.600519", "sh600519"},
		{"1.sh600519", Hint{}, CN, "SSE", "600519", Stock, "1.600519", "sh600519"},
		{"000001", Hint{}, CN, "SZSE", "000001", Stock, "0.000001", "sz000001"},
		{"920819", Hint{}, CN, "BSE", "920819", Stock, "0.920819", "bj920819"},
		{"0.430047", Hint{}, CN, "BSE", "430047", Stock, "0.430047", "bj430047"},
		{"0.bj430047", Hint{}, CN, "BSE", "430047", Stock, "0.430047", "bj430047"},
		{"00700", Hint{}, HK, "HKEX", "00700", Stock, "116.00700", "hk00700"},
		{"700", Hint{Market: HK}, HK, "HKEX", "00700", Stock, "116.00700", "hk00700"},
		{"700.HK", Hint{}, HK, "HKEX", "00700", Stock, "116.00700", "hk00700"},
		{"116.700", Hint{}, HK, "HKEX", "00700", Stock, "116.00700", "hk00700"},
		{"HK00700", Hint{}, HK, "HKEX", "00700", Stock, "116.00700", "hk00700"},
		{"00700", Hint{Market: CN}, CN, "SZSE", "00700", Stock, "0.00700", "sz00700"},
		{"AAPL", Hint{}, US, "US", "AAPL", Stock, "105.AAPL", "usAAPL"},
		{"usaapl", Hint{}, US, "US", "AAPL", Stock, "105.AAPL", "usAAPL"},
		{"usAAPL", Hint{Exchange: "NASDAQ"}, US, "NASDAQ", "AAPL", Stock, "105.AAPL", "usAAPL"},
		{"106.baba", Hint{}, US, "NYSE", "BABA", Stock, "106.BABA", "usBABA"},
		{"BRK.B", Hint{}, US, "US", "BRK.B", Stock, "105.BRK.B", "usBRK.B"},
		{"usb.a", Hint{}, US, "US", "USB.A", Stock, "105.USB.A", "usUSB.A"},
		{"USB", Hint{}, US, "US", "USB", Stock, "105.USB", "usUSB"},
		{"HKD", Hint{}, US, "US", "HKD", Stock, "105.HKD", "usHKD"},
		{"SHOP", Hint{}, US, "US", "SHOP", Stock, "105.SHOP", "usSHOP"},
		{"HSI", Hint{}, US, "US", "HSI", Stock, "105.HSI", "usHSI"},
		{"HSI", Hint{Market: HK}, HK, "HSI", "HSI", Index, "100.HSI", "hkHSI"},
		{"hkHSI", Hint{}, HK, "HSI", "HSI", Index, "100.HSI", "hkHSI"},
		{"hkhsi", Hint{}, HK, "HSI", "HSI", Index, "100.HSI", "hkHSI"},
		{"DJI", Hint{Market: US}, US, "US", "DJI", Index, "100.DJIA", "usDJI"},
		{"usDJI", Hint{}, US, "US", "DJI", Index, "100.DJIA", "usDJI"},
		{"DJI", Hint{Market: US, AssetType: Stock}, US, "US", "DJI", Stock, "105.DJI", "usDJI"},
		{"930955", Hint{}, CN, "CSI", "930955", Index, "2.930955", ""},
		{"2.H30533", Hint{}, CN, "CSI", "H30533", Index, "2.H30533", ""},
		{"HSHCI", Hint{}, HK, "HSI", "HSHCI", Index, "124.HSHCI", ""},
		{"GDAXI", Hint{}, Global, "DAX", "GDAXI", Index, "100.GDAXI", ""},
		{"90.BK0475", Hint{}, CN, "SSE", "BK0475", Board, "90.BK0475", ""},
		{"CFFEX.IF2412", Hint{}, CN, "CFFEX", "IF2412", Futures, "", ""},
		{"rb2510", Hint{AssetType: Futures}, CN, "SHFE", "RB2510", Futures, "", ""},
		{"GC", Hint{Market: Global, Exchange: "COMEX"}, Global, "COMEX", "GC", Futures, "", ""},
	}
	for _, tt := range cases {
		t.Run(tt.input+"/"+string(tt.hint.Market)+"/"+string(tt.hint.AssetType), func(t *testing.T) {
			s, err := Normalize(tt.input, tt.hint)
			if err != nil {
				t.Fatal(err)
			}
			if s.Market != tt.market || s.Exchange != tt.exchange || s.Code != tt.code || s.AssetType != tt.asset || s.Input != tt.input {
				t.Fatalf("unexpected symbol: %+v", s)
			}
			em, err := ToEastmoney(s)
			if tt.em == "" {
				if err == nil {
					t.Fatalf("expected Eastmoney rejection, got %s", em)
				}
			} else if err != nil || em != tt.em {
				t.Fatalf("Eastmoney=%s, %v; want %s", em, err, tt.em)
			}
			tx, err := ToTencent(s)
			if tt.tx == "" {
				if err == nil {
					t.Fatalf("expected Tencent rejection, got %s", tx)
				}
			} else if err != nil || tx != tt.tx {
				t.Fatalf("Tencent=%s, %v; want %s", tx, err, tt.tx)
			}
			if s.AssetType == Futures && s.Variety == "" {
				t.Fatal("missing futures variety")
			}
		})
	}
}
func TestConflictingAssertions(t *testing.T) {
	cases := []struct {
		input string
		hint  Hint
	}{
		{"", Hint{}}, {"?!", Hint{}}, {"600519.SH", Hint{Exchange: "SZSE"}},
		{"AAPL", Hint{Market: HK}}, {"600519", Hint{Exchange: "HKEX"}},
		{"90.BK0475", Hint{AssetType: Stock}}, {"600519", Hint{AssetType: Board}},
		{"CFFEX.IF2412", Hint{AssetType: Stock}}, {"GC", Hint{Market: Global}},
		{"1.sz000001", Hint{}}, {"hk00700.SZ", Hint{}}, {"930955.SH", Hint{}},
		{"sh930955", Hint{}}, {"HSHCI", Hint{AssetType: Stock}},
	}
	for _, tt := range cases {
		t.Run(tt.input, func(t *testing.T) {
			_, err := Normalize(tt.input, tt.hint)
			if sdkerrors.CodeOf(err) != sdkerrors.InvalidSymbol {
				t.Fatalf("expected INVALID_SYMBOL, got %v", err)
			}
		})
	}
}
func TestTencentBatch(t *testing.T) {
	got := TencentBatch([]string{"600519", "sh600519", "sz000001", "930955", "AAPL", ""}, CN)
	if !reflect.DeepEqual(got.Keys, []string{"sh600519", "sz000001"}) || len(got.Invalid) != 3 {
		t.Fatalf("%+v", got)
	}
}
func FuzzNormalize(f *testing.F) {
	for _, s := range []string{"600519", "hkHSI", "USB", "0.bj430047", "\x00", "", "1.", "90.", "CFFEX."} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		s, err := Normalize(input)
		if err != nil {
			return
		}
		if s.Market == "" || s.Exchange == "" || s.Code == "" || s.AssetType == "" {
			t.Fatalf("malformed symbol: %+v", s)
		}
	})
}
