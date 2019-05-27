package main

import (
	"fmt"
	"os"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/httpserver"
	log "github.com/nofacedb/facedb/internal/logger"
)

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

	srcAddr := ""
	if (cfg.HTTPServerCFG.KeyPath != "") && (cfg.HTTPServerCFG.CrtPath != "") {
		srcAddr = "https://" + cfg.HTTPServerCFG.Name
	} else {
		srcAddr = "http://" + cfg.HTTPServerCFG.Name
	}

	var awIdx uint64
	frs := facerecognition.CreateScheduler(cfg.FaceRecognitionCFG, srcAddr, &awIdx)
	cps := controlpanels.CreateScheduler(cfg.ControlPanelsCFG, srcAddr, &awIdx)

	fs, err := facedb.CreateFaceStorage(&(cfg.FaceStorageCFG), logger)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

	serv, err := httpserver.CreateHTTPServer(cfg, frs, cps, fs, logger)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

	serv.Run()
}
