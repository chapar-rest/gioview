package explorer

import (
	"errors"
	"image/color"
	"log"
	"slices"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/oligo/gioview/menu"
	"github.com/oligo/gioview/navi"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gv "github.com/oligo/gioview/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	folderIcon, _     = widget.NewIcon(icons.FileFolder)
	folderOpenIcon, _ = widget.NewIcon(icons.FileFolderOpen)
	fileIcon, _       = widget.NewIcon(icons.ActionDescription)
)

var _ navi.NavSection = (*FileTreeNav)(nil)
var _ navi.NavItem = (*EntryNavItem)(nil)

type FileTreeNav struct {
	title string
	root  *navi.NavItemStyle
}

type EntryNavItem struct {
	state          *EntryNode
	menuOptionFunc MenuOptionFunc
	onSelectFunc   OnSelectFunc

	parent   navi.NavItem
	children []navi.NavItem
	label     *gv.Editable
	expaned   bool
	needSync  bool

}

type MenuOptionFunc func(gtx C, item *EntryNavItem) [][]menu.MenuOption
type OnSelectFunc func(gtx C, item *EntryNode) view.Intent

// Construct a FileTreeNav object that loads files and folders from rootDir. The skipFolders
// parameter allows you to specify folder name prefixes to exclude from the navigation.
func NewFileTreeNav(drawer *navi.NavDrawer, title string, navRoot *EntryNavItem) *FileTreeNav {
	return &FileTreeNav{
		title: title,
		root:  navi.NewNavItem(navRoot, drawer),
	}
}

func (tn *FileTreeNav) Attach(drawer *navi.NavDrawer) {
	// NOOP
}

func (tn *FileTreeNav) Title() string {
	return tn.title
}

func (tn *FileTreeNav) Layout(gtx C, th *theme.Theme) D {
	return tn.root.Layout(gtx, th)
}

// Construct a file tree object that loads files and folders from rootDir.
// `skipFolders` allows you to specify folder name prefixes to exclude from the navigation.
// `menuOptionFunc` is used to define the operations allowed by context menu(use right click to active it).
// `onSelectFunc` defines what action to take when a navigable item is clicked (files or folders).
func NewEntryNavItem(rootDir string, menuOptionFunc MenuOptionFunc, onSelectFunc OnSelectFunc) *EntryNavItem {
	tree, err := NewFileTree(rootDir)
	if err != nil {
		log.Fatal(err)
	}

	//tree.Print()

	if err != nil {
		log.Println("load file tree failed", err)
		return nil
	}

	return &EntryNavItem{
		parent:         nil,
		state:          tree,
		menuOptionFunc: menuOptionFunc,
		onSelectFunc:   onSelectFunc,
		expaned:        true,
	}

}

func (eitem *EntryNavItem) Icon() *widget.Icon {
	if eitem.state.Kind() == FolderNode {
		if eitem.expaned {
			return folderOpenIcon
		}
		return folderIcon
	}

	return fileIcon
}

func (eitem *EntryNavItem) OnSelect(gtx C) view.Intent {
	eitem.expaned = !eitem.expaned
	if eitem.expaned {
		eitem.needSync = true
	}

	if eitem.state.Kind() == FileNode && eitem.onSelectFunc != nil {
		return eitem.onSelectFunc(gtx, eitem.state)
	}

	return view.Intent{}

}

func (eitem *EntryNavItem) Layout(gtx layout.Context, th *theme.Theme, textColor color.NRGBA) D {

	if eitem.label == nil {
		eitem.label = gv.EditableLabel(th.TextSize, eitem.state.Name(), func(text string) {
			err := eitem.state.UpdateName(text)
			if err != nil {
				log.Println("err: ", err)
			}
		})
	}

	eitem.label.Color = textColor
	return eitem.label.Layout(gtx, th)
}

func (eitem *EntryNavItem) IsDir() bool {
	return eitem.state.IsDir()
}

func (eitem *EntryNavItem) SetMenuOptions(menuOptionFunc MenuOptionFunc) {
	eitem.menuOptionFunc = menuOptionFunc
}

func (eitem *EntryNavItem) ContextMenuOptions(gtx C) ([][]menu.MenuOption, bool) {

	if eitem.menuOptionFunc != nil {
		return eitem.menuOptionFunc(gtx, eitem), false
	}

	return nil, false
}

func (eitem *EntryNavItem) Children() []navi.NavItem {
	if eitem.state.Kind() == FileNode {
		return nil
	}

	if !eitem.expaned {
		return nil
	}

	if eitem.children == nil {
		eitem.buildChildren(true)
	}

	if eitem.needSync {
		eitem.buildChildren(true)
		eitem.needSync = false
	}

	return eitem.children
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
			parent:         eitem,
			state:          c,
			menuOptionFunc: eitem.menuOptionFunc,
			onSelectFunc:   eitem.onSelectFunc,
			expaned:        false,
			needSync:       false,
		})
	}
}

// StartEditing inits and focused on the editor to accept user input.
func (eitem *EntryNavItem) StartEditing(gtx C) {
	eitem.label.SetEditing(true)
}

// Create file or subfolder under the current folder.
// File or subfolder is inserted at the beginning of the children.
func (eitem *EntryNavItem) CreateChild(gtx C, kind NodeKind) error {
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
		// TODO: use modal to show the error if user provided one.
		log.Println(err)
		return err
	}

	//eitem.StartEditing(gtx)

	child := &EntryNavItem{
		parent:         eitem,
		state:          eitem.state.Children()[0],
		menuOptionFunc: eitem.menuOptionFunc,
		onSelectFunc:   eitem.onSelectFunc,
		expaned:        false,
		needSync:       false,
	}

	eitem.children = slices.Insert[[]navi.NavItem, navi.NavItem](eitem.children, 0, child)
	// focus the child input
	child.StartEditing(gtx)

	return nil
}

func (eitem *EntryNavItem) Remove() error {
	if eitem.parent == nil {
		return errors.New("cannot remove root dir/file")
	}

	err := eitem.state.Delete(true)
	if err != nil {
		return err
	}

	(eitem.parent).(*EntryNavItem).needSync = true
	return nil
}

// File or folder name of this node
func (eitem *EntryNavItem) Name() string {
	return eitem.state.Name()
}

// File or folder path of this node
func (eitem *EntryNavItem) Path() string {
	return eitem.state.Path
}

// EntryNode kind of this node
func (eitem *EntryNavItem) Kind() NodeKind {
	return eitem.state.Kind()
}
