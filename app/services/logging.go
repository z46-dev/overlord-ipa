package services

import "github.com/z46-dev/golog"

// serviceLogger returns a child logger or nil when logging is not configured.
func serviceLogger(parent *golog.Logger, prefix string, color golog.ColorCode) (logger *golog.Logger) {
	if parent == nil {
		return
	}

	logger = parent.SpawnChild().Prefix(prefix, color)
	return
}
