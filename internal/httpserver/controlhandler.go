package httpserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/proto"
)

type imgControlDoneReq struct {
	Headers   proto.Headers            `json:"headers"`
	Cmd       string                   `json:"cmd"`
	ID        uint64                   `json:"id"`
	FacesData []controlpanels.FaceData `json:"faces_data"`
}

func (rest *restAPI) putControlHandler(resp http.ResponseWriter, req *http.Request) {
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

	imgControlDoneReq := &imgControlDoneReq{}
	if err := json.Unmarshal(buff, imgControlDoneReq); err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	awVal := rest.cps.PopAwaitingImage(controlpanels.AwaitingKey{
		SrcAddr: imgControlDoneReq.Headers.SrcAddr,
		ID:      imgControlDoneReq.ID,
	})

	fmt.Println(imgControlDoneReq.Cmd)
	fmt.Println(imgControlDoneReq.ID)
	fmt.Println(imgControlDoneReq.FacesData)
	fmt.Println(awVal)
}
