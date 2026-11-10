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
	c.state.LibraryTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)

	setupLibraryTableHeader(c.state.LibraryTable)

	// Quick actions on selected row: Enter (dialog), + (increment), - (decrement)
	c.state.LibraryTable.SetSelectedFunc(func(row, column int) {
		if row <= 0 || row > len(c.state.UserLibrary) {
			return
		}
		item := c.state.UserLibrary[row-1]
		c.showUpdateProgressDialog(item)
	})

	c.state.LibraryTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, _ := c.state.LibraryTable.GetSelection()
		if row > 0 && row <= len(c.state.UserLibrary) {
			item := c.state.UserLibrary[row-1]

			switch event.Rune() {
			case '+', '=', ']':
				c.quickStepChapter(item, 1)
				return nil
			case '-', '_', '[':
				c.quickStepChapter(item, -1)
				return nil
			case 'r', 'R':
				c.refreshLibrary()
				return nil
			}
		}
		return event
	})

	actionGuide := tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow:b]HOTKEYS:[-:-:-] [green][+][white] / [green][]][white] Next Ch  │  [yellow][-][white] / [yellow][[][white] Prev Ch  │  [cyan][Enter][white] Edit  │  [magenta][r][white] Refresh  │  [blue]⚡ TCP :9090 Auto-Sync[white]")

	guestNotice := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(c.state.LibraryTable, 0, 1, true).
		AddItem(guestNotice, 1, 0, false).
		AddItem(actionGuide, 1, 0, false)
	flex.SetBorder(true).
		SetTitle(" 📚 My Reading Library (HTTP REST + TCP Port 9090 Live Progress Sync) ").
		SetTitleColor(tcell.ColorGreen)

	return flex
}

func setupLibraryTableHeader(table *tview.Table) {
	headers := []string{"ID", "Manga Title", "Progress", "Completion", "Reading Status", "Lock Ver."}
	for col, h := range headers {
		cell := tview.NewTableCell(fmt.Sprintf(" [yellow:b]%s[-:-:-]", h)).
			SetTextColor(tcell.ColorYellow).
			SetSelectable(false).
			SetAlign(tview.AlignLeft)
		table.SetCell(0, col, cell)
	}
}

// populateLibraryTable renders the library rows with progress bars and badges
func populateLibraryTable(table *tview.Table, items []LibraryItem, catalog []*models.Manga) {
	if table == nil {
		return
	}

	// Map total chapters from catalog cache if available
	totals := make(map[string]int)
	for _, m := range catalog {
		totals[m.ID] = m.TotalChapters
	}

	// Clear previous rows except header
	for i := table.GetRowCount() - 1; i > 0; i-- {
		table.RemoveRow(i)
	}

	for idx, it := range items {
		row := idx + 1
		total := totals[it.MangaID]

		// Progress string
		progressStr := fmt.Sprintf(" Ch. %d", it.CurrentChapter)
		if total > 0 {
			progressStr = fmt.Sprintf(" Ch. %d / %d", it.CurrentChapter, total)
		}

		// Progress bar
		bar := renderProgressBar(it.CurrentChapter, total)

		// Status Badge
		statusBadge := fmt.Sprintf("[green:b] %s [-:-:-]", strings.ToUpper(it.Status))
		if strings.ToLower(it.Status) == "plan_to_read" {
			statusBadge = fmt.Sprintf("[cyan:b] %s [-:-:-]", strings.ToUpper(it.Status))
		} else if strings.ToLower(it.Status) == "completed" {
			statusBadge = fmt.Sprintf("[blue:b] %s [-:-:-]", strings.ToUpper(it.Status))
		}

		table.SetCell(row, 0, tview.NewTableCell(" "+it.MangaID).SetTextColor(tcell.ColorGray))
		table.SetCell(row, 1, tview.NewTableCell(" "+it.Title).SetTextColor(tcell.ColorWhite))
		table.SetCell(row, 2, tview.NewTableCell(progressStr).SetTextColor(tcell.ColorYellow))
		table.SetCell(row, 3, tview.NewTableCell(bar))
		table.SetCell(row, 4, tview.NewTableCell(" "+statusBadge))
		table.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf(" v%d", it.Version)).SetTextColor(tcell.ColorLightGray))
	}
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
	if c.state.Username == "" {
		c.state.SetStatus("[yellow]Guest Mode: Please log in (Press 0 or F10) to access personal reading library[white]")
		return
	}

	c.state.SetStatus("[yellow]Synchronizing library from server...[white]")
	go func() {
		items, err := c.api.FetchLibrary()
		c.state.App.QueueUpdateDraw(func() {
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Fetch library failed: %v", err))
				return
			}
			c.state.mu.Lock()
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
	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" 📝 Edit Progress: %s ", item.Title)).
		SetTitleColor(tcell.ColorYellow)

	var newChapterStr = fmt.Sprintf("%d", item.CurrentChapter)
	var newStatus = item.Status

	form.AddInputField("Current Chapter", newChapterStr, 12, nil, func(text string) {
		newChapterStr = text
	})

	statuses := []string{"reading", "completed", "plan_to_read"}
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

	form.AddButton("Save & Broadcast TCP", func() {
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
				c.refreshLibrary()
				c.state.SetStatus(fmt.Sprintf("[green]Updated '%s' to Ch. %d (%s) - TCP event dispatched[white]", item.Title, ch, newStatus))
			})
		}()
	})

	form.AddButton("Cancel", func() {
		c.state.Pages.RemovePage("update_dialog")
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(form, 9, 0, true).
		AddItem(statusFeedback, 2, 0, false)

	c.state.Pages.AddPage("update_dialog", centerPrimitive(flex, 56, 13), true, true)
}
