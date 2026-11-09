package main

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	app := tview.NewApplication()

	banner := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("\n[yellow]============================================[white]\n" +
			"[green]       MangaHub Terminal User Interface       [white]\n" +
			"[yellow]============================================[white]\n\n" +
			"[cyan]Network-Centric Programming (IT096IIU)[white]\n" +
			"Authors: Student Team\n\n" +
			"[white]Status: TUI Scaffolding initialized.\n" +
			"[gray]Press [yellow]'q'[gray] or [yellow]'Ctrl+C'[gray] to exit.\n")

	banner.SetBorder(true).SetTitle(" MangaHub TUI Client ")

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			app.Stop()
			return nil
		}
		return event
	})

	if err := app.SetRoot(banner, true).EnableMouse(true).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
