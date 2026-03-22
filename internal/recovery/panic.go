// Package recovery provides panic recovery utilities for goroutines.
// Prevents individual goroutine panics from crashing the entire application.
package recovery

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// PanicInfo captures information about a recovered panic.
type PanicInfo struct {
	Recovered interface{}   // The panic value
	Stack     []byte        // Stack trace
	Timestamp time.Time     // When the panic occurred
}

// Handler is a function that handles recovered panics.
// Can be used for logging, monitoring, or custom recovery logic.
type Handler func(info PanicInfo)

// DefaultHandler is the default panic handler.
// It prints panic information to stderr.
var DefaultHandler Handler = func(info PanicInfo) {
	fmt.Printf("[PANIC RECOVERED] %v\nStack:\n%s\n", info.Recovered, info.Stack)
}

var globalHandler struct {
	sync.RWMutex
	handler Handler
}

func init() {
	globalHandler.handler = DefaultHandler
}

// SetGlobalHandler sets the global panic handler.
// Safe to call concurrently.
func SetGlobalHandler(h Handler) {
	globalHandler.Lock()
	defer globalHandler.Unlock()
	globalHandler.handler = h
}

// GetGlobalHandler returns the current global panic handler.
func GetGlobalHandler() Handler {
	globalHandler.RLock()
	defer globalHandler.RUnlock()
	return globalHandler.handler
}

// SafeGoroutine runs a function in a goroutine with panic recovery.
// If fn panics, the panic is recovered and passed to the global handler.
// The goroutine will not crash the application.
//
// Example:
//
//	go recovery.SafeGoroutine(func() {
//	    // Risky code here
//	})
func SafeGoroutine(fn func()) {
	go func() {
		defer HandlePanic()
		fn()
	}()
}

// SafeGoroutineWithContext runs a function in a goroutine with panic recovery and context.
// If fn panics, the panic is recovered and passed to the global handler.
// If ctx is cancelled before fn starts, the goroutine exits immediately.
//
// Example:
//
//	go recovery.SafeGoroutineWithContext(ctx, func(ctx context.Context) {
//	    // Risky code here that respects ctx
//	})
func SafeGoroutineWithContext(ctx context.Context, fn func(context.Context)) {
	go func() {
		defer HandlePanic()
		fn(ctx)
	}()
}

// HandlePanic recovers from a panic and invokes the global handler.
// Should be called with defer at the beginning of any goroutine or function
// that might panic.
//
// Example:
//
//	func myFunction() {
//	    defer recovery.HandlePanic()
//	    // Risky code here
//	}
func HandlePanic() {
	if r := recover(); r != nil {
		info := PanicInfo{
			Recovered: r,
			Stack:     debug.Stack(),
			Timestamp: time.Now(),
		}

		handler := GetGlobalHandler()
		if handler != nil {
			handler(info)
		} else {
			// Fallback to default handler
			DefaultHandler(info)
		}
	}
}

// SafeWrap wraps a function with panic recovery and returns a new function.
// The returned function will recover from panics and invoke the global handler.
// Useful for passing functions to APIs that don't support panic recovery.
//
// Example:
//
//	http.HandleFunc("/endpoint", recovery.SafeWrap(myHandler))
func SafeWrap(fn func()) func() {
	return func() {
		defer HandlePanic()
		fn()
	}
}

// SafeWrapWithContext wraps a context-aware function with panic recovery.
// The returned function will recover from panics and invoke the global handler.
//
// Example:
//
//	http.HandleFunc("/endpoint", recovery.SafeWrapWithContext(myHandler))
func SafeWrapWithContext(fn func(context.Context)) func(context.Context) {
	return func(ctx context.Context) {
		defer HandlePanic()
		fn(ctx)
	}
}

// SafeErrgroupGo wraps errgroup.Group.Go with panic recovery.
// If fn panics, the panic is recovered and converted to an error.
// The error is returned to the errgroup, which will cancel the context.
//
// Example:
//
//	g, gctx := errgroup.WithContext(ctx)
//	recovery.SafeErrgroupGo(&g, func() error {
//	    // Risky code here
//	    return nil
//	})
func SafeErrgroupGo(g interface{ Go(func() error) }, fn func() error) {
	g.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				// Convert panic to error
				info := PanicInfo{
					Recovered: r,
					Stack:     debug.Stack(),
					Timestamp: time.Now(),
				}

				handler := GetGlobalHandler()
				if handler != nil {
					handler(info)
				}
			}
		}()
		return fn()
	})
}

// Must panics if err is not nil.
// Useful for initialization code that should fail fast on error.
// Unlike regular panic(), this uses the recovery system.
//
// Example:
//
//	recovery.Must(os.MkdirAll("/tmp/data", 0755))
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// MustValue returns v if err is nil, otherwise panics with err.
// Useful for function calls that return a value and an error.
//
// Example:
//
//	data := recovery.MustValue(os.ReadFile("config.json"))
func MustValue[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
