package tui

import (
	"fmt"
	"strings"

	"mangahub/pkg/models"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// buildBrowserView constructs the 2-pane Manga Catalog & Details view
func (c *UIController) buildBrowserView() tview.Primitive {
	// 1. Search Bar at Top of Left Pane
	searchBar := tview.NewInputField().
		SetLabel(" 🔍 Search: ").
		SetFieldWidth(32).
		SetPlaceholder("Title or author... (/ to search)")

	// 2. Catalog Table
	c.state.CatalogTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)

	c.setupCatalogTableHeader()

	// 3. Right-side Details View
	c.state.DetailView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	c.state.DetailView.SetBorder(true).
		SetTitle(" 📖 Manga Details (Unary gRPC via Gin) ").
		SetTitleColor(tcell.ColorTeal)
	c.state.DetailView.SetText("\n [yellow]Select any manga from the catalog to view details.[white]\n\n" +
		" • Press [yellow]Enter[white] or [yellow]'a'[white] to add to personal library.\n" +
		" • Press [yellow]/[white] to search catalog.\n" +
		" • Press [yellow]r[white] to refresh.")

	// Search Actions
	searchBar.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			c.refreshMangaCatalog(searchBar.GetText())
			c.state.App.SetFocus(c.state.CatalogTable)
		} else if key == tcell.KeyEscape || key == tcell.KeyDown {
			c.state.App.SetFocus(c.state.CatalogTable)
		}
	})

	// Table Dynamic Selection -> Update details panel live
	c.state.CatalogTable.SetSelectionChangedFunc(func(row, column int) {
		if row <= 0 || row > len(c.state.MangaCatalog) {
			return
		}
		m := c.state.MangaCatalog[row-1]
		c.renderMangaDetails(m)
	})

	// Table Selection Action -> Add to library
	c.state.CatalogTable.SetSelectedFunc(func(row, column int) {
		if row <= 0 || row > len(c.state.MangaCatalog) {
			return
		}
		m := c.state.MangaCatalog[row-1]
		c.showAddToLibraryDialog(m)
	})

	// Keyboard shortcuts inside catalog table
	c.state.CatalogTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'a', 'A':
			row, _ := c.state.CatalogTable.GetSelection()
			if row > 0 && row <= len(c.state.MangaCatalog) {
				c.showAddToLibraryDialog(c.state.MangaCatalog[row-1])
				return nil
			}
		case '/':
			c.state.App.SetFocus(searchBar)
			return nil
		case 'r', 'R':
			searchBar.SetText("")
			c.refreshMangaCatalog("")
			return nil
		}
		return event
	})

	leftPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(searchBar, 1, 0, false).
		AddItem(c.state.CatalogTable, 0, 1, true)
	leftPane.SetBorder(true).
		SetTitle(" 📚 Manga Catalog (HTTP REST -> gRPC :9092) ").
		SetTitleColor(tcell.ColorTeal)

	split := tview.NewFlex().
		AddItem(leftPane, 0, 3, true).
		AddItem(c.state.DetailView, 0, 2, false)

	return split
}

func (c *UIController) setupCatalogTableHeader() {
	headers := []struct {
		Text  string
		Width int
	}{
		{"ID", 10},
		{"Title", 28},
		{"Author", 18},
		{"Genres", 20},
		{"Chapters", 10},
		{"Status", 12},
	}

	for col, h := range headers {
		cell := tview.NewTableCell(fmt.Sprintf(" [yellow:b]%s[-:-:-]", h.Text)).
			SetTextColor(tcell.ColorYellow).
			SetSelectable(false).
			SetAlign(tview.AlignLeft)
		c.state.CatalogTable.SetCell(0, col, cell)
	}
}

func (c *UIController) refreshMangaCatalog(query string) {
	c.state.SetStatus(fmt.Sprintf("[yellow]Fetching catalog from gRPC (:9092) for '%s'...[white]", query))
	go func() {
		mangas, err := c.api.FetchManga(query, "")
		c.state.App.QueueUpdateDraw(func() {
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Fetch catalog failed: %v", err))
				c.state.DetailView.SetText(fmt.Sprintf(
					"\n [red:b]ERROR: Could not reach Catalog Service[-:-:-]\n\n"+
						" [white]Details: %v\n\n"+
						" [yellow]NOTE:[white] If Catalog Service (:9092) is stopped, the backend\n"+
						" returns HTTP 503 (bounded failure within 2-second deadline).",
					err,
				))
				return
			}

			c.state.MangaCatalog = mangas
			table := c.state.CatalogTable
			for i := table.GetRowCount() - 1; i > 0; i-- {
				table.RemoveRow(i)
			}

			for idx, m := range mangas {
				row := idx + 1
				genres := strings.Join(m.Genres, ", ")

				statusColor := "[green]"
				if strings.ToLower(m.Status) == "ongoing" {
					statusColor = "[yellow]"
				}

				table.SetCell(row, 0, tview.NewTableCell(" "+m.ID).SetTextColor(tcell.ColorGray))
				table.SetCell(row, 1, tview.NewTableCell(" "+m.Title).SetTextColor(tcell.ColorWhite))
				table.SetCell(row, 2, tview.NewTableCell(" "+m.Author).SetTextColor(tcell.ColorLightGray))
				table.SetCell(row, 3, tview.NewTableCell(" "+genres).SetTextColor(tcell.ColorCadetBlue))
				table.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf(" %d", m.TotalChapters)).SetTextColor(tcell.ColorYellow))
				table.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf(" %s%s[white]", statusColor, strings.ToUpper(m.Status))))
			}

			c.state.SetStatus(fmt.Sprintf("[green]Loaded %d manga from Catalog Service[white]", len(mangas)))
			if len(mangas) > 0 {
				c.renderMangaDetails(mangas[0])
			}
		})
	}()
}

func (c *UIController) renderMangaDetails(m *models.Manga) {
	genres := strings.Join(m.Genres, ", ")
	statusBadge := fmt.Sprintf("[green:b] %s [-:-:-]", strings.ToUpper(m.Status))
	if strings.ToLower(m.Status) == "ongoing" {
		statusBadge = fmt.Sprintf("[yellow:b] %s [-:-:-]", strings.ToUpper(m.Status))
	}

	detailText := fmt.Sprintf(
		"\n [yellow:b]%s[-:-:-]\n"+
			" [gray]ID:[white] %s    [gray]Status:[white] %s\n"+
			" [gray]Author:[white] [cyan]%s[white]\n"+
			" [gray]Genres:[white] [blue]%s[white]\n"+
			" [gray]Total Chapters:[white] [yellow:b]%d chapters[-:-:-]\n\n"+
			" [yellow:b]Synopsis / Summary:[-:-:-]\n %s\n\n"+
			" ────────────────────────────────────────────────────────────\n"+
			" [green]Press [Enter] or 'a' to Add to Personal Library[white]\n"+
			" [gray]Press '/' to search | 'r' to refresh catalog[white]",
		m.Title, m.ID, statusBadge, m.Author, genres, m.TotalChapters, m.Description,
	)
	c.state.DetailView.SetText(detailText)
}

func (c *UIController) showAddToLibraryDialog(m *models.Manga) {
	if c.state.Username == "" {
		c.state.SetStatus("[red]Please login first (Press 0 or F10) to add to library[white]")
		return
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Add '%s' to your reading library?", m.Title)).
		AddButtons([]string{"Reading", "Plan to Read", "Completed", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			c.state.Pages.RemovePage("dialog")
			if buttonLabel == "Cancel" {
				return
			}
			status := strings.ToLower(strings.ReplaceAll(buttonLabel, " ", "_"))
			go func() {
				err := c.api.AddToLibrary(m.ID, status)
				c.state.App.QueueUpdateDraw(func() {
					if err != nil {
						c.state.SetStatus(fmt.Sprintf("[red]Add to library failed: %v", err))
					} else {
						c.state.SetStatus(fmt.Sprintf("[green]Added '%s' to library as '%s'![white]", m.Title, status))
						c.refreshLibrary()
					}
				})
			}()
		})

	c.state.Pages.AddPage("dialog", modal, true, true)
}
