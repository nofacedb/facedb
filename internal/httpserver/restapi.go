package httpserver

import (
	"net/http"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/facerecognition"
)

type restAPI struct {
	immedResp bool
	frsch     *facerecognition.Scheduler
}

func createRestAPI(cfg *cfgparser.CFG) (*restAPI, error) {
	frsch := facerecognition.CreateScheduler(cfg.FaceRecognitionCFG)

	return &restAPI{
		immedResp: cfg.HTTPServerCFG.ImmedResp,
		frsch:     frsch,
	}, nil
}

func (rest *restAPI) bindHandlers() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", rest.notificationHandler)

	return mux
}
