package ui

import (
	"fmt"

	"github.com/20after4/configdir"
	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/res"
	"github.com/dweymouth/supersonic/ui/browsing"
	"github.com/dweymouth/supersonic/ui/controller"
	"github.com/dweymouth/supersonic/ui/os"
	"github.com/dweymouth/supersonic/ui/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var (
	ShortcutReload      = desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: os.ControlModifier}
	ShortcutSearch      = desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: os.ControlModifier}
	ShortcutCloseWindow = desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: os.ControlModifier}

	ShortcutNavOne   = desktop.CustomShortcut{KeyName: fyne.Key1, Modifier: os.ControlModifier}
	ShortcutNavTwo   = desktop.CustomShortcut{KeyName: fyne.Key2, Modifier: os.ControlModifier}
	ShortcutNavThree = desktop.CustomShortcut{KeyName: fyne.Key3, Modifier: os.ControlModifier}
	ShortcutNavFour  = desktop.CustomShortcut{KeyName: fyne.Key4, Modifier: os.ControlModifier}
	ShortcutNavFive  = desktop.CustomShortcut{KeyName: fyne.Key5, Modifier: os.ControlModifier}
	ShortcutNavSix   = desktop.CustomShortcut{KeyName: fyne.Key6, Modifier: os.ControlModifier}
	ShortcutNavSeven = desktop.CustomShortcut{KeyName: fyne.Key7, Modifier: os.ControlModifier}

	NavShortcuts = []desktop.CustomShortcut{ShortcutNavOne, ShortcutNavTwo, ShortcutNavThree,
		ShortcutNavFour, ShortcutNavFive, ShortcutNavSix, ShortcutNavSeven}
)

type MainWindow struct {
	Window fyne.Window

	App          *backend.App
	Router       browsing.Router
	Controller   *controller.Controller
	BrowsingPane *browsing.BrowsingPane
	BottomPanel  *BottomPanel

	theme          *theme.MyTheme
	haveSystemTray bool
	container      *fyne.Container
}

func NewMainWindow(fyneApp fyne.App, appName, displayAppName, appVersion string, app *backend.App, size fyne.Size) MainWindow {
	m := MainWindow{
		App:          app,
		Window:       fyneApp.NewWindow(displayAppName),
		BrowsingPane: browsing.NewBrowsingPane(app),
		theme:        theme.NewMyTheme(&app.Config.Theme, configdir.LocalConfig(appName, "themes")),
	}

	m.theme.NormalFont = app.Config.Application.FontNormalTTF
	m.theme.BoldFont = app.Config.Application.FontBoldTTF
	fyneApp.Settings().SetTheme(m.theme)

	if app.Config.Application.EnableSystemTray {
		m.SetupSystemTrayMenu(displayAppName, fyneApp)
	}
	m.Controller = &controller.Controller{
		AppVersion: appVersion,
		MainWindow: m.Window,
		App:        app,
	}
	m.Router = browsing.NewRouter(app, m.Controller, m.BrowsingPane)
	// inject controller dependencies
	m.Controller.NavHandler = m.Router.NavigateTo
	m.Controller.ReloadFunc = m.BrowsingPane.Reload
	m.Controller.CurPageFunc = m.BrowsingPane.CurrentPage

	m.BottomPanel = NewBottomPanel(app.Player, app.PlaybackManager, m.Controller)
	m.BottomPanel.ImageManager = app.ImageManager
	m.container = container.NewBorder(nil, m.BottomPanel, nil, nil, m.BrowsingPane)
	m.Window.SetContent(m.container)
	m.Window.Resize(size)
	app.PlaybackManager.OnSongChange(func(song, _ *mediaprovider.Track) {
		if song == nil {
			m.Window.SetTitle(displayAppName)
			return
		}
		m.Window.SetTitle(fmt.Sprintf("%s – %s · %s", song.Name, song.ArtistNames[0], displayAppName))
	})
	app.ServerManager.OnServerConnected(func() {
		m.BrowsingPane.EnableNavigationButtons()
		m.Router.NavigateTo(m.StartupPage())
		// check if found new version on startup
		if t := app.UpdateChecker.VersionTagFound(); t != "" && t != app.Config.Application.LastCheckedVersion {
			if t != app.VersionTag() {
				m.ShowNewVersionDialog(displayAppName, t)
			}
			m.App.Config.Application.LastCheckedVersion = t
		}
		// register callback for the ongoing periodic update check
		m.App.UpdateChecker.OnUpdatedVersionFound = func() {
			t := m.App.UpdateChecker.VersionTagFound()
			if t != app.VersionTag() {
				m.ShowNewVersionDialog(displayAppName, t)
			}
			m.App.Config.Application.LastCheckedVersion = t
		}
	})
	app.ServerManager.OnLogout(func() {
		m.BrowsingPane.DisableNavigationButtons()
		m.BrowsingPane.SetPage(nil)
		m.BrowsingPane.ClearHistory()
		m.Controller.PromptForLoginAndConnect()
	})
	m.BrowsingPane.AddSettingsMenuItem("Log Out", func() { app.ServerManager.Logout(true) })
	m.BrowsingPane.AddSettingsMenuItem("Switch Servers", func() { app.ServerManager.Logout(false) })
	m.BrowsingPane.AddSettingsMenuItem("Rescan Library", func() { app.ServerManager.Server.RescanLibrary() })
	m.BrowsingPane.AddSettingsMenuSeparator()
	m.BrowsingPane.AddSettingsMenuItem("Check for Updates", func() {
		go func() {
			if t := app.UpdateChecker.CheckLatestVersionTag(); t != "" && t != app.VersionTag() {
				m.ShowNewVersionDialog(displayAppName, t)
			} else {
				dialog.ShowInformation("No new version found",
					"You are running the latest version of "+displayAppName,
					m.Window)
			}
		}()
	})
	m.BrowsingPane.AddSettingsMenuItem("Settings...", m.showSettingsDialog)
	m.BrowsingPane.AddSettingsMenuItem("About...", m.Controller.ShowAboutDialog)
	m.addNavigationButtons()
	m.BrowsingPane.DisableNavigationButtons()
	m.addShortcuts()
	return m
}

func (m *MainWindow) StartupPage() controller.Route {
	switch m.App.Config.Application.StartupPage {
	case "Favorites":
		return controller.FavoritesRoute()
	case "Playlists":
		return controller.PlaylistsRoute()
	default:
		return controller.AlbumsRoute()
	}
}

func (m *MainWindow) SetupSystemTrayMenu(appName string, fyneApp fyne.App) {
	if desk, ok := fyneApp.(desktop.App); ok {
		menu := fyne.NewMenu(appName,
			fyne.NewMenuItem("Play/Pause", func() {
				_ = m.App.Player.PlayPause()
			}),
			fyne.NewMenuItem("Previous", func() {
				_ = m.App.Player.SeekBackOrPrevious()
			}),
			fyne.NewMenuItem("Next", func() {
				_ = m.App.Player.SeekNext()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Volume +10%", func() {
				vol := m.App.PlaybackManager.Volume()
				vol = vol + int(float64(vol)*0.1)
				// will clamp to range for us
				m.App.PlaybackManager.SetVolume(vol)
			}),
			fyne.NewMenuItem("Volume -10%", func() {
				vol := m.App.PlaybackManager.Volume()
				vol = vol - int(float64(vol)*0.1)
				m.App.PlaybackManager.SetVolume(vol)
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Show", m.Window.Show),
			fyne.NewMenuItem("Hide", m.Window.Hide),
		)
		desk.SetSystemTrayMenu(menu)
		desk.SetSystemTrayIcon(res.ResAppicon256Png)
		m.haveSystemTray = true
	}
}

func (m *MainWindow) HaveSystemTray() bool {
	return m.haveSystemTray
}

func (m *MainWindow) ShowNewVersionDialog(appName, versionTag string) {
	contentStr := fmt.Sprintf("A new version of %s (%s) is available",
		appName, versionTag)
	m.Controller.QueueShowModalFunc(func() {
		dialog.ShowCustomConfirm("A new version is available",
			"Go to release page", "Skip this version",
			widget.NewLabel(contentStr), func(show bool) {
				if show {
					fyne.CurrentApp().OpenURL(m.App.UpdateChecker.LatestReleaseURL())
				}
				m.App.Config.Application.LastCheckedVersion = versionTag
			}, m.Window)
	})
}

func (m *MainWindow) addNavigationButtons() {
	m.BrowsingPane.AddNavigationButton(theme.NowPlayingIcon, func() {
		m.Router.NavigateTo(controller.NowPlayingRoute(""))
	})
	m.BrowsingPane.AddNavigationButton(theme.FavoriteIcon, func() {
		m.Router.NavigateTo(controller.FavoritesRoute())
	})
	m.BrowsingPane.AddNavigationButton(theme.AlbumIcon, func() {
		m.Router.NavigateTo(controller.AlbumsRoute())
	})
	m.BrowsingPane.AddNavigationButton(theme.ArtistIcon, func() {
		m.Router.NavigateTo(controller.ArtistsRoute())
	})
	m.BrowsingPane.AddNavigationButton(theme.GenreIcon, func() {
		m.Router.NavigateTo(controller.GenresRoute())
	})
	m.BrowsingPane.AddNavigationButton(theme.PlaylistIcon, func() {
		m.Router.NavigateTo(controller.PlaylistsRoute())
	})
	m.BrowsingPane.AddNavigationButton(theme.TracksIcon, func() {
		m.Router.NavigateTo(controller.TracksRoute())
	})
}

func (m *MainWindow) addShortcuts() {
	for _, sh := range os.BackShortcuts {
		m.Canvas().AddShortcut(&sh, func(_ fyne.Shortcut) {
			m.BrowsingPane.GoBack()
			// TODO: reset focus only if something inside the page had focus
			m.Canvas().Focus(nil)
		})
	}
	for _, sh := range os.ForwardShortcuts {
		m.Canvas().AddShortcut(&sh, func(_ fyne.Shortcut) {
			m.BrowsingPane.GoForward()
			m.Canvas().Focus(nil)
		})
	}
	if os.SettingsShortcut != nil {
		m.Canvas().AddShortcut(os.SettingsShortcut, func(_ fyne.Shortcut) {
			m.showSettingsDialog()
		})
	}

	m.Canvas().AddShortcut(&ShortcutReload, func(_ fyne.Shortcut) {
		m.BrowsingPane.Reload()
	})
	m.Canvas().AddShortcut(&ShortcutSearch, func(_ fyne.Shortcut) {
		if s := m.BrowsingPane.GetSearchBarIfAny(); s != nil {
			m.Window.Canvas().Focus(s)
		}
	})
	m.Canvas().AddShortcut(&fyne.ShortcutSelectAll{}, func(_ fyne.Shortcut) {
		m.BrowsingPane.SelectAll()
	})
	m.Canvas().AddShortcut(&ShortcutCloseWindow, func(_ fyne.Shortcut) {
		if m.App.Config.Application.CloseToSystemTray && m.HaveSystemTray() {
			m.Window.Hide()
		}
	})

	for i, ns := range NavShortcuts {
		m.Canvas().AddShortcut(&ns, func(i int) func(fyne.Shortcut) {
			return func(fyne.Shortcut) {
				m.BrowsingPane.ActivateNavigationButton(i)
			}
		}(i))
	}

	m.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyEscape:
			m.Controller.CloseEscapablePopUp()
		case fyne.KeySpace:
			m.App.Player.PlayPause()
		}
	})
}

func (m *MainWindow) showSettingsDialog() {
	m.Controller.ShowSettingsDialog(func() {
		fyne.CurrentApp().Settings().SetTheme(m.theme)
	}, m.theme.ListThemeFiles())
}

func (m *MainWindow) Show() {
	m.Window.Show()
}

func (m *MainWindow) Canvas() fyne.Canvas {
	return m.Window.Canvas()
}

func (m *MainWindow) SetTitle(title string) {
	m.Window.SetTitle(title)
}

func (m *MainWindow) SetContent(c fyne.CanvasObject) {
	m.Window.SetContent(c)
}
