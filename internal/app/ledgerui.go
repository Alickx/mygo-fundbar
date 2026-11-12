package app

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/ledger"
	"fundbar/internal/model"
	"fundbar/internal/store"
)

// txRef names a fund's row, for an edit or a delete a click or a menu asked
// for.
type txRef struct {
	code string
	id   int
}

// ledgerUndo is the rows a delete removed, which the toast it leaves puts
// back.
type ledgerUndo struct {
	code string
	txns []ledger.Txn
}

// txForm is the transaction the card is adding or editing: a draft of its
// own, until the user saves it or cancels.
type txForm struct {
	open bool
	code string
	// id is the row being edited, and zero while adding. session tells one
	// draft from the next: its fields get a key of their own, so a new draft
	// starts with fresh editors.
	id      int
	session int
	// kind is the index of the chosen row of txKinds.
	kind       int
	date       time.Time
	sharesText string
	priceText  string
	feeText    string
	amountText string
	// byAmount says a buy's first field is the money paid rather than the
	// shares: the price turns one into the other.
	byAmount bool
	// priceAuto says the price still follows the date, and navNote is what
	// the date's net asset value says.
	priceAuto bool
	navNote   string
	err       string
}

// txKinds are the kinds the form offers, in the order they show. The opening
// row is edited by itself, without this control.
var txKinds = []struct {
	label string
	kind  ledger.Kind
}{
	{"买入", ledger.Buy},
	{"卖出", ledger.Sell},
	{"现金分红", ledger.Dividend},
	{"红利再投", ledger.Reinvest},
}

// kindIndex is where a kind sits in txKinds, or the first row for a kind the
// form does not offer.
func kindIndex(k ledger.Kind) int {
	for i, entry := range txKinds {
		if entry.kind == k {
			return i
		}
	}
	return 0
}

// beginTxn opens an empty draft for a fund. The first row on a position
// typed by hand begins with that position as its opening row.
func (a *App) beginTxn(f *Fund) {
	a.tx = txForm{
		open: true, code: f.Code, session: a.tx.session + 1, date: now(),
		priceAuto: true,
	}
	a.prefillNav(f)
}

// beginEditTxn opens a row of the fund's ledger in the form.
func (a *App) beginEditTxn(f *Fund, id int) {
	var row ledger.Txn
	found := false
	for _, t := range f.Txns {
		if t.ID == id {
			row, found = t, true
			break
		}
	}
	if !found {
		return
	}
	form := txForm{open: true, code: f.Code, id: id, session: a.tx.session + 1}
	if row.Kind == ledger.Open {
		form.sharesText = trim(row.Shares, 4)
		if row.Price > 0 {
			form.priceText = trim(row.Price, 4)
		}
	} else {
		form.kind = kindIndex(row.Kind)
		form.date = parseDay(row.Date)
		if form.date.IsZero() {
			form.date = now()
		}
		form.sharesText = trim(row.Shares, 4)
		switch row.Kind {
		case ledger.Buy, ledger.Sell:
			if row.Price > 0 {
				form.priceText = trim(row.Price, 4)
			}
			if row.Fee > 0 {
				form.feeText = trim(row.Fee, 2)
			}
		case ledger.Dividend:
			form.amountText = trim(row.Amount, 2)
		case ledger.Reinvest:
			form.amountText = trim(row.Shares, 4)
		}
	}
	a.tx = form
	if row.Kind != ledger.Open {
		a.prefillNav(f)
	}
}

// cancelTxn folds the draft away, keeping the rows as they were.
func (a *App) cancelTxn() {
	a.tx.open = false
	a.tx.code = ""
	a.tx.id = 0
	a.tx.err = ""
	a.tx.navNote = ""
}

// ledgerBase is the rows a draft is added to: the fund's ledger, or the
// position typed by hand as an opening row. A position of no shares opens
// nothing.
func ledgerBase(f *Fund) []ledger.Txn {
	if f.ledgered() {
		return f.Txns
	}
	if f.Shares > 0 {
		return []ledger.Txn{ledger.Opening(f.Shares, f.Cost)}
	}
	return nil
}

// newRowID is the id Put gives the next row, after the ids it hands rows that
// have none.
func newRowID(txns []ledger.Txn) int {
	id := ledger.NextID(txns)
	for _, t := range txns {
		if t.ID == 0 {
			id++
		}
	}
	return id
}

// prefillNav fills the price from the fund's own values when it still follows
// the date, and says what the date's value is either way.
func (a *App) prefillNav(f *Fund) {
	day := a.tx.date.In(cst).Format("2006-01-02")
	nav, note, ok := navOn(f.Navs, day)
	a.tx.navNote = note
	if !a.tx.priceAuto {
		return
	}
	if ok {
		a.tx.priceText = fmt.Sprintf("%.4f", nav.Unit)
	} else {
		a.tx.priceText = ""
	}
}

// navOn is the value a transaction on date uses: that day's published value,
// the next one when the day has none, and none at all when the fund has not
// published a later one. The note says which of the three it is.
func navOn(navs []model.Nav, date string) (model.Nav, string, bool) {
	for _, n := range navs {
		if n.Date == date {
			return n, fmt.Sprintf("当日净值 %.4f", n.Unit), true
		}
	}
	for _, n := range navs {
		if n.Date > date {
			return n, fmt.Sprintf("非交易日，按 %s 的净值 %.4f", n.Date, n.Unit), true
		}
	}
	return model.Nav{}, "该日净值尚未公布，可稍后补填", false
}

// parseNumber reads a field that may hold thousands separators, and reports
// whether it held a number at all.
func parseNumber(text string) (float64, bool) {
	s := strings.ReplaceAll(strings.TrimSpace(text), ",", "")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// buySharesFromAmount turns the money a buy took, fee included, into the
// shares the ledger keeps. The ledger rounds shares to four decimals, so the
// preview and the saved row agree.
func buySharesFromAmount(amount, fee, price float64) float64 {
	if price <= 0 {
		return 0
	}
	return math.Round((amount-fee)/price*1e4) / 1e4
}

// requiredNumber reads a field every row needs, with the message a blank
// field gets and the one a value outside the range gets.
func requiredNumber(text, blank, bad string, positive bool) (float64, string) {
	if strings.TrimSpace(text) == "" {
		return 0, blank
	}
	v, ok := parseNumber(text)
	if !ok || (positive && v <= 0) {
		return 0, bad
	}
	return v, ""
}

// txOpening reports that the draft edits the fund's opening row.
func (a *App) txOpening(f *Fund) bool {
	if a.tx.id == 0 {
		return false
	}
	for _, t := range f.Txns {
		if t.ID == a.tx.id {
			return t.Kind == ledger.Open
		}
	}
	return false
}

// txRow is the draft as a row, or the message that keeps it from being one.
func (a *App) txRow(f *Fund) (ledger.Txn, string) {
	if a.txOpening(f) {
		shares, msg := requiredNumber(a.tx.sharesText, "请输入份额", "份额应大于 0", true)
		if msg != "" {
			return ledger.Txn{}, msg
		}
		cost := 0.0
		if strings.TrimSpace(a.tx.priceText) != "" {
			v, ok := parseNumber(a.tx.priceText)
			if !ok || v < 0 {
				return ledger.Txn{}, "净值应大于 0"
			}
			cost = v
		}
		return ledger.Txn{Kind: ledger.Open, Shares: shares, Price: cost}, ""
	}

	kind := txKinds[a.tx.kind].kind
	date := a.tx.date.In(cst).Format("2006-01-02")
	if date > now().Format("2006-01-02") {
		return ledger.Txn{}, "日期不能晚于今天"
	}
	switch kind {
	case ledger.Buy, ledger.Sell:
		// A buy may be given as the money it took instead of the shares it
		// bought: the amount includes the fee, as a fund platform's 买入金额
		// does, so the fee comes out before the price divides it.
		var shares, price, amount float64
		byAmount := kind == ledger.Buy && a.tx.byAmount
		var msg string
		if byAmount {
			amount, msg = requiredNumber(a.tx.amountText, "请输入买入金额", "买入金额应大于 0", true)
		} else {
			shares, msg = requiredNumber(a.tx.sharesText, "请输入份额", "份额应大于 0", true)
		}
		if msg != "" {
			return ledger.Txn{}, msg
		}
		price, msg = requiredNumber(a.tx.priceText, "请输入成交净值", "净值应大于 0", true)
		if msg != "" {
			return ledger.Txn{}, msg
		}
		fee := 0.0
		if strings.TrimSpace(a.tx.feeText) != "" {
			v, ok := parseNumber(a.tx.feeText)
			if !ok || v < 0 {
				return ledger.Txn{}, "手续费不能为负"
			}
			fee = v
		}
		if byAmount {
			if fee >= amount {
				return ledger.Txn{}, "手续费不能超过买入金额"
			}
			shares = buySharesFromAmount(amount, fee, price)
		}
		if kind == ledger.Sell && fee > shares*price {
			return ledger.Txn{}, "手续费不能超过卖出金额"
		}
		return ledger.Txn{Date: date, Kind: kind, Shares: shares, Price: price, Fee: fee}, ""
	case ledger.Dividend:
		amount, msg := requiredNumber(a.tx.amountText, "请输入分红金额", "分红金额应大于 0", true)
		if msg != "" {
			return ledger.Txn{}, msg
		}
		return ledger.Txn{Date: date, Kind: ledger.Dividend, Amount: amount}, ""
	case ledger.Reinvest:
		shares, msg := requiredNumber(a.tx.amountText, "请输入再投份额", "份额应大于 0", true)
		if msg != "" {
			return ledger.Txn{}, msg
		}
		return ledger.Txn{Date: date, Kind: ledger.Reinvest, Shares: shares}, ""
	}
	return ledger.Txn{}, "交易记录有误：未知的交易类型"
}

// txProblemText words a problem the ledger found in Chinese.
func txProblemText(p ledger.Problem) string {
	switch p {
	case ledger.Oversell:
		return "卖出份额超过当日持有"
	case ledger.TwoOpenings:
		return "有多笔期初持仓"
	case ledger.UnknownKind:
		return "未知的交易类型"
	case ledger.BadShares:
		return "份额无效"
	case ledger.BadPrice:
		return "净值无效"
	case ledger.BadFee:
		return "手续费无效"
	case ledger.BadAmount:
		return "分红金额无效"
	case ledger.BadDate:
		return "日期无效"
	}
	return "交易记录有误"
}

// txSaveError words why Put or Remove refused a change. A sale the draft
// itself made too large names the shares it could sell; one the change breaks
// later names that day.
func txSaveError(err error, ourID int) string {
	var problem *ledger.Error
	if errors.As(err, &problem) {
		if problem.Problem == ledger.Oversell {
			if problem.ID == ourID {
				return "卖出份额超过当日可卖的 " + sharesText(problem.Held, 4) + " 份"
			}
			if problem.Date != "" {
				return "这样改动后，" + problem.Date + " 的卖出将超过持有份额"
			}
		}
		return "交易记录有误：" + txProblemText(problem.Problem)
	}
	return "交易记录保存失败：" + err.Error()
}

// replayProblem reports that the rows already have a problem, which the
// ledger lets a change go through for.
func replayProblem(txns []ledger.Txn) bool {
	_, _, err := ledger.Replay(txns)
	return err != nil
}

// commitTxn puts the draft into the ledger, or says why it cannot.
func (a *App) commitTxn(f *Fund) {
	if !a.tx.open || a.tx.code != f.Code {
		return
	}
	t, msg := a.txRow(f)
	if msg != "" {
		a.tx.err = msg
		return
	}
	// The draft's fields were read above; this is the ledger's own word on
	// the row they make, before it is put among the others.
	if err := ledger.Check(t); err != nil {
		a.tx.err = txSaveError(err, 0)
		return
	}
	base := ledgerBase(f)
	ourID := a.tx.id
	if ourID == 0 {
		ourID = newRowID(base)
	} else {
		t.ID = a.tx.id
	}
	txns, _, err := ledger.Put(base, t)
	// Rows that already had a problem stay editable: the change goes in even
	// when the warning remains, so that a broken file can be put right a row
	// at a time. A change that breaks sound rows is refused as a whole.
	if err != nil && !replayProblem(base) {
		a.tx.err = txSaveError(err, ourID)
		return
	}
	if err := a.setLedger(f, txns); err != nil {
		// The rows did not reach the file. The draft stays open, so that the
		// user can try again rather than find the row gone at the next
		// launch.
		a.tx.err = "交易记录保存失败：" + err.Error()
		return
	}
	// A save after a delete supersedes the rows that toast would put back.
	a.txUndo = ledgerUndo{}
	a.cancelTxn()
}

// deleteTxn removes a row, remembering the rows it had so that the toast it
// leaves can put them back.
func (a *App) deleteTxn(f *Fund, id int) bool {
	old := append([]ledger.Txn(nil), f.Txns...)
	txns, _, err := ledger.Remove(f.Txns, id)
	if err != nil && !replayProblem(f.Txns) {
		msg := txSaveError(err, id)
		a.txUndo = ledgerUndo{}
		if a.tx.open && a.tx.code == f.Code {
			a.tx.err = msg
		} else {
			a.txNote = msg
		}
		return false
	}
	if err := a.setLedger(f, txns); err != nil {
		// Nothing was removed: the rows are as the file still has them, and
		// there is nothing for a toast to put back.
		msg := "交易记录保存失败：" + err.Error()
		a.txUndo = ledgerUndo{}
		if a.tx.open && a.tx.code == f.Code {
			a.tx.err = msg
		} else {
			a.txNote = msg
		}
		return false
	}
	a.txUndo = ledgerUndo{code: f.Code, txns: old}
	if a.tx.open && a.tx.code == f.Code {
		a.cancelTxn()
	}
	a.txNote = "已删除这笔交易"
	return true
}

// undoTxn puts back the rows the last delete removed.
func (a *App) undoTxn() {
	if a.txUndo.code == "" {
		return
	}
	f := a.byCode(a.txUndo.code)
	txns := a.txUndo.txns
	a.txUndo = ledgerUndo{}
	if f == nil {
		return
	}
	if err := a.setLedger(f, txns); err != nil {
		a.txNote = "交易记录保存失败：" + err.Error()
	}
}

// toManual returns a fund to a position typed by hand, keeping the shares
// and cost it holds now. The rows are set aside rather than deleted, and the
// toast it leaves puts them back: they are what the user typed in, and no
// network can give them back.
func (a *App) toManual(f *Fund) {
	old := append([]ledger.Txn(nil), f.Txns...)
	shares, cost := f.Shares, f.Cost
	f.Txns, f.steps = nil, nil
	f.pos = ledger.Position{}
	f.ledgerErr = nil
	f.Shares, f.Cost = shares, cost
	f.editText = false
	store.ShelveLedger(a.dir, f.Code)
	a.cancelTxn()
	if len(old) > 0 {
		a.txUndo = ledgerUndo{code: f.Code, txns: old}
		a.txNote = "已改回手动录入"
	}
	a.changed()
}

// accountWarning says why the rows could not be read or replayed, and "" when
// they are sound.
func accountWarning(f *Fund) string {
	if f.ledgerErr == nil {
		return ""
	}
	if errors.Is(f.ledgerErr, store.ErrLedgerUnreadable) {
		return "交易记录文件损坏，已另存为 .bad 文件；当前显示的是上次保存的份额与成本"
	}
	var problem *ledger.Error
	if errors.As(f.ledgerErr, &problem) {
		return "交易记录有误：" + txProblemText(problem.Problem)
	}
	return "交易记录读取失败：" + f.ledgerErr.Error()
}

// holdingNote is the summary the folded card shows.
func holdingNote(f *Fund, editing bool) string {
	if !f.ledgered() {
		if editing {
			return "修改自动保存"
		}
		if f.Shares <= 0 {
			return "未录入"
		}
		note := sharesText(f.Shares, 4) + " 份 · 成本 " + fmt.Sprintf("%.4f", f.Cost)
		if targets := targetLine(f); targets != "" {
			note += " · " + targets
		}
		return note
	}
	count := len(f.Txns)
	if f.pos.Shares <= 0 {
		return "已清仓 · 累计收益 " + signedMoney(f.TotalProfit()) +
			fmt.Sprintf(" · %d 笔交易", count)
	}
	note := sharesText(f.Shares, 4) + " 份 · 成本 " + fmt.Sprintf("%.4f", f.Cost) +
		fmt.Sprintf(" · %d 笔交易", count)
	if !editing {
		if targets := targetLine(f); targets != "" {
			note += " · " + targets
		}
	}
	return note
}

// holdingBody is the part of the card under the header, which unfolds and
// folds away rather than taking the room at once.
func (a *App) holdingBody(c *ui.Context, t *ui.Theme, f *Fund) {
	box := ui.Box(c).ClipY()
	target := float32(0)
	if a.editing {
		target = 1
	}
	h := box.AnimateTo("holding-form", target, ui.Smooth)
	if h < 1 {
		box.Height(max(0, min(h, 1)) * a.holdBodyH)
	}
	// A folded card builds nothing: fields no one can see should not take
	// the keyboard's focus, and a long ledger should not be laid out every
	// frame for a card that is closed.
	if !a.editing && h < 0.001 {
		return
	}
	box.Children(func() {
		inner := ui.Column(c).Padding(12, 0, 0, 0).Gap(12)
		inner.Children(func() {
			if f.ledgered() {
				a.ledgerBody(c, t, f)
			} else {
				a.manualBody(c, t, f)
			}
		})
		// Bounds is the previous frame's box: the content is measured as it
		// laid out, and the clip above follows it as the card opens.
		a.holdBodyH = inner.Bounds().H
	})
}

// manualBody is the position typed by hand, with the invitation to keep the
// transactions instead.
func (a *App) manualBody(c *ui.Context, t *ui.Theme, f *Fund) {
	if note := accountWarning(f); note != "" {
		ui.Text(c, note).FontSize(11).TextColor(t.Danger).SingleLine().
			Ellipsis("…").Tooltip(note)
	}
	if !f.editText {
		f.sharesText = trim(f.Shares, 4)
		f.costText = trim(f.Cost, 4)
		f.takeText = targetText(f.TakeProfit)
		f.stopText = targetText(f.StopLoss)
		f.editText = true
	}
	ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
		ui.Column(c).Gap(5).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, "持有份额").FontSize(11.5).TextColor(t.TextMuted)
			if ui.TextInput(c, &f.sharesText).Placeholder("0.00").
				Label("持有份额输入").Changed() &&
				readNumber(f.sharesText, &f.Shares, 0, 1e10) {
				a.changed()
			}
		})
		ui.Column(c).Gap(5).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, "成本价").FontSize(11.5).TextColor(t.TextMuted)
			if ui.TextInput(c, &f.costText).Placeholder("0.0000").
				Label("成本价输入").Changed() &&
				readNumber(f.costText, &f.Cost, 0, 1e6) {
				a.changed()
			}
		})
	})
	a.targetFields(c, t, f)
	if a.tx.open && a.tx.code == f.Code {
		a.txForm(c, t, f)
		return
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		const invite = "买卖过多次？记下每笔交易，份额与成本自动计算"
		ui.Text(c, invite).FontSize(11).TextColor(t.TextMuted).Grow(1).Basis(0).
			SingleLine().Ellipsis("…").Tooltip(invite)
		if ui.Button(c, "记一笔交易").Shrink(0).Clicked() {
			a.beginTxn(f)
		}
	})
}

// ledgerBody is the ledger's rows, the form, and the price targets.
func (a *App) ledgerBody(c *ui.Context, t *ui.Theme, f *Fund) {
	a.txToolbar(c, t, f)
	if note := accountWarning(f); note != "" {
		ui.Text(c, note).FontSize(11).TextColor(t.Danger).SingleLine().
			Ellipsis("…").Tooltip(note)
	}
	if a.tx.open && a.tx.code == f.Code {
		a.txForm(c, t, f)
	}
	a.txList(c, t, f)
	a.targetFields(c, t, f)
}

// txToolbar is the ledger's count, the way back to two numbers, and the
// button that adds a row.
func (a *App) txToolbar(c *ui.Context, t *ui.Theme, f *Fund) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "交易记录 · %d 笔", len(f.Txns)).FontSize(11.5).
			TextColor(t.TextMuted).NoWrap().Grow(1).Basis(0)
		ui.PopoverMenuButton(c, "⋯", func(m *ui.Menu) {
			item := m.Item("改回手动录入")
			item.Tooltip("保留当前份额与成本；交易记录另存备份，可撤销")
			if item.Chosen() {
				a.wantManual = f.Code
			}
		}).Shrink(0).Tooltip("更多")
		if ui.Button(c, "记一笔").Shrink(0).Clicked() {
			a.beginTxn(f)
		}
	})
}

// txForm draws the draft row: what it did, on which day, and how much.
func (a *App) txForm(c *ui.Context, t *ui.Theme, f *Fund) {
	opening := a.txOpening(f)
	if opening {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "期初持仓").FontSize(12.5).FontWeight(600).NoWrap()
			ui.Text(c, "最早的一笔，不填日期").FontSize(11).
				TextColor(t.TextMuted).NoWrap()
		})
	} else {
		ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
			ui.Column(c).Gap(5).Shrink(0).Children(func() {
				ui.Text(c, "交易类型").FontSize(11.5).TextColor(t.TextMuted)
				ui.Segmented(c, &a.tx.kind, "买入", "卖出", "现金分红", "红利再投").
					Label("交易类型").OnChange(func() { a.tx.err = "" })
			})
			ui.Column(c).Gap(5).Grow(1).Basis(0).MinWidth(0).Children(func() {
				ui.Text(c, "交易日期").FontSize(11.5).TextColor(t.TextMuted)
				if ui.DateInput(c, &a.tx.date).Label("交易日期").Changed() {
					a.prefillNav(f)
					a.tx.err = ""
				}
			})
		})
	}

	switch {
	case opening:
		ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
			a.txField(c, t, "份额", &a.tx.sharesText, "0.0000", "份额输入", "shares",
				func() { a.tx.err = "" })
			a.txField(c, t, "成本价", &a.tx.priceText, "0.0000", "成本价输入", "price",
				func() { a.tx.err = "" })
		})
	default:
		kind := txKinds[a.tx.kind].kind
		switch kind {
		case ledger.Buy, ledger.Sell:
			// A sale suggests the shares held; a buy shares nothing.
			held := "0.0000"
			if kind == ledger.Sell && f.Shares > 0 {
				held = sharesText(f.Shares, 4)
			}
			ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
				if kind == ledger.Buy {
					a.txBuyField(c, t)
				} else {
					a.txField(c, t, "份额", &a.tx.sharesText, held, "份额输入", "shares",
						func() { a.tx.err = "" })
				}
				a.txField(c, t, "成交净值", &a.tx.priceText, "0.0000", "成交净值输入", "price",
					func() {
						a.tx.priceAuto = false
						a.tx.navNote = ""
						a.tx.err = ""
					})
				a.txField(c, t, "手续费", &a.tx.feeText, "0.00", "手续费输入", "fee",
					func() { a.tx.err = "" })
			})
		case ledger.Dividend:
			ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
				a.txField(c, t, "分红金额", &a.tx.amountText, "0.00", "分红金额输入", "amount",
					func() { a.tx.err = "" })
			})
		case ledger.Reinvest:
			ui.Row(c).Gap(18).AlignItems(ui.End).Children(func() {
				a.txField(c, t, "再投份额", &a.tx.amountText, "0.0000", "再投份额输入", "amount",
					func() { a.tx.err = "" })
			})
		}
	}

	status, bad := a.txStatus(f)
	color := t.TextMuted
	if bad {
		color = t.Danger
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, status).FontSize(11).TextColor(color).FontFeatures("tnum").
			SingleLine().Ellipsis("…").Tooltip(status).Grow(1).Basis(0)
		if a.tx.id != 0 {
			if ui.Button(c, "删除").Shrink(0).Clicked() {
				a.deleteTxn(f, a.tx.id)
			}
		}
		if ui.Button(c, "取消").Shrink(0).Clicked() {
			a.cancelTxn()
		}
		if ui.PrimaryButton(c, "保存").Shrink(0).Clicked() {
			a.commitTxn(f)
		}
	})
}

// txField is one labelled field of the draft, with a key of its own for the
// draft's session.
func (a *App) txField(c *ui.Context, t *ui.Theme, label string, text *string, placeholder, accLabel, name string, changed func()) {
	ui.Column(c).Gap(5).Grow(1).Basis(0).MinWidth(0).Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted)
		key := fmt.Sprintf("tx-%d-%s", a.tx.session, name)
		// Key before construction: an input already built has its editor.
		if ui.TextInput(c.Key(key), text).Placeholder(placeholder).Label(accLabel).
			Changed() {
			changed()
		}
	})
}

// txBuyField is a buy's first field: the shares bought or the money it took,
// with the switch between them on the label's line. The price and the fee do
// not change with the choice; the ledger keeps shares either way.
func (a *App) txBuyField(c *ui.Context, t *ui.Theme) {
	text, label, placeholder, accLabel, name :=
		&a.tx.sharesText, "份额", "0.0000", "份额输入", "shares"
	swap, tip := "按金额", "改用买入金额记录：金额含手续费，按成交净值自动折算份额"
	if a.tx.byAmount {
		text, label, placeholder, accLabel, name =
			&a.tx.amountText, "买入金额", "0.00", "买入金额输入", "buy-amount"
		swap, tip = "按份额", "改用份额记录：按成交净值自动折算买入金额"
	}
	ui.Column(c).Gap(5).Grow(1).Basis(0).MinWidth(0).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(t.TextMuted).NoWrap()
			ui.Spacer(c)
			b := ui.ButtonBase(c).Shrink(0).Padding(0, 2)
			color := t.Accent
			if b.Hovered() {
				color = t.AccentHover
			}
			b.Children(func() {
				ui.Text(c, swap).FontSize(11).TextColor(color).NoWrap()
			}).Tooltip(tip)
			if b.Clicked() {
				a.toggleBuyInput()
			}
		})
		key := fmt.Sprintf("tx-%d-%s", a.tx.session, name)
		if ui.TextInput(c.Key(key), text).Placeholder(placeholder).Label(accLabel).
			Changed() {
			a.tx.err = ""
		}
	})
}

// toggleBuyInput trades the buy's first field between shares and money,
// carrying the value shown over at the day's price.
func (a *App) toggleBuyInput() {
	a.tx.byAmount = !a.tx.byAmount
	a.tx.err = ""
	price, ok := parseNumber(a.tx.priceText)
	if !ok || price <= 0 {
		return
	}
	fee, feeOK := parseNumber(a.tx.feeText)
	if !feeOK || fee < 0 {
		fee = 0
	}
	if a.tx.byAmount {
		if shares, ok := parseNumber(a.tx.sharesText); ok {
			a.tx.amountText = trim(shares*price+fee, 2)
		}
		return
	}
	if amount, ok := parseNumber(a.tx.amountText); ok && amount > fee {
		a.tx.sharesText = trim(buySharesFromAmount(amount, fee, price), 4)
	}
}

// txStatus is the line under the draft's fields: the money it adds up to and
// what the date's value says, or the problem that keeps it from being saved.
func (a *App) txStatus(f *Fund) (string, bool) {
	if a.tx.err != "" {
		return a.tx.err, true
	}
	opening := a.txOpening(f)
	kind := ledger.Open
	if !opening {
		kind = txKinds[a.tx.kind].kind
	}
	shares, sharesOK := parseNumber(a.tx.sharesText)
	price, priceOK := parseNumber(a.tx.priceText)
	fee := 0.0
	feeOK := true
	if strings.TrimSpace(a.tx.feeText) != "" {
		fee, feeOK = parseNumber(a.tx.feeText)
	}
	amount, amountOK := parseNumber(a.tx.amountText)

	var line string
	switch kind {
	case ledger.Open:
		switch {
		case sharesOK && priceOK:
			line = "成本合计 " + money(shares*price) + " 元"
		case sharesOK:
			line = sharesText(shares, 4) + " 份"
		}
	case ledger.Buy:
		switch {
		case a.tx.byAmount && amountOK && amount > 0 && feeOK && fee >= amount:
			return "手续费不能超过买入金额", true
		case a.tx.byAmount && amountOK && amount > 0 && priceOK && price > 0 && feeOK:
			line = "份额 " + sharesText(buySharesFromAmount(amount, fee, price), 4) +
				" 份 · 买入金额 " + money(amount) + " 元（含手续费 " + money(fee) + "）"
		case !a.tx.byAmount && sharesOK && priceOK && feeOK:
			line = "金额 " + money(shares*price+fee) + " 元（含手续费 " + money(fee) + "）"
		}
	case ledger.Sell:
		if sharesOK && priceOK && feeOK {
			line = "到账 " + money(shares*price-fee) + " 元（已扣手续费 " + money(fee) + "）"
		}
	case ledger.Dividend:
		if amountOK {
			line = "分红 " + money(amount) + " 元"
		}
	case ledger.Reinvest:
		if amountOK {
			line = "再投 " + sharesText(amount, 4) + " 份"
		}
	}
	if !opening && a.tx.navNote != "" {
		if line != "" {
			line += " · "
		}
		line += a.tx.navNote
	}
	return line, false
}

// txList lists the rows newest first, the opening row last. A long ledger
// scrolls inside the card, as the list of daily values does, rather than
// pushing the rest of the fund out of the window.
func (a *App) txList(c *ui.Context, t *ui.Theme, f *Fund) {
	if len(f.steps) == 0 {
		ui.Text(c, "还没有交易记录").FontSize(11.5).TextColor(t.TextMuted)
		return
	}
	const (
		dateW   = 76
		priceW  = 64
		amountW = 86
		// txListRows is how many rows show before the list scrolls, and
		// txRowHeight what one takes with the hairline between rows.
		txListRows  = 8
		txRowHeight = 28
	)
	ui.Scroll(c).Height(float32(min(len(f.steps), txListRows))*txRowHeight + 2).
		Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				shown := 0
				for i := len(f.steps) - 1; i >= 0; i-- {
					step := f.steps[i]
					row := ui.Row(c).Height(27).Padding(0, 7).Radius(6).
						AlignItems(ui.Center).Gap(8)
					if shown%2 == 1 {
						row.Background(t.Text.Alpha(0.035))
					}
					tip := txRowTip(step)
					row.Tooltip(tip)
					id := step.Txn.ID
					row.ContextMenu(func(m *ui.Menu) {
						if m.Item("编辑").Chosen() {
							a.wantTxEdit = txRef{code: f.Code, id: id}
						}
						if m.Item("删除").Danger().Chosen() {
							a.wantTxDelete = txRef{code: f.Code, id: id}
						}
					})
					row.Children(func() {
						date := step.Txn.Date
						if step.Txn.Kind == ledger.Open {
							date = "期初"
						}
						ui.Text(c, date).FontSize(11.5).TextColor(t.TextMuted).
							FontFeatures("tnum").NoWrap().Width(dateW)
						a.txKindPill(c, t, step.Txn.Kind)
						shares := "—"
						if step.Txn.Kind != ledger.Dividend {
							shares = trim(step.Txn.Shares, 4)
						}
						ui.Text(c, shares).FontSize(11.5).FontFeatures("tnum").
							NoWrap().Grow(1).Basis(0).TextAlign(ui.End)
						price := "—"
						if step.Txn.Price > 0 {
							price = fmt.Sprintf("%.4f", step.Txn.Price)
						}
						ui.Text(c, price).FontSize(11.5).TextColor(t.TextMuted).
							FontFeatures("tnum").NoWrap().Width(priceW).TextAlign(ui.End)
						amount, has := txAmount(step.Txn)
						text := "—"
						if has {
							text = money(amount)
						}
						ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted).
							FontFeatures("tnum").NoWrap().Width(amountW).TextAlign(ui.End)
					})
					if row.Clicked() {
						a.wantTxEdit = txRef{code: f.Code, id: step.Txn.ID}
					}
					shown++
				}
			})
		})
}

// txKindPill names a row's kind in its color: a buy rises and a sale falls,
// and the rest are neutral.
func (a *App) txKindPill(c *ui.Context, t *ui.Theme, kind ledger.Kind) {
	switch kind {
	case ledger.Buy:
		color := up(t, 1)
		pill(c, "买入", color, color.Alpha(0.12)).Shrink(0)
	case ledger.Sell:
		color := up(t, -1)
		pill(c, "卖出", color, color.Alpha(0.12)).Shrink(0)
	case ledger.Dividend:
		pill(c, "现金分红", t.TextMuted, t.Text.Alpha(0.06)).Shrink(0)
	case ledger.Reinvest:
		pill(c, "红利再投", t.TextMuted, t.Text.Alpha(0.06)).Shrink(0)
	case ledger.Open:
		pill(c, "期初持仓", t.TextMuted, t.Text.Alpha(0.06)).Shrink(0)
	}
}

// txAmount is the money a row moved: what a buy paid, a sale received, or a
// dividend paid.
func txAmount(t ledger.Txn) (float64, bool) {
	switch t.Kind {
	case ledger.Buy:
		return t.Shares*t.Price + t.Fee, true
	case ledger.Sell:
		return t.Shares*t.Price - t.Fee, true
	case ledger.Dividend:
		return t.Amount, true
	}
	return 0, false
}

// txRowTip tells the story of a row: what it cost, where it left the
// position, and what a sale realised.
func txRowTip(step ledger.Step) string {
	var parts []string
	if step.Txn.Fee > 0 {
		parts = append(parts, "手续费 "+money(step.Txn.Fee)+" 元")
	}
	parts = append(parts, "交易后 "+sharesText(step.Shares, 4)+" 份")
	if step.Cost > 0 {
		parts = append(parts, "成本 "+fmt.Sprintf("%.4f", step.Cost))
	}
	if step.Realized != 0 {
		parts = append(parts, "已实现 "+signedMoney(step.Realized))
	}
	return strings.Join(parts, " · ")
}

// txToast shows the line the card leaves after a change, with its undo when
// the change removed rows.
func (a *App) txToast(c *ui.Context) {
	if a.txNote == "" {
		return
	}
	note := a.txNote
	a.txNote = ""
	if a.txUndo.code == "" {
		c.Toast(note)
		return
	}
	c.ToastAction(note, "撤销", a.undoTxn)
}

// profitTip is the long story of the cumulative gain: what the shares held
// gained, what sales took, what the fund paid, and what was put in.
func profitTip(f *Fund) string {
	if !f.ledgered() {
		return ""
	}
	holding := f.Shares*f.EstNav() - f.pos.Basis
	lines := []string{
		"持有收益 " + signedMoney(holding) + "（" + pct(f.HoldingPct()) + "）",
		"已实现收益 " + signedMoney(f.pos.Realized),
		"现金分红 " + signedMoney(f.pos.Dividends),
		"累计投入 " + money(f.pos.Invested) + " 元（含手续费 " + money(f.pos.Fees) + " 元）",
	}
	if f.pos.CostUnknown {
		lines = append(lines, "期初成本未填，累计收益含本金")
	}
	return strings.Join(lines, "\n")
}

// sharesText writes a count of shares with thousands separators and at most n
// decimals.
func sharesText(v float64, n int) string {
	s := trim(v, n)
	whole, frac, cut := strings.Cut(s, ".")
	out := groupDigits(whole)
	if cut && frac != "" {
		out += "." + frac
	}
	return out
}
