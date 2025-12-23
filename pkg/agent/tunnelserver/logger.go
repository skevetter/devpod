package tunnelserver

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/log"
	"github.com/skevetter/log/scanner"
	"github.com/skevetter/log/survey"
)

func NewTunnelLogger(ctx context.Context, client tunnel.TunnelClient, debug bool) log.Logger {
	level := logrus.InfoLevel
	if debug {
		level = logrus.DebugLevel
	}

	logger := &tunnelLogger{
		ctx:     ctx,
		client:  client,
		level:   level,
		logChan: make(chan *tunnel.LogMessage, 1000), // Buffer size of 1000 messages
	}

	go logger.worker()

	return logger
}

type tunnelLogger struct {
	ctx     context.Context
	level   logrus.Level
	client  tunnel.TunnelClient
	logChan chan *tunnel.LogMessage
	fields  logrus.Fields
}

func (s *tunnelLogger) worker() {
	for {
		select {
		case msg := <-s.logChan:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			_, _ = s.client.Log(ctx, msg)
			// ignore error since we can't use the logger itself
			cancel()
		case <-s.ctx.Done():
			return
		}
	}
}

// formatMessage appends structured fields to the message
func (s *tunnelLogger) formatMessage(message string) string {
	if len(s.fields) == 0 {
		return message
	}

	var fieldsStr strings.Builder
	for k, v := range s.fields {
		fmt.Fprintf(&fieldsStr, " %s=%v", k, v)
	}
	return message + fieldsStr.String()
}

func (s *tunnelLogger) Debug(args ...any) {
	if s.level < logrus.DebugLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_DEBUG,
		Message:  s.formatMessage(fmt.Sprintln(args...)),
	}
}

func (s *tunnelLogger) Debugf(format string, args ...any) {
	if s.level < logrus.DebugLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_DEBUG,
		Message:  s.formatMessage(fmt.Sprintf(format, args...) + "\n"),
	}
}

func (s *tunnelLogger) Info(args ...any) {
	if s.level < logrus.InfoLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_INFO,
		Message:  s.formatMessage(fmt.Sprintln(args...)),
	}
}

func (s *tunnelLogger) Infof(format string, args ...any) {
	if s.level < logrus.InfoLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_INFO,
		Message:  s.formatMessage(fmt.Sprintf(format, args...) + "\n"),
	}
}

func (s *tunnelLogger) Warn(args ...any) {
	if s.level < logrus.WarnLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_WARNING,
		Message:  s.formatMessage(fmt.Sprintln(args...)),
	}
}

func (s *tunnelLogger) Warnf(format string, args ...any) {
	if s.level < logrus.WarnLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_WARNING,
		Message:  s.formatMessage(fmt.Sprintf(format, args...) + "\n"),
	}
}

func (s *tunnelLogger) Error(args ...any) {
	if s.level < logrus.ErrorLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_ERROR,
		Message:  s.formatMessage(fmt.Sprintln(args...)),
	}
}

func (s *tunnelLogger) Errorf(format string, args ...any) {
	if s.level < logrus.ErrorLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_ERROR,
		Message:  s.formatMessage(fmt.Sprintf(format, args...) + "\n"),
	}
}

func (s *tunnelLogger) Fatal(args ...any) {
	if s.level < logrus.FatalLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_ERROR,
		Message:  s.formatMessage(fmt.Sprintln(args...)),
	}

	os.Exit(1)
}

func (s *tunnelLogger) Fatalf(format string, args ...any) {
	if s.level < logrus.FatalLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_ERROR,
		Message:  s.formatMessage(fmt.Sprintf(format, args...) + "\n"),
	}

	os.Exit(1)
}

func (s *tunnelLogger) Done(args ...any) {
	if s.level < logrus.InfoLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_DONE,
		Message:  s.formatMessage(fmt.Sprintln(args...)),
	}
}

func (s *tunnelLogger) Donef(format string, args ...any) {
	if s.level < logrus.InfoLevel {
		return
	}

	s.logChan <- &tunnel.LogMessage{
		LogLevel: tunnel.LogLevel_DONE,
		Message:  s.formatMessage(fmt.Sprintf(format, args...) + "\n"),
	}
}

func (s *tunnelLogger) Print(level logrus.Level, args ...any) {
	switch level {
	case logrus.InfoLevel:
		s.Info(args...)
	case logrus.DebugLevel:
		s.Debug(args...)
	case logrus.WarnLevel:
		s.Warn(args...)
	case logrus.ErrorLevel:
		s.Error(args...)
	case logrus.FatalLevel:
		s.Fatal(args...)
	case logrus.PanicLevel:
		s.Fatal(args...)
	case logrus.TraceLevel:
		s.Debug(args...)
	}
}

func (s *tunnelLogger) Printf(level logrus.Level, format string, args ...any) {
	switch level {
	case logrus.InfoLevel:
		s.Infof(format, args...)
	case logrus.DebugLevel:
		s.Debugf(format, args...)
	case logrus.WarnLevel:
		s.Warnf(format, args...)
	case logrus.ErrorLevel:
		s.Errorf(format, args...)
	case logrus.FatalLevel:
		s.Fatalf(format, args...)
	case logrus.PanicLevel:
		s.Fatalf(format, args...)
	case logrus.TraceLevel:
		s.Debugf(format, args...)
	}
}

func (s *tunnelLogger) SetLevel(level logrus.Level) {
	s.level = level
}

func (s *tunnelLogger) GetLevel() logrus.Level {
	return s.level
}

func (s *tunnelLogger) Writer(level logrus.Level, raw bool) io.WriteCloser {
	if s.level < level {
		return &log.NopCloser{Writer: io.Discard}
	}

	reader, writer := io.Pipe()
	go func() {
		sa := scanner.NewScanner(reader)
		for sa.Scan() {
			if raw {
				s.WriteString(level, sa.Text()+"\n")
			} else {
				s.Print(level, sa.Text())
			}
		}
	}()

	return writer
}

func (s *tunnelLogger) WriteString(level logrus.Level, message string) {
	if s.level < level {
		return
	}

	// TODO: support this correctly
	s.Print(level, message)
}

func (s *tunnelLogger) WriteLevel(level logrus.Level, message []byte) (int, error) {
	if s.level < level {
		return 0, nil
	}

	s.Print(level, string(message))
	return len(message), nil
}

func (s *tunnelLogger) Question(params *survey.QuestionOptions) (string, error) {
	return "", fmt.Errorf("not supported")
}

func (s *tunnelLogger) ErrorStreamOnly() log.Logger {
	return s
}

func (s *tunnelLogger) WithFields(fields logrus.Fields) log.Logger {
	newFields := make(logrus.Fields)
	maps.Copy(newFields, s.fields)
	maps.Copy(newFields, fields)

	return &tunnelLogger{
		ctx:     s.ctx,
		client:  s.client,
		level:   s.level,
		logChan: s.logChan,
		fields:  newFields,
	}
}

func (*tunnelLogger) LogrLogSink() logr.LogSink {
	return nil
}
