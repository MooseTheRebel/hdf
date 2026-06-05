package notify

import "github.com/gen2brain/beeep"

// Notifier is the interface for sending desktop notifications.
type Notifier interface {
	Send(title, message string) error
}

type defaultNotifier struct{}

func (defaultNotifier) Send(title, message string) error {
	return beeep.Notify(title, message, "")
}

// Default is the production Notifier. Tests substitute their own implementation.
var Default Notifier = defaultNotifier{}

// Send sends a desktop notification using the Default notifier.
func Send(title, message string) error {
	return Default.Send(title, message)
}
