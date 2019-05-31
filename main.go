package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/httpserver"
	log "github.com/nofacedb/facedb/internal/logger"
	"github.com/nofacedb/facedb/internal/schedulers"
	"github.com/nofacedb/facedb/internal/storages"
	"github.com/nofacedb/facedb/internal/version"
)

func createSrcAddr(cfg *cfgparser.CFG) string {
	if cfg.HTTPServerCFG.KeyPath != "" && cfg.HTTPServerCFG.CrtPath != "" {
		return fmt.Sprintf(
			"https://%s:%d",
			cfg.HTTPServerCFG.Addr,
			cfg.HTTPServerCFG.Port)
	}
	return fmt.Sprintf(
		"http://%s:%d",
		cfg.HTTPServerCFG.Addr,
		cfg.HTTPServerCFG.Port)

}

func main() {
	cfg, err := cfgparser.GetCFG()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger, err := log.CreateLogger(&(cfg.LoggerCFG))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger.Debugf("FACEDB (%s) server was started...", version.Version)

	logger.Debug("connecting to CLICKHOUSE DB...")
	db, err := storages.CreateClickHouseDBConn(&(cfg.StorageCFG), logger)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}
	logger.Debug("successfully connected to CLICKHOUSE DB")
	defer db.Close()

	logger.Debug("initializing FACE STORAGE...")
	fStorage := storages.CreateFaceStorage(db, cfg.StorageCFG.SineBoundary)
	logger.Debug("FACE STORAGE was successfully initialized")

	srcAddr := createSrcAddr(cfg)

	logger.Debug("initializing HTTP CLIENT...")
	client := &http.Client{
		Timeout: time.Millisecond *
			time.Duration(cfg.HTTPClientCFG.TimeoutMS),
	}
	logger.Debug("HTTP CLIENT was successfully initialized")

	logger.Debug("initializing FACE RECOGNIZERS SCHEDULER...")
	frScheduler := schedulers.CreateFaceRecognitionScheduler(&(cfg.FaceRecognizersCFG), srcAddr, client, logger)
	logger.Debug("FACE RECOGNIZERS SCHEDULER was successfully initialized")

	logger.Debug("initializing CONTROL PANELS SCHEDULER...")
	cpScheduler := schedulers.CreateControlPanelScheduler(&(cfg.ControlPanelsCFG), srcAddr, client, logger)
	logger.Debug("CONTROL PANELS SCHEDULER was successfully initialized")

	logger.Debug("initializing HTTP SERVER...")
	server := httpserver.CreateHTTPServer(
		cfg, srcAddr,
		frScheduler, cpScheduler,
		fStorage, client, logger)
	logger.Debug("HTTP SERVER was successfully initialized")

	server.Run()

	logger.Debugf("FACEDB (%s) server was stopped...", version.Version)
}
