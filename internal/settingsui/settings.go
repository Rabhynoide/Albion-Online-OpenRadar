package settingsui

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/segmentio/encoding/json"

	"github.com/nospy/albion-openradar/internal/capture"
	"github.com/nospy/albion-openradar/internal/logger"
)

// newSettingsPage is a port of internal/templates/pages/settings.gohtml. Sections backed by
// capture.Config (network.json) - Debug's pcap/server-logs toggles, Network, Hub, Market region -
// read/write that store directly (capture.ReadConfig/MutateConfig, the same functions
// cmd/radar/main.go and internal/server's settings handlers already use); everything else is
// internal/syncsettings via Store, like the other pages.
func newSettingsPage(deps Deps, s *Store) fyne.CanvasObject {
	sections := container.NewVBox(
		newDisplaySection(s),
		widget.NewSeparator(),
		newLoggingSection(s),
		widget.NewSeparator(),
		newDebugSection(deps, s),
		widget.NewSeparator(),
		newNetworkSection(deps),
		widget.NewSeparator(),
		newHubSection(deps.AppDir),
		widget.NewSeparator(),
		newMarketSection(deps.AppDir),
		widget.NewSeparator(),
		newDangerZoneSection(s),
	)
	return container.NewVScroll(sections)
}

func newDisplaySection(s *Store) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabelWithStyle("Affichage", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		syncCheck(s, "settingShowMap", "Fond de carte", false),
		syncCheck(s, "settingResourceCount", "Nombre de ressources", false),
		syncCheck(s, "settingResourceDistance", "Indicateur de distance", false),
		syncCheck(s, "settingResourceColorBadges", "Badges colorés par tier", false),
		widget.NewSeparator(),
		syncCheck(s, "settingResourceClusters", "Clusters de ressources", false),
		widget.NewLabel("Rayon (mètres, 10-100)"),
		syncNumberEntry(s, "settingClusterRadius", 30, 10, 100),
		widget.NewLabel("Taille minimum (2-10)"),
		syncNumberEntry(s, "settingClusterMinSize", 2, 2, 10),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Overlay carte", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Zoom (0.25 - 4.0, x1 par défaut)"),
		syncNumberEntry(s, "settingOverlayZoom", 1.0, 0.25, 4.0),
		widget.NewLabel("Opacité du fond de carte (0-100%, 100 par défaut)"),
		syncNumberEntry(s, "settingOverlayMapOpacity", 100, 0, 100),
	)
}

func newLoggingSection(s *Store) fyne.CanvasObject {
	level := widget.NewSelect([]string{"OFF", "ERROR", "WARN", "INFO", "DEBUG"}, func(v string) {
		s.SetString("logLevel", v)
	})
	level.SetSelected(s.GetString("logLevel", "WARN"))

	categories := container.NewGridWithColumns(3,
		syncCheck(s, "categorySystem", "System", false),
		syncCheck(s, "categoryNetwork", "Network", false),
		syncCheck(s, "categoryMap", "Map", false),
		syncCheck(s, "categoryPlayers", "Players", false),
		syncCheck(s, "categoryMobs", "Mobs", false),
		syncCheck(s, "categoryHarvestables", "Harvestables", false),
		syncCheck(s, "categoryDungeons", "Dungeons", false),
		syncCheck(s, "categoryFishing", "Fishing", false),
		syncCheck(s, "categoryRendering", "Rendering", false),
	)

	return container.NewVBox(
		widget.NewLabelWithStyle("Logs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Niveau de log"),
		level,
		syncCheck(s, "settingLogToConsole", "Console", false),
		syncCheck(s, "settingLogToServer", "Logs serveur (JSONL)", false),
		widget.NewLabel("Filtres de catégorie (DEBUG & INFO uniquement)"),
		categories,
	)
}

func newDebugSection(deps Deps, s *Store) fyne.CanvasObject {
	cfg, _ := capture.ReadConfig(deps.AppDir)

	serverLogs := widget.NewCheck("Sauvegarder les logs backend", func(v bool) {
		_ = capture.MutateConfig(deps.AppDir, func(c *capture.Config) { c.Logging.ServerLogsEnabled = v })
		if deps.Logger != nil {
			deps.Logger.SetEnabled(v)
		}
	})
	serverLogs.SetChecked(cfg.Logging.ServerLogsEnabled)

	var pcap *widget.Check
	pcap = widget.NewCheck("Enregistrer la capture réseau (pcap)", func(v bool) {
		if v {
			if deps.Capture == nil || deps.Capture.StartRecording(deps.CaptureDir) != nil {
				logger.PrintWarn("SETTINGS", "pcap recording could not start")
				pcap.SetChecked(false)
				_ = capture.MutateConfig(deps.AppDir, func(c *capture.Config) { c.Logging.PcapRecording = false })
				return
			}
		} else if deps.Capture != nil {
			_ = deps.Capture.StopRecording()
		}
		_ = capture.MutateConfig(deps.AppDir, func(c *capture.Config) { c.Logging.PcapRecording = v })
	})
	pcap.SetChecked(cfg.Logging.PcapRecording)

	// WebSocket performance toggles default to true - matches the web page's own
	// "if get() === null, setBool(true)" one-time hydration, done here by writing the resolved
	// value straight back so subsequent reads never hit the "missing key" branch again either.
	s.SetBool("settingWsCoalescing", s.GetBool("settingWsCoalescing", true))
	s.SetBool("settingWsThrottling", s.GetBool("settingWsThrottling", true))

	exportStatus := widget.NewLabel("")
	exportBtn := widget.NewButton("Exporter les données de debug", func() {
		path, err := exportDebugData(deps.AppDir, s)
		if err != nil {
			exportStatus.SetText("Échec de l'export : " + err.Error())
			return
		}
		exportStatus.SetText("Exporté : " + path)
	})

	return container.NewVBox(
		widget.NewLabelWithStyle("Debug", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		serverLogs, pcap,
		widget.NewLabel("Performance WebSocket"),
		syncCheck(s, "settingWsCoalescing", "Regroupement des évènements (Move/Health)", true),
		syncCheck(s, "settingWsThrottling", "Limitation des mises à jour de vie", true),
		exportBtn, exportStatus,
	)
}

// exportDebugData ports settings.gohtml's downloadLogsBtn: every syncsettings key prefixed
// "setting" (matching the web version's own localStorage-key filter), written to a timestamped
// JSON file in appDir instead of triggering a browser download.
func exportDebugData(appDir string, s *Store) (string, error) {
	settings, err := s.All()
	if err != nil {
		return "", err
	}
	all := map[string]string{}
	for k, v := range settings {
		if strings.HasPrefix(k, "setting") {
			all[k] = v
		}
	}
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	path := appDir + "/openradar-debug-" + timestamp + ".json"
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func newNetworkSection(deps Deps) fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Réseau", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	status := widget.NewLabel("")
	list := container.NewVBox()
	lanTitle := widget.NewLabelWithStyle("Accès LAN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	lanHint := widget.NewLabel("Accessible depuis d'autres appareils du même réseau local. Indépendant des interfaces de capture ci-dessus.")
	lanList := container.NewVBox()
	for _, addr := range capture.LANAddresses() {
		raw := fmt.Sprintf("http://%s:%d/", addr, deps.ServerPort)
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		lanList.Add(widget.NewHyperlink(raw, u))
	}
	if len(lanList.Objects) == 0 {
		lanList.Add(widget.NewLabel("Aucune adresse LAN détectée."))
	}
	body := container.NewVBox(title, status, list)

	var checks map[string]*widget.Check
	var ifaces []capture.NetworkInterface

	refreshState := func() {
		if deps.Capture == nil {
			return
		}
		st := deps.Capture.State()
		names := make([]string, 0, len(st.Active))
		for _, a := range st.Active {
			names = append(names, fmt.Sprintf("%s [%s]", a.Description, a.Address))
		}
		if len(names) == 0 {
			status.SetText("Capture : aucune interface active (" + string(st.Status) + ")")
		} else {
			status.SetText("Capture active sur : " + strings.Join(names, ", "))
		}
	}
	// Web polls /api/network/state every 5s so the status banner reflects a dropped/plugged
	// interface without user action (settings.gohtml). Mirror that here instead of only updating
	// on the manual refresh/apply buttons.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			fyne.Do(refreshState)
		}
	}()

	rebuild := func(fresh []capture.NetworkInterface) {
		ifaces = fresh
		cfg, _ := capture.ReadConfig(deps.AppDir)
		persisted := make(map[string]bool, len(cfg.CaptureInterfaces))
		for _, p := range cfg.CaptureInterfaces {
			persisted[p.Name] = true
		}
		checks = make(map[string]*widget.Check, len(ifaces))
		list.RemoveAll()
		for _, i := range capture.RankCandidates(ifaces) {
			label := fmt.Sprintf("%s (%s) [%s] %s", i.Description, i.Address, i.Name, capture.Categorize(i.Name, i.Description))
			c := widget.NewCheck(label, nil)
			c.SetChecked(persisted[i.Name])
			checks[i.Name] = c
			list.Add(c)
		}
		refreshState()
	}
	rebuild(deps.AllIfaces)

	apply := widget.NewButton("Appliquer la sélection", func() {
		if deps.Capture == nil {
			return
		}
		desired := make([]capture.NetworkInterface, 0)
		for _, i := range ifaces {
			if c, ok := checks[i.Name]; ok && c.Checked {
				desired = append(desired, i)
			}
		}
		if err := deps.Capture.Reconfigure(desired); err != nil {
			status.SetText("Erreur : " + err.Error())
			return
		}
		persisted := make([]capture.PersistedInterface, 0, len(desired))
		for _, i := range desired {
			persisted = append(persisted, capture.PersistedInterface{Name: i.Name, Description: i.Description})
		}
		_ = capture.MutateConfig(deps.AppDir, func(c *capture.Config) { c.CaptureInterfaces = persisted })
		refreshState()
	})
	refresh := widget.NewButton("Rafraîchir la liste", func() {
		fresh, err := capture.EnumerateInterfaces()
		if err != nil {
			status.SetText("Erreur : " + err.Error())
			return
		}
		rebuild(fresh)
	})

	body.Add(container.NewHBox(apply, refresh))
	body.Add(widget.NewSeparator())
	body.Add(lanTitle)
	body.Add(lanHint)
	body.Add(lanList)
	return body
}

func newHubSection(appDir string) fyne.CanvasObject {
	cfg, _ := capture.ReadConfig(appDir)

	urlEntry := widget.NewEntry()
	urlEntry.SetText(cfg.Hub.URL)
	urlEntry.SetPlaceHolder("http://hub.example.com:8090")
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetText(cfg.Hub.Secret)
	secretEntry.SetPlaceHolder("group secret")
	status := widget.NewLabel("")

	enabled := widget.NewCheck("Activer le Hub", func(v bool) {
		_ = capture.MutateConfig(appDir, func(c *capture.Config) { c.Hub.Enabled = v })
	})
	enabled.SetChecked(cfg.Hub.Enabled)

	save := widget.NewButton("Enregistrer", func() {
		if err := capture.MutateConfig(appDir, func(c *capture.Config) {
			c.Hub.URL = urlEntry.Text
			c.Hub.Secret = secretEntry.Text
		}); err != nil {
			status.SetText("Échec de l'enregistrement")
			return
		}
		status.SetText("Enregistré")
	})

	return container.NewVBox(
		widget.NewLabelWithStyle("Hub (routes partagées)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Connecte ce client à un Hub OpenRadar auto-hébergé pour partager les routes d'Avalon découvertes avec le groupe."),
		enabled,
		widget.NewLabel("URL du Hub"), urlEntry,
		widget.NewLabel("Secret partagé"), secretEntry,
		container.NewHBox(save, status),
	)
}

func newMarketSection(appDir string) fyne.CanvasObject {
	cfg, _ := capture.ReadConfig(appDir)
	region := widget.NewSelect([]string{"europe", "americas", "asia"}, func(v string) {
		_ = capture.MutateConfig(appDir, func(c *capture.Config) { c.Market.Region = v })
	})
	if cfg.Market.Region == "" {
		region.SetSelected("europe")
	} else {
		region.SetSelected(cfg.Market.Region)
	}
	return container.NewVBox(
		widget.NewLabelWithStyle("Marché", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Région utilisée pour récupérer les prix (page Market, dans le navigateur) quand aucun Hub n'est configuré/joignable."),
		region,
	)
}

func newDangerZoneSection(s *Store) fyne.CanvasObject {
	resetBtn := widget.NewButton("Réinitialiser tous les réglages", func() {
		dialog.ShowConfirm("Réinitialiser tous les réglages",
			"Cette action remet tous les réglages à leur valeur par défaut. Elle est irréversible.",
			func(ok bool) {
				if ok {
					s.Clear()
				}
			}, fyne.CurrentApp().Driver().AllWindows()[0])
	})
	resetBtn.Importance = widget.DangerImportance

	return container.NewVBox(
		widget.NewLabelWithStyle("Zone de danger", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		resetBtn,
	)
}
