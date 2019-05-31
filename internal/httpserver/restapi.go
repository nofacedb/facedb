package httpserver

import (
	"net/http"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/schedulers"
	"github.com/nofacedb/facedb/internal/storages"
	log "github.com/sirupsen/logrus"
)

const (
	apiBase             = `/api/v1`
	apiPutImage         = apiBase + `/put_image`
	apiPutFacesData     = apiBase + `/put_faces_data`
	apiPutControl       = apiBase + `/put_control`
	apiAddControlObject = apiBase + `/add_control_object`
)

type restAPI struct {
	srcAddr     string
	imgPath     string
	frScheduler *schedulers.FaceRecognitionScheduler
	cpScheduler *schedulers.ControlPanelScheduler
	fStorage    *storages.FaceStorage
	client      *http.Client
	logger      *log.Logger
}

func createRestAPI(cfg *cfgparser.CFG,
	srcAddr, imgPath string,
	frScheduler *schedulers.FaceRecognitionScheduler,
	cpScheduler *schedulers.ControlPanelScheduler,
	fStorage *storages.FaceStorage,
	client *http.Client, logger *log.Logger) *restAPI {
	return &restAPI{
		srcAddr:     srcAddr,
		imgPath:     imgPath,
		frScheduler: frScheduler,
		cpScheduler: cpScheduler,
		fStorage:    fStorage,
		client:      client,
		logger:      logger,
	}
}

func (rest *restAPI) bindHandlers() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(apiPutImage, rest.putImageHandler)
	mux.HandleFunc(apiPutFacesData, rest.putFacesDataReqHandler)
	mux.HandleFunc(apiPutControl, rest.putControlHandler)
	mux.HandleFunc(apiAddControlObject, rest.addControlObjectHandler)

	return mux
}
