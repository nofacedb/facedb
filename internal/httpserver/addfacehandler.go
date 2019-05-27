package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/nofacedb/facedb/internal/controlpanels"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

const apiV1AddFace = "/api/v1/add_face"

type addFaceReq struct {
	Headers    proto.Headers `json:"headers"`
	ID         uint64        `json:"id"`
	COB        *facedb.COB   `json:"cob"`
	ImgsNumber *int          `json:"imgs_number"`
	ImgBuff    *string       `json:"img_buff"`
}

func (rest *restAPI) addFaceHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Debugf("got \"%s\" message", apiV1AddFace)
	if req.Method != "POST" {
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

	addFaceReq := &addFaceReq{}
	if err = json.Unmarshal(buff, addFaceReq); err != nil {
		rest.logger.Error(errors.Wrap(err, "unable to unmarshal request body JSON"))
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	rest.logger.Debugf("source: \"%s\"", addFaceReq.Headers.SrcAddr)

	if rest.immedResp {
		go func() {
			if err := processAddFaceRequest(rest, addFaceReq); err != nil {
				rest.logger.Error(errors.Wrap(err, "unablte to process add face req"))
			}
		}()
		resp.WriteHeader(http.StatusOK)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

func processAddFaceRequest(rest *restAPI, addFaceReq *addFaceReq) error {
	key := controlpanels.AwaitingKey{
		SrcAddr: addFaceReq.Headers.SrcAddr,
		ID:      addFaceReq.ID,
	}
	if !rest.cps.CheckAwaitingFace(key) {
		rest.cps.PushAwaitingFace(key, &controlpanels.AwaitingFaceVal{
			AwaitingImgBuffs: make(map[uint64][]byte),
			AwaitingFaces:    make(map[uint64]facerecognition.Face),
		})
	}
	if (addFaceReq.COB != nil) && (addFaceReq.ImgsNumber != nil) {
		if err := insertNewCOB(rest, addFaceReq, key); err != nil {
			return errors.Wrap(err, "unable to insert new COB")
		}
	} else if addFaceReq.ImgBuff != nil {
		if err := insertNewImg(rest, addFaceReq, key); err != nil {
			return errors.Wrap(err, "unable to insert new image")
		}
	} else {
		return fmt.Errorf("invalid JSON structure")
	}
	return nil
}

func insertNewCOB(rest *restAPI, addFaceReq *addFaceReq, key controlpanels.AwaitingKey) error {
	rest.logger.Debug("got JSON data")
	cob, err := rest.fs.SelectCOBByPassport(addFaceReq.COB.Passport)
	if err != nil {
		return errors.Wrap(err, "cob already exists")
	}
	if *(cob.ID) != facedb.UNKNOWNFIELD {
		return fmt.Errorf("object with passport \"%s\" already exists", addFaceReq.COB.Passport)
	}
	addFaceReq.COB.TS = new(time.Time)
	*addFaceReq.COB.TS = time.Now()
	if err = rest.fs.InsertCOB([]facedb.COB{*addFaceReq.COB}); err != nil {
		return errors.Wrap(err, "unable to insert new control object to database")
	}
	cob, err = rest.fs.SelectCOBByPassport(addFaceReq.COB.Passport)
	if err != nil {
		return errors.Wrap(err, "unable to get inserted control object UUID")
	}
	rest.cps.PushAwaitingFaceID(key, *cob.ID, *addFaceReq.ImgsNumber)
	return nil
}

func insertNewImg(rest *restAPI, addFaceReq *addFaceReq, key controlpanels.AwaitingKey) error {
	rest.logger.Debug("got image")
	ID := rest.frs.GenerateID()
	cli, url, err := rest.frs.GetFaceProcessor()
	if err != nil {
		return errors.Wrap(err, "unable to create request for faceprocessor")
	}

	imgBuffStr, err := base64.StdEncoding.DecodeString(*addFaceReq.ImgBuff)
	if err != nil {
		return errors.Wrap(err, "unable to decode image buffer")
	}

	imgBuff := []byte(imgBuffStr)
	rest.cps.PushAwaitingFaceImg(key, ID, imgBuff)
	faces, immed, err := rest.frs.GetFaces(imgBuff, ID, cli, url)
	if err != nil {
		return errors.Wrap(err, "unable to get faces")
	}
	if !immed {
		fmt.Println(faces)
	}

	return nil
}
