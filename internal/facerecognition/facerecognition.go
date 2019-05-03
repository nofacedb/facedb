package facerecognition

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"strings"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
)

// Scheduler controls work of face recognition process.
// It selects faceprocessor for current task by round-robin.
type Scheduler struct {
	cfg           cfgparser.FaceRecognitionCFG
	idx           uint64
	mu            sync.RWMutex
	clients       []*http.Client
	defaultClient *http.Client
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
		cfg:     cfg,
		idx:     0,
		mu:      sync.RWMutex{},
		clients: clients,
		defaultClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(cfg.TimeoutMS),
		},
	}
}

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

// Faces struct contains faces data, got from faceprocessor.
type Faces struct {
	Faces []Face `json:"faces"`
}

// Face struct is a pair of FaceBox and FacialFeatures.
type Face struct {
	Box            FaceBox   `json:"box"`
	FacialFeatures []float64 `json:"features"`
}

// BytesToImage converts bytes buffer to Golang image.
func BytesToImage(imgBuff []byte) (image.Image, string, error) {
	return image.Decode(bytes.NewReader(imgBuff))
}

// ImageTask contains task for faceprocessor.
type ImageTask struct {
	ImgBuff string `json:"img_buff"`
}

// GetFaces returns Faces from image buffer.
func (frs *Scheduler) GetFaces(imgBuff []byte) (*Faces, error) {
	frs.mu.RLock()
	idx := frs.idx % uint64(len(frs.cfg.FaceProcessors))
	url := frs.cfg.FaceProcessors[idx]
	var cli *http.Client
	if frs.clients[idx] != nil {
		cli = frs.clients[idx]
	} else {
		frs.idx++
		cli = frs.defaultClient
	}
	frs.mu.RUnlock()

	base64ImgBuff := make([]byte, base64.StdEncoding.EncodedLen(len(imgBuff)))
	base64.StdEncoding.Encode(base64ImgBuff, imgBuff)
	data, err := json.Marshal(ImageTask{ImgBuff: string(base64ImgBuff)})
	if err != nil {
		return nil, errors.Wrap(err, "unable to marshal image buffer to JSON")
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create http-request")
	}

	resp, err := cli.Do(req)
	if err != nil {
		frs.mu.Lock()
		if frs.cfg.FaceProcessors[idx] == url {
			frs.cfg.FaceProcessors = append(frs.cfg.FaceProcessors[:idx], frs.cfg.FaceProcessors[idx+1:]...)
			frs.clients = append(frs.clients[:idx], frs.clients[idx+1:]...)
		}
		frs.mu.Unlock()

		return nil, errors.Wrap(err, "unable to get response from faceprocessor")
	}

	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read response from body")
	}

	faces := &Faces{}
	if err := json.Unmarshal(buff, faces); err != nil {
		return nil, errors.Wrap(err, "unable to read faces array")
	}

	return faces, nil
}

// ChangeableImage is changeable cover over Golang image.Image.
type ChangeableImage struct {
	image.Image
	changedPixels map[image.Point]color.Color
}

// CreateChangeableImage returns new changeable image from Golang image.Image.
func CreateChangeableImage(img image.Image) *ChangeableImage {
	// Not using parameters becaues of embedded structure.
	return &ChangeableImage{img, map[image.Point]color.Color{}}
}

// Set changes image pixel color.
func (cimg *ChangeableImage) Set(x, y int, c color.Color) {
	cimg.changedPixels[image.Point{x, y}] = c
}

// At returns image pixel color.
func (cimg *ChangeableImage) At(x, y int) color.Color {
	if c := cimg.changedPixels[image.Point{x, y}]; c != nil {
		return c
	}
	return cimg.Image.At(x, y)
}

// FrameFaces puts every face, found in image, to colored box.
func FrameFaces(img image.Image, faces *Faces, c color.Color) *ChangeableImage {
	cimg := CreateChangeableImage(img)

	for _, face := range faces.Faces {
		faceBox := face.Box
		for j := faceBox.Left(); j < faceBox.Right(); j++ {
			cimg.Set(int(j), int(faceBox.Bottom())-1, c)
			cimg.Set(int(j), int(faceBox.Bottom()), c)
			cimg.Set(int(j), int(faceBox.Bottom())+1, c)
			cimg.Set(int(j), int(faceBox.Top())-1, c)
			cimg.Set(int(j), int(faceBox.Top()), c)
			cimg.Set(int(j), int(faceBox.Top())+1, c)
		}
		for j := faceBox.Top(); j < faceBox.Bottom(); j++ {
			cimg.Set(int(faceBox.Left())-1, int(j), c)
			cimg.Set(int(faceBox.Left()), int(j), c)
			cimg.Set(int(faceBox.Left())+1, int(j), c)
			cimg.Set(int(faceBox.Right())-1, int(j), c)
			cimg.Set(int(faceBox.Right()), int(j), c)
			cimg.Set(int(faceBox.Right())+1, int(j), c)
		}
	}

	return cimg
}
