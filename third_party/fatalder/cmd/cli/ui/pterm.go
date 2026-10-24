package ui

import (
	"io"
	"os"

	"github.com/pterm/pterm"
)

var defaultWriter io.Writer = os.Stdout
var Theme = pterm.ThemeDefault
var (
	// Info returns a PrefixPrinter, which can be used to print text with an "info" Prefix.
	Info = pterm.PrefixPrinter{
		MessageStyle: &Theme.InfoMessageStyle,
		Prefix: pterm.Prefix{
			Style: &Theme.InfoPrefixStyle,
			Text:  "INFO",
		},
		Writer: defaultWriter,
	}

	// Warning returns a PrefixPrinter, which can be used to print text with a "warning" Prefix.
	Warning = pterm.PrefixPrinter{
		MessageStyle: &Theme.WarningMessageStyle,
		Prefix: pterm.Prefix{
			Style: &Theme.WarningPrefixStyle,
			Text:  "WARN",
		},
		Writer: defaultWriter,
	}

	// Success returns a PrefixPrinter, which can be used to print text with a "success" Prefix.
	Success = pterm.PrefixPrinter{
		MessageStyle: &Theme.SuccessMessageStyle,
		Prefix: pterm.Prefix{
			Style: &Theme.SuccessPrefixStyle,
			Text:  "SUCC",
		},
		Writer: defaultWriter,
	}

	// Error returns a PrefixPrinter, which can be used to print text with an "error" Prefix.
	Error = pterm.PrefixPrinter{
		MessageStyle: &Theme.ErrorMessageStyle,
		Prefix: pterm.Prefix{
			Style: &Theme.ErrorPrefixStyle,
			Text:  "ERRO",
		},
		Writer: defaultWriter,
	}

	// Fatal returns a PrefixPrinter, which can be used to print text with an "fatal" Prefix.
	// NOTICE: Fatal terminates the application immediately!
	Fatal = pterm.PrefixPrinter{
		MessageStyle: &Theme.FatalMessageStyle,
		Prefix: pterm.Prefix{
			Style: &Theme.FatalPrefixStyle,
			Text:  "FATA",
		},
		Fatal:  true,
		Writer: defaultWriter,
	}

	// Debug Prints debug messages. By default it will only print if PrintDebugMessages is true.
	// You can change PrintDebugMessages with EnableDebugMessages and DisableDebugMessages, or by setting the variable itself.
	Debug = pterm.PrefixPrinter{
		MessageStyle: &Theme.DebugMessageStyle,
		Prefix: pterm.Prefix{
			Text:  "DBUG",
			Style: &Theme.DebugPrefixStyle,
		},
		Debugger: true,
		Writer:   defaultWriter,
	}

	// Description returns a PrefixPrinter, which can be used to print text with a "description" Prefix.
	Description = pterm.PrefixPrinter{
		MessageStyle: &Theme.DescriptionMessageStyle,
		Prefix: pterm.Prefix{
			Style: &Theme.DescriptionPrefixStyle,
			Text:  "DESC",
		},
		Writer: defaultWriter,
	}
)
var InteractiveTextInput = pterm.InteractiveTextInputPrinter{
	DefaultText: "Input text",
	Delimiter:   ": ",
	TextStyle:   &Theme.PrimaryStyle,
	Mask:        "",
}
var InteractiveConfirm = pterm.InteractiveConfirmPrinter{
	DefaultText:  "Please confirm",
	TextStyle:    &Theme.PrimaryStyle,
	ConfirmText:  "Yes",
	ConfirmStyle: &Theme.SuccessMessageStyle,
	RejectText:   "No",
	RejectStyle:  &Theme.ErrorMessageStyle,
	SuffixStyle:  &Theme.SecondaryStyle,
	Delimiter:    ": ",
}
