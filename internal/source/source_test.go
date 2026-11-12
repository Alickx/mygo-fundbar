package source

import (
	"testing"
	"time"

	"fundbar/internal/model"
)

func TestSymbol(t *testing.T) {
	cases := map[string]string{
		"1.600519":  "sh600519",
		"0.000858":  "sz000858",
		"116.00883": "hk00883",
		"105.AAPL":  "usAAPL",
		"6098":      "", // a foreign stock of a QDII fund: no price here
		"":          "",
	}
	for in, want := range cases {
		if got := Symbol(in); got != want {
			t.Errorf("Symbol(%q) = %q, want %q", in, got, want)
		}
	}
}

// A slice of a holdings page, as the fund site sends it.
const holdingsHTML = `<div class='box'><div class='boxitem w790'><h4 class='t'><label class='right lab2 xq505'>&nbsp;&nbsp;&nbsp;&nbsp;来源：天天基金&nbsp;&nbsp;&nbsp;&nbsp;截止至：<font class='px12'>2026-09-30</font></label></h4><table class='w782 comm tzxq'><tbody><tr><td>1</td><td><a href='//quote.eastmoney.com/unify/r/1.600519'>600519</a></td><td class='tol'><a href='//quote.eastmoney.com/unify/r/1.600519'>贵州茅台</a></td><td class='tor'><span data-id='dq600519'></span></td><td class='tor'><span data-id='zd600519'></span></td><td class='xglj'><a href='ccbdxq_005827_600519.html' class='red'>变动详情</a></td><td class='tor'>6.37%</td><td class='tor'>95.85</td><td class='tor'>120,638.73</td></tr><tr><td>2</td><td><a href='//quote.eastmoney.com/unify/r/116.00883'>00883</a></td><td class='tol'><a href='//quote.eastmoney.com/unify/r/116.00883'>中国海洋石油</a></td><td class='tor'><span data-id='dq00883'></span></td><td class='tor'><span data-id='zd00883'></span></td><td class='xglj'><a href='ccbdxq_005827_00883.html'>变动详情</a></td><td class='tor'>6.22%</td><td class='tor'>1,000.00</td><td class='tor'>20,000.00</td></tr></tbody></table></div></div>`

func TestSecID(t *testing.T) {
	cases := map[string]string{
		"sh600519": "1.600519",
		"sz000858": "0.000858",
		"hk00700":  "116.00700",
		"usAAPL":   "105.AAPL",
	}
	for in, want := range cases {
		got, ok := secID(in)
		if !ok || got != want {
			t.Errorf("secID(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := secID("600519"); ok {
		t.Error("expected no secid without a market")
	}
}

func TestSplitConcepts(t *testing.T) {
	got := splitConcepts("白酒, 超级品牌，央国企改革; ")
	want := []string{"白酒", "超级品牌", "央国企改革"}
	if len(got) != len(want) {
		t.Fatalf("concepts = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("concept %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseHoldings(t *testing.T) {
	got := parseHoldings(holdingsHTML)
	if len(got) != 2 {
		t.Fatalf("parsed %d holdings, want 2", len(got))
	}
	want := []struct {
		symbol, code, name string
		weight             float64
	}{
		{"sh600519", "600519", "贵州茅台", 6.37},
		{"hk00883", "00883", "中国海洋石油", 6.22},
	}
	for i, w := range want {
		h := got[i]
		if h.Symbol != w.symbol || h.Code != w.code || h.Name != w.name || h.Weight != w.weight {
			t.Errorf("holding %d = %+v, want %+v", i, h, w)
		}
	}
}

func TestJSValues(t *testing.T) {
	src := `var fS_name = "招商中证白酒指数(LOF)A";var fS_code = "161725";
var Data_netWorthTrend = [{"x":1432656000000,"y":1.0,"equityReturn":0,"unitMoney":""},{"x":1794153600000,"y":0.5859,"equityReturn":-0.63,"unitMoney":""}];
var stockCodesNew =["1.600519","116.00883"];`
	if got := jsString(src, "fS_name"); got != "招商中证白酒指数(LOF)A" {
		t.Errorf("fS_name = %q", got)
	}
	if got := jsArray(src, "Data_netWorthTrend"); got[:2] != "[{" || got[len(got)-2:] != "}]" {
		t.Errorf("net worth array = %q", got)
	}
	if got := jsArray(src, "stockCodesNew"); got != `["1.600519","116.00883"]` {
		t.Errorf("stock codes = %q", got)
	}
	if got := jsArray(src, "Missing"); got != "" {
		t.Errorf("missing array = %q", got)
	}
}

// A real answer line of an index, GBK-decoded, as the quote endpoint sends
// it: the same field layout the stocks use.
const indexQuote = `v_sh000001="1~上证指数~000001~3680.64~3663.72~3655.66~433120083~0~0~0.00~0~0.00~0~0.00~0~0.00~0~0.00~0~0.00~0~0.00~0~0.00~0~0.00~0~0.00~0~~20261111161427~16.92~0.46~3689.65~3650.31~3680.64/433120083/736154792000~433120083~73615479~0.89~16.70~~3689.65~3650.31~1.07~578335.90~646127.31~0.00~-1~-1~0.87~0~36";`

func TestParseIndexQuote(t *testing.T) {
	out := map[string]model.Quote{}
	parseQuotes(indexQuote, out)
	q, ok := out["sh000001"]
	if !ok {
		t.Fatalf("parsed %+v, want sh000001", out)
	}
	if q.Price != 3680.64 || q.Pct != 0.46 {
		t.Fatalf("quote = %+v, want 3680.64 and +0.46%%", q)
	}
	if got := q.At.Format("2006-01-02 15:04:05"); got != "2026-11-11 16:14:27" {
		t.Fatalf("quote time = %q", got)
	}
}

func TestParseQuoteTime(t *testing.T) {
	cn := parseQuoteTime("20261109161440")
	if cn.IsZero() || cn.Hour() != 16 || cn.Day() != 9 {
		t.Fatalf("CN quote time = %v", cn)
	}
	hk := parseQuoteTime("2026/11/09 16:08:10")
	if hk.IsZero() || hk.Hour() != 16 {
		t.Fatalf("HK quote time = %v", hk)
	}
	if !parseQuoteTime("").IsZero() {
		t.Fatal("expected no time")
	}
}

// An answer of the search: funds, an index and a stock.
const searchJSON = `{"ErrCode":0,"Datas":[
{"CODE":"161725","NAME":"招商中证白酒指数(LOF)A","CATEGORY":700,"CATEGORYDESC":"基金","FundBaseInfo":{"FTYPE":"指数型-股票","DWJZ":0.5859}},
{"CODE":"399997","NAME":"中证白酒","CATEGORY":2,"CATEGORYDESC":"指数"},
{"CODE":"600519","NAME":"贵州茅台","CATEGORY":1,"CATEGORYDESC":"股票"}]}`

func TestParseSearch(t *testing.T) {
	hits, err := parseSearch([]byte(searchJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %+v, want one fund", hits)
	}
	if hits[0].Code != "161725" || hits[0].Type != "指数型-股票" {
		t.Fatalf("hit = %+v", hits[0])
	}
}

// An answer of the net worth list, newest first, with a day the site has no
// value for.
const navListJSON = `{"Data":{"LSJZList":[
{"FSRQ":"2026-11-10","DWJZ":"1.2447","LJJZ":"1.6485","JZZZL":"-0.16"},
{"FSRQ":"2026-11-09","DWJZ":"1.2467","LJJZ":"1.6505","JZZZL":"0.04"},
{"FSRQ":"2026-11-08","DWJZ":"","LJJZ":"","JZZZL":""}]}}`

func TestParseNavList(t *testing.T) {
	got := parseNavList([]byte(navListJSON), "2026-11-09")
	if len(got) != 1 {
		t.Fatalf("navs = %+v, want one newer than 2026-11-09", got)
	}
	want := model.Nav{Date: "2026-11-10", Unit: 1.2447, Acc: 1.6485, Pct: -0.16}
	if got[0] != want {
		t.Errorf("nav = %+v, want %+v", got[0], want)
	}
}

func TestNavDue(t *testing.T) {
	cases := []struct {
		at   time.Time
		want string
	}{
		// A weekday morning: yesterday's value is the newest.
		{time.Date(2026, 11, 10, 9, 30, 0, 0, cst), "2026-11-09"},
		// A weekday evening: the day's value is out.
		{time.Date(2026, 11, 10, 21, 0, 0, 0, cst), "2026-11-10"},
		// A Monday morning: Friday's value.
		{time.Date(2026, 11, 9, 8, 0, 0, 0, cst), "2026-11-06"},
		// The weekend: Friday's value.
		{time.Date(2026, 11, 14, 12, 0, 0, 0, cst), "2026-11-13"},
		{time.Date(2026, 11, 15, 12, 0, 0, 0, cst), "2026-11-13"},
		// The first day after the National Day holiday, in the morning: the
		// value from before the holiday.
		{time.Date(2026, 10, 8, 10, 0, 0, 0, cst), "2026-09-30"},
		// A holiday evening: nothing is published, so the value before the
		// holiday stays the newest.
		{time.Date(2026, 10, 5, 21, 0, 0, 0, cst), "2026-09-30"},
	}
	for _, c := range cases {
		if got := navDue(c.at); got != c.want {
			t.Errorf("navDue(%s) = %s, want %s", c.at.Format(time.RFC3339), got, c.want)
		}
	}
}
