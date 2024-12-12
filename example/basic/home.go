package main

import (
	"github.com/oligo/gioview/explorer"
	"github.com/oligo/gioview/navi"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type HomeView struct {
	view.ViewManager
	sidebar      *NavDrawer
	tabbar       *navi.Tabbar
	currentModal *view.ModalView
}

func (hv *HomeView) ID() string {
	return "Home"
}
func (hv *HomeView) update(gtx C) {
	// handle events and states update
}

func (hv *HomeView) Layout(gtx C, th *theme.Theme) layout.Dimensions {
	hv.update(gtx)
	return hv.LayoutMain(gtx, th)
}

func (hv *HomeView) LayoutMain(gtx C, th *theme.Theme) layout.Dimensions {
	dims := layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Start,
	}.Layout(gtx,
		// navdrawer
		layout.Rigid(func(gtx C) D {
			return NaviDrawerStyle{
				NavDrawer: hv.sidebar,
				Inset: layout.Inset{
					Top:    unit.Dp(20),
					Bottom: unit.Dp(20),
					Left:   unit.Dp(20),
				},
				Bg:    th.Bg2,
				Width: unit.Dp(max(gtx.Constraints.Max.X/(6*int(gtx.Metric.PxPerDp)), 220)),
			}.Layout(gtx, th)

		}),
		// switchable view
		layout.Flexed(1, func(gtx C) D {
			// draw the background
			gtx.Constraints.Min = gtx.Constraints.Max
			rect := clip.Rect{Max: gtx.Constraints.Max}
			paint.FillShape(gtx.Ops, th.Bg, rect.Op())

			return layout.Flex{
				Axis:      layout.Vertical,
				Alignment: layout.Middle,
			}.Layout(gtx,
				// horizontal navbar
				layout.Rigid(func(gtx C) D {
					return hv.tabbar.Layout(gtx, th)
				}),
				layout.Rigid(func(gtx C) D {
					return layout.Spacer{Height: unit.Dp(1)}.Layout(gtx)
				}),

				layout.Flexed(1, func(gtx C) D {
					if hv.CurrentView() == nil {
						return view.EmptyView{}.Layout(gtx, th)
					}
					return hv.CurrentView().Layout(gtx, th)
				}),
			)
		}),
	)

	if hv.currentModal == nil {
		if modal := hv.NextModalView(); modal != nil {
			hv.currentModal = modal
			hv.currentModal.Background = th.Bg
			hv.currentModal.Radius = unit.Dp(8)
			hv.currentModal.MaxWidth = unit.Dp(760)
			hv.currentModal.MaxHeight = 0.6
			hv.currentModal.ShowUp(gtx)
		}
	} else {
		// closing modal view
		if hv.currentModal.IsClosed(gtx) {
			hv.FinishModalView()
			hv.currentModal = nil
		} else {
			hv.currentModal.Layout(gtx, th)
		}
	}

	return dims
}

func newHome(window *app.Window) *HomeView {
	vm := view.DefaultViewManager(window)

	fileChooser, _ = explorer.NewFileChooser(vm)

	sidebar := NewNavDrawer(vm)
	sidebar.AddSection(SimpleItemSection(viewIcon, "Tabviews & Image", func(item *navi.NavTree) {
		sidebar.OnItemSelected(item)
		intent := view.Intent{Target: ExampleViewID, ShowAsModal: false}
		_ = vm.RequestSwitch(intent)
	}))

	sidebar.AddSection(SimpleItemSection(viewIcon, "Editor Example", func(item *navi.NavTree) {
		sidebar.OnItemSelected(item)
		intent := view.Intent{Target: EditorExampleViewID, ShowAsModal: false}
		_ = vm.RequestSwitch(intent)
	}))

	sidebar.AddSection(SimpleItemSection(viewIcon, "File Explorer", func(item *navi.NavTree) {
		sidebar.OnItemSelected(item)
		intent := view.Intent{Target: ExplorerViewID, ShowAsModal: false}
		_ = vm.RequestSwitch(intent)
	}))

	fileTree, _ := explorer.NewEntryNavItem("../../")
	sidebar.AddSection(NewFileTreeNav("File Explorer", fileTree, func(item *navi.NavTree) {
		sidebar.OnItemSelected(item)
		//intent := view.Intent{Target: EditorExampleViewID, ShowAsModal: false}
		//_ = vm.RequestSwitch(intent)
	}))

	vm.Register(ExampleViewID, func() view.View { return NewExampleView(vm) })
	vm.Register(EditorExampleViewID, NewEditorExample)
	vm.Register(ExplorerViewID, NewFileExplorerView)

	return &HomeView{
		ViewManager: vm,
		tabbar:      navi.NewTabbar(vm, &navi.TabbarOptions{MaxVisibleActions: 2}),
		sidebar:     sidebar,
	}
}

var (
	viewIcon, _ = widget.NewIcon(icons.ActionViewModule)
)

var (
	fileChooser *explorer.FileChooser
)
