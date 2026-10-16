package server_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/server"
	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
)

func TestRunStopsGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := server.New("127.0.0.1:0", http.NotFoundHandler(), server.Options{ReadHeaderTimeout: time.Second}, logging.Discard())

	drained := make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		errc <- server.Run(ctx, logging.Discard(), server.Shutdown{
			Timeout: time.Second,
			OnDrain: func() { close(drained) },
		}, srv)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	select {
	case <-drained:
	default:
		t.Fatal("OnDrain not called")
	}
}

func TestRunFailsOnBusyPortWithoutLeakingServers(t *testing.T) {
	var lc net.ListenConfig
	busy, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	first := server.New("127.0.0.1:0", http.NotFoundHandler(), server.Options{}, logging.Discard())
	second := server.New(busy.Addr().String(), http.NotFoundHandler(), server.Options{}, logging.Discard())
	if err := server.Run(context.Background(), logging.Discard(), server.Shutdown{}, first, second); err == nil {
		t.Fatal("expected a listen error")
	}
}
