package shell

import (
	"strings"

	"aego/editor/core/context"
	"aego/engine/i18n"
)

// This is an explicit export, not an automatic recovery or a replacement for
// Save. An external directory can be on a different filesystem from the
// project, and is chosen by the user rather than guessed from /tmp or HOME.
func (a *App) askRecoveryCopy() {
	docs, project := a.ctx.Docs, a.ctx.Project
	a.ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.shell.recovery_copy_title", "Save emergency recovery copies"),
		Message: i18n.Tr("editor.shell.recovery_copy_notice", "Choose an existing directory on a filesystem with free space. A new private folder will contain copies of current unsaved documents; project assets and existing recovery files are not replaced. Earlier pending recovery files are not included. This is not Save, and recovery is never automatic. Temporary storage may disappear after reboot."),
		Fields:  []string{i18n.Tr("editor.shell.recovery_copy_directory", "Absolute destination directory")},
		Values:  []string{""},
		Buttons: []string{i18n.Tr("editor.shell.recovery_copy_save", "Save copies"), i18n.Tr("editor.common.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) != 1 || docs == nil || a.ctx.Docs != docs || a.ctx.Project != project {
				a.ctx.Notify(context.LevelError, i18n.Tr("editor.shell.recovery_copy_project_changed", "The project changed before the recovery copy was saved."))
				return
			}
			output, count, err := docs.WriteRecoveryCopy(strings.TrimSpace(values[0]))
			if err != nil {
				if output != "" {
					a.ctx.Notify(context.LevelError, i18n.Tr("editor.shell.recovery_copy_partial", "Recovery copy is incomplete in {path}: {error}. Keep the editor open and try another location.", i18n.Str("path", output), i18n.Str("error", err.Error())))
				} else {
					a.ctx.Notify(context.LevelError, i18n.Tr("editor.shell.recovery_copy_failed", "Could not save recovery copies: {error}. Keep the editor open and try another location.", i18n.Str("error", err.Error())))
				}
				return
			}
			a.ctx.Notify(context.LevelInfo, i18n.Tr("editor.shell.recovery_copy_saved", "Saved {count} emergency recovery copies in {path}. These are not saved project assets and will not restore automatically.", i18n.Int("count", int64(count)), i18n.Str("path", output)))
		},
	})
}
