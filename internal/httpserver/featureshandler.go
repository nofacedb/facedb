package httpserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

type imgTaskDoneReq struct {
	Headers proto.Headers          `json:"headers"`
	ID      uint64                 `json:"id"`
	Faces   []facerecognition.Face `json:"faces"`
}

func (rest *restAPI) putFeaturesHandler(resp http.ResponseWriter, req *http.Request) {
	if req.Method != "PUT" {
		resp.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	buff, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	imgTaskDoneReq := &imgTaskDoneReq{}
	if err := json.Unmarshal(buff, imgTaskDoneReq); err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	imgBuff := rest.frs.PopAwaitingImage(facerecognition.AwaitingKey{
		SrcAddr: imgTaskDoneReq.Headers.SrcAddr,
		ID:      imgTaskDoneReq.ID,
	})
	if imgBuff == nil {
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	if rest.immedResp {
		go processPutFeaturesRequest(rest, imgBuff, imgTaskDoneReq.Faces)
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processPutFeaturesRequest(rest, imgBuff, imgTaskDoneReq.Faces); err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusOK)

}

func processPutFeaturesRequest(rest *restAPI, imgBuff []byte, faces []facerecognition.Face) error {
	// Работа с БД.
	facesData := make([]controlpanels.FaceData, 0, len(faces))
	facialFeatures := make([][]float64, 0, len(faces))
	for _, face := range faces {
		cob, err := rest.fs.SelectCOBByFF(face.FacialFeatures)
		if err != nil {
			fmt.Println(err)
			return errors.Wrap(err, "unable to select control object by facial features vector from facedb")
		}
		facesData = append(facesData, controlpanels.FaceData{
			Box: face.Box,
			COB: cob,
		})
		facialFeatures = append(facialFeatures, face.FacialFeatures)
	}
	newFacesData, immed, err := rest.cps.Notify(imgBuff, facesData, facialFeatures)
	if err != nil {
		return errors.Wrap(err, "unable to send notification to control panel")
	}

	if !immed {
		fmt.Println(newFacesData)
	}

	return nil
}
