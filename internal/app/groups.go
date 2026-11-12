package app

import (
	"fmt"
	"strings"

	"fundbar/internal/store"
)

// holdingGroup is the group the funds the user holds or follows live in: the
// main list, and what the menu bar item adds up.
const holdingGroup = ""

// inGroup is the funds of a group, in the order the user put them in.
func (a *App) inGroup(group string) []*Fund {
	funds := make([]*Fund, 0, len(a.all))
	for _, f := range a.all {
		if f.In(group) {
			funds = append(funds, f)
		}
	}
	return funds
}

// visible is the fund the list shows at i, or nil.
func (a *App) visible(i int) *Fund {
	if i < 0 || i >= len(a.funds) {
		return nil
	}
	return a.funds[i]
}

// rebuildList puts the active group's funds in the order the list shows them,
// and chooses the fund the group showed last.
func (a *App) rebuildList() {
	a.funds = a.inGroup(a.group)
	a.sel = -1
	if code := a.selCode[a.group]; code != "" {
		for i, f := range a.funds {
			if f.Code == code {
				a.sel = i
				break
			}
		}
	}
	if a.sel < 0 && len(a.funds) > 0 {
		a.sel = 0
		a.selCode[a.group] = a.funds[0].Code
	}
}

// syncSelection remembers the fund the active group has chosen.
func (a *App) syncSelection() {
	if f := a.visible(a.sel); f != nil {
		a.selCode[a.group] = f.Code
	}
}

// choose makes a fund of the active group the chosen one.
func (a *App) choose(code string) {
	a.selCode[a.group] = code
	for i, f := range a.funds {
		if f.Code == code {
			a.sel = i
			return
		}
	}
}

// holds reports that the windows show the shares the user holds: the holding
// group does, and a made group says only how its funds moved.
func (a *App) holds(f *Fund) bool { return a.group == holdingGroup && f.Shares > 0 }

// hasGroup reports that a group the user made has this id.
func (a *App) hasGroup(id string) bool {
	for _, g := range a.groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

// groupName is a group's name, as the windows write it.
func (a *App) groupName(id string) string {
	if id == holdingGroup {
		return "持有"
	}
	for _, g := range a.groups {
		if g.ID == id {
			return g.Name
		}
	}
	return "分组"
}

// SelectGroup lists a group in the windows, and remembers the choice.
func (a *App) SelectGroup(id string) {
	if id != holdingGroup && !a.hasGroup(id) {
		return
	}
	a.syncSelection()
	// Another group lists other funds: the look-through of the old one has
	// nothing to do with what the new one shows.
	a.exitPortfolio()
	a.group = id
	a.rebuildList()
	a.cancelGroupEdit()
	a.save()
	a.redraw()
}

// beginNewGroup unfolds the field of a new group.
func (a *App) beginNewGroup() {
	a.groupEditing = true
	a.groupEditID = ""
	a.groupDraft = ""
	a.groupErr = ""
}

// beginRenameGroup unfolds the field of a group's name.
func (a *App) beginRenameGroup(id string) {
	if id == holdingGroup || !a.hasGroup(id) {
		return
	}
	a.groupEditing = true
	a.groupEditID = id
	a.groupDraft = a.groupName(id)
	a.groupErr = ""
}

// cancelGroupEdit folds the field away, keeping the groups as they were.
func (a *App) cancelGroupEdit() {
	a.groupEditing = false
	a.groupEditID = ""
	a.groupDraft = ""
	a.groupErr = ""
}

// CommitGroup makes the group the field names, or renames the one it edits,
// and shows it.
func (a *App) CommitGroup() {
	name := strings.TrimSpace(a.groupDraft)
	if name == "" {
		a.groupErr = "分组名称不能为空"
		return
	}
	for _, g := range a.groups {
		if g.Name == name && g.ID != a.groupEditID {
			a.groupErr = "已有同名分组"
			return
		}
	}
	if a.groupEditID != "" {
		for i := range a.groups {
			if a.groups[i].ID == a.groupEditID {
				a.groups[i].Name = name
			}
		}
	} else {
		a.syncSelection()
		a.groups = append(a.groups, store.Group{ID: a.nextGroupID(), Name: name})
		a.group = a.groups[len(a.groups)-1].ID
		a.selCode[a.group] = ""
		a.rebuildList()
	}
	a.cancelGroupEdit()
	a.save()
	a.redraw()
}

// nextGroupID is an id no group uses.
func (a *App) nextGroupID() string {
	for i := 1; ; i++ {
		id := fmt.Sprintf("g%d", i)
		if !a.hasGroup(id) {
			return id
		}
	}
}

// DeleteGroup forgets a group the user made. A fund that was only in it is no
// longer followed.
func (a *App) DeleteGroup(id string) {
	if id == holdingGroup || !a.hasGroup(id) {
		return
	}
	var groups []store.Group
	for _, g := range a.groups {
		if g.ID != id {
			groups = append(groups, g)
		}
	}
	a.groups = groups
	all := make([]*Fund, 0, len(a.all))
	for _, f := range a.all {
		kept := make([]string, 0, len(f.Groups))
		for _, g := range f.Groups {
			if g != id {
				kept = append(kept, g)
			}
		}
		f.Groups = kept
		if len(kept) == 0 {
			store.DropSnapshot(a.dir, f.Code)
			// As Remove does: the rows are set aside, not deleted.
			store.ShelveLedger(a.dir, f.Code)
			continue
		}
		all = append(all, f)
	}
	a.all = all
	a.pruneEstimates()
	delete(a.selCode, id)
	if a.group == id {
		a.group = holdingGroup
	}
	a.rebuildList()
	a.save()
	a.updateTray()
	a.redraw()
}

// dropFund is the funds without f.
func dropFund(funds []*Fund, f *Fund) []*Fund {
	out := make([]*Fund, 0, len(funds))
	for _, x := range funds {
		if x != f {
			out = append(out, x)
		}
	}
	return out
}

// applyPending does what a menu asked for, before the frame that shows the
// outcome is built.
func (a *App) applyPending() {
	// A click on a list row while the look-through showed says which fund to
	// show on the way out: the list chose a scratch row rather than the fund
	// the pane had been showing.
	if a.portfolio && a.listSel >= 0 {
		i := a.listSel
		a.listSel = -1
		a.showFundRow(i)
	}
	// A group without shares, or another group altogether, has no portfolio
	// to look through.
	if a.portfolio && !a.portfolioAvailable() {
		a.exitPortfolio()
	}
	if a.pendingRemove >= 0 {
		i := a.pendingRemove
		a.pendingRemove = -1
		a.Remove(i)
	}
	if a.pendingLeave >= 0 {
		i := a.pendingLeave
		a.pendingLeave = -1
		a.LeaveGroup(i)
	}
	if a.wantNewGroup {
		a.wantNewGroup = false
		a.beginNewGroup()
	}
	if a.wantRename != "" {
		id := a.wantRename
		a.wantRename = ""
		a.beginRenameGroup(id)
	}
	if a.wantDeleteGroup != "" {
		id := a.wantDeleteGroup
		a.wantDeleteGroup = ""
		a.DeleteGroup(id)
	}
	if a.wantJoin >= 0 && a.wantJoinGroup != "" {
		i, id := a.wantJoin, a.wantJoinGroup
		a.wantJoin, a.wantJoinGroup = -1, ""
		a.JoinGroup(i, id)
	}
	if a.wantGroupSet {
		id := a.wantGroup
		a.wantGroup, a.wantGroupSet = "", false
		a.SelectGroup(id)
	}
	if a.wantTxEdit.code != "" {
		ref := a.wantTxEdit
		a.wantTxEdit = txRef{}
		if f := a.byCode(ref.code); f != nil {
			a.beginEditTxn(f, ref.id)
		}
	}
	if a.wantTxDelete.code != "" {
		ref := a.wantTxDelete
		a.wantTxDelete = txRef{}
		if f := a.byCode(ref.code); f != nil {
			a.deleteTxn(f, ref.id)
		}
	}
	if a.wantManual != "" {
		code := a.wantManual
		a.wantManual = ""
		if f := a.byCode(code); f != nil {
			a.toManual(f)
		}
	}
}

// JoinGroup lists the fund at i in a group it is not in yet.
func (a *App) JoinGroup(i int, id string) {
	f := a.visible(i)
	if f == nil || f.In(id) || (id != holdingGroup && !a.hasGroup(id)) {
		return
	}
	f.Groups = append(f.Groups, id)
	a.save()
	a.updateTray()
	a.redraw()
}

// pickFund shows a fund in the window, switching to a group that lists it
// when the active one does not.
func (a *App) pickFund(code string) {
	f := a.byCode(code)
	if f == nil {
		return
	}
	// A fund asked for from the menu bar is a fund's detail, not the
	// portfolio overview.
	a.exitPortfolio()
	if !f.In(a.group) {
		next := f.Groups[0]
		for _, g := range f.Groups {
			if g == holdingGroup {
				next = holdingGroup
				break
			}
		}
		a.syncSelection()
		a.group = next
	}
	a.selCode[a.group] = code
	a.rebuildList()
	a.pick = -1
}
