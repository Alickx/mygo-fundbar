package app

import "sort"

// PortfolioInsight is what the user owns through the funds they hold: each
// fund's share of the portfolio, and the industries and stocks its quarterly
// reports disclose, added up by what each fund is worth now. The four lists
// are sorted largest first.
type PortfolioInsight struct {
	Funds      []InsightFund
	Industries []InsightIndustry
	Stocks     []InsightStock
	Overlaps   []InsightOverlap
	// Total is what the held funds are worth, Count how many there are.
	Total float64
	Count int
	// IndustryTotal is the share of the portfolio the industries account
	// for, StockTotal the same for the disclosed holdings, and StockCount how
	// many distinct stocks those holdings are.
	IndustryTotal float64
	StockTotal    float64
	StockCount    int
}

// InsightFund is one held fund's place in the portfolio.
type InsightFund struct {
	Code, Name string
	// Value is what the user's shares are worth; Weight is the fund's share
	// of the portfolio, in percent.
	Value  float64
	Weight float64
}

// InsightIndustry is an industry of the funds' reports, as a share of the
// whole portfolio in percent.
type InsightIndustry struct {
	Name   string
	Weight float64
}

// InsightStock is a top-ten holding, as a share of the whole portfolio in
// percent. Pct is its change today from any fund that holds it, and Quoted
// says whether a price was available. Funds is how many held funds report
// it among their top ten.
type InsightStock struct {
	Symbol string
	Name   string
	Weight float64
	Pct    float64
	Quoted bool
	Funds  int
}

// InsightOverlap is a pair of held funds that share top-ten holdings. Weight
// is the sum, over the common stocks, of the smaller of the two funds'
// weights, in percentage points of each fund's net assets: how much of the
// pair is the same companies. Common names them.
type InsightOverlap struct {
	ACode, AName string
	BCode, BName string
	Weight       float64
	Common       []string
}

// heldFund is a fund that counts, with its share of the portfolio as a
// fraction, which the sums below weight it by.
type heldFund struct {
	f     *Fund
	share float64
}

// portfolioInsight adds up the funds the user holds in the holding group,
// weighted by each fund's value now. It is a function of its input alone, so
// the view and the tests share the arithmetic, and it reads no clock: a fund
// outside the holding group, or one without shares, does not count.
func portfolioInsight(funds []*Fund) PortfolioInsight {
	var in PortfolioInsight
	var held []heldFund
	for _, f := range funds {
		if f == nil || f.Shares <= 0 || !f.In(holdingGroup) {
			continue
		}
		held = append(held, heldFund{f: f})
		in.Total += f.Value()
	}
	in.Count = len(held)
	if in.Count == 0 {
		return in
	}
	for i := range held {
		if in.Total > 0 {
			held[i].share = held[i].f.Value() / in.Total
		}
		in.Funds = append(in.Funds, InsightFund{
			Code: held[i].f.Code, Name: fundName(held[i].f),
			Value: held[i].f.Value(), Weight: held[i].share * 100,
		})
	}
	sort.SliceStable(in.Funds, func(i, j int) bool {
		if in.Funds[i].Value != in.Funds[j].Value {
			return in.Funds[i].Value > in.Funds[j].Value
		}
		return in.Funds[i].Code < in.Funds[j].Code
	})

	in.Industries, in.IndustryTotal = industriesOf(held)
	in.Stocks, in.StockTotal, in.StockCount = stocksOf(held)
	in.Overlaps = overlapsOf(held)
	return in
}

// industriesOf adds up each industry's weight over the held funds: a fund
// worth a tenth of the portfolio adds a tenth of each of its industries.
func industriesOf(held []heldFund) ([]InsightIndustry, float64) {
	weights := map[string]float64{}
	for _, h := range held {
		for _, ind := range h.f.Industries {
			if ind.Name == "" || ind.Weight <= 0 {
				continue
			}
			weights[ind.Name] += h.share * ind.Weight
		}
	}
	out := make([]InsightIndustry, 0, len(weights))
	var total float64
	for name, weight := range weights {
		out = append(out, InsightIndustry{Name: name, Weight: weight})
		total += weight
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return out[i].Name < out[j].Name
	})
	return out, total
}

// stocksOf adds up each stock's weight over the held funds. A holding's key
// is its symbol, or its name when the report gives no symbol; two stocks of
// the same name on different exchanges stay apart.
func stocksOf(held []heldFund) ([]InsightStock, float64, int) {
	type stock struct {
		in    InsightStock
		funds map[int]bool
	}
	byKey := map[string]*stock{}
	for i, h := range held {
		for _, holding := range h.f.Holdings {
			key := holdingKey(holding.Symbol, holding.Name)
			if key == "" || holding.Weight <= 0 {
				continue
			}
			s, ok := byKey[key]
			if !ok {
				s = &stock{
					in:    InsightStock{Symbol: holding.Symbol, Name: holding.Name},
					funds: map[int]bool{},
				}
				byKey[key] = s
			}
			if s.in.Name == "" {
				s.in.Name = holding.Name
			}
			s.in.Weight += h.share * holding.Weight
			s.funds[i] = true
			if !s.in.Quoted && holding.Symbol != "" {
				if q, ok := h.f.Quotes[holding.Symbol]; ok {
					s.in.Pct, s.in.Quoted = q.Pct, true
				}
			}
		}
	}
	out := make([]InsightStock, 0, len(byKey))
	var total float64
	for _, s := range byKey {
		s.in.Funds = len(s.funds)
		total += s.in.Weight
		out = append(out, s.in)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out, total, len(out)
}

// overlapsOf compares each pair of held funds by the top-ten stocks they
// share. A pair with nothing in common is left out; the rest come largest
// first, and a tie by fund code.
func overlapsOf(held []heldFund) []InsightOverlap {
	var out []InsightOverlap
	for i := 0; i < len(held); i++ {
		for j := i + 1; j < len(held); j++ {
			a, b := held[i].f, held[j].f
			aw, bw := holdingWeights(a), holdingWeights(b)
			var overlap float64
			var common []string
			seen := map[string]bool{}
			for _, holding := range b.Holdings {
				key := holdingKey(holding.Symbol, holding.Name)
				if key == "" || seen[key] {
					continue
				}
				seen[key] = true
				wa, ok := aw[key]
				if !ok {
					continue
				}
				overlap += min(wa, bw[key])
				name := holding.Name
				if name == "" {
					name = key
				}
				common = append(common, name)
			}
			if len(common) == 0 {
				continue
			}
			out = append(out, InsightOverlap{
				ACode: a.Code, AName: fundName(a),
				BCode: b.Code, BName: fundName(b),
				Weight: overlap, Common: common,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		if out[i].ACode != out[j].ACode {
			return out[i].ACode < out[j].ACode
		}
		return out[i].BCode < out[j].BCode
	})
	return out
}

// holdingWeights is a fund's top-ten weights keyed as its holdings are.
func holdingWeights(f *Fund) map[string]float64 {
	weights := make(map[string]float64, len(f.Holdings))
	for _, h := range f.Holdings {
		if key := holdingKey(h.Symbol, h.Name); key != "" && h.Weight > 0 {
			weights[key] += h.Weight
		}
	}
	return weights
}

// holdingKey identifies a stock across funds: its symbol, or its name when
// the report gives no symbol.
func holdingKey(symbol, name string) string {
	if symbol != "" {
		return symbol
	}
	return name
}
