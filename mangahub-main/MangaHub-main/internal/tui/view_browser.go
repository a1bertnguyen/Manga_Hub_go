package tui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"mangahub/pkg/models"
	"strings"
)

func (c *UIController) buildBrowserView() tview.Primitive {
	search := tview.NewInputField().SetLabel(" Search  ").SetPlaceholder("Title or author...").SetFieldWidth(0)
	search.SetFieldBackgroundColor(colorField).SetFieldTextColor(colorText).SetLabelColor(colorAccent)
	search.SetPlaceholderTextColor(colorMuted)
	genre := tview.NewDropDown().SetLabel(" Genre   ").SetFieldWidth(20)
	genre.SetOptions([]string{"All genres", "Action", "Adventure", "Comedy", "Drama", "Fantasy", "Romance", "Sci-Fi", "Shounen", "Seinen", "Shoujo", "Josei"}, nil)
	genre.SetCurrentOption(0).SetSelectedFunc(func(value string, _ int) {
		c.genre = value
		if value == "All genres" {
			c.genre = ""
		}
		c.refreshMangaCatalog(search.GetText())
	})
	genre.SetFieldBackgroundColor(colorField).SetFieldTextColor(colorText).SetLabelColor(colorAccent)
	table := tview.NewTable()
	styleTable(table)
	c.state.CatalogTable = table
	c.setupCatalogTableHeader()
	details := tview.NewTextView().SetDynamicColors(true).SetWordWrap(true).SetScrollable(true)
	stylePanel(details.Box, "MANGA DETAILS")
	details.SetText("[#5eead4::b]Find your next story.[-:-:-]\n\nSelect a title to explore its synopsis, author and chapters.\n\n[#8fa4bd]Sign in through Account to browse the catalog and build your library.[-]")
	c.state.DetailView = details
	search.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			c.refreshMangaCatalog(search.GetText())
			c.state.App.SetFocus(table)
		}
		if key == tcell.KeyEscape {
			c.state.App.SetFocus(table)
		}
	})
	table.SetSelectionChangedFunc(func(row, _ int) {
		if row > 0 && row <= len(c.state.MangaCatalog) {
			m := c.state.MangaCatalog[row-1]
			c.loadMangaDetailAsync(m.ID, m)
		}
	})
	table.SetSelectedFunc(func(row, _ int) {
		if row > 0 && row <= len(c.state.MangaCatalog) {
			c.showAddToLibraryDialog(c.state.MangaCatalog[row-1])
		}
	})
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case '/':
			c.state.App.SetFocus(search)
			return nil
		case 'g', 'G':
			c.state.App.SetFocus(genre)
			return nil
		case 'r', 'R':
			c.refreshMangaCatalog(search.GetText())
			return nil
		case 'a', 'A':
			row, _ := table.GetSelection()
			if row > 0 && row <= len(c.state.MangaCatalog) {
				c.showAddToLibraryDialog(c.state.MangaCatalog[row-1])
			}
			return nil
		}
		return event
	})
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(search, 1, 0, false).AddItem(genre, 2, 0, false).AddItem(table, 0, 1, true)
	stylePanel(left.Box, "DISCOVER / CATALOG")
	c.focus["browser"] = []tview.Primitive{table, details, search, genre}
	return newAdaptiveSplit(left, details, 105)
}

func (c *UIController) setupCatalogTableHeader() {
	for col, title := range []string{"TITLE", "CH.", "STATUS"} {
		cell := tview.NewTableCell(" " + title).SetSelectable(false).SetTextColor(colorMuted).SetAttributes(tcell.AttrBold)
		c.state.CatalogTable.SetCell(0, col, cell)
	}
}

func (c *UIController) refreshMangaCatalog(query string) {
	c.search = query
	c.catalogRequest++
	request := c.catalogRequest
	genre := c.genre
	if c.state.username() == "" {
		c.state.DetailView.SetText("[#5eead4::b]Your next read starts here.[-:-:-]\n\nSign in or create an account to explore the catalog.\n\n[#8fa4bd]Press F10 to open Account.[-]")
		c.state.SetStatus("Sign in to browse the catalog. Press F10 for Account.")
		return
	}
	c.state.SetStatus("Loading catalog...")
	go func() {
		mangas, err := c.api.FetchManga(query, genre)
		c.state.App.QueueUpdateDraw(func() {
			if request != c.catalogRequest {
				return
			}
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Catalog unavailable: %s[-]", tview.Escape(err.Error())))
				c.state.DetailView.SetText(fmt.Sprintf("[red::b]Could not load the catalog[-:-:-]\n\n%s\n\n[#8fa4bd]Check the backend and catalog processes, then press r to retry. If your session expired, sign in again with F10.[-]", tview.Escape(err.Error())))
				return
			}
			c.detailRequest++ // Invalidate detail responses from the previous result set.
			c.state.MangaCatalog = mangas
			table := c.state.CatalogTable
			table.Clear()
			c.setupCatalogTableHeader()
			for i, m := range mangas {
				table.SetCell(i+1, 0, tview.NewTableCell(" "+tview.Escape(m.Title)).SetTextColor(colorText).SetExpansion(1).SetMaxWidth(40))
				table.SetCell(i+1, 1, tview.NewTableCell(fmt.Sprint(m.TotalChapters)).SetTextColor(colorAccent).SetAlign(tview.AlignRight))
				table.SetCell(i+1, 2, tview.NewTableCell(" "+tview.Escape(strings.ToUpper(m.Status))).SetTextColor(colorMuted).SetMaxWidth(10))
			}
			if len(mangas) == 0 {
				table.SetCell(1, 0, tview.NewTableCell(" No matching titles").SetSelectable(false).SetTextColor(colorMuted))
				c.state.DetailView.SetText("[#5eead4::b]No stories found.[-:-:-]\n\nTry another title, author or genre.\n\n[#8fa4bd]Use / to search or g to change genre.[-]")
			} else {
				table.Select(1, 0).ScrollToBeginning()
				c.loadMangaDetailAsync(mangas[0].ID, mangas[0])
			}
			c.state.SetStatus(fmt.Sprintf("%d titles found. Select a story to see more.", len(mangas)))
		})
	}()
}

func (c *UIController) loadMangaDetailAsync(mangaID string, fallback *models.Manga) {
	c.detailRequest++
	request := c.detailRequest
	if fallback != nil {
		c.renderMangaDetails(fallback)
	}
	go func() {
		detail, err := c.api.GetMangaDetail(mangaID)
		c.state.App.QueueUpdateDraw(func() {
			if request != c.detailRequest {
				return
			}
			if err != nil {
				c.state.SetStatus(fmt.Sprintf("[red]Detail refresh failed: %s[-]", tview.Escape(err.Error())))
				return
			}
			c.renderMangaDetails(detail)
		})
	}()
}

func (c *UIController) renderMangaDetails(m *models.Manga) {
	c.state.DetailView.SetText(fmt.Sprintf(
		"[#5eead4::b]%s[-:-:-]\n[#8fa4bd]by %s[-]\n\n[#fbbf24]%s[-]  /  %d chapters\n\n[#8fa4bd]GENRES[-]\n%s\n\n[#8fa4bd]SYNOPSIS[-]\n%s\n\n[#5eead4]Enter / a  Add to library[-]",
		tview.Escape(m.Title), tview.Escape(m.Author), tview.Escape(strings.ToUpper(m.Status)), m.TotalChapters, tview.Escape(strings.Join(m.Genres, " / ")), tview.Escape(m.Description)))
	c.state.DetailView.ScrollToBeginning()
}

func (c *UIController) showAddToLibraryDialog(m *models.Manga) {
	if c.state.username() == "" {
		c.state.SetStatus("[red]Please login first (Press 0 or F10) to add to library[white]")
		return
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Add '%s' to your reading library?", tview.Escape(m.Title))).
		AddButtons([]string{"Reading", "Plan to Read", "Completed", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			c.state.Pages.RemovePage("dialog")
			c.state.App.SetFocus(c.state.CatalogTable)
			if buttonIndex < 0 || buttonLabel == "Cancel" {
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

	modal.SetBackgroundColor(colorPanel).SetButtonBackgroundColor(colorAccent).SetButtonTextColor(colorBackground)
	c.state.Pages.AddPage("dialog", modal, true, true)
	c.state.App.SetFocus(modal)
}
