package notify

import (
	"fmt"
	"log"

	"github.com/gen2brain/beeep"
)

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
	return beeep.Notify(title, message, "")
}

// criticalNotifier uses beeep.Alert — a more prominent OS alert
// (NSAlert on macOS, MessageBox on Windows) that is visually distinct
// from the wails window and works even when the GUI is closed.
// TODO: Linux desktop support
type criticalNotifier struct{}

func (criticalNotifier) Send(title, message string) error {
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
		if err := SendCritical(title, message); err != nil {
			log.Printf("[CRITICAL] failed to send OS notification: %v", err)
		}
	}
}
