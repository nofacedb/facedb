package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/nofacedb/facedb/internal/proto"
	"github.com/nofacedb/facedb/internal/schedulers"
	"github.com/nofacedb/facedb/internal/storages"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

func validatePutFacesDataReq(req *http.Request) (*proto.PutFacesDataReq, *proto.ErrorData) {
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

	putFacesdataReq := &proto.PutFacesDataReq{}
	if err := json.Unmarshal(data, putFacesdataReq); err != nil {
		return nil, &proto.ErrorData{
			Code: proto.CorruptedBodyCode,
			Info: "corrupted request body",
			Text: err.Error(),
		}
	}

	return putFacesdataReq, nil
}

func (rest *restAPI) putFacesDataReqHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Infof("got request on \"%s\"", apiPutFacesData)
	putFacesDataReq, errorData := validatePutFacesDataReq(req)
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
		putFacesDataReq.Header.SrcAddr,
		putFacesDataReq.Header.UUID)

	k := putFacesDataReq.Header.UUID
	if putFacesDataReq.ErrorData != nil {
		rest.logger.Warnf("\"%s\" couldn't process image with UUID \"%s\": [%d] %s; dropping image",
			putFacesDataReq.Header.SrcAddr, k,
			putFacesDataReq.ErrorData.Code, putFacesDataReq.ErrorData.Text)
		rest.frScheduler.AwImgsQ.Pop(k)
	} else {
		go rest.processPutFacesDataReq(putFacesDataReq)
	}

	resp.WriteHeader(http.StatusOK)
	e := &proto.ImmedResp{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    k,
		},
	}
	re, _ := json.Marshal(e)
	resp.Write(re)
}

func (rest *restAPI) processPutFacesDataReq(putFacesDataReq *proto.PutFacesDataReq) {
	k := putFacesDataReq.Header.UUID
	awImg := rest.frScheduler.AwImgsQ.Pop(k)
	if awImg != nil {
		rest.logger.Debugf("successfully poped \"PutImageReq\" with UUID \"%s\" from queue", awImg.UUID)
		processFacesDataReqOnAwImg(rest, awImg, putFacesDataReq)
		return
	}
	awCob := rest.cpScheduler.ACOQ.GetAwaitingCobByImgID(k)
	if awCob != nil {
		rest.logger.Debugf("successfully got \"AwaitingControlObject\" with UUID \"%s\" from queue", awCob.UUID)
		processFacesDataReqOnAwCob(rest, awCob, putFacesDataReq)
		return
	}
	rest.logger.Warnf("got unknown \"FacesData\" with UUID \"%s\" from queue", putFacesDataReq.Header.UUID)
}

func processFacesDataReqOnAwImg(rest *restAPI, awImg *schedulers.AwaitingImage, putFacesDataReq *proto.PutFacesDataReq) {
	awImg.FaceBoxes = make([]proto.FaceBox, 0, len(putFacesDataReq.FacesData))
	awImg.FacialFeaturesVectors = make([]proto.FacialFeaturesVector, 0, len(putFacesDataReq.FacesData))
	for _, facesdata := range putFacesDataReq.FacesData {
		awImg.FaceBoxes = append(awImg.FaceBoxes, facesdata.FaceBox)
		awImg.FacialFeaturesVectors = append(awImg.FacialFeaturesVectors, facesdata.FacialFeaturesVector)
	}

	cobs := make([]proto.ControlObject, 0, len(awImg.FaceBoxes))
	for i := 0; i < len(awImg.FaceBoxes); i++ {
		defaultCob := proto.CreateDefaultControlObject()
		cobs = append(cobs, *defaultCob)
	}
	for i, ffv := range awImg.FacialFeaturesVectors {
		cob, err := rest.fStorage.SelectControlObjectByFFV(ffv)
		if err != nil {
			rest.logger.Warn(errors.Wrapf(err,
				"unable to retrieve data for %d-th face on image with UUID \"%s\"",
				i, awImg.UUID))
			continue
		}
		cobs[i] = *cob
	}

	if rest.cpScheduler.GetControlPanelsNum() == 0 {
		rest.logger.Debug("no controlpanels are available, so all data will be pushed to DB immediately")
		processFacesDataReqOnAwImgImmedToDB(rest, awImg, cobs)
		return
	}
	processFacesDataReqOnAwImgDeferred(rest, awImg, cobs)
}

func processFacesDataReqOnAwImgImmedToDB(rest *restAPI, awImg *schedulers.AwaitingImage, cobs []proto.ControlObject) {

}

func processFacesDataReqOnAwImgDeferred(rest *restAPI, awImg *schedulers.AwaitingImage, cobs []proto.ControlObject) {
	notifyControlReq := &proto.NotifyControlReq{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    awImg.UUID,
		},
		ImgBuff:             awImg.ImgBuff,
		ImageControlObjects: make([]proto.ImageControlObject, 0, len(awImg.FaceBoxes)),
	}
	for i := 0; i < len(awImg.FaceBoxes); i++ {
		ico := proto.ImageControlObject{
			ControlObject: cobs[i],
			FaceBox:       awImg.FaceBoxes[i],
		}
		notifyControlReq.ImageControlObjects = append(notifyControlReq.ImageControlObjects, ico)
	}

	k := awImg.UUID
	v := &schedulers.AwaitingControl{
		TS:                    time.Now(),
		SrcAddr:               notifyControlReq.Header.SrcAddr,
		UUID:                  notifyControlReq.Header.UUID,
		ImgBuff:               awImg.ImgBuff,
		ImageControlObjects:   notifyControlReq.ImageControlObjects,
		FacialFeaturesVectors: awImg.FacialFeaturesVectors,
	}
	if err := rest.cpScheduler.ACQ.Push(k, v); err != nil {
		rest.logger.Error(errors.Wrapf(err, "unable to insert awaiting control for image \"%s\"", k))
		return
	}
	rest.logger.Debugf("successfully pushed \"NotifyControlReq\" with UUID \"%s\" to queue", awImg.UUID)

	if err := rest.cpScheduler.SendNotifyControlReq(notifyControlReq, true, ""); err != nil {
		rest.logger.Error(err)
		rest.cpScheduler.ACQ.Pop(k)
		return
	}
	rest.logger.Debugf("successfully pushed \"NotifyControlReq\" with UUID \"%s\" to controlpanel", awImg.UUID)
}

func processFacesDataReqOnAwCob(rest *restAPI, awCob *schedulers.AwaitingControlObject, putFacesDataReq *proto.PutFacesDataReq) {
	awCob.Mu.Lock()
	rest.logger.Debugf("got facial features for another one image for \"AwaitingControlObject\" with UUID \"%s\"", awCob.UUID)
	k := putFacesDataReq.Header.UUID
	if len(putFacesDataReq.FacesData) == 0 {
		delete(awCob.Images, k)
		awCob.ControlObjectPart.ImagesNum--
		rest.logger.Warnf("facerecognizer \"%s\" didn't found faces on image with key \"%s\"",
			putFacesDataReq.Header.SrcAddr, k)
	} else {
		awCob.FacesData[k] = putFacesDataReq.FacesData[0]
	}
	if len(awCob.FacesData) != int(awCob.ControlObjectPart.ImagesNum) {
		awCob.Mu.Unlock()
		return
	}
	awCob.Mu.Unlock()
	rest.logger.Debugf("got all facial features for \"AwaitingControlObject\" with UUID \"%s\"", awCob.UUID)

	// Inserting new ControlObject.
	cob := awCob.ControlObjectPart.ControlObject
	dbCob, err := rest.fStorage.SelectControlObjectByPassport(cob.Passport)
	if err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to select control object by passport; partial commit possible"))
		return
	}
	if dbCob.ID == proto.DefaultStringField {
		cob.ID = uuid.Must(uuid.NewV4()).String()
		if err = rest.fStorage.InsertControlObjects([]proto.ControlObject{cob}); err != nil {
			rest.logger.Error(errors.Wrap(err, "unable to insert control object; partial commit possible"))
		}
	} else {
		cob.ID = dbCob.ID
	}

	ffvs := make([]storages.FFV, 0, len(awCob.FacesData))
	imgs := make([]storages.Img, 0, len(awCob.FacesData))
	for _, v := range awCob.FacesData {
		UUID := uuid.Must(uuid.NewV4()).String()
		img := storages.Img{
			ID:      UUID,
			TS:      time.Now(),
			Path:    rest.imgPath + "/" + UUID + ".jpg",
			FaceIDs: []string{cob.ID},
		}
		imgs = append(imgs, img)
		ffv := storages.FFV{
			ID:                   uuid.Must(uuid.NewV4()).String(),
			CobID:                cob.ID,
			ImgID:                img.ID,
			FaceBox:              v.FaceBox,
			FacialFeaturesVector: v.FacialFeaturesVector,
		}
		ffvs = append(ffvs, ffv)
	}

	if err = rest.fStorage.InsertImgs(imgs); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to insert images; partial commit possible"))
		return
	}

	if err = rest.fStorage.InsertFFVs(ffvs); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to insert ffvs; partial commit possible"))
		return
	}

	rest.logger.Debugf("pushed \"AwaitingControlObject\" with UUID \"%s\" to ClickHouse DB", awCob.UUID)

	req := proto.ImmedResp{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    awCob.UUID,
		},
	}
	url := awCob.SrcAddr + "/api/v1/notify_add_control_object"
	data, err := json.Marshal(req)
	if err != nil {
		rest.logger.Warn(errors.Wrap(err, "unable to marshal \"NotifyAddControlObjectReq\" to JSON"))
	}
	httpReq, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to create \"NotifyAddControlObjectReq\" HTTP request"))
		return
	}
	_, err = rest.client.Do(httpReq)
	if err != nil {
		rest.logger.Error(errors.Wrapf(err, "unable to send \"NotifyAddControlObjectReq\" to controlpanel \"%s\"", url))
	}

	rest.logger.Debugf("notified controlpanel \"%s\" about inserting UUID \"%s\" to ClickHouse DB",
		rest.srcAddr, awCob.UUID)
}
