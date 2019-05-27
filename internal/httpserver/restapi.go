package httpserver

import (
	"net/http"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/facerecognition"
	log "github.com/sirupsen/logrus"
)

type restAPI struct {
	immedResp bool
	srcAddr   string
	frs       *facerecognition.Scheduler
	cps       *controlpanels.Scheduler
	fs        *facedb.FaceStorage
	logger    *log.Logger
}

func createRestAPI(cfg *cfgparser.CFG,
	frs *facerecognition.Scheduler,
	cps *controlpanels.Scheduler,
	fs *facedb.FaceStorage,
	logger *log.Logger) (*restAPI, error) {
	return &restAPI{
		immedResp: cfg.HTTPServerCFG.ImmedResp,
		frs:       frs,
		cps:       cps,
		fs:        fs,
		logger:    logger,
	}, nil
}

func (rest *restAPI) bindHandlers() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(apiV1PutImg, rest.putImgHandler)
	mux.HandleFunc(apiV1PutFBs, rest.putFBsHandler)
	mux.HandleFunc(apiV1PutFFs, rest.putFFsHandler)
	mux.HandleFunc(apiV1PutControl, rest.putControlHandler)
	mux.HandleFunc(apiV1AddFace, rest.addFaceHandler)

	return mux
}
