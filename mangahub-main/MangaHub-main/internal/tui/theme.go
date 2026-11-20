package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"sync"
)

var (
	themeOnce       sync.Once
	colorBackground = tcell.GetColor("#0b1220")
	colorPanel      = tcell.GetColor("#111d2e")
	colorField      = tcell.GetColor("#1b2b40")
	colorBorder     = tcell.GetColor("#33465e")
	colorText       = tcell.GetColor("#dce6f2")
	colorMuted      = tcell.GetColor("#8fa4bd")
	colorAccent     = tcell.GetColor("#5eead4")
)

func applyTheme() {
	themeOnce.Do(func() {
		tview.Styles.PrimitiveBackgroundColor = colorBackground
		tview.Styles.ContrastBackgroundColor = colorField
		tview.Styles.MoreContrastBackgroundColor = colorBorder
		tview.Styles.BorderColor = colorBorder
		tview.Styles.TitleColor = colorAccent
		tview.Styles.PrimaryTextColor = colorText
		tview.Styles.SecondaryTextColor = colorMuted
		tview.Styles.TertiaryTextColor = colorAccent
		tview.Styles.InverseTextColor = colorBackground
	})
}

func stylePanel(box *tview.Box, title string) {
	box.SetBackgroundColor(colorPanel).SetBorder(true).SetBorderColor(colorBorder).
		SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft).SetTitleColor(colorAccent)
	box.SetBorderPadding(1, 1, 1, 1)
}

func styleTable(table *tview.Table) {
	table.SetBorders(false).SetFixed(1, 0).SetSelectable(true, false).
		SetSeparator(' ').SetSelectedStyle(tcell.StyleDefault.Background(colorAccent).Foreground(colorBackground).Bold(true))
	table.SetBackgroundColor(colorPanel)
}

func styleForm(form *tview.Form) {
	form.SetBackgroundColor(colorPanel)
	form.SetLabelColor(colorMuted).SetFieldBackgroundColor(colorField).SetFieldTextColor(colorText).
		SetButtonBackgroundColor(colorAccent).SetButtonTextColor(colorBackground).
		SetButtonActivatedStyle(tcell.StyleDefault.Background(colorText).Foreground(colorBackground).Bold(true))
}

func hint(text string) *tview.TextView {
	return tview.NewTextView().SetDynamicColors(true).SetTextColor(colorMuted).SetText(" " + text)
}

// adaptiveSplit keeps the details readable in narrow terminals by stacking panes.
type adaptiveSplit struct {
	*tview.Flex
	first, second tview.Primitive
	breakpoint    int
	stacked       bool
}

func newAdaptiveSplit(first, second tview.Primitive, breakpoint int) *adaptiveSplit {
	return &adaptiveSplit{Flex: tview.NewFlex(), first: first, second: second, breakpoint: breakpoint}
}

func (s *adaptiveSplit) Draw(screen tcell.Screen) {
	_, _, width, _ := s.GetRect()
	stacked := width < s.breakpoint
	if s.GetItemCount() == 0 || s.stacked != stacked {
		s.stacked = stacked
		s.Clear()
		if stacked {
			s.SetDirection(tview.FlexRow).AddItem(s.first, 0, 3, true).AddItem(s.second, 0, 2, false)
		} else {
			s.SetDirection(tview.FlexColumn).AddItem(s.first, 0, 3, true).AddItem(s.second, 0, 2, false)
		}
	}
	s.Flex.Draw(screen)
}
