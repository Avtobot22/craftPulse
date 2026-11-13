package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/atlassiancheck"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	configFile := flag.String("config", "", "private runtime configuration JSON")
	flag.Parse()
	c, err := atlassiancheck.LoadConfig(*configFile)
	if err != nil {
		return err
	}
	if flag.NArg() == 0 {
		return errors.New("command required: serve, ticket ROLE, status, check ROLE, report, erase ROLE")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if flag.Arg(0) != "serve" {
		return control(ctx, c, flag.Arg(0), flag.Arg(1))
	}
	stand, err := atlassiancheck.New(c)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: c.Listen, Handler: stand.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		stand.RunReports(ctx, func(err error) { log.Printf("privacy reporting: %v", err) })
	}()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err = <-done:
		stop()
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		err = server.Shutdown(shutdown)
	}
	<-workerDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func control(ctx context.Context, c atlassiancheck.Config, command, role string) error {
	target := "http://" + c.Listen + "/control/" + url.PathEscape(command) + "?role=" + url.QueryEscape(role)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("construct control request: %w", err)
	}
	r.Header.Set("Authorization", "Bearer "+c.ControlSecret)
	client := &http.Client{
		Timeout:       95 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(r)
	if err != nil {
		return errors.New("stand control request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("stand control returned HTTP %d", response.StatusCode)
	}
	if _, err = io.Copy(os.Stdout, io.LimitReader(response.Body, 64<<10)); err != nil {
		return fmt.Errorf("read control response: %w", err)
	}
	return nil
}
