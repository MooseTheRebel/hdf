package notify

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/gen2brain/beeep"
)

// ModeEnv names the environment variable that redirects notifications.
// When it is set to ModeLog, notifications are written to stderr instead of
// being shown by the OS — used by the e2e suite, which runs the daemon and
// must not pop up (or block on) real desktop alerts.
const (
	ModeEnv = "HDF_NOTIFY"
	ModeLog = "log"
)

// logOnly reports whether notifications should go to stderr rather than the OS.
func logOnly() bool { return os.Getenv(ModeEnv) == ModeLog }

// stderr is where log-mode notifications are written; a var so tests can capture it.
var stderr io.Writer = os.Stderr

func writeNotification(kind, title, message string) error {
	_, err := fmt.Fprintf(stderr, "hdf %s: %s: %s\n", kind, title, message)

	return err
}

// Notifier is the interface for sending desktop notifications.
type Notifier interface {
	Send(title, message string) error
}

// NotifyLevel classifies the severity of a daemon notification.
type NotifyLevel int

// NotifyLevel constants classify daemon notification severity.
const (
	LevelInfo     NotifyLevel = iota // informational; written to log only
	LevelWarning                     // written to log; surfaced at next hdf changes-push/changes-pull
	LevelCritical                    // log + distinct OS alert, independent of wails
)

func (l NotifyLevel) String() string {
	switch l {
	case LevelInfo:
		return "INFO"
	case LevelWarning:
		return "WARN"
	case LevelCritical:
		return "CRITICAL"
	default:
		return fmt.Sprintf("LEVEL(%d)", int(l))
	}
}

type defaultNotifier struct{}

func (defaultNotifier) Send(title, message string) error {
	if logOnly() {
		return writeNotification("notification", title, message)
	}

	return beeep.Notify(title, message, "")
}

// criticalNotifier uses beeep.Alert — a more prominent OS alert
// (NSAlert on macOS, MessageBox on Windows) that is visually distinct
// from the wails window and works even when the GUI is closed.
// TODO: Linux desktop support.
type criticalNotifier struct{}

func (criticalNotifier) Send(title, message string) error {
	if logOnly() {
		return writeNotification("alert", title, message)
	}

	return beeep.Alert(title, message, "")
}

// Default is the production Notifier for drift/informational alerts.
var Default Notifier = defaultNotifier{}

// Critical is the production Notifier for serious daemon errors.
// It sends a prominent OS-level alert independent of the wails window.
var Critical Notifier = criticalNotifier{}

// Send sends a desktop notification using the Default notifier.
func Send(title, message string) error { return Default.Send(title, message) }

// SendCritical sends a prominent OS alert using the Critical notifier.
func SendCritical(title, message string) error { return Critical.Send(title, message) }

// LogAndNotify writes msg to the standard logger, then dispatches:
//   - LevelInfo/Warning → log only (warnings are surfaced at push/pull via PendingWarnings)
//   - LevelCritical     → log + Critical OS alert independent of the wails window
func LogAndNotify(level NotifyLevel, title, message string) {
	log.Printf("[%s] %s: %s", level, title, message)

	if level == LevelCritical {
		err := SendCritical(title, message)
		if err != nil {
			log.Printf("[CRITICAL] failed to send OS notification: %v", err)
		}
	}
}
