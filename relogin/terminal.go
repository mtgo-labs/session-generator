package relogin

import (
	"os"

	"github.com/mtgo-labs/mtgo/telegram"
)

// TerminalOptions returns Options pre-configured with terminal-based
// CodeFunc and PassFunc from mtgo. Convenience for CLI usage.
func TerminalOptions(apiID int64, apiHash, session string) Options {
	return Options{
		APIID:    apiID,
		APIHash:  apiHash,
		Session:  session,
		CodeFunc: telegram.TerminalCodeFunc(),
		PassFunc: telegram.TerminalPasswordFunc(),
		Output:   os.Stderr,
	}
}
