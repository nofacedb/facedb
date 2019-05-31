package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/h2non/filetype"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/nofacedb/facedb/internal/schedulers"
	uuid "github.com/satori/go.uuid"
)

func validateAddControlObjectReq(req *http.Request) (*proto.AddControlObjectReq, *proto.ErrorData) {
	if req.Method != httpPostMethod {
		return nil, &proto.ErrorData{
			Code: proto.InvalidRequestMethodCode,
			Info: "invalid request method",
			Text: fmt.Sprintf("expected \"%s\", got \"%s\"",
				httpPostMethod, req.Method),
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

	addControlObjectReq := &proto.AddControlObjectReq{}
	if err := json.Unmarshal(data, addControlObjectReq); err != nil {
		return nil, &proto.ErrorData{
			Code: proto.CorruptedBodyCode,
			Info: "corrupted request body",
			Text: err.Error(),
		}
	}

	if addControlObjectReq.ImagePart != nil {
		imgBuffStr, err := base64.StdEncoding.DecodeString(addControlObjectReq.ImagePart.ImgBuff)
		if err != nil {
			return nil, &proto.ErrorData{
				Code: proto.CorruptedBodyCode,
				Info: "corrupted request body",
				Text: err.Error(),
			}
		}
		imgBuff := []byte(imgBuffStr)
		kind, err := filetype.Match(imgBuff)
		if err != nil {
			return nil, &proto.ErrorData{
				Code: proto.CorruptedBodyCode,
				Info: "corrupted request body",
				Text: err.Error(),
			}
		}
		if kind == filetype.Unknown {
			return nil, &proto.ErrorData{
				Code: proto.CorruptedBodyCode,
				Info: "corrupted request body",
				Text: "unable to recognize image type",
			}
		}
	}

	return addControlObjectReq, nil
}

func (rest *restAPI) addControlObjectHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Infof("got request \"%s\"", apiAddControlObject)
	addControlObjectReq, errorData := validateAddControlObjectReq(req)
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

	k := addControlObjectReq.Header.UUID
	v := &schedulers.AwaitingControlObject{
		TS:        time.Now(),
		SrcAddr:   addControlObjectReq.Header.SrcAddr,
		UUID:      k,
		Images:    make(map[string]proto.ImagePart),
		FacesData: make(map[string]proto.FaceData),
	}
	if _, err := rest.cpScheduler.ACOQ.PushWithCheck(k, v); err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		e := &proto.ImmedResp{
			Header: proto.Header{
				SrcAddr: rest.srcAddr,
			},
			ErrorData: &proto.ErrorData{
				Code: proto.InternalServerError,
				Info: "unable to push request to queue",
				Text: err.Error(),
			},
		}
		re, _ := json.Marshal(e)
		resp.Write(re)
		return
	}

	go rest.processAddControlObjectReq(addControlObjectReq)

	resp.WriteHeader(http.StatusOK)
	e := &proto.ImmedResp{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    addControlObjectReq.Header.UUID,
		},
	}
	re, _ := json.Marshal(e)
	resp.Write(re)
}

func (rest *restAPI) processAddControlObjectReq(addControlObjectReq *proto.AddControlObjectReq) {
	k := addControlObjectReq.Header.UUID
	awCob := rest.cpScheduler.ACOQ.Get(k)
	if awCob == nil {
		rest.logger.Warnf("unable to find \"AddControlObjectReq\" with UUID \"%s\"", k)
		return
	}
	rest.logger.Debugf("successfully got \"AddControlObjectReq\" with UUID \"%s\"", k)
	awCob.Mu.Lock()
	if addControlObjectReq.ControlObjectPart != nil {
		rest.logger.Debug("got Control Object")
		awCob.ControlObjectPart = addControlObjectReq.ControlObjectPart
		awCob.Mu.Unlock()
		return
	}
	rest.logger.Debug("got image")
	imgK := uuid.Must(uuid.NewV4()).String()
	awCob.Images[imgK] = *addControlObjectReq.ImagePart
	awCob.Mu.Unlock()

	processImageReq := &proto.ProcessImageReq{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    imgK,
		},
		ImgBuff: addControlObjectReq.ImagePart.ImgBuff,
	}
	if addControlObjectReq.ImagePart.FaceBox != nil {
		processImageReq.FaceBoxes = []proto.FaceBox{addControlObjectReq.ImagePart.FaceBox}
	}

	if err := rest.frScheduler.SendProcessImageReq(processImageReq); err != nil {
		rest.logger.Error(err)
		rest.frScheduler.AwImgsQ.Pop(k)
		return
	}
	rest.logger.Debugf("successfully sent \"ProcessImageReq\" with UUID \"%s\" to facerecognizer", imgK)

}
