package httpserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

const apiV1PutFFs = "/api/v1/put_ffs"

type imgTaskDoneReq struct {
	Headers proto.Headers          `json:"headers"`
	ID      uint64                 `json:"id"`
	Faces   []facerecognition.Face `json:"faces"`
}

func (rest *restAPI) putFFsHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Debugf("got \"%s\" message", apiV1PutFFs)
	if req.Method != "PUT" {
		rest.logger.Error(fmt.Errorf("invalid request method: %s", req.Method))
		resp.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	buff, err := ioutil.ReadAll(req.Body)
	if err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to read request body"))
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	imgTaskDoneReq := &imgTaskDoneReq{}
	if err := json.Unmarshal(buff, imgTaskDoneReq); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to unmarshal request body JSON"))
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	rest.logger.Debugf("source: \"%s\"", imgTaskDoneReq.Headers.SrcAddr)

	imgBuff, err := rest.frs.PopAwaitingImage(facerecognition.AwaitingKey{
		SrcAddr: imgTaskDoneReq.Headers.SrcAddr,
		ID:      imgTaskDoneReq.ID,
	})
	if err != nil {
		key := rest.cps.FindAwaitingFaceKey(imgTaskDoneReq.ID)
		if key == nil {
			resp.WriteHeader(http.StatusBadRequest)
			return
		} else {
			if len(imgTaskDoneReq.Faces) == 0 {
				resp.WriteHeader(http.StatusBadRequest)
				return
			}
			rest.cps.PushAwaitingFFs(*key, imgTaskDoneReq.ID, imgTaskDoneReq.Faces[0])
			if ok, err := rest.cps.IsAwaitingFaceReady(*key); ok && err == nil {
				if rest.immedResp {
					go func() {
						if err := processPutFeaturesOnAddFaceEnd(rest, *key); err != nil {
							rest.logger.Error(errors.Wrap(err, "unable to process add_face features request"))
							resp.WriteHeader(http.StatusBadRequest)
							return
						}
					}()
					resp.WriteHeader(http.StatusOK)
					return
				}
				if err := processPutFeaturesOnAddFaceEnd(rest, *key); err != nil {
					rest.logger.Error(errors.Wrap(err, "unable to process add_face features request"))
					resp.WriteHeader(http.StatusBadRequest)
					return
				}
			}
			return
		}
	}

	if rest.immedResp {
		go func() {
			if err := processPutFeaturesRequest(rest, imgBuff, imgTaskDoneReq.Faces); err != nil {
				rest.logger.Error(errors.Wrap(err, "unable to process features request"))
				// TODO.
			}
		}()
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processPutFeaturesRequest(rest, imgBuff, imgTaskDoneReq.Faces); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to process features request"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

func processPutFeaturesOnAddFaceEnd(rest *restAPI, key controlpanels.AwaitingKey) error {
	awFace, err := rest.cps.PopAwaitingFace(key)
	if err != nil {
		return errors.Wrap(err, "unable to pop awaiting face")
	}
	for _, v := range awFace.AwaitingFaces {
		ff := facedb.FF{
			COBID: awFace.ID,
			IMGID: "00000000-0000-0000-0000-000000000000",
			Box:   v.Box,
			FF:    v.FacialFeatures,
		}
		if err := rest.fs.InsertFF([]facedb.FF{ff}); err != nil {
			return errors.Wrap(err, "unable to insert facial features vector")
		}
		continue
	}
	return nil
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
