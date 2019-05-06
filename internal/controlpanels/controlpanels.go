package controlpanels

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

// FaceData ...
type FaceData struct {
	Box        facerecognition.FaceBox `json:"box"`
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Patronymic string                  `json:"patronymic"`
	Surname    string                  `json:"surname"`
	Passport   string                  `json:"passport"`
	PhoneNum   string                  `json:"phone_num"`
}

// AwaitingKey is key for awaiting images.
type AwaitingKey struct {
	SrcAddr string
	ID      uint64
}

// AwaitingVal is key for awaiting images.
type AwaitingVal struct {
	ImgBuff        []byte
	FacesData      []FaceData
	FacialFeatures [][]float64
}

// Scheduler controls
type Scheduler struct {
	cfg           cfgparser.ControlPanelsCFG
	idx           uint64
	mu            sync.RWMutex
	clients       []*http.Client
	defaultClient *http.Client

	// Awaiting images.
	awImgIdx     uint64
	awaitingImgs map[AwaitingKey]*AwaitingVal
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
func CreateScheduler(cfg cfgparser.ControlPanelsCFG) *Scheduler {
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
		idx:     0,
		mu:      sync.RWMutex{},
		clients: clients,
		defaultClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(cfg.TimeoutMS),
		},
		awaitingImgs: make(map[AwaitingKey]*AwaitingVal),
	}
}

// PopAwaitingImage pops awaiting image from Scheduler's memory.
func (cps *Scheduler) PopAwaitingImage(key AwaitingKey) *AwaitingVal {
	awVal, ok := cps.awaitingImgs[key]
	if !ok {
		return nil
	}
	delete(cps.awaitingImgs, key)
	return awVal
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
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
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
		}] = &AwaitingVal{
			ImgBuff:        imgBuff,
			FacesData:      faces,
			FacialFeatures: facialFeatures,
		}
		return nil, true, nil
	}
	return imgNotifyResp.FacesData, false, nil
}
