package facerecognition

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	cfg cfgparser.FaceRecognitionCFG

	// Clients.
	clientIdx     uint64
	mu            sync.RWMutex
	clients       []*http.Client
	defaultClient *http.Client

	// Awaiting images.
	awImgIdx     uint64
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
func CreateScheduler(cfg cfgparser.FaceRecognitionCFG) *Scheduler {
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
			cfg.FaceProcessors[i] = strings.Replace(faceProc, "unix://", "http://", -1)
		} else {
			clients = append(clients, nil)
		}
	}
	return &Scheduler{
		cfg:       cfg,
		clientIdx: 0,
		mu:        sync.RWMutex{},
		clients:   clients,
		defaultClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(cfg.TimeoutMS),
		},
		awaitingImgs: make(map[AwaitingKey][]byte),
	}
}

// PopAwaitingImage pops awaiting image from Scheduler's memory.
func (frs *Scheduler) PopAwaitingImage(key AwaitingKey) []byte {
	imgBuff, ok := frs.awaitingImgs[key]
	if !ok {
		return nil
	}
	delete(frs.awaitingImgs, key)
	return imgBuff
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
func (frs *Scheduler) GetFaces(imgBuff []byte) ([]Face, bool, error) {
	frs.mu.RLock()
	clientIdx := frs.clientIdx % uint64(len(frs.cfg.FaceProcessors))
	url := frs.cfg.FaceProcessors[clientIdx]
	var cli *http.Client
	if frs.clients[clientIdx] != nil {
		cli = frs.clients[clientIdx]
	} else {
		frs.clientIdx++
		cli = frs.defaultClient
	}
	frs.mu.RUnlock()

	ID := atomic.AddUint64(&(frs.awImgIdx), 1)

	base64ImgBuff := make([]byte, base64.StdEncoding.EncodedLen(len(imgBuff)))
	base64.StdEncoding.Encode(base64ImgBuff, imgBuff)
	data, err := json.Marshal(ImgTaskReq{
		Headers: proto.Headers{
			SrcAddr: "",
			Immed:   false,
		},
		ID:      ID,
		ImgBuff: string(base64ImgBuff),
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to marshal image buffer to JSON")
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to create http-request")
	}

	resp, err := cli.Do(req)
	if err != nil {
		frs.mu.Lock()
		if frs.cfg.FaceProcessors[clientIdx] == url {
			frs.cfg.FaceProcessors = append(frs.cfg.FaceProcessors[:clientIdx], frs.cfg.FaceProcessors[clientIdx+1:]...)
			frs.clients = append(frs.clients[:clientIdx], frs.clients[clientIdx+1:]...)
		}
		frs.mu.Unlock()

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

	if imgTaskResp.Headers.Immed {
		frs.awaitingImgs[AwaitingKey{
			SrcAddr: "",
			ID:      ID,
		}] = imgBuff
		return nil, true, nil
	}

	return imgTaskResp.Faces, false, nil
}
