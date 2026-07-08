package ui

import (
	"errors"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// backlogDialog is the non-check-in dialog that lists all backlog items and
// lets the user adopt them into today's plan or shuttle them between sections.
type backlogDialog struct {
	app          *App
	dialog       *qt.QDialog
	scrollArea   *qt.QScrollArea
	refreshTimer *qt.QTimer
}

// buildBacklogDialog constructs the Backlog manager dialog without showing it.
func (a *App) buildBacklogDialog() (*backlogDialog, error) {
	bd := &backlogDialog{app: a}

	bd.dialog = qt.NewQDialog(a.window.win.QWidget)
	bd.dialog.SetWindowTitle("Backlog")
	bd.dialog.SetMinimumWidth(460)
	mainLayout := qt.NewQVBoxLayout(bd.dialog.QWidget)

	bd.scrollArea = qt.NewQScrollArea2()
	bd.scrollArea.SetWidgetResizable(true)
	bd.scrollArea.SetMaximumHeight(360)
	// Suppress the horizontal scrollbar: a single long item must not push the
	// "Add to today's plan" / "Move" buttons of every row off the right edge.
	bd.scrollArea.SetHorizontalScrollBarPolicy(qt.ScrollBarAlwaysOff)
	mainLayout.AddWidget(bd.scrollArea.QWidget)

	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Close)
	buttons.OnRejected(bd.dialog.Reject)
	mainLayout.AddWidget(buttons.QWidget)

	// Single-shot timer for deferred row rebuilds: same reason as
	// mainWindow.scheduleRefresh — the clicked button must finish delivery
	// before we destroy and recreate the rows widget.
	bd.refreshTimer = qt.NewQTimer2(bd.dialog.QObject)
	bd.refreshTimer.SetSingleShot(true)
	bd.refreshTimer.OnTimeout(func() {
		if err := bd.populateRows(); err != nil {
			bd.app.reportError(err)
		}
	})

	if err := bd.populateRows(); err != nil {
		return nil, err
	}
	return bd, nil
}

// scheduleRefresh defers a row rebuild to the next event-loop iteration.
func (bd *backlogDialog) scheduleRefresh() {
	bd.refreshTimer.Start(0)
}

// populateRows reloads the backlog and rebuilds the scroll area content.
// Calling SetWidget replaces (and Qt-deletes) the previous container.
func (bd *backlogDialog) populateRows() error {
	backlog, err := bd.app.store.LoadBacklog()
	if err != nil {
		return err
	}

	container := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(container)
	layout.SetContentsMargins(4, 4, 4, 4)

	if len(backlog.Current) == 0 && len(backlog.NextWeek) == 0 {
		empty := qt.NewQLabel3("Nothing in the backlog.")
		empty.SetTextFormat(qt.PlainText)
		layout.AddWidget(empty.QWidget)
	} else {
		if len(backlog.Current) > 0 {
			hdr := qt.NewQLabel3("<b>This week</b>")
			layout.AddWidget(hdr.QWidget)
			for _, text := range backlog.Current {
				layout.AddWidget(bd.buildRow(text, true))
			}
		}
		if len(backlog.NextWeek) > 0 {
			hdr := qt.NewQLabel3("<b>Next week</b>")
			layout.AddWidget(hdr.QWidget)
			for _, text := range backlog.NextWeek {
				layout.AddWidget(bd.buildRow(text, false))
			}
		}
	}
	layout.AddStretch()

	bd.scrollArea.SetWidget(container)
	return nil
}

// backlogLabelWidth is the pixel budget for the item label in a backlog row:
// 460 px dialog minus 12 px row margins minus ~76 px for two 32 px icon
// buttons with 4 px spacing between them and 4 px between the label and the
// first button.  Using a fixed constant avoids reading the unshown viewport.
const backlogLabelWidth = 360

// buildRow renders one backlog item as a horizontal row with a text label
// and two icon tool buttons: "Plan Today" and "Move to next/this week".
func (bd *backlogDialog) buildRow(text string, isCurrent bool) *qt.QWidget {
	row := qt.NewQWidget2()
	layout := qt.NewQHBoxLayout(row)
	layout.SetContentsMargins(6, 2, 6, 2)

	// Elide using pixel width so CJK / emoji runes do not overflow the label
	// budget.  The full text is always available via the tooltip.
	fm := qt.QApplication_FontMetrics()
	displayText := fm.ElidedText(text, qt.ElideRight, backlogLabelWidth)

	label := qt.NewQLabel3(displayText)
	label.SetTextFormat(qt.PlainText)
	label.SetToolTip(text) // always set so the full text is reachable on hover

	// Plan Today: adopt into today's plan and remove from backlog.
	planBtn := qt.NewQToolButton2()
	planBtn.SetIcon(adoptIcon())
	planBtn.SetToolButtonStyle(qt.ToolButtonIconOnly)
	planBtn.SetToolTip("Add to today's plan")
	planBtn.SetAccessibleName("Add to today's plan")
	planBtn.OnClicked(func() {
		if err := bd.app.store.AdoptFromBacklog(time.Now(), text); err != nil {
			bd.app.reportError(err)
		} else {
			bd.app.notifyAdopt(text)
		}
		bd.app.window.scheduleRefresh() // today's plan may have gained the item
		bd.scheduleRefresh()
	})

	// Move button: shuttle between Current and NextWeek.
	moveBtn := qt.NewQToolButton2()
	moveBtn.SetToolButtonStyle(qt.ToolButtonIconOnly)
	if isCurrent {
		moveBtn.SetIcon(postponeIcon())
		moveBtn.SetToolTip("Move to next week")
		moveBtn.SetAccessibleName("Move to next week")
		moveBtn.OnClicked(func() {
			if err := bd.app.store.MoveBacklogItem(text, true); err != nil {
				bd.handleBacklogMoveErr(err)
			}
			bd.scheduleRefresh()
		})
	} else {
		moveBtn.SetIcon(thisWeekIcon())
		moveBtn.SetToolTip("Move to this week")
		moveBtn.SetAccessibleName("Move to this week")
		moveBtn.OnClicked(func() {
			if err := bd.app.store.MoveBacklogItem(text, false); err != nil {
				bd.handleBacklogMoveErr(err)
			}
			bd.scheduleRefresh()
		})
	}

	layout.AddWidget2(label.QWidget, 1)
	layout.AddWidget(planBtn.QWidget)
	layout.AddWidget(moveBtn.QWidget)
	return row
}

// adoptIcon draws a downward-pointing arrow (the mirror of the up-arrow used
// for move-to-backlog), so the icon language stays consistent: up = park for
// later, down = bring into today's plan. Drawn in mid-gray like postponeIcon.
func adoptIcon() *qt.QIcon {
	const size = 16
	pixmap := qt.NewQPixmap2(size, size)
	pixmap.FillWithFillColor(qt.NewQColor11(0, 0, 0, 0))
	painter := qt.NewQPainter2(pixmap.QPaintDevice)
	painter.SetRenderHint(qt.QPainter__Antialiasing)
	pen := qt.NewQPen3(qt.NewQColor3(140, 140, 140))
	pen.SetWidth(2)
	painter.SetPenWithPen(pen)
	// Vertical shaft of the arrow.
	painter.DrawLine2(8, 2, 8, 12)
	// Downward arrowhead: two lines meeting at the bottom tip.
	painter.DrawLine2(4, 8, 8, 13)
	painter.DrawLine2(12, 8, 8, 13)
	painter.End()
	return qt.NewQIcon2(pixmap)
}

// handleBacklogMoveErr distinguishes a not-found error (the file was edited
// while the dialog was open) from real I/O errors. Not-found shows a friendly
// one-liner; other errors go to reportError.
func (bd *backlogDialog) handleBacklogMoveErr(err error) {
	if errors.Is(err, store.ErrBacklogItemNotFound) {
		qt.QMessageBox_Information2(bd.dialog.QWidget,
			"Daily Progress Logger",
			"This item is no longer in the backlog.",
			qt.QMessageBox__Ok)
		return
	}
	bd.app.reportError(err)
}

// thisWeekIcon draws a left-pointing chevron mirroring postponeIcon, used for
// the "Move to this week" button on Next-week backlog rows.
func thisWeekIcon() *qt.QIcon {
	const size = 16
	pixmap := qt.NewQPixmap2(size, size)
	pixmap.FillWithFillColor(qt.NewQColor11(0, 0, 0, 0))
	painter := qt.NewQPainter2(pixmap.QPaintDevice)
	painter.SetRenderHint(qt.QPainter__Antialiasing)
	pen := qt.NewQPen3(qt.NewQColor3(140, 140, 140))
	pen.SetWidth(2)
	painter.SetPenWithPen(pen)
	// Left-pointing chevron: two lines meeting at the left tip.
	painter.DrawLine2(12, 4, 3, 8)
	painter.DrawLine2(12, 12, 3, 8)
	painter.End()
	return qt.NewQIcon2(pixmap)
}
