// Package logging configures the shared logrus logger. Output goes to stdout by
// default (logName==""); a named file redirects there.
package logging

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// Setup returns a configured logger. When logName is non-empty, logs are written
// to that file; otherwise to stdout. debug raises the level to Debug.
func Setup(logName string, debug bool) (*logrus.Logger, error) {
	l := logrus.New()
	l.SetFormatter(&logrus.JSONFormatter{})
	l.SetLevel(logrus.InfoLevel)
	if debug {
		l.SetLevel(logrus.DebugLevel)
	}

	var out io.Writer = os.Stdout
	if logName != "" {
		f, err := os.OpenFile(logName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		out = f
	}
	l.SetOutput(out)
	return l, nil
}
