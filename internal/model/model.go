// Package model holds the fund data the app shows and the arithmetic that
// turns a fund's holdings and the day's stock prices into an estimate.
package model

import (
	"math"
	"time"
)

// Holding is a stock a fund reports holding, as its quarterly report gives it.
type Holding struct {
	// Symbol is the exchange's symbol of the stock, such as "sh600519".
	Symbol string
	Code   string
	Name   string
	// Weight is the stock's share of the fund's net assets, in percent.
	Weight float64
}

// Nav is one day's net asset value of a fund.
type Nav struct {
	Date string
	// Unit is 单位净值, Acc 累计净值.
	Unit float64
	Acc  float64
	// Pct is the day's change, in percent.
	Pct float64
}

// Industry is a share of a fund's net assets in one industry, as its
// quarterly report gives it.
type Industry struct {
	Name string
	// Weight is the industry's share of the fund's net assets, in percent.
	Weight float64
}

// StockProfile says what a stock is: the industry it belongs to, and the
// themes it is traded with.
type StockProfile struct {
	Industry string
	Concepts []string
}

// Quote is a stock's price now.
type Quote struct {
	Symbol string
	Price  float64
	// Pct is the change since the previous close, in percent.
	Pct float64
	// At is when the exchange last priced it.
	At time.Time
}

// Estimate is a fund's estimated change today, made of the fund's holdings
// and their prices.
type Estimate struct {
	// Pct is the estimated change of the fund's net asset value, in percent.
	Pct float64
	// Cover is the share of the fund's net assets the holdings account for,
	// in percent: how much of the fund the estimate is made of.
	Cover float64
	// Stocks is how many holdings had a price.
	Stocks int
}

// EstimateFund estimates a fund's change today from its holdings and their
// quotes, keyed by symbol. Only holdings quoted today count, and the rest of
// the fund is taken to move as they do: the sum of each holding's share
// times its change, over the share that is covered.
func EstimateFund(holdings []Holding, quotes map[string]Quote) (Estimate, bool) {
	var weighted, cover float64
	var n int
	for _, h := range holdings {
		q, ok := quotes[h.Symbol]
		if !ok || h.Weight <= 0 {
			continue
		}
		weighted += h.Weight * q.Pct
		cover += h.Weight
		n++
	}
	if cover <= 0 || n == 0 {
		return Estimate{}, false
	}
	return Estimate{Pct: weighted / cover, Cover: cover, Stocks: n}, true
}

// Confidence says how much to trust an estimate, from 0 to 1.
func (e Estimate) Confidence() float64 {
	c := e.Cover / 60
	return math.Max(0, math.Min(1, c))
}
