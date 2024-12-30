package explorer

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"io"
	"log"
	"slices"
	"strings"
	"unsafe"

	"gioui.org/gesture"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/menu"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/navi"
	"github.com/oligo/gioview/theme"
	gv "github.com/oligo/gioview/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

const (
	// For DnD use.
	EntryMIME = "gioview/file-entry"
	// For read from clipboard use.
	mimeText = "application/text"
)

var (
	// Icons used in file tree.
	FolderIcon, _     = widget.NewIcon(icons.FileFolder)
	FolderOpenIcon, _ = widget.NewIcon(icons.FileFolderOpen)
	FileIcon, _       = widget.NewIcon(icons.ActionDescription)
	// File tree icon size
	IconSize = unit.Dp(14)
)

var _ navi.NavItem = (*EntryNavItem)(nil)

// EntryNavItem is a navigable file node. When used with [navi.NavTree],
// it renders a file tree, and an optional context menu.
// Supported features:
//  1. in-place edit/rename, press ESC to escape editing.
//  2. context menu.
//  3. Shortcuts: ctlr/cmd+c, ctrl/cmd+v, ctrl/cmd+p.
//  4. copy from external file/folder.
//  5. Delete files/folders by moving them to trash bin.
//  5. Drag & Drop support. External components can also subscribe the transfer events by
//     using transfer.TargetFilter with the EntryMIME type.
//  6. Restore states from states data.
type EntryNavItem struct {
	state    *EntryNode
	parent   *EntryNavItem
	children []navi.NavItem
	label    *gv.Editable
	expanded bool
	needSync bool
	// A cut/paste mark.
	isCut     bool
	click     gesture.Click
	draggable widget.Draggable
	// entered and dnsInited are for Drag and Drop op.
	entered   bool
	dndInited bool
	reader    *strings.Reader
	// Used to set context menu options.
	MenuOptionFunc MenuOptionFunc
	// Used to set what to be done when a item is clicked.
	OnSelectFunc OnSelectFunc
	// Used to decide whether the DnD drop can continue.
	OnDropConfirmFunc OnDropConfirmFunc
}

type TreeState struct {
	Path     string
	Expanded bool
	Children []*TreeState
}

type MenuOptionFunc func(gtx C, item *EntryNavItem) [][]menu.MenuOption
type OnSelectFunc func(item *EntryNode)
type OnDropConfirmFunc func(srcPath string, dest *EntryNode, onConfirmed func())

// Construct a file tree object that loads files and folders from rootDir.
func NewEntryNavItem(rootDir string) (*EntryNavItem, error) {
	tree, err := NewFileTree(rootDir)
	if err != nil {
		return nil, err
	}

	return &EntryNavItem{
		parent:   nil,
		state:    tree,
		expanded: true,
	}, nil

}

func (eitem *EntryNavItem) icon() *widget.Icon {
	if eitem.state.Kind() == FolderNode {
		if eitem.expanded {
			return FolderOpenIcon
		}
		return FolderIcon
	}

	return FileIcon
}

func (eitem *EntryNavItem) OnSelect() {
	eitem.expanded = !eitem.expanded
	if eitem.expanded {
		eitem.needSync = true
	}

	if eitem.state.Kind() == FileNode && eitem.OnSelectFunc != nil {
		eitem.OnSelectFunc(eitem.state)
	}
}

func (eitem *EntryNavItem) Layout(gtx layout.Context, th *theme.Theme, textColor color.NRGBA) D {
	eitem.Update(gtx)

	macro := op.Record(gtx.Ops)
	dims := eitem.layout(gtx, th, textColor)
	call := macro.Stop()

	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	if eitem.isCut {
		defer paint.PushOpacity(gtx.Ops, 0.6).Pop()
	}
	// draw a highlighted background for the potential drop target.
	if eitem.droppable() {
		paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, 0xb6)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}
	event.Op(gtx.Ops, eitem)
	eitem.click.Add(gtx.Ops)
	call.Add(gtx.Ops)

	return dims
}

func (eitem *EntryNavItem) layout(gtx layout.Context, th *theme.Theme, textColor color.NRGBA) D {
	if eitem.label == nil {
		eitem.label = gv.EditableLabel(eitem.state.Name(), func(text string) {
			err := eitem.state.UpdateName(text)
			if err != nil {
				log.Println("err: ", err)
			} else {
				eitem.needSync = true
			}
		})
	}

	eitem.label.Color = textColor
	eitem.label.TextSize = th.TextSize

	return eitem.draggable.Layout(gtx,
		func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if eitem.icon() == nil {
						return layout.Dimensions{}
					}
					return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
						iconColor := th.ContrastBg
						return misc.Icon{Icon: eitem.icon(), Color: iconColor, Size: IconSize}.Layout(gtx, th)
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return eitem.label.Layout(gtx, th)
				}),
			)
		},
		func(gtx C) D {
			return eitem.layoutDraggingBox(gtx, th)
		},
	)

}

func (eitem *EntryNavItem) droppable() bool {
	return eitem.entered && eitem.dndInited && !eitem.draggable.Dragging()
}

func (eitem *EntryNavItem) layoutDraggingBox(gtx C, th *theme.Theme) D {
	if !eitem.draggable.Dragging() {
		return D{}
	}

	offset := eitem.draggable.Pos()
	if offset.Round().X == 0 && offset.Round().Y == 0 {
		return D{}
	}

	macro := op.Record(gtx.Ops)
	dims := func(gtx C) D {
		return widget.Border{
			Color:        th.ContrastBg,
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				lb := material.Label(th.Theme, th.TextSize, eitem.Name())
				lb.Color = th.ContrastFg
				return lb.Layout(gtx)
			})
		})
	}(gtx)
	call := macro.Stop()

	defer clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(8))).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: misc.WithAlpha(th.ContrastBg, 0xb6)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	defer paint.PushOpacity(gtx.Ops, 0.8).Pop()
	call.Add(gtx.Ops)

	return dims
}

func (eitem *EntryNavItem) IsDir() bool {
	return eitem.state.IsDir()
}

func (eitem *EntryNavItem) SetMenuOptions(menuOptionFunc MenuOptionFunc) {
	eitem.MenuOptionFunc = menuOptionFunc
}

func (eitem *EntryNavItem) ContextMenuOptions(gtx C) ([][]menu.MenuOption, bool) {

	if eitem.MenuOptionFunc != nil {
		return eitem.MenuOptionFunc(gtx, eitem), false
	}

	return nil, false
}

func (eitem *EntryNavItem) Children() ([]navi.NavItem, bool) {
	if eitem.state.Kind() == FileNode {
		return nil, false
	}

	if !eitem.expanded {
		return nil, false
	}

	changed := false
	if eitem.children == nil || eitem.needSync {
		eitem.buildChildren(true)
		eitem.needSync = false
		changed = true
	}

	return eitem.children, changed
}

func (eitem *EntryNavItem) buildChildren(sync bool) {
	eitem.children = eitem.children[:0]
	if sync {
		err := eitem.state.Refresh(hiddenFileFilter)
		if err != nil {
			log.Println(err)
		}
	}
	for _, c := range eitem.state.Children() {
		eitem.children = append(eitem.children, &EntryNavItem{
			parent:            eitem,
			state:             c,
			MenuOptionFunc:    eitem.MenuOptionFunc,
			OnSelectFunc:      eitem.OnSelectFunc,
			OnDropConfirmFunc: eitem.OnDropConfirmFunc,
			expanded:          false,
			needSync:          false,
		})
	}
}

func (eitem *EntryNavItem) Refresh() {
	eitem.expanded = true
	eitem.needSync = true
}

// StartEditing inits and focused on the editor to accept user input.
func (eitem *EntryNavItem) StartEditing(gtx C) {
	eitem.label.SetEditing(true)
}

// Create file or subfolder under the current folder.
// File or subfolder is inserted at the beginning of the children.
func (eitem *EntryNavItem) CreateChild(gtx C, kind NodeKind, postAction func(node *EntryNode)) error {
	if eitem.state.Kind() == FileNode {
		return nil
	}

	var err error
	if kind == FileNode {
		err = eitem.state.AddChild("new file", FileNode)
	} else {
		err = eitem.state.AddChild("new folder", FolderNode)
	}

	if err != nil {
		return err
	}

	childNode := eitem.state.Children()[0]

	child := &EntryNavItem{
		parent:            eitem,
		state:             childNode,
		MenuOptionFunc:    eitem.MenuOptionFunc,
		OnSelectFunc:      eitem.OnSelectFunc,
		OnDropConfirmFunc: eitem.OnDropConfirmFunc,
		expanded:          false,
		needSync:          false,
	}

	child.label = gv.EditableLabel(childNode.Name(), func(text string) {
		err := childNode.UpdateName(text)
		if err != nil {
			log.Println("update name err: ", err)
		}
		if postAction != nil {
			postAction(childNode)
		}
	})

	eitem.children = slices.Insert[[]navi.NavItem, navi.NavItem](eitem.children, 0, child)
	// focus the child input
	child.StartEditing(gtx)

	return nil
}

func (eitem *EntryNavItem) Remove() error {
	if eitem.parent == nil {
		return errors.New("cannot remove root dir/file")
	}

	err := eitem.state.Delete()
	if err != nil {
		return err
	}

	eitem.parent.needSync = true
	return nil
}

// File or folder name of this node
func (eitem *EntryNavItem) Name() string {
	return eitem.state.Name()
}

func (eitem *EntryNavItem) Parent() *EntryNavItem {
	return eitem.parent
}

// File or folder path of this node
func (eitem *EntryNavItem) Path() string {
	return eitem.state.Path
}

// EntryNode kind of this node
func (eitem *EntryNavItem) Kind() NodeKind {
	return eitem.state.Kind()
}

func (eitem *EntryNavItem) Expanded() bool {
	return eitem.expanded
}

func (eitem *EntryNavItem) SetExpanded(expanded bool) {
	eitem.expanded = expanded
}

// Restore restores the tree states by applying state to the current node and its children.
func (eitem *EntryNavItem) Restore(state *TreeState) {
	if state.Path != eitem.state.Path {
		return
	}

	eitem.expanded = state.Expanded
	if len(state.Children) <= 0 {
		return
	}

	stateMap := make(map[string]*TreeState, len(state.Children))
	for _, st := range state.Children {
		stateMap[st.Path] = st
	}

	eitem.buildChildren(true)
	for _, child := range eitem.children {
		child := child.(*EntryNavItem)
		if !child.state.IsDir() {
			continue
		}

		if st, exists := stateMap[child.Path()]; exists {
			child.Restore(st)
		}
	}
}

// Snapshot saves states of the expanded [EntryNavItem] node, and the states of its children.
func (eitem *EntryNavItem) Snapshot() *TreeState {
	if !eitem.state.IsDir() || !eitem.expanded {
		return nil
	}

	state := &TreeState{Path: eitem.Path(), Expanded: eitem.expanded}

	for _, child := range eitem.children {
		child := child.(*EntryNavItem)
		if !child.state.IsDir() {
			continue
		}

		if childState := child.Snapshot(); childState != nil {
			state.Children = append(state.Children, childState)
		}
	}

	return state
}

// Move file to the current dir or the dir of the current file. Set removeOld to false to
// simulate a copy OP.
func (eitem *EntryNavItem) OnPaste(data string, removeOld bool, src *EntryNavItem) error {
	// when paste destination is a normal file node, use its parent dir to ease the CUT/COPY operations.
	dest := eitem
	if !eitem.IsDir() && eitem.parent != nil {
		dest = eitem.parent
	}

	pathes := strings.Split(string(data), "\n")
	if removeOld {
		for _, p := range pathes {
			err := dest.state.Move(p)
			if err != nil {
				return err
			}

			if src != nil && src.parent != nil {
				src.isCut = false
				parent := src.parent
				parent.children = slices.DeleteFunc(parent.children, func(chd navi.NavItem) bool {
					entry := chd.(*EntryNavItem)
					return entry.Path() == p
				})
			}
		}
	} else {
		for _, p := range pathes {
			err := dest.state.Copy(p)
			if err != nil {
				return err
			}
		}
	}

	dest.needSync = true
	dest.expanded = true
	return nil
}

func (eitem *EntryNavItem) OnCopyOrCut(gtx C, isCut bool) {
	gtx.Execute(clipboard.WriteCmd{Type: mimeText, Data: io.NopCloser(EncodeClipboardData(eitem, eitem.Path(), isCut))})
	eitem.isCut = isCut
}

func (eitem *EntryNavItem) Update(gtx C) error {
	// re-create the reader to make sure each DnD/Copy/Cut operation can use it to read data.
	if eitem.reader == nil || eitem.reader.Len() <= 0 {
		eitem.reader = strings.NewReader(eitem.state.Path)
	}

	// focus conflicts with editable. so subscribe editable's key events here.
	filters := []event.Filter{
		key.Filter{Focus: eitem.label, Name: "C", Required: key.ModShortcut},
		key.Filter{Focus: eitem.label, Name: "V", Required: key.ModShortcut},
		key.Filter{Focus: eitem.label, Name: "X", Required: key.ModShortcut},
		transfer.TargetFilter{Target: eitem, Type: mimeText}, //for copy, cut and paste
	}
	if eitem.state.IsDir() {
		filters = append(filters,
			// For DnD. This ensures only dir can be dragged and dropped to.
			transfer.TargetFilter{Target: eitem, Type: EntryMIME},
			// Detect if pointer is inside of the dir item, so we can highlight it when dropping items to it.
			pointer.Filter{Target: eitem, Kinds: pointer.Enter | pointer.Leave},
		)
	}

	for {
		ke, ok := gtx.Event(filters...)
		if !ok {
			break
		}

		switch event := ke.(type) {
		case key.Event:
			if !event.Modifiers.Contain(key.ModShortcut) {
				break
			}

			switch event.Name {
			// Initiate a paste operation, by requesting the clipboard contents; other
			// half is in DataEvent.
			case "V":
				gtx.Execute(clipboard.ReadCmd{Tag: eitem})

			// Copy or Cut selection -- ignored if nothing selected.
			case "C", "X":
				eitem.OnCopyOrCut(gtx, event.Name == "X")
			}

		case pointer.Event:
			if event.Kind == pointer.Enter {
				eitem.entered = true
			} else if event.Kind == pointer.Leave {
				eitem.entered = false
			}

		case transfer.InitiateEvent:
			eitem.dndInited = true
		case transfer.CancelEvent:
			eitem.dndInited = false
			eitem.entered = false
		case transfer.DataEvent:
			// read the clipboard content:
			reader := event.Open()
			defer reader.Close()
			content, err := io.ReadAll(reader)
			if err != nil {
				return err
			}

			defer gtx.Execute(op.InvalidateCmd{})

			switch event.Type {
			case mimeText:
				//FIXME: clipboard data might be invalid file path.
				p, err := ParseClipboardData(content)
				if err == nil {
					if err := eitem.OnPaste(p.Data, p.IsCut, p.GetSrc()); err != nil {
						return err
					}
				} else {
					if err := eitem.OnPaste(string(content), false, nil); err != nil {
						return err
					}
				}
			case EntryMIME:
				// Origin of transfer.OfferCmd is kept by gio
				source, isFromEntryItem := reader.(*EntryNavItem)
				if !isFromEntryItem {
					break
				}
				if source == eitem || source.parent == eitem {
					break
				}

				if eitem.OnDropConfirmFunc != nil {
					eitem.OnDropConfirmFunc(string(content), eitem.state, func() {
						eitem.OnPaste(string(content), true, source)
					})
				} else {
					return eitem.OnPaste(string(content), true, source)
				}
			}

		}
	}

	// Process click of the eitem.
	// Use guest.Click to detect press&release of pointer. This prevents conflicts with dragging events.
	for {
		e, ok := eitem.click.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick {
			eitem.OnSelect()
		}
	}

	//Process transfer.RequestEvent for draggable.
	if eitem.draggable.Type == "" {
		eitem.draggable.Type = EntryMIME
	}
	if m, ok := eitem.draggable.Update(gtx); ok {
		eitem.draggable.Offer(gtx, m, eitem)
	}

	return nil
}

// Implelments io.ReadCloser for widget.Draggable.
func (eitem *EntryNavItem) Read(p []byte) (n int, err error) {

	return eitem.reader.Read(p)
}

func (eitem *EntryNavItem) Close() error {
	return nil
}

// ClipboardData is exported to enable in-app copy&paste.
type ClipboardData struct {
	IsCut bool    `json:"isCut"`
	Data  string  `json:"data"`
	Src   uintptr `json:"src"`
}

func (p *ClipboardData) GetSrc() *EntryNavItem {
	return (*EntryNavItem)(unsafe.Pointer(p.Src))
}

func EncodeClipboardData(src *EntryNavItem, data string, isCut bool) io.Reader {
	p := ClipboardData{Data: data, IsCut: isCut, Src: uintptr(unsafe.Pointer(src))}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(p)
	if err != nil {
		panic(err)
	}

	return strings.NewReader(buf.String())
}

func ParseClipboardData(buf []byte) (*ClipboardData, error) {
	p := ClipboardData{}
	err := json.Unmarshal(buf, &p)
	if err != nil {
		return nil, err
	}

	return &p, nil
}
