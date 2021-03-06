// Copyright 2020 Schibsted

// Package plog is used for logging and sending logs to a log server, where
// they'll be aggregated into log objects and kept safe even if the client
// program exists or crashes. Can also be used without the server in which
// case a fallback writer is used instead.
//
// The LogMsg class of functions are the latest and most useful ones.
// You can either use the package level LogMsg with any key you wish
// or use e.g. plog.Info.LogMsg for a standard "INFO" key.
//
// A Setup function is provided to apply this package to the standard
// log interface and also to the other sebase packages. It's recommended
// to call Setup if you use this package.
//
// For advanced usage recursive plog contexts can be opened. They then
// log a JSON object once all the contexts under the root are closed.
// If plogd is running it will detect if a context was not closed
// properly and log an "@interrupted" key which might be useful to find
// which request caused an error.
//
// Context based logging is done either via the package level Ctx function or
// Level.Ctx. Info logging is thus likely plog.Info.Ctx(ctx).LogMsg or
// plog.Info.CtxMsg as a shortcut.  These apply filters that can be stored in
// the context via slog.ContextWithFilter.
//
// You can also store a plog context or another logger
// function using plog.ContextWithLogger. Those can also be used to apply
// context to the logs. If a plog context is used it can be retrieved using
// CtxPlog.
//
// Various compatibility and shortcut functions also exist, for example there
// are Print and Printf package level functions.
package plog

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/schibsted/sebase/util/pkg/slog"
	"golang.org/x/crypto/ssh/terminal"
)

// SetupLevel is the threshold for logging, only this and higher prio levels
// will be logged. Modified by Setup.
var SetupLevel = Info

// Setup initializes the default plog context and changes SetupLevel based on a
// string. Changes the functions used by the slog package to plog and also
// calls log.SetOutput(plog.Info) to redirect log.Printf output to this
// package, as well as log.SetFlags(0). Finally checks if FallbackWriter is a
// TTY. If so, changes FallbackFormatter to FallbackFormatterSimple.
func Setup(appname, lvl string) {
	SetupLevel = LogLevel(lvl, SetupLevel)
	Default = NewPlogLog(appname)
	log.SetOutput(Info)
	log.SetFlags(0)

	// Check for Fd function implemented by *os.File.
	type Fd interface {
		Fd() uintptr
	}
	fd, ok := FallbackWriter.(Fd)
	if ok && terminal.IsTerminal(int(fd.Fd())) {
		FallbackFormatter = FallbackFormatterSimple
	}
	slog.Critical = Critical.LogMsg
	slog.Error = Error.LogMsg
	slog.Warning = Warning.LogMsg
	slog.Info = Info.LogMsg
	slog.Debug = Debug.LogMsg
}

// Shutdown closes Default, properly disconnecting from the logging server.
func Shutdown() {
	Default.Close()
	Default = nil
}

// Log logs to Default if not nil, or to FallbackWriter if Default is nil.
// Might return errors from json.Marshal, if it fails to encode the value
// given.
// This is a low level function, you likely want to use Level.LogMsg instead,
// or the package level LogMsg if you wish to customize the key.
func Log(key string, value interface{}) error {
	if Default != nil {
		return Default.Log(key, value)
	}
	jw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = FallbackFormatter([]FallbackKey{{key, 0}}, jw)
	return err
}

// LogDict logs a JSON dictionary from the variadic arguments, which are parsed
// with slog.KVsMap. Logs to Default or to FallbackWriter if nil.
// Deprecated in favor of LogMsg.
func LogDict(key string, kvs ...interface{}) error {
	return Log(key, slog.KVsMap(kvs...))
}

// LogMsg logs a message together with a JSON dictionary from the variadic
// arguments, which are parsed by slog.KVsMap. Logs to Default or to
// FallbackWriter if Default is nil.
// This function does not return errors. If json encoding fails it
// converts to a string and tries again, adding a "log-error" key.
func LogMsg(key, msg string, kvs ...interface{}) {
	m := slog.KVsMap(kvs...)
	m["msg"] = msg
	errWrap(Log, key, m)
}

// errWrap calls f with key and value. If it returns an error, it tries it best
// to convert value into something that won't error on json.Marshal and tries again.
// If possible it adds the error with a "log-error" key.
func errWrap(f func(key string, value interface{}) error, key string, value interface{}) {
	err := f(key, value)
	if err == nil {
		return
	}
	// Error, convert all values to strings.
	switch value := value.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{}, len(value))
		for k, v := range value {
			m[k] = fmt.Sprint(v)
		}
		m["log-error"] = err.Error()
		f(key, m)
	default:
		f(key, fmt.Sprint(value))
	}
}

// Print is shorthand for Info.Print
func Print(value ...interface{}) {
	Info.Print(value...)
}

// Printf is shorthand for Info.Printf
func Printf(fmt string, value ...interface{}) {
	Info.Printf(fmt, value...)
}
