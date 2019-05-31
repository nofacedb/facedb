package httpserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"

	"github.com/nofacedb/facedb/internal/proto"
	"github.com/nofacedb/facedb/internal/schedulers"
	"github.com/nofacedb/facedb/internal/storages"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

func validatePutControlReq(req *http.Request) (*proto.PutControlReq, *proto.ErrorData) {
	if req.Method != httpPutMethod {
		return nil, &proto.ErrorData{
			Code: proto.InvalidRequestMethodCode,
			Info: "invalid request method",
			Text: fmt.Sprintf("expected \"%s\", got \"%s\"",
				httpPutMethod, req.Method),
		}
	}

	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, &proto.ErrorData{
			Code: proto.CorruptedBodyCode,
			Info: "corrupted request body",
			Text: err.Error(),
		}
	}
	putControlReq := &proto.PutControlReq{}
	if err := json.Unmarshal(data, putControlReq); err != nil {
		return nil, &proto.ErrorData{
			Code: proto.CorruptedBodyCode,
			Info: "corrupted request body",
			Text: err.Error(),
		}
	}
	return putControlReq, nil
}

func (rest *restAPI) putControlHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Infof("got request on \"%s\"", apiPutControl)
	putControlReq, errorData := validatePutControlReq(req)
	if errorData != nil {
		rest.logger.Warnf("unable to process request: [%d] (\"%s\")",
			errorData.Code, errorData.Text)
		resp.WriteHeader(http.StatusBadRequest)
		e := &proto.ImmedResp{
			Header: proto.Header{
				SrcAddr: rest.srcAddr,
			},
			ErrorData: errorData,
		}
		re, _ := json.Marshal(e)
		resp.Write(re)
		return
	}

	rest.logger.Debugf("SrcAddr: \"%s\", UUID: \"%s\"",
		putControlReq.Header.SrcAddr,
		putControlReq.Header.UUID)

	go rest.processPutControlReq(putControlReq)

	resp.WriteHeader(http.StatusOK)
	e := &proto.ImmedResp{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    putControlReq.Header.UUID,
		},
	}
	re, _ := json.Marshal(e)
	resp.Write(re)
}

func (rest *restAPI) processPutControlReq(putControlReq *proto.PutControlReq) {
	k := putControlReq.Header.UUID
	awControl := rest.cpScheduler.ACQ.Pop(k)
	if awControl == nil {
		return
	}

	switch putControlReq.Command {
	case proto.CancelCommand:
		processPutControlReqOnCancelCommand(rest, awControl, putControlReq)
	case proto.ProcessAgainCommand:
		processPutControlReqOnProcessAgainCommand(rest, awControl, putControlReq)
	case proto.SubmitCommand:
		processPutControlReqOnSubmitCommand(rest, awControl, putControlReq)
	}
}

func processPutControlReqOnCancelCommand(
	rest *restAPI,
	awControl *schedulers.AwaitingControl,
	putControlReq *proto.PutControlReq) {
	rest.logger.Debugf("cancelling request for image: %s\n", putControlReq.Header.UUID)
}

func processPutControlReqOnProcessAgainCommand(
	rest *restAPI,
	awControl *schedulers.AwaitingControl,
	putControlReq *proto.PutControlReq) {
	k := awControl.UUID
	v := &schedulers.AwaitingImage{
		TS:        time.Now(),
		SrcAddr:   putControlReq.Header.SrcAddr,
		UUID:      k,
		ImgBuff:   awControl.ImgBuff,
		FaceBoxes: make([]proto.FaceBox, 0, len(putControlReq.ImageControlObjects)),
	}
	for _, imgCob := range putControlReq.ImageControlObjects {
		v.FaceBoxes = append(v.FaceBoxes, imgCob.FaceBox)
	}

	if err := rest.frScheduler.AwImgsQ.Push(k, v); err != nil {
		err = errors.Wrapf(err, "unable to push \"PutImageReq\" with UUID \"%s\"to queue", k)
		rest.logger.Warnf(err.Error())
		// TODO.
		return
	}
	rest.logger.Debugf("successfully pushed \"PutImageReq\" with UUID \"%s\" to queue", k)

	processImageReq := &proto.ProcessImageReq{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    k,
		},
		ImgBuff:   awControl.ImgBuff,
		FaceBoxes: v.FaceBoxes,
	}
	if err := rest.frScheduler.SendProcessImageReq(processImageReq); err != nil {
		rest.logger.Error(err)
		rest.frScheduler.AwImgsQ.Pop(k)
		// TODO.
		return
	}
	rest.logger.Debugf("successfully sent \"ProcessImageReq\" with UUID \"%s\" to facerecognizer", k)
}

func isExistingFaceBox(facebox proto.FaceBox, awControl *schedulers.AwaitingControl) int {
	for i, imgCob := range awControl.ImageControlObjects {
		if reflect.DeepEqual(facebox, imgCob.FaceBox) {
			return i
		}
	}
	return -1
}

func processPutControlReqOnSubmitCommand(
	rest *restAPI,
	awControl *schedulers.AwaitingControl,
	putControlReq *proto.PutControlReq) {

	ffvsToInsert := make([]proto.FacialFeaturesVector, 0)
	fbsToInsert := make([]proto.FaceBox, 0)
	cobsToInsert := make([]proto.ControlObject, 0)
	shouldInsert := make([]bool, 0)

	for _, imgCob := range putControlReq.ImageControlObjects {
		idx := isExistingFaceBox(imgCob.FaceBox, awControl)
		if idx == -1 {
			continue
		}

		ffvsToInsert = append(ffvsToInsert, awControl.FacialFeaturesVectors[idx])
		fbsToInsert = append(fbsToInsert, imgCob.FaceBox)
		cobsToInsert = append(cobsToInsert, imgCob.ControlObject)
		if (&awControl.ImageControlObjects[idx].ControlObject).Compare(&(imgCob.ControlObject)) {
			shouldInsert = append(shouldInsert, false)
		} else {
			shouldInsert = append(shouldInsert, true)
		}
	}

	faceIDs := make([]string, 0, len(ffvsToInsert))

	// Inserting new ControlObjects.
	for i, cob := range cobsToInsert {
		if !shouldInsert[i] {
			faceIDs = append(faceIDs, cob.ID)
			continue
		}
		dbCob, err := rest.fStorage.SelectControlObjectByPassport(cob.Passport)
		if err != nil {
			rest.logger.Error(errors.Wrap(err, "unable to select control object by passport; partial commit possible"))
			return
		}
		if dbCob.ID != proto.DefaultStringField {
			faceIDs = append(faceIDs, dbCob.ID)
			continue
		}
		cob.ID = uuid.Must(uuid.NewV4()).String()
		if err = rest.fStorage.InsertControlObjects([]proto.ControlObject{cob}); err != nil {
			rest.logger.Error(errors.Wrap(err, "unable to insert control object; partial commit possible"))
			return
		}
		faceIDs = append(faceIDs, cob.ID)
	}

	// Inserting new image.
	img := storages.Img{
		ID:      uuid.Must(uuid.NewV4()).String(),
		TS:      time.Now(),
		Path:    rest.imgPath + "/" + awControl.UUID + ".jpg",
		FaceIDs: faceIDs,
	}
	if err := rest.fStorage.InsertImgs([]storages.Img{img}); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to insert image; partial commit possible"))
		return
	}

	// Inserting new facial features vectors.
	ffvs := make([]storages.FFV, 0, len(ffvsToInsert))
	for i, ffv := range ffvsToInsert {
		ffvs = append(ffvs, storages.FFV{
			ID:                   uuid.Must(uuid.NewV4()).String(),
			CobID:                faceIDs[i],
			ImgID:                img.ID,
			FaceBox:              fbsToInsert[i],
			FacialFeaturesVector: ffv,
		})
	}
	if err := rest.fStorage.InsertFFVs(ffvs); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to insert ffvs; partial commit possible"))
		return
	}

	rest.logger.Debugf("successfully inserted image with UUID \"%s\" to DB", awControl.UUID)
}
