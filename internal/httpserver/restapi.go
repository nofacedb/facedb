package httpserver

import (
	"net/http"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/pkg/errors"
)

type restAPI struct {
	immedResp bool

	frs *facerecognition.Scheduler
	cps *controlpanels.Scheduler
	fs  *facedb.FaceStorage
}

func createRestAPI(cfg *cfgparser.CFG) (*restAPI, error) {
	frs := facerecognition.CreateScheduler(cfg.FaceRecognitionCFG)
	cps := controlpanels.CreateScheduler(cfg.ControlPanelsCFG)
	fs, err := facedb.CreateFaceStorage(cfg.FaceStorageCFG)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to facedb")
	}
	return &restAPI{
		immedResp: cfg.HTTPServerCFG.ImmedResp,
		frs:       frs,
		cps:       cps,
		fs:        fs,
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
