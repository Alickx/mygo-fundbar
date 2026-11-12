// Command probe compares the app's estimate of a fund's change with the
// change the fund published, for the funds named on the command line.
//
//	go run ./tools/probe 161725 005827
package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"fundbar/internal/model"
	"fundbar/internal/source"
)

func main() {
	codes := os.Args[1:]
	if len(codes) == 0 {
		codes = []string{"161725", "005827", "110022", "003096"}
	}
	ctx := context.Background()
	c := source.New()

	type row struct {
		code, name   string
		cover, est   float64
		actual, diff float64
		navDate      string
		holdings     int
	}
	var rows []row
	symbols := map[string]bool{}
	funds := map[string]*source.Fund{}
	holdings := map[string][]model.Holding{}
	for _, code := range codes {
		f, err := c.Fund(ctx, code)
		if err != nil {
			fmt.Printf("%s 读取基金失败: %v\n", code, err)
			continue
		}
		hs, _, err := c.Holdings(ctx, code)
		if err != nil {
			fmt.Printf("%s 读取持仓失败: %v\n", code, err)
		}
		funds[code] = f
		holdings[code] = hs
		for _, h := range hs {
			symbols[h.Symbol] = true
		}
	}
	var list []string
	for s := range symbols {
		list = append(list, s)
	}
	sort.Strings(list)
	quotes, err := c.Quotes(ctx, list)
	if err != nil {
		fmt.Println("行情失败:", err)
	}

	for _, code := range codes {
		f := funds[code]
		if f == nil {
			continue
		}
		hs := holdings[code]
		est, ok := model.EstimateFund(hs, quotes)
		if !ok {
			fmt.Printf("%s %s 无法估算\n", code, f.Name)
			continue
		}
		est = model.ScaleEstimate(est, f.StockPosition)
		last := f.Navs[len(f.Navs)-1]
		rows = append(rows, row{
			code: code, name: f.Name, cover: est.Cover, est: est.Pct,
			actual: last.Pct, diff: est.Pct - last.Pct, navDate: last.Date,
			holdings: len(hs),
		})
	}

	fmt.Printf("%-8s %-30s %8s %8s %8s %8s\n", "代码", "名称", "覆盖", "估算", "实际", "误差")
	for _, r := range rows {
		name := []rune(r.name)
		if len(name) > 24 {
			name = name[:24]
		}
		fmt.Printf("%-8s %-30s %7.1f%% %+7.2f%% %+7.2f%% %+7.2f%%\n",
			r.code, string(name), r.cover, r.est, r.actual, r.diff)
	}
}
