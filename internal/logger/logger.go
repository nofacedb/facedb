package logger

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Logger is simple wrapper for sirupsen/logrus.
type Logger struct {
	logrus.Logger
}

// CreateLogger ...
func CreateLogger(cfg *cfgparser.LoggerCFG) (*logrus.Logger, error) {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	switch cfg.Output {
	case "stdout":
		l.Out = os.Stdout
	case "stderr":
		l.Out = os.Stderr
	default:
		output := filepath.FromSlash(cfg.Output)
		dname := filepath.Dir(output)
		if err := os.MkdirAll(dname, 0766); err != nil {
			return nil, errors.Wrap(err, "unable to create log dir")
		}
		fname := filepath.Base(output)
		fname = strings.Replace(fname, "%t", time.Now().Format(cfg.TimestampFormat), -1)
		f, err := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create log file")
		}
		l.Out = f
	}
	textFormatter := &logrus.TextFormatter{}
	if cfg.UseColors {
		textFormatter.ForceColors = true
	} else {
		textFormatter.DisableColors = true
	}
	if cfg.UseTimestamp {
		textFormatter.FullTimestamp = true
		textFormatter.TimestampFormat = cfg.TimestampFormat
	} else {
		textFormatter.DisableTimestamp = true
	}
	l.SetFormatter(textFormatter)
	if cfg.NonBlocking {
		l.SetNoLock()
	}
	return l, nil
}
