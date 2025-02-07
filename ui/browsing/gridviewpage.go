package browsing

import (
	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/ui/controller"
	"github.com/dweymouth/supersonic/ui/util"
	"github.com/dweymouth/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var _ Page = (*GridViewPage)(nil)

// Base widget for grid view pages
type GridViewPage struct {
	widget.BaseWidget

	adapter GridViewPageAdapter
	pool    *util.WidgetPool
	mp      mediaprovider.MediaProvider
	im      *backend.ImageManager

	grid            *widgets.GridView
	gridState       *widgets.GridViewState
	searchGridState *widgets.GridViewState

	title      *widget.RichText
	sortOrder  *sortOrderSelect
	filterBtn  *widgets.AlbumFilterButton
	filter     *mediaprovider.AlbumFilter
	searcher   *widgets.SearchEntry
	searchText string

	container *fyne.Container
}

// Base type for pages that show an iterable GridView
type GridViewPageAdapter interface {
	// Returns the title for the page
	Title() string

	// Returns the base album filter for this page, if any.
	// A filterable page with no base filters applied should return a zero-valued
	// *AlbumFilter, *not* nil. (Nil means unfilterable and no filter button created.)
	Filter() *mediaprovider.AlbumFilter

	// Returns the cover placeholder resource for the page
	PlaceholderResource() fyne.Resource

	// Returns the route for the page
	Route() controller.Route

	// Returns the ActionButton for this page, if any
	ActionButton() *widget.Button

	// Returns the iterator for the given sortOrder and filter.
	// (Non-album pages can ignore the filter argument)
	Iter(sortOrder string, filter mediaprovider.AlbumFilter) widgets.GridViewIterator

	// Returns the iterator for the given search query and filter.
	SearchIter(query string, filter mediaprovider.AlbumFilter) widgets.GridViewIterator

	// Function that connects the GridView callbacks to the appropriate action handlers.
	ConnectGridActions(*widgets.GridView)
}

type SortableGridViewPageAdapter interface {
	// Returns the list of sort orders and the initially selected sort order
	SortOrders() ([]string, string)

	// Saves the given sort order setting.
	SaveSortOrder(string)
}

type sortOrderSelect struct {
	widget.Select
}

func NewSortOrderSelect(options []string, onChanged func(string)) *sortOrderSelect {
	s := &sortOrderSelect{
		Select: widget.Select{
			Options:   options,
			OnChanged: onChanged,
		},
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *sortOrderSelect) MinSize() fyne.Size {
	return fyne.NewSize(170, s.Select.MinSize().Height)
}

func NewGridViewPage(
	adapter GridViewPageAdapter,
	pool *util.WidgetPool,
	mp mediaprovider.MediaProvider,
	im *backend.ImageManager,
) *GridViewPage {
	gp := &GridViewPage{
		adapter: adapter,
		pool:    pool,
		mp:      mp,
		im:      im,
		filter:  adapter.Filter(),
	}
	gp.ExtendBaseWidget(gp)
	gp.createTitleAndSort()

	iter := adapter.Iter(gp.getSortOrder(), gp.getFilter())
	if g := pool.Obtain(util.WidgetTypeGridView); g != nil {
		gp.grid = g.(*widgets.GridView)
		gp.grid.Placeholder = adapter.PlaceholderResource()
		gp.grid.Reset(iter)
	} else {
		gp.grid = widgets.NewGridView(iter, im, adapter.PlaceholderResource())
	}
	adapter.ConnectGridActions(gp.grid)
	gp.createSearchAndFilter()
	gp.createContainer()
	return gp
}

func (g *GridViewPage) createTitleAndSort() {
	g.title = widget.NewRichText(&widget.TextSegment{
		Text:  g.adapter.Title(),
		Style: widget.RichTextStyle{SizeName: theme.SizeNameHeadingText},
	})
	if s, ok := g.adapter.(SortableGridViewPageAdapter); ok {
		sorts, selected := s.SortOrders()
		g.sortOrder = NewSortOrderSelect(sorts, g.onSortOrderChanged)
		g.sortOrder.Selected = selected
	}
}

func (g *GridViewPage) createSearchAndFilter() {
	g.searcher = widgets.NewSearchEntry()
	g.searcher.Text = g.searchText
	g.searcher.OnSearched = g.OnSearched
	if g.filter != nil {
		disableGenres := len(g.filter.Genres) > 0
		genreFn := g.mp.GetGenres
		if disableGenres {
			// genre filter is disabled for this page, so no need to actually call genre list fetching function
			genreFn = func() ([]*mediaprovider.Genre, error) { return nil, nil }
		}
		g.filterBtn = widgets.NewAlbumFilterButton(g.filter, genreFn)
		g.filterBtn.GenreDisabled = disableGenres
		g.filterBtn.OnChanged = g.Reload
	}
}

func (g *GridViewPage) createContainer() {
	header := container.NewHBox(util.NewHSpace(6), g.title)
	if g.sortOrder != nil {
		sortVbox := container.NewVBox(layout.NewSpacer(), g.sortOrder, layout.NewSpacer())
		header.Add(sortVbox)
	}
	if b := g.adapter.ActionButton(); b != nil {
		btnVbox := container.NewVBox(layout.NewSpacer(), b, layout.NewSpacer())
		header.Add(btnVbox)
	}
	header.Add(layout.NewSpacer())
	if g.filterBtn != nil {
		header.Add(container.NewCenter(g.filterBtn))
	}
	searchVbox := container.NewVBox(layout.NewSpacer(), g.searcher, layout.NewSpacer())
	header.Add(searchVbox)
	header.Add(util.NewHSpace(12))
	g.container = container.NewBorder(header, nil, nil, nil, g.grid)
}

func (g *GridViewPage) Reload() {
	if g.searchText != "" {
		g.doSearch(g.searchText)
	} else {
		g.grid.Reset(g.adapter.Iter(g.getSortOrder(), g.getFilter()))
	}
}

func (g *GridViewPage) Route() controller.Route {
	return g.adapter.Route()
}

var _ Searchable = (*GridViewPage)(nil)

func (g *GridViewPage) SearchWidget() fyne.Focusable {
	return g.searcher
}

func (g *GridViewPage) OnSearched(query string) {
	if query == "" {
		if g.sortOrder != nil {
			g.sortOrder.Enable()
		}
		g.grid.ResetFromState(g.gridState)
		g.searchGridState = nil
	} else {
		if g.sortOrder != nil {
			g.sortOrder.Disable()
		}
		g.doSearch(query)
	}
	g.searchText = query
}

func (g *GridViewPage) doSearch(query string) {
	if g.searchText == "" {
		g.gridState = g.grid.SaveToState()
	}
	g.grid.Reset(g.adapter.SearchIter(query, g.getFilter()))
}

func (g *GridViewPage) onSortOrderChanged(order string) {
	g.adapter.(SortableGridViewPageAdapter).SaveSortOrder(g.getSortOrder())
	g.grid.Reset(g.adapter.Iter(g.getSortOrder(), g.getFilter()))
}

func (g *GridViewPage) getFilter() mediaprovider.AlbumFilter {
	if g.filter != nil {
		return *g.filter
	}
	return mediaprovider.AlbumFilter{}
}

func (g *GridViewPage) getSortOrder() string {
	if g.sortOrder != nil {
		return g.sortOrder.Selected
	}
	return ""
}

func (g *GridViewPage) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.container)
}

type savedGridViewPage struct {
	adapter         GridViewPageAdapter
	im              *backend.ImageManager
	mp              mediaprovider.MediaProvider
	searchText      string
	filter          *mediaprovider.AlbumFilter
	pool            *util.WidgetPool
	sortOrder       string
	gridState       *widgets.GridViewState
	searchGridState *widgets.GridViewState
}

func (g *GridViewPage) Save() SavedPage {
	sa := &savedGridViewPage{
		adapter:         g.adapter,
		pool:            g.pool,
		mp:              g.mp,
		im:              g.im,
		searchText:      g.searchText,
		filter:          g.filter,
		sortOrder:       g.getSortOrder(),
		gridState:       g.gridState,
		searchGridState: g.searchGridState,
	}
	if g.searchText == "" {
		sa.gridState = g.grid.SaveToState()
	} else {
		sa.searchGridState = g.grid.SaveToState()
	}
	g.grid.Clear()
	g.pool.Release(util.WidgetTypeGridView, g.grid)
	return sa
}

func (s *savedGridViewPage) Restore() Page {
	gp := &GridViewPage{
		adapter:         s.adapter,
		pool:            s.pool,
		mp:              s.mp,
		im:              s.im,
		gridState:       s.gridState,
		searchGridState: s.searchGridState,
		searchText:      s.searchText,
		filter:          s.filter,
	}
	gp.ExtendBaseWidget(gp)

	gp.createTitleAndSort()
	state := s.gridState
	if s.searchText != "" {
		if gp.sortOrder != nil {
			gp.sortOrder.Disable()
		}
		state = s.searchGridState
	}
	if g := gp.pool.Obtain(util.WidgetTypeGridView); g != nil {
		gp.grid = g.(*widgets.GridView)
		gp.grid.ResetFromState(state)
	} else {
		gp.grid = widgets.NewGridViewFromState(state)
	}
	gp.adapter.ConnectGridActions(gp.grid)
	gp.createSearchAndFilter()
	gp.createContainer()
	return gp
}
