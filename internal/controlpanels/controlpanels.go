package controlpanels

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/facedb"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

const recAPIV1NotifyImg = "/api/v1/notify_img"

// FaceData ...
type FaceData struct {
	Box facerecognition.FaceBox `json:"box"`
	COB facedb.COB              `json:"cob"`
}

// AwaitingKey is key for awaiting images.
type AwaitingKey struct {
	SrcAddr string
	ID      uint64
}

// AwaitingImgVal is val for awaiting images.
type AwaitingImgVal struct {
	ImgBuff        []byte
	FacesData      []FaceData
	FacialFeatures [][]float64
}

// AwaitingFaceVal is val for awaiting faces.
type AwaitingFaceVal struct {
	ID                  string
	ImgsNumber          int
	ProcessedImgsNumber int
	AwaitingImgBuffs    map[uint64][]byte
	AwaitingFaces       map[uint64]facerecognition.Face
}

// Scheduler controls
type Scheduler struct {
	cfg     cfgparser.ControlPanelsCFG
	srcAddr string

	idx           uint64
	clientsMu     sync.RWMutex
	clients       []*http.Client
	defaultClient *http.Client

	// Awaiting images.
	awImgIdx     uint64
	awImgMu      sync.RWMutex
	awaitingImgs map[AwaitingKey]*AwaitingImgVal

	// Awaitinig Faces.
	awFacesImgIdx *uint64
	awFacesMu     sync.RWMutex
	awaitingFaces map[AwaitingKey]*AwaitingFaceVal
}

type unixDialer struct {
	net.Dialer
}

func (d *unixDialer) Dial(network, address string) (net.Conn, error) {
	parts := strings.Split(address, ":")
	path := parts[0]
	dotCount := strings.Count(path, ".")
	if strings.HasSuffix(path, ".sock") {
		dotCount--
	}
	path = strings.Replace(path, ".", "/", dotCount)
	if path[0] != '~' {
		path = "/" + path
	}
	return d.Dialer.Dial("unix", path)
}

// CreateScheduler creates new Scheduler.
func CreateScheduler(cfg cfgparser.ControlPanelsCFG,
	srcAddr string, awFacesIdx *uint64) *Scheduler {
	clients := make([]*http.Client, 0, len(cfg.ControlPanels))
	for i, controlPanel := range cfg.ControlPanels {
		if strings.HasPrefix(controlPanel, "unix://") {
			// Copied with minimal changes from go/src/net/http/transport.go[37:53]
			transport := &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				Dial: (&unixDialer{net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				},
				}).Dial,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			}
			clients = append(clients, &http.Client{
				Transport: transport,
				Timeout:   time.Millisecond * time.Duration(cfg.TimeoutMS),
			})
			cfg.ControlPanels[i] = strings.Replace(controlPanel, "unix://", "http://", -1)
		} else {
			clients = append(clients, nil)
		}
	}
	return &Scheduler{
		cfg:     cfg,
		srcAddr: srcAddr,

		idx:       0,
		clientsMu: sync.RWMutex{},
		clients:   clients,
		defaultClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(cfg.TimeoutMS),
		},

		awImgIdx:     0,
		awImgMu:      sync.RWMutex{},
		awaitingImgs: make(map[AwaitingKey]*AwaitingImgVal),

		awFacesImgIdx: awFacesIdx,
		awFacesMu:     sync.RWMutex{},
		awaitingFaces: make(map[AwaitingKey]*AwaitingFaceVal),
	}
}

// PushAwaitingImage ...
func (cps *Scheduler) PushAwaitingImage(key AwaitingKey, val *AwaitingImgVal) error {
	cps.awImgMu.Lock()
	defer cps.awImgMu.Unlock()
	_, ok := cps.awaitingImgs[key]
	if ok {
		return fmt.Errorf("awaiting image with choosen key already exists")
	}
	cps.awaitingImgs[key] = val
	return nil
}

// PopAwaitingImage ...
func (cps *Scheduler) PopAwaitingImage(key AwaitingKey) (*AwaitingImgVal, error) {
	cps.awImgMu.Lock()
	defer cps.awImgMu.Unlock()
	awVal, ok := cps.awaitingImgs[key]
	if !ok {
		return nil, fmt.Errorf("no awaiting image with choosen key")
	}
	delete(cps.awaitingImgs, key)
	return awVal, nil
}

// PushAwaitingFace ...
func (cps *Scheduler) PushAwaitingFace(key AwaitingKey, val *AwaitingFaceVal) error {
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	_, ok := cps.awaitingFaces[key]
	if ok {
		return fmt.Errorf("awaiting face with choosen key already exists")
	}
	cps.awaitingFaces[key] = val
	return nil
}

// FindAwaitingFaceKey ...
func (cps *Scheduler) FindAwaitingFaceKey(id uint64) *AwaitingKey {
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	for k0, v0 := range cps.awaitingFaces {
		for k1 := range v0.AwaitingImgBuffs {
			if k1 == id {
				return &k0
			}
		}
	}
	return nil
}

// CheckAwaitingFace ...
func (cps *Scheduler) CheckAwaitingFace(key AwaitingKey) bool {
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	_, ok := cps.awaitingFaces[key]
	return ok
}

// PushAwaitingFaceID ...
func (cps *Scheduler) PushAwaitingFaceID(key AwaitingKey, id string, imgsNumber int) error {
	if ok := cps.CheckAwaitingFace(key); !ok {
		return fmt.Errorf("no awaiting face with choosen key")
	}
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	v := cps.awaitingFaces[key]
	v.ID = id
	v.ImgsNumber = imgsNumber
	cps.awaitingFaces[key] = v
	return nil
}

// PushAwaitingFaceImg ...
func (cps *Scheduler) PushAwaitingFaceImg(key AwaitingKey, imgID uint64, imgBuff []byte) error {
	if ok := cps.CheckAwaitingFace(key); !ok {
		return fmt.Errorf("no awaiting face with choosen key")
	}
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	v := cps.awaitingFaces[key]
	v.AwaitingImgBuffs[imgID] = imgBuff
	cps.awaitingFaces[key] = v
	return nil
}

// PushAwaitingFFs ...
func (cps *Scheduler) PushAwaitingFFs(key AwaitingKey, imgID uint64, face facerecognition.Face) error {
	if ok := cps.CheckAwaitingFace(key); !ok {
		return fmt.Errorf("no awaiting face with choosen key")
	}
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	v := cps.awaitingFaces[key]
	v.AwaitingFaces[imgID] = face
	v.ProcessedImgsNumber++
	cps.awaitingFaces[key] = v
	return nil
}

// IsAwaitingFaceReady ...
func (cps *Scheduler) IsAwaitingFaceReady(key AwaitingKey) (bool, error) {
	if ok := cps.CheckAwaitingFace(key); !ok {
		return false, fmt.Errorf("no awaiting face with choosen key")
	}
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	v := cps.awaitingFaces[key]
	// Shitty code.
	return v.ImgsNumber == v.ProcessedImgsNumber && v.ID != "", nil
}

// PopAwaitingFace ...
func (cps *Scheduler) PopAwaitingFace(key AwaitingKey) (*AwaitingFaceVal, error) {
	cps.awFacesMu.Lock()
	defer cps.awFacesMu.Unlock()
	awVal, ok := cps.awaitingFaces[key]
	if !ok {
		return nil, fmt.Errorf("no awaiting image with choosen key")
	}
	delete(cps.awaitingImgs, key)
	return awVal, nil
}

// ImgNotifyReq ...
type ImgNotifyReq struct {
	Headers   proto.Headers `json:"headers"`
	ID        uint64        `json:"id"`
	ImgBuff   string        `json:"img_buff"`
	FacesData []FaceData    `json:"faces_data"`
}

// ImgNotifyResp ...
type ImgNotifyResp struct {
	Headers   proto.Headers `json:"headers"`
	Cmd       *string       `json:"cmd"`
	ID        *uint64       `json:"id"`
	FacesData []FaceData    `json:"faces_data"`
}

// Notify sends notifications to all control panels.
func (cps *Scheduler) Notify(imgBuff []byte, faces []FaceData, facialFeatures [][]float64) ([]FaceData, bool, error) {
	idx := 0
	url := cps.cfg.ControlPanels[idx]
	var cli *http.Client
	if cps.clients[idx] != nil {
		cli = cps.clients[idx]
	} else {
		cps.idx++
		cli = cps.defaultClient
	}

	ID := atomic.AddUint64(&(cps.awImgIdx), 1)

	base64ImgBuff := make([]byte, base64.StdEncoding.EncodedLen(len(imgBuff)))
	base64.StdEncoding.Encode(base64ImgBuff, imgBuff)
	data, err := json.Marshal(ImgNotifyReq{
		Headers: proto.Headers{
			SrcAddr: "",
			Immed:   false,
		},
		ID:        ID,
		ImgBuff:   string(base64ImgBuff),
		FacesData: faces,
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to marshal faces data to JSON")
	}
	req, err := http.NewRequest("PUT", url+recAPIV1NotifyImg, bytes.NewReader(data))
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to create http-request")
	}

	resp, err := cli.Do(req)
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to get response from faceprocessor")
	}

	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to read response from body")
	}

	imgNotifyResp := &ImgNotifyResp{}
	if err := json.Unmarshal(buff, imgNotifyResp); err != nil {
		return nil, false, errors.Wrap(err, "unable to unmarshal response JSON")
	}

	if imgNotifyResp.Headers.Immed {
		cps.awaitingImgs[AwaitingKey{
			SrcAddr: "",
			ID:      ID,
		}] = &AwaitingImgVal{
			ImgBuff:        imgBuff,
			FacesData:      faces,
			FacialFeatures: facialFeatures,
		}
		return nil, true, nil
	}
	return imgNotifyResp.FacesData, false, nil
}
