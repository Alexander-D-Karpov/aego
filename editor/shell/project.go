package shell

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/sysopen"
	"aego/editor/core/workspace"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/engine/i18n"
	"aego/platform"
	"aego/project"
)

const (
	CommandNewProject   = "file.new_project"
	CommandOpenProject  = "file.open_project"
	CommandRecentPrefix = "file.recent."
	CommandCloseProject = "file.close_project"
	CommandRunTests     = "play.run_tests"
	CommandOpenLogs     = "help.open_logs"
	CommandCrashReports = "help.crash_reports"
	CommandReportBug    = "help.report_bug"
	CommandInstallGo    = "help.install_go"
	recentCommands      = 5
	issuesURL           = "https://git.akarpov.ru/sanspie/aego/issues/new"
	repositoryURL       = "https://git.akarpov.ru/sanspie/aego"
	developerURL        = "https://about.akarpov.ru"
	telegramURL         = "https://t.me/sanspie"
	goDownloadURL       = "https://go.dev/dl/"
	tabRecent           = "recent"
	tabNew              = "new"
)

var errNoRelaunch = errors.New("switching projects is not available in this editor")

func (a *App) registerProject() error {
	canSwitch := func() bool { return a.opt.Relaunch != nil }
	list := []command.Desc{
		{ID: CommandNewProject, Name: "New Project…", Category: "File", Order: 50, Enabled: canSwitch,
			Run: func() error { return a.switchTo("", tabNew) }},
		{ID: CommandOpenProject, Name: "Open Project…", Category: "File", Order: 60, Enabled: canSwitch, Run: a.openProject},
		{ID: CommandCloseProject, Name: "Close Project", Category: "File", Order: 890, Enabled: canSwitch,
			Run: func() error { return a.switchTo("", tabRecent) }},
		{ID: CommandOpenLogs, Name: "Open Logs Folder", Category: "Help", Order: 200,
			Enabled: func() bool { return a.opt.LogsDir != "" },
			Run:     func() error { return sysopen.Open(a.opt.LogsDir) }},
		{ID: CommandCrashReports, Name: "Crash Reports", Category: "Help", Order: 210,
			Run: func() error {
				a.ws.Show(a.crashesPanel.ID())
				return nil
			}},
		{ID: CommandReportBug, Name: "Report a Bug…", Category: "Help", Order: 220,
			Run: func() error { return sysopen.Open(a.bugURL()) }},
	}
	for i := 0; i < recentCommands; i++ {
		name := "Open Recent " + strconv.Itoa(i+1)
		n := i + 1
		list = append(list, command.Desc{
			ID: CommandRecentPrefix + strconv.Itoa(i+1), Name: name, Category: "File", Order: 70 + i,
			Label: func() string {
				a.recentList()
				if i < len(a.recentLabels) {
					return a.recentLabels[i]
				}
				return i18n.Tr("editor.shell.open_recent_n", "Open Recent {n}", i18n.Int("n", int64(n)))
			},
			Enabled: func() bool {
				_, ok := a.recentAt(i)
				return ok && canSwitch()
			},
			Run: func() error {
				if e, ok := a.recentAt(i); ok {
					return a.switchTo(e.Path, "")
				}
				return nil
			},
		})
	}
	if a.opt.RunTests != nil {
		list = append(list, command.Desc{ID: CommandRunTests, Name: "Run Script Tests", Category: "Play", Order: 400,
			Enabled: func() bool { return !a.testsRunning }, Run: a.runTests})
	}
	if a.opt.NativeNote != "" {
		list = append(list, command.Desc{ID: CommandInstallGo, Name: "Install Go…", Category: "Help", Order: 230,
			Run: func() error { return sysopen.Open(goDownloadURL) }})
	}
	for _, d := range list {
		if err := a.ctx.Commands.Register(d); err != nil {
			return err
		}
	}
	return nil
}

func recentName(e launcher.Entry) string {
	if e.Name != "" {
		return e.Name
	}
	return filepath.Base(e.Path)
}

// Include the path when names collide, both in the menu and command search.
func recentNames(list []launcher.Entry) []string {
	counts := make(map[string]int, len(list))
	for _, e := range list {
		counts[recentName(e)]++
	}
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = recentName(e)
		if counts[names[i]] > 1 {
			names[i] += " — " + e.Path
		}
	}
	return names
}

func (a *App) recentList() []launcher.Entry {
	if a.opt.Recent == "" {
		return nil
	}
	now := a.eng.Platform().Now()
	if a.recentLoaded && now < a.recentNext {
		return a.recent
	}
	list, _ := launcher.LoadRecent(a.opt.Recent)
	out := list[:0]
	for _, e := range list {
		if !e.Missing && e.Path != a.opt.Project.Root {
			out = append(out, e)
		}
	}
	a.recent, a.recentNext, a.recentLoaded = out, now+2, true
	a.recentLabels = a.recentLabels[:0]
	for _, name := range recentNames(out) {
		a.recentLabels = append(a.recentLabels, i18n.Tr("editor.shell.open_recent_item", "Open {name}", i18n.Str("name", name)))
	}
	return a.recent
}

func (a *App) recentAt(i int) (launcher.Entry, bool) {
	list := a.recentList()
	if i < 0 || i >= len(list) {
		return launcher.Entry{}, false
	}
	return list[i], true
}

func (a *App) leave(target, tab string) {
	if err := a.opt.Relaunch(target, tab); err != nil {
		a.ctx.Notify(context.LevelError, i18n.Note("editor.shell.cannot_start_the_editor_again", "cannot start the editor again: {error}", i18n.Str("error", err.Error())))
		return
	}
	a.quitting = true
	a.eng.Quit()
}

func (a *App) switchTo(target, tab string) error {
	if a.opt.Relaunch == nil {
		return errNoRelaunch
	}
	if !a.ctx.Docs.Dirty() {
		a.leave(target, tab)
		return nil
	}
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.shell.unsaved_changes", "Unsaved changes"),
		Message: i18n.Tr("editor.shell.save_the_changed_scenes_and", "Save the changed scenes and scripts before leaving this project?"),
		Buttons: []string{i18n.Tr("editor.shell.save_and_continue", "Save and Continue"), i18n.Tr("editor.shell.discard", "Discard"), i18n.Tr("editor.shell.cancel", "Cancel")},
		Then: func(choice int) {
			switch choice {
			case 0:
				if err := a.saveAll(); err != nil {
					a.ctx.Notify(context.LevelError, err.Error())
					return
				}
				a.leave(target, tab)
			case 1:
				a.leave(target, tab)
			}
		},
	})
	return nil
}

func (a *App) openProject() error {
	open := func(path string) {
		root, ok := launcher.ProjectRoot(strings.TrimSpace(path))
		if !ok {
			a.ctx.Notify(context.LevelError, i18n.Note("editor.shell.no_filename_in_path", "No {filename} in {path}", i18n.Str("filename", project.FileName), i18n.Str("path", path)))
			return
		}
		if err := a.switchTo(root, ""); err != nil {
			a.ctx.Notify(context.LevelError, err.Error())
		}
	}
	if pick := sysopen.Picker(a.eng.Window(), platform.DialogFolder, "", ""); pick != nil {
		pick(filepath.Dir(a.opt.Project.Root), open)
		return nil
	}
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.shell.open_project", "Open project"),
		Fields:  []string{i18n.Tr("editor.shell.folder", "Folder")},
		Values:  []string{filepath.Dir(a.opt.Project.Root)},
		Buttons: []string{i18n.Tr("editor.shell.open", "Open"), i18n.Tr("editor.shell.cancel", "Cancel")},
		Submit:  func(v []string) { open(v[0]) },
	})
	return nil
}

func (a *App) dropProject() bool {
	if len(a.drops) != 1 {
		return false
	}
	root, ok := launcher.ProjectRoot(a.drops[0])
	if !ok {
		return false
	}
	if root != a.opt.Project.Root {
		if err := a.switchTo(root, ""); err != nil {
			a.ctx.Notify(context.LevelError, err.Error())
		}
	}
	return true
}

func (a *App) pickFolder(start string, done func(string)) {
	if pick := sysopen.Picker(a.eng.Window(), platform.DialogFolder, "", ""); pick != nil {
		pick(start, done)
	}
}

func (a *App) importAssetFile(dir string) {
	project, pipeline := a.ctx.Project, a.ctx.Pipeline
	apply := func(file string) {
		if a.ctx.Project != project || a.ctx.Pipeline != pipeline {
			a.ctx.Notify(context.LevelError, i18n.Tr("editor.assets.import_project_changed", "The project changed before import completed."))
			return
		}
		if file != "" {
			a.assetsPanel.ImportFiles([]string{file}, dir)
		}
	}
	if win := a.eng.Window(); win != nil {
		if picker, ok := win.(platform.FileDialogs); ok {
			err := picker.ShowFileDialog(platform.FileDialog{Kind: platform.DialogOpenFile, Start: a.opt.Project.Root}, func(file string, ok bool) {
				if ok {
					apply(file)
				}
			})
			if err == nil {
				return
			}
		}
	}
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.assets.import_file", "Import File…"),
		Fields:  []string{i18n.Tr("editor.assets.external_file_path", "File path")},
		Buttons: []string{i18n.Tr("editor.art.import_action", "Import"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) > 0 {
				apply(strings.TrimSpace(values[0]))
			}
		},
	})
}

func (a *App) bugURL() string {
	body := "Aego " + engine.Version + ", " + runtime.GOOS + "/" + runtime.GOARCH + ", GPU: " + a.eng.Caps().Renderer +
		"\n\nWhat happened:\n\nWhat you expected:\n\nSteps to reproduce:\n\nThe editor log is in Help → Open Logs Folder.\n"
	return issuesURL + "?body=" + url.QueryEscape(body)
}

func (a *App) openSupportURL(target string) {
	if err := sysopen.Open(target); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
	}
}

func (a *App) runTests() error {
	a.testsRunning = true
	a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.running_script_tests", "Running script tests…"))
	return a.opt.RunTests(a.testsDone)
}

func (a *App) testsDone(out string, failed bool) {
	a.testsRunning = false
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		switch {
		case line == "":
		case strings.Contains(line, "FAIL") || strings.Contains(line, "error"):
			a.scriptLog.Error(line)
		default:
			a.scriptLog.Info(line)
		}
	}
	a.ws.Show(workspace.Console)
	if failed {
		a.ctx.Notify(context.LevelError, i18n.Note("editor.shell.script_tests_failed_see_console", "Script tests failed, see Console and Problems"))
		return
	}
	a.ctx.Notify(context.LevelInfo, i18n.Note("editor.shell.script_tests_passed", "Script tests passed"))
}

func (a *App) nativeHint() string {
	if a.opt.NativeNote == "" {
		return ""
	}
	return i18n.Tr("editor.shell.go_not_found_native_code", "Go not found — native code is not built (Help → Install Go)")
}

func (a *App) OpenAgentProject(root string) {
	if err := a.switchTo(root, ""); err != nil {
		a.ctx.Notify(context.LevelWarn, err.Error())
	}
}
