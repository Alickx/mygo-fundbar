package source

import "testing"

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
