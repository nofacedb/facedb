package httpserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

const apiV1PutControl = "/api/v1/put_control"

type imgControlDoneReq struct {
	Headers   proto.Headers            `json:"headers"`
	Cmd       string                   `json:"cmd"`
	ID        uint64                   `json:"id"`
	FacesData []controlpanels.FaceData `json:"faces_data"`
}

func (rest *restAPI) putControlHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Debugf("got \"%s\" message", apiV1PutControl)
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

	imgControlDoneReq := &imgControlDoneReq{}
	if err := json.Unmarshal(buff, imgControlDoneReq); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to unmarshal request body JSON"))
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	rest.logger.Debugf("source: \"%s\"", imgControlDoneReq.Headers.SrcAddr)

	if rest.immedResp {
		go func() {
			if err := processControlHandlerRequest(rest, imgControlDoneReq); err != nil {
				rest.logger.Error(errors.Wrap(err, "unable to process put control request"))
			}
		}()
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processControlHandlerRequest(rest, imgControlDoneReq); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to process put control request"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

func processControlHandlerRequest(rest *restAPI, imgControlDoneReq *imgControlDoneReq) error {
	awVal, err := rest.cps.PopAwaitingImage(controlpanels.AwaitingKey{
		SrcAddr: imgControlDoneReq.Headers.SrcAddr,
		ID:      imgControlDoneReq.ID,
	})
	if err != nil {
		return err
	}

	switch imgControlDoneReq.Cmd {
	case "submit":
		if err := controlHandlerOnSubmit(rest, awVal, imgControlDoneReq); err != nil {
			return errors.Wrap(err, "unable to process submit request")
		}
	case "recognize_again":
		if err := controlHandlerOnRecognizeAgain(rest, awVal, imgControlDoneReq); err != nil {
			return errors.Wrap(err, "unable to process rec_again request")
		}
	case "cancel":
		if err := controlHandlerOnCancel(rest, awVal, imgControlDoneReq); err != nil {
			return errors.Wrap(err, "unable to process cancel request")
		}
	default:
		return fmt.Errorf("unknown type of request: \"%s\"", imgControlDoneReq.Cmd)
	}

	return nil
}

func controlHandlerOnSubmit(rest *restAPI, awVal *controlpanels.AwaitingImgVal, req *imgControlDoneReq) error {
	for _, reqFaceData := range req.FacesData {
		idx := -1
		for i, awFaceData := range awVal.FacesData {
			if reflect.DeepEqual(reqFaceData.Box, awFaceData.Box) {
				idx = i
				break
			}
		}
		if idx == -1 {
			rest.logger.Debug("skipping another one face")
			continue
		}
		rest.logger.Debug("pushing another one face to DB")
		awFaceData := awVal.FacesData[idx]
		if facedb.CmpCOBsByAll(reqFaceData.COB, awFaceData.COB) {
			ff := facedb.FF{
				COBID: *(awFaceData.COB.ID),
				IMGID: "00000000-0000-0000-0000-000000000000",
				Box:   reqFaceData.Box,
				FF:    awVal.FacialFeatures[idx],
			}
			if err := rest.fs.InsertFF([]facedb.FF{ff}); err != nil {
				return errors.Wrap(err, "unable to insert facial features vector")
			}
			continue
		}
		if *(awFaceData.COB.ID) == facedb.UNKNOWNFIELD {
			reqFaceData.COB.TS = new(time.Time)
			*(reqFaceData.COB.TS) = time.Now()
			cob, err := rest.fs.SelectCOBByPassport(reqFaceData.COB.Passport)
			if err != nil {
				return errors.Wrap(err, "unable to get inserted control object UUID")
			}
			ID := *(cob.ID)
			if *(cob.ID) == facedb.UNKNOWNFIELD {
				if err = rest.fs.InsertCOB([]facedb.COB{reqFaceData.COB}); err != nil {
					return errors.Wrap(err, "unable to insert new control object to database")
				}
				cob, err = rest.fs.SelectCOBByPassport(reqFaceData.COB.Passport)
				if err != nil {
					return errors.Wrap(err, "unable to get inserted control object UUID")
				}
				ID = *(cob.ID)
			}
			ff := facedb.FF{
				COBID: ID,
				IMGID: "00000000-0000-0000-0000-000000000000",
				Box:   reqFaceData.Box,
				FF:    awVal.FacialFeatures[idx],
			}
			if err := rest.fs.InsertFF([]facedb.FF{ff}); err != nil {
				return errors.Wrap(err, "unable to insert facial features vector")
			}
		}
	}

	return nil
}

func controlHandlerOnRecognizeAgain(rest *restAPI, awVal *controlpanels.AwaitingImgVal, req *imgControlDoneReq) error {
	fmt.Println("TODO")
	return nil
}

func controlHandlerOnCancel(rest *restAPI, awVal *controlpanels.AwaitingImgVal, req *imgControlDoneReq) error {
	rest.logger.Debug("cancelling img request")
	return nil
}
