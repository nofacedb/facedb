package httpserver

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func (rest *restAPI) notificationHandler(resp http.ResponseWriter, req *http.Request) {
	imgBuff, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	if rest.immedResp {
		go processImage(rest, imgBuff)
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processImage(rest, imgBuff); err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

func processImage(rest *restAPI, imgBuff []byte) error {
	faces, err := rest.frsch.GetFaces(imgBuff)
	if err != nil {
		return err
	}
	for _, face := range faces {
		fmt.Printf("%+v ", face.FaceBox)
	}
	return nil
}
