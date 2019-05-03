package httpserver

import (
	"net/http"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facerecognition"
)

type restAPI struct {
	immedResp bool

	frs *facerecognition.Scheduler
	cps *controlpanels.Scheduler
}

func createRestAPI(cfg *cfgparser.CFG) (*restAPI, error) {
	frs := facerecognition.CreateScheduler(cfg.FaceRecognitionCFG)
	cps := controlpanels.CreateScheduler(cfg.ControlPanelsCFG)
	return &restAPI{
		immedResp: cfg.HTTPServerCFG.ImmedResp,
		frs:       frs,
		cps:       cps,
	}, nil
}

func (rest *restAPI) bindHandlers() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", rest.notificationHandler)

	return mux
}
