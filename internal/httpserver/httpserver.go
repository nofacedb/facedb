package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
)

// HTTPServer struct contains all server configuration and work.
type HTTPServer struct {
	rest    *restAPI
	serv    *http.Server
	keyPath string
	crtPath string
}

// CreateHTTPServer creates new HTTPServer.
func CreateHTTPServer(cfg *cfgparser.CFG) (*HTTPServer, error) {
	rest, err := createRestAPI(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create rest API")
	}
	return &HTTPServer{
		rest: rest,
		serv: &http.Server{
			Addr:         cfg.HTTPServerCFG.Addr,
			Handler:      rest.bindHandlers(),
			WriteTimeout: time.Millisecond * time.Duration(cfg.HTTPServerCFG.WriteTimeoutMS),
			ReadTimeout:  time.Millisecond * time.Duration(cfg.HTTPServerCFG.ReadTimeoutMS),
		},
		keyPath: cfg.HTTPServerCFG.KeyPath,
		crtPath: cfg.HTTPServerCFG.CrtPath,
	}, nil
}

// Run starts HTTPServer.
func (s *HTTPServer) Run() {
	go func() {
		if (s.keyPath == "") && (s.crtPath == "") {
			if err := s.serv.ListenAndServe(); err != http.ErrServerClosed {
				fmt.Println("error", err)
			}
		} else {
			if err := s.serv.ListenAndServeTLS(
				s.keyPath,
				s.crtPath); err != http.ErrServerClosed {
				fmt.Println("error", err)
			}
		}
	}()

	s.gracefulShutdown()
}

func (s *HTTPServer) gracefulShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(5))
	defer cancel()
	if err := s.serv.Shutdown(ctx); err != nil {
		fmt.Println("error")
	} else {
		fmt.Println("hooray")
	}
}
