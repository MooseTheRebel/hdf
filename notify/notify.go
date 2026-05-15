package notify

import "github.com/gen2brain/beeep"

func Send(title, message string) error {
	return beeep.Notify(title, message, "")
}
