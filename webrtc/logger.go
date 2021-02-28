package webrtc

import (
	"github.com/edaniels/golog"
	"github.com/pion/logging"
)

// LoggerFactory wraps a golog.Logger for use with pion's webrtc logging system.
type LoggerFactory struct {
	Logger golog.Logger
}

type logger struct {
	logger golog.Logger
}

func (l logger) Trace(msg string) {
	l.logger.Debug(msg)
}

func (l logger) Tracef(format string, args ...interface{}) {
	l.logger.Debugf(format, args...)
}

func (l logger) Debug(msg string) {
	l.logger.Debug(msg)
}

func (l logger) Debugf(format string, args ...interface{}) {
	l.logger.Debugf(format, args...)
}

func (l logger) Info(msg string) {
	l.logger.Info(msg)
}

func (l logger) Infof(format string, args ...interface{}) {
	l.logger.Infof(format, args...)
}

func (l logger) Warn(msg string) {
	l.logger.Warn(msg)
}

func (l logger) Warnf(format string, args ...interface{}) {
	l.logger.Warnf(format, args...)
}

func (l logger) Error(msg string) {
	l.logger.Error(msg)
}

func (l logger) Errorf(format string, args ...interface{}) {
	l.logger.Errorf(format, args...)
}

// NewLogger returns a new webrtc logger under the given scope.
func (lf LoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	return logger{lf.Logger.Named(scope)}
}
