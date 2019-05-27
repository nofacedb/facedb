package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/h2non/filetype"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

const apiV1PutImg = "/api/v1/put_img"

type putImgReq struct {
	Headers proto.Headers `json:"headers"`
	ImgBuff string        `json:"img_buff"`
}

func (rest *restAPI) putImgHandler(resp http.ResponseWriter, req *http.Request) {
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

	putImgReq := &putImgReq{}
	if err = json.Unmarshal(buff, putImgReq); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to unmarshal request body JSON"))
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	imgBuffStr, err := base64.StdEncoding.DecodeString(putImgReq.ImgBuff)
	if err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to decode image buffer"))
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	imgBuff := []byte(imgBuffStr)

	if rest.immedResp {
		go func() {
			if err := processPutImageRequest(rest, imgBuff); err != nil {
				rest.logger.Error(errors.Wrap(err, "unable to process put image req"))
				// TODO.
			}
		}()
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processPutImageRequest(rest, imgBuff); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to process put image req"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

const (
	pngExt  = "png"
	jpegExt = "jpg"
	bmpExt  = "bmp"
)

func processPutImageRequest(rest *restAPI, imgBuff []byte) error {
	kind, err := filetype.Match(imgBuff)
	if err != nil {
		return errors.Wrap(err, "unable to recognize type of image")
	}
	if kind == filetype.Unknown {
		return fmt.Errorf("unable to recognize type of image")
	}

	ID := rest.frs.GenerateID()
	cli, url, err := rest.frs.GetFaceProcessor()
	if err != nil {
		return errors.Wrap(err, "unable to create request for faceprocessor")
	}
	key := facerecognition.AwaitingKey{
		SrcAddr: url,
		ID:      ID,
	}
	if err = rest.frs.PushAwaitingImage(key, imgBuff); err != nil {
		return errors.Wrap(err, "unable to push image to awaiting queue")
	}
	faces, immed, err := rest.frs.GetFaces(imgBuff, ID, cli, url)
	if err != nil {
		rest.frs.PopAwaitingImage(key)
		return errors.Wrap(err, "unable to get faces")
	}

	if !immed {
		rest.frs.PopAwaitingImage(key)
		return processPutFeaturesRequest(rest, imgBuff, faces)
	}

	return nil
}
