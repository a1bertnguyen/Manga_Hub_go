package tui

import (
	"fmt"
	"strconv"
	"strings"

	"mangahub/pkg/models"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildLibraryView constructs the User Library view with TCP sync integration
func (c *UIController) buildLibraryView() tview.Primitive {
	table := tview.NewTable()
	styleTable(table)
	c.state.LibraryTable = table
	setupLibraryTableHeader(table)
	populateLibraryTable(table, c.state.UserLibrary, c.state.MangaCatalog)
	table.SetSelectedFunc(func(row, _ int) {
		c.state.mu.RLock()
		if row <= 0 || row > len(c.state.UserLibrary) {
			c.state.mu.RUnlock()
			return
		}
		item := c.state.UserLibrary[row-1]
		c.state.mu.RUnlock()
		c.showUpdateProgressDialog(item)
	})
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'r' || event.Rune() == 'R' {
			c.refreshLibrary()
			return nil
		}
		row, _ := table.GetSelection()
		c.state.mu.RLock()
		if row <= 0 || row > len(c.state.UserLibrary) {
			c.state.mu.RUnlock()
			return event
		}
		item := c.state.UserLibrary[row-1]
		c.state.mu.RUnlock()
		switch event.Rune() {
		case '+', '=', ']':
			c.quickStepChapter(item, 1)
			return nil
		case '-', '_', '[':
			c.quickStepChapter(item, -1)
			return nil
		}
		return event
	})
	panel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(hint("Pick up where you left off. Progress syncs across your signed-in sessions."), 2, 0, false).
		AddItem(table, 0, 1, true)
	stylePanel(panel.Box, "MY LIBRARY / READING PROGRESS")
	c.focus["library"] = []tview.Primitive{table}
	return panel
}

func setupLibraryTableHeader(table *tview.Table) {
	for col, title := range []string{"TITLE", "CHAPTER", "PROGRESS", "STATUS"} {
		table.SetCell(0, col, tview.NewTableCell(" "+title).SetSelectable(false).SetTextColor(colorMuted).SetAttributes(tcell.AttrBold))
	}
}

func populateLibraryTable(table *tview.Table, items []LibraryItem, catalog []*models.Manga) {
	if table == nil {
		return
	}
	selected, _ := table.GetSelection()
	totals := make(map[string]int)
	for _, m := range catalog {
		totals[m.ID] = m.TotalChapters
	}
	table.Clear()
	setupLibraryTableHeader(table)
	if len(items) == 0 {
		table.SetCell(1, 0, tview.NewTableCell(" Your library is waiting.").SetTextColor(colorAccent).SetSelectable(false))
		table.SetCell(2, 0, tview.NewTableCell(" Sign in (F10), then add a title from Catalog (F1).").SetTextColor(colorMuted).SetSelectable(false))
		return
	}
	for i, item := range items {
		chapter := fmt.Sprintf(" %d", item.CurrentChapter)
		if total := totals[item.MangaID]; total > 0 {
			chapter = fmt.Sprintf(" %d / %d", item.CurrentChapter, total)
		}
		table.SetCell(i+1, 0, tview.NewTableCell(" "+tview.Escape(item.Title)).SetTextColor(colorText).SetExpansion(1).SetMaxWidth(36))
		table.SetCell(i+1, 1, tview.NewTableCell(chapter).SetTextColor(colorAccent))
		table.SetCell(i+1, 2, tview.NewTableCell(renderProgressBar(item.CurrentChapter, totals[item.MangaID])))
		table.SetCell(i+1, 3, tview.NewTableCell(" "+tview.Escape(strings.ReplaceAll(strings.ToUpper(item.Status), "_", " "))).SetTextColor(colorMuted))
	}
	table.Select(max(1, min(selected, len(items))), 0)
}

func renderProgressBar(current, total int) string {
	if total <= 0 {
		return "[gray]--[white]"
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}

	pct := (current * 100) / total
	barWidth := 10
	filled := (current * barWidth) / total
	empty := barWidth - filled

	barColor := "[green]"
	if pct < 30 {
		barColor = "[yellow]"
	}

	return fmt.Sprintf(" %s%s[gray]%s[white] %3d%%", barColor, strings.Repeat("█", filled), strings.Repeat("░", empty), pct)
}

func (c *UIController) refreshLibrary() {
	if c.state.username() == "" {
		c.state.SetStatus("[yellow]Guest Mode: Please log in (Press 0 or F10) to access personal reading library[white]")
		return
	}

	c.state.SetStatus("[yellow]Synchronizing library from server...[white]")
	userID := c.state.userID()
	go func() {
		items, err := c.api.FetchLibrary()
		c.state.App.QueueUpdateDraw(func() {
			if c.state.userID() != userID {
				return
			}
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Fetch library failed: %v", err))
				return
			}
			c.state.mu.Lock()
			// A slow HTTP refresh must not overwrite a newer TCP event.
			byID := make(map[string]LibraryItem)
			for _, item := range c.state.UserLibrary {
				byID[item.MangaID] = item
			}
			for i := range items {
				if newer, ok := byID[items[i].MangaID]; ok && newer.Version > items[i].Version {
					items[i] = newer
				}
				delete(byID, items[i].MangaID)
			}
			for _, item := range byID {
				items = append(items, item)
			}
			c.state.UserLibrary = items
			c.state.mu.Unlock()

			populateLibraryTable(c.state.LibraryTable, items, c.state.MangaCatalog)
			c.state.SetStatus(fmt.Sprintf("[green]Library synchronized: %d manga items[white]", len(items)))
		})
	}()
}

func (c *UIController) quickStepChapter(item LibraryItem, step int) {
	newCh := item.CurrentChapter + step
	if newCh < 0 {
		newCh = 0
	}

	c.state.SetStatus(fmt.Sprintf("[yellow]Updating %s to Ch. %d via HTTP (broadcasting to TCP)...[white]", item.Title, newCh))
	go func() {
		err := c.api.UpdateProgress(item.MangaID, newCh, item.Status)
		c.state.App.QueueUpdateDraw(func() {
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Update failed: %v", err))
				return
			}
			c.refreshLibrary()
			c.state.SetStatus(fmt.Sprintf("[green]Updated '%s' to chapter %d! TCP sync dispatched.[white]", item.Title, newCh))
		})
	}()
}

func (c *UIController) showUpdateProgressDialog(item LibraryItem) {
	form := tview.NewForm()
	styleForm(form)
	form.SetBorder(true).
		SetTitle(" Edit reading progress ").
		SetTitleColor(tcell.ColorYellow)

	var newChapterStr = fmt.Sprintf("%d", item.CurrentChapter)
	var newStatus = item.Status

	form.AddInputField("Current Chapter", newChapterStr, 12, nil, func(text string) {
		newChapterStr = text
	})

	statuses := []string{"reading", "completed", "plan_to_read", "on-hold", "dropped"}
	initIdx := 0
	for i, s := range statuses {
		if s == item.Status {
			initIdx = i
			break
		}
	}
	form.AddDropDown("Reading Status", statuses, initIdx, func(option string, optionIndex int) {
		newStatus = option
	})

	statusFeedback := tview.NewTextView().SetDynamicColors(true)

	form.AddButton("Save progress", func() {
		ch, err := strconv.Atoi(strings.TrimSpace(newChapterStr))
		if err != nil || ch < 0 {
			statusFeedback.SetText("[red]Please enter a valid non-negative integer")
			return
		}

		go func() {
			err := c.api.UpdateProgress(item.MangaID, ch, newStatus)
			c.state.App.QueueUpdateDraw(func() {
				if err != nil {
					statusFeedback.SetText(fmt.Sprintf("[red]Update failed: %v", err))
					return
				}
				c.state.Pages.RemovePage("update_dialog")
				c.state.App.SetFocus(c.state.LibraryTable)
				c.refreshLibrary()
				c.state.SetStatus(fmt.Sprintf("[green]Updated '%s' to Ch. %d (%s) - TCP event dispatched[white]", item.Title, ch, newStatus))
			})
		}()
	})

	form.AddButton("Cancel", func() {
		c.state.Pages.RemovePage("update_dialog")
		c.state.App.SetFocus(c.state.LibraryTable)
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(form, 11, 0, true).
		AddItem(statusFeedback, 2, 0, false)

	form.SetCancelFunc(func() { c.state.Pages.RemovePage("update_dialog"); c.state.App.SetFocus(c.state.LibraryTable) })
	c.state.Pages.AddPage("update_dialog", centerPrimitive(flex, 60, 15), true, true)
	c.state.App.SetFocus(form)
}
