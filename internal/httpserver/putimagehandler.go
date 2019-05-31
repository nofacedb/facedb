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
	"github.com/pkg/errors"
)

func validatePutImageReq(req *http.Request) (*proto.PutImageReq, *proto.ErrorData) {
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

	putImageReq := &proto.PutImageReq{}
	if err = json.Unmarshal(data, putImageReq); err != nil {
		return nil, &proto.ErrorData{
			Code: proto.CorruptedBodyCode,
			Info: "corrupted request body",
			Text: err.Error(),
		}
	}

	imgBuffStr, err := base64.StdEncoding.DecodeString(putImageReq.ImgBuff)
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

	return putImageReq, nil
}

func (rest *restAPI) putImageHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Infof("got request on \"%s\"", apiPutImage)
	putImageReq, errorData := validatePutImageReq(req)
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
		putImageReq.Header.SrcAddr,
		putImageReq.Header.UUID)

	k := putImageReq.Header.UUID
	v := &schedulers.AwaitingImage{
		TS:        time.Now(),
		SrcAddr:   putImageReq.Header.SrcAddr,
		UUID:      k,
		ImgBuff:   putImageReq.ImgBuff,
		FaceBoxes: putImageReq.FaceBoxes,
	}
	if err := rest.frScheduler.AwImgsQ.Push(k, v); err != nil {
		err = errors.Wrapf(err, "unable to push \"PutImageReq\" with UUID \"%s\"to queue", k)
		rest.logger.Warnf(err.Error())
		resp.WriteHeader(http.StatusBadRequest)
		e := &proto.ImmedResp{
			Header: proto.Header{
				SrcAddr: rest.srcAddr,
				UUID:    k,
			},
			ErrorData: &proto.ErrorData{
				Code: proto.InternalServerError,
				Info: "unable to push \"PutImageReq\" to queue",
				Text: err.Error(),
			},
		}
		re, _ := json.Marshal(e)
		resp.Write(re)
		return
	}
	rest.logger.Debugf("successfully pushed \"PutImageReq\" with UUID \"%s\" to queue", k)

	go rest.processPutImageReq(putImageReq)

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

func (rest *restAPI) processPutImageReq(putImageReq *proto.PutImageReq) {
	k := putImageReq.Header.UUID
	processImageReq := &proto.ProcessImageReq{
		Header: proto.Header{
			SrcAddr: rest.srcAddr,
			UUID:    k,
		},
		ImgBuff:   putImageReq.ImgBuff,
		FaceBoxes: putImageReq.FaceBoxes,
	}

	if err := rest.frScheduler.SendProcessImageReq(processImageReq); err != nil {
		rest.logger.Error(err)
		rest.frScheduler.AwImgsQ.Pop(k)
		return
	}
	rest.logger.Debugf("successfully sent \"ProcessImageReq\" with UUID \"%s\" to facerecognizer", k)
}
