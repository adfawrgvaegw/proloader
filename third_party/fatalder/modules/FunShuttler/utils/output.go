package utils

import (
	"github.com/Yeah114/FunShuttler/i18n"

	"github.com/pterm/pterm"
)

func ShowError(shortMsg, detailMsg string) {
	Log.Error(pterm.Red(shortMsg))
	if detailMsg != "" {
		Log.Error(pterm.Red(i18n.T(i18n.Utils_Error_DetailedError), detailMsg))
	}
}
