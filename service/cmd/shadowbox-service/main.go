package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"shadowbox/service/internal/app"
	"shadowbox/service/internal/engine"
	"shadowbox/service/internal/platform"
	"shadowbox/service/internal/profile"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tun-worker" {
		fs := flag.NewFlagSet("tun-worker", flag.ExitOnError)
		p := fs.String("session", "", "private session prefix")
		_ = fs.Parse(os.Args[2:])
		if *p == "" {
			os.Exit(2)
		}
		if e := engine.RunTunWorker(*p); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	dir := flag.String("data-dir", platform.DataDir(), "configuration directory")
	version := flag.Bool("version", false, "print version")
	flag.Parse()
	if *version {
		fmt.Println("ShadowBox 0.1.0 / Neko sing-box " + engine.Version)
		return
	}
	if e := run(*dir); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	lock := flock.New(filepath.Join(dir, "service.lock"))
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("后台服务已在运行")
	}
	defer lock.Unlock()
	a, e := app.New(dir)
	if e != nil {
		return e
	}
	defer a.Close()
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer listener.Close()
	token := profile.NewID() + profile.NewID()
	discovery := map[string]any{"endpoint": "http://" + listener.Addr().String(), "token": token, "pid": os.Getpid(), "version": "0.1.0"}
	b, _ := json.Marshal(discovery)
	if e = os.WriteFile(filepath.Join(dir, "service.json"), b, 0600); e != nil {
		return e
	}
	server := http.Server{Handler: a.Handler(token), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Schedule(ctx)
	if a.Settings().AutoConnect {
		go func() { _ = a.Connect("") }()
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	select {
	case e = <-done:
		if e != http.ErrServerClosed {
			return e
		}
	case <-sig:
	case <-a.Shutdown:
	}
	cancel()
	ctxClose, cancelClose := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelClose()
	_ = server.Shutdown(ctxClose)
	return nil
}
