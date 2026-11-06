package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"
)

func startPprof(addr string) (net.Addr, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	server := &http.Server{}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("pprof server stopped", "error", err)
		}
	}()

	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Warn("stop pprof server", "error", err)
		}
	}

	return listener.Addr(), stop, nil
}
