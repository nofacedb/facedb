package httpserver

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/h2non/filetype"
	"github.com/pkg/errors"
)

func (rest *restAPI) putImageHandler(resp http.ResponseWriter, req *http.Request) {
	if req.Method != "PUT" {
		resp.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	imgBuff, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	if rest.immedResp {
		go processPutImageRequest(rest, imgBuff)
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processPutImageRequest(rest, imgBuff); err != nil {
		fmt.Println(err)
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

	faces, immed, err := rest.frs.GetFaces(imgBuff)
	if err != nil {
		return errors.Wrap(err, "unable to get faces")
	}

	if !immed {
		return processPutFeaturesRequest(rest, imgBuff, faces)
	}

	return nil
}
