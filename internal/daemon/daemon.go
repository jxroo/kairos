package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/server"
)

type Daemon struct {
	cfg    *config.Config
	srv    *server.Server
	logger *zap.Logger
	addr   string
}

func New(cfg *config.Config, srv *server.Server, logger *zap.Logger) *Daemon {
	return &Daemon{cfg: cfg, srv: srv, logger: logger}
}

func (d *Daemon) Run(ctx context.Context) error {
	listenAddr := fmt.Sprintf("%s:%d", d.cfg.Server.Host, d.cfg.Server.Port)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", listenAddr, err)
	}
	d.addr = ln.Addr().String()
	d.logger.Info("daemon started", zap.String("addr", d.addr))

	httpSrv := &http.Server{Handler: d.srv.Handler()}

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
		d.logger.Info("shutting down daemon")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

func (d *Daemon) Addr() string {
	return d.addr
}
