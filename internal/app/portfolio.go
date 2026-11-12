package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
)

// portfolioAvailable reports that there is a portfolio to look through: the
// holding group is listed and at least one of its funds has shares.
func (a *App) portfolioAvailable() bool {
	if a.group != holdingGroup {
		return false
	}
	for _, f := range a.all {
		if f.In(holdingGroup) && f.Shares > 0 {
			return true
		}
	}
	return false
}

// togglePortfolio shows the look-through, or returns to the chosen fund.
// The choice of fund stays as it was, so 返回基金 goes back to it.
func (a *App) togglePortfolio() {
	a.listSel = -1
	a.portfolio = !a.portfolio
}

// exitPortfolio leaves the look-through for a fund's detail. The list may
// have been choosing a scratch row while the pane showed.
func (a *App) exitPortfolio() {
	a.portfolio = false
	a.listSel = -1
}

// showFundByCode leaves the look-through and shows a fund of the list.
func (a *App) showFundByCode(code string) {
	a.exitPortfolio()
	a.choose(code)
}

// showFundRow leaves the look-through and shows the fund at a list row.
func (a *App) showFundRow(i int) {
	f := a.visible(i)
	if f == nil {
		a.exitPortfolio()
		return
	}
	a.showFundByCode(f.Code)
}

// portfolioView draws the look-through: the title, then what the funds own
// by value, by industry and by stock, and how much the funds share.
func (a *App) portfolioView(c *ui.Context, t *ui.Theme) {
	in := portfolioInsight(a.inGroup(holdingGroup))
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "组合透视").FontSize(19).FontWeight(600).NoWrap().Shrink(0)
			ui.Textf(c, "持有 %d 只 · 市值 %s", in.Count, money(in.Total)).
				FontSize(12).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			ui.Spacer(c)
			if ui.Button(c, "分享").Shrink(0).
				Tooltip("复制今日收益分享图，只含涨跌幅").
				Clicked() {
				// The render must wait for this frame to close.
				a.afterFrame(func() { a.CopyShare(false) })
			}
		})
		ui.Text(c, "按各基金季报披露的前十大重仓股与行业配置，以当前市值加权估算；"+
			"季报之外的持仓不在其中。").FontSize(11).TextColor(t.TextMuted)
	})
	a.portfolioAssets(c, t, in)
	a.portfolioIndustries(c, t, in)
	a.portfolioStocks(c, t, in)
	a.portfolioOverlaps(c, t, in)
}

// portfolioAssets lists the held funds by what they are worth: each fund's
// share of the portfolio, and a click to show it.
func (a *App) portfolioAssets(c *ui.Context, t *ui.Theme, in PortfolioInsight) {
	ui.Column(c).Gap(9).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "资产分布").FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			ui.Spacer(c)
			ui.Textf(c, "%d 只基金", in.Count).FontSize(11.5).
				TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
		})
		body := card(c, t)
		body.Gap(3).Children(func() {
			if len(in.Funds) == 0 {
				ui.Text(c, "还没有持有基金").FontSize(11.5).TextColor(t.TextMuted)
				return
			}
			maxWeight := in.Funds[0].Weight
			for _, f := range in.Funds {
				f := f
				row := ui.Row(c).Padding(4, 7).Radius(6).Gap(10).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Tooltip(f.Name + " · " + money(f.Value))
				row.Children(func() {
					ui.Text(c, f.Name).FontSize(12).Grow(1).Basis(0).MinWidth(0).
						SingleLine().Ellipsis("…").Tooltip(f.Name)
					ui.Text(c, money(f.Value)).FontSize(12).
						FontFeatures("tnum").NoWrap().
						Width(90).TextAlign(ui.End)
					progress(c, f.Weight, maxWeight, 104)
					ui.Textf(c, "%.2f%%", f.Weight).FontSize(11.5).
						TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
						Width(52).TextAlign(ui.End)
				})
				if row.Clicked() {
					a.showFundByCode(f.Code)
				}
			}
		})
	})
}

// portfolioIndustries adds up the industries of every fund's report,
// weighted by the fund's share of the portfolio.
func (a *App) portfolioIndustries(c *ui.Context, t *ui.Theme, in PortfolioInsight) {
	ui.Column(c).Gap(9).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			ui.Text(c, "行业分布").FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			ui.Spacer(c)
			if more := len(in.Industries) - portfolioIndustriesShown; more > 0 {
				ui.Textf(c, "另有 %d 个较小行业", more).FontSize(11.5).
					TextColor(t.TextMuted).NoWrap()
			}
			ui.Textf(c, "合计 %.1f%%", in.IndustryTotal).FontSize(11.5).
				TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
				Tooltip("其余为现金、债券与未披露的持仓")
		})
		body := card(c, t)
		body.Gap(9).Children(func() {
			if len(in.Industries) == 0 {
				ui.Text(c, "持有的基金都没有披露行业配置").FontSize(11.5).
					TextColor(t.TextMuted)
				return
			}
			shown := in.Industries
			if len(shown) > portfolioIndustriesShown {
				shown = shown[:portfolioIndustriesShown]
			}
			maxWeight := shown[0].Weight
			for _, ind := range shown {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, ind.Name).FontSize(11.5).Grow(1).Basis(0).MinWidth(0).
						SingleLine().Ellipsis("…").Tooltip(ind.Name)
					progress(c, ind.Weight, maxWeight, 118)
					ui.Textf(c, "%.2f%%", ind.Weight).FontSize(11.5).
						TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
						Width(52).TextAlign(ui.End)
				})
			}
		})
	})
}

// portfolioStocks lists the stocks the funds' reports disclose, by their
// share of the whole portfolio: the same companies may sit in more than one
// fund, and the row says how many.
func (a *App) portfolioStocks(c *ui.Context, t *ui.Theme, in PortfolioInsight) {
	const numW = 66
	ui.Column(c).Gap(9).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			ui.Text(c, "重仓股穿透").FontSize(13).FontWeight(600).NoWrap().Shrink(0)
			ui.Spacer(c)
			if in.StockCount > 0 {
				ui.Textf(c, "已披露重仓合计 %.1f%% · %d 只股票",
					in.StockTotal, in.StockCount).FontSize(11.5).
					TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
					Tooltip("前十大重仓股按当前市值加总后占组合的比例；" +
						"其余为现金、债券、未披露的持仓与季报之外的重仓股")
			}
		})
		body := card(c, t)
		body.Gap(4).Children(func() {
			if len(in.Stocks) == 0 {
				ui.Text(c, "持有的基金都没有披露重仓股").FontSize(11.5).
					TextColor(t.TextMuted)
				return
			}
			ui.Row(c).Padding(0, 7, 2, 7).Children(func() {
				ui.Text(c, "名称").FontSize(11).TextColor(t.TextMuted).Grow(1)
				ui.Text(c, "组合占比").FontSize(11).TextColor(t.TextMuted).
					Width(numW).TextAlign(ui.End)
				ui.Text(c, "今日涨跌").FontSize(11).TextColor(t.TextMuted).
					Width(numW).TextAlign(ui.End)
				ui.Text(c, "持有基金").FontSize(11).TextColor(t.TextMuted).
					Width(70).TextAlign(ui.End)
			})
			shown := in.Stocks
			if len(shown) > portfolioStocksShown {
				shown = shown[:portfolioStocksShown]
			}
			for i, st := range shown {
				st := st
				row := ui.Row(c).Padding(3, 7).Radius(6).AlignItems(ui.Center)
				if i%2 == 1 {
					row.Background(t.Text.Alpha(0.035))
				}
				row.Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						ui.Row(c).Gap(5).NoWrap().AlignItems(ui.Center).Children(func() {
							ui.Text(c, st.Name).FontSize(12).SingleLine().Ellipsis("…").
								Tooltip(st.Name).Shrink(1).MinWidth(0)
							marketTag(c, t, model.Holding{Symbol: st.Symbol, Name: st.Name})
						})
					})
					ui.Textf(c, "%.2f%%", st.Weight).FontSize(11.5).
						TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
						Width(numW).TextAlign(ui.End)
					change := "—"
					if st.Quoted {
						change = pct(st.Pct)
					}
					ui.Text(c, change).FontSize(11.5).TextColor(up(t, st.Pct)).
						FontFeatures("tnum").NoWrap().Width(numW).TextAlign(ui.End)
					held := ""
					if st.Funds > 1 {
						held = fmt.Sprintf("%d 只基金持有", st.Funds)
					}
					ui.Text(c, held).FontSize(11).TextColor(t.TextMuted).NoWrap().
						Width(70).TextAlign(ui.End)
				})
			}
		})
	})
}

// portfolioOverlaps says how much each pair of funds owns the same
// companies, comparing the top-ten holdings their reports disclose.
func (a *App) portfolioOverlaps(c *ui.Context, t *ui.Theme, in PortfolioInsight) {
	if in.Count < 2 {
		return
	}
	ui.Column(c).Gap(9).Children(func() {
		ui.Text(c, "基金重合度").FontSize(13).FontWeight(600).NoWrap().
			Tooltip("只比较各基金季报披露的前十大重仓股")
		body := card(c, t)
		body.Gap(5).Children(func() {
			if len(in.Overlaps) == 0 {
				ui.Text(c, "持有的基金之间没有相同的重仓股").FontSize(11.5).
					TextColor(t.TextMuted)
				return
			}
			for i, o := range in.Overlaps {
				if i >= portfolioOverlapsShown {
					break
				}
				names := fmtPair(o.AName, o.BName)
				tip := "共同持有：" + strings.Join(o.Common, "、")
				row := ui.Row(c).Gap(10).AlignItems(ui.Center).
					Padding(3, 7).Radius(6)
				if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Tooltip(tip)
				row.Children(func() {
					ui.Text(c, names).FontSize(12).Grow(1).Basis(0).MinWidth(0).
						SingleLine().Ellipsis("…").Tooltip(tip)
					ui.Textf(c, "重合 %.1f%% · %d 只相同",
						o.Weight, len(o.Common)).FontSize(11.5).
						TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().Shrink(0)
				})
			}
		})
	})
}

// What the look-through shows before the rest waits behind a count.
const (
	portfolioIndustriesShown = 8
	portfolioStocksShown     = 10
	portfolioOverlapsShown   = 5
)

// progress draws a bar as wide as v is of max, as the industry card does.
func progress(c *ui.Context, v, max float64, width float32) {
	value := 0.0
	if max > 0 {
		value = v / max
	}
	ui.Progress(c, math.Max(0, math.Min(1, value))).
		Width(width).Shrink(0)
}

// fmtPair names a pair of funds on one line.
func fmtPair(a, b string) string { return a + " × " + b }
