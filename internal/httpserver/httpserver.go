package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/schedulers"
	"github.com/nofacedb/facedb/internal/storages"
	"github.com/pkg/errors"
)

const (
	httpGetMethod  = "GET"
	httpPostMethod = "POST"
	httpPutMethod  = "PUT"
)

// HTTPServer struct contains all server configuration and work.
type HTTPServer struct {
	rest    *restAPI
	serv    *http.Server
	logger  *log.Logger
	keyPath string
	crtPath string
}

// CreateHTTPServer creates new HTTPServer.
func CreateHTTPServer(
	cfg *cfgparser.CFG, srcAddr string,
	frScheduler *schedulers.FaceRecognitionScheduler,
	cpScheduler *schedulers.ControlPanelScheduler,
	fStorage *storages.FaceStorage,
	client *http.Client, logger *log.Logger) *HTTPServer {
	rest := createRestAPI(
		cfg, srcAddr, cfg.StorageCFG.ImgPath,
		frScheduler, cpScheduler, fStorage,
		client, logger)
	return &HTTPServer{
		rest: rest,
		serv: &http.Server{
			Addr: fmt.Sprintf("%s:%d",
				cfg.HTTPServerCFG.Addr,
				cfg.HTTPServerCFG.Port),
			Handler: rest.bindHandlers(),
			WriteTimeout: time.Millisecond *
				time.Duration(cfg.HTTPServerCFG.WriteTimeoutMS),
			ReadTimeout: time.Millisecond *
				time.Duration(cfg.HTTPServerCFG.ReadTimeoutMS),
		},
		logger:  logger,
		keyPath: cfg.HTTPServerCFG.KeyPath,
		crtPath: cfg.HTTPServerCFG.CrtPath,
	}
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
		s.logger.Error(errors.Wrap(err, "unable to shutdown server successfully"))
	} else {
		s.logger.Info("server was shutdowned successfully")
	}
}
