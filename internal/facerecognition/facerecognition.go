package facerecognition

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"strings"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

const recAPIV1RpocImg = "/api/v1/proc_img"

// FaceBox struct represents coordinates of one face in image.
type FaceBox []uint64

// Top returns top coordinate.
func (fb FaceBox) Top() uint64 {
	return fb[0]
}

// Right returns top coordinate.
func (fb FaceBox) Right() uint64 {
	return fb[1]
}

// Bottom returns top coordinate.
func (fb FaceBox) Bottom() uint64 {
	return fb[2]
}

// Left returns top coordinate.
func (fb FaceBox) Left() uint64 {
	return fb[3]
}

// Face struct is a pair of FaceBox and FacialFeatures.
type Face struct {
	Box            FaceBox   `json:"box"`
	FacialFeatures []float64 `json:"features"`
}

// AwaitingKey is key for awaiting images.
type AwaitingKey struct {
	SrcAddr string
	ID      uint64
}

// Scheduler controls work of face recognition process.
// It selects faceprocessor for current task by round-robin.
type Scheduler struct {
	// Config.
	cfg     cfgparser.FaceRecognitionCFG
	srcAddr string

	// Clients.
	clientIdx     uint64
	clientsMu     sync.RWMutex
	clients       []*http.Client
	defaultClient *http.Client

	// Awaiting images.
	awImgIdx     *uint64
	awImgMu      sync.Mutex
	awaitingImgs map[AwaitingKey][]byte
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
func CreateScheduler(cfg cfgparser.FaceRecognitionCFG,
	srcAddr string, awImgIdx *uint64) *Scheduler {
	clients := make([]*http.Client, 0, len(cfg.FaceProcessors))
	for i, faceProc := range cfg.FaceProcessors {
		if strings.HasPrefix(faceProc, "unix://") {
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
			cfg.FaceProcessors[i] = faceProc[len("unix://"):]
		} else {
			clients = append(clients, nil)
		}
	}
	return &Scheduler{
		cfg:     cfg,
		srcAddr: srcAddr,

		clientIdx: 0,
		clientsMu: sync.RWMutex{},
		clients:   clients,
		defaultClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(cfg.TimeoutMS),
		},

		awImgIdx:     awImgIdx,
		awaitingImgs: make(map[AwaitingKey][]byte),
	}
}

// GenerateID ...
func (frs *Scheduler) GenerateID() uint64 {
	return atomic.AddUint64(frs.awImgIdx, 1)
}

// PushAwaitingImage ...
func (frs *Scheduler) PushAwaitingImage(key AwaitingKey, imgBuff []byte) error {
	frs.awImgMu.Lock()
	defer frs.awImgMu.Unlock()
	_, ok := frs.awaitingImgs[key]
	if ok {
		return fmt.Errorf("awaiting image with choosen key already exists")
	}
	frs.awaitingImgs[key] = imgBuff
	return nil
}

// PopAwaitingImage ...
func (frs *Scheduler) PopAwaitingImage(key AwaitingKey) ([]byte, error) {
	frs.awImgMu.Lock()
	defer frs.awImgMu.Unlock()
	imgBuff, ok := frs.awaitingImgs[key]
	if !ok {
		return nil, fmt.Errorf("no awaiting image with choosen key")
	}
	delete(frs.awaitingImgs, key)
	return imgBuff, nil
}

// GetFaceProcessor ...
func (frs *Scheduler) GetFaceProcessor() (*http.Client, string, error) {
	frs.clientsMu.RLock()
	defer frs.clientsMu.RUnlock()
	if len(frs.cfg.FaceProcessors) == 0 {
		return nil, "", fmt.Errorf("no faceprocessors are available")
	}
	clientIdx := frs.clientIdx % uint64(len(frs.cfg.FaceProcessors))
	url := frs.cfg.FaceProcessors[clientIdx]
	var cli *http.Client
	if frs.clients[clientIdx] != nil {
		cli = frs.clients[clientIdx]
	} else {
		frs.clientIdx++
		cli = frs.defaultClient
	}

	return cli, url, nil
}

// DeleteFaceProcessor ...
func (frs *Scheduler) DeleteFaceProcessor(url string) {
	frs.clientsMu.Lock()
	defer frs.clientsMu.Unlock()
	clientIdx := 0
	for i := 0; i < len(frs.cfg.FaceProcessors); i++ {
		if frs.cfg.FaceProcessors[i] == url {
			clientIdx = i
			break
		}
	}
	frs.cfg.FaceProcessors = append(frs.cfg.FaceProcessors[:clientIdx], frs.cfg.FaceProcessors[clientIdx+1:]...)
	frs.clients = append(frs.clients[:clientIdx], frs.clients[clientIdx+1:]...)
}

// BytesToImage converts bytes buffer to Golang image.
func BytesToImage(imgBuff []byte) (image.Image, string, error) {
	return image.Decode(bytes.NewReader(imgBuff))
}

// ImgTaskReq contains task for faceprocessor.
type ImgTaskReq struct {
	Headers proto.Headers `json:"headers"`
	ID      uint64        `json:"id"`
	ImgBuff string        `json:"img_buff"`
}

// ImgTaskResp ...
type ImgTaskResp struct {
	Headers proto.Headers `json:"headers"`
	ID      *uint64       `json:"id"`
	Faces   []Face        `json:"faces"`
}

// GetFaces returns Faces from image buffer.
func (frs *Scheduler) GetFaces(imgBuff []byte, ID uint64, cli *http.Client, url string) ([]Face, bool, error) {
	base64ImgBuff := make([]byte, base64.StdEncoding.EncodedLen(len(imgBuff)))
	base64.StdEncoding.Encode(base64ImgBuff, imgBuff)
	data, err := json.Marshal(ImgTaskReq{
		Headers: proto.Headers{
			SrcAddr: frs.srcAddr,
			Immed:   false,
		},
		ID:      ID,
		ImgBuff: string(base64ImgBuff),
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to marshal image buffer to JSON")
	}
	req, err := http.NewRequest("PUT", url+recAPIV1RpocImg, bytes.NewReader(data))
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to create http-request")
	}

	resp, err := cli.Do(req)
	if err != nil {
		frs.DeleteFaceProcessor(url)
		return nil, false, errors.Wrap(err, "unable to get response from faceprocessor")
	}

	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to read response from body")
	}

	imgTaskResp := &ImgTaskResp{}
	if err := json.Unmarshal(buff, imgTaskResp); err != nil {
		return nil, false, errors.Wrap(err, "unable to unmarshal response JSON")
	}

	return imgTaskResp.Faces, imgTaskResp.Headers.Immed, nil
}
