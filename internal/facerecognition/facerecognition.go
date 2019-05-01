package facerecognition

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"
	"unsafe"

	"strings"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
)

const (
	unixSock = byte(0)
	inetSock = byte(1)
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

const (
	uint64Var  = uint64(0)
	float64Var = float64(0.0)
)

const (
	uint64Size  = uint64(unsafe.Sizeof(uint64Var))
	float64Size = uint64(unsafe.Sizeof(float64Var))
)

func readUint64(buff []byte, shift uint64) uint64 {
	return *(*uint64)(unsafe.Pointer(&buff[shift]))
}

func readFloat64(buff []byte, shift uint64) float64 {
	return *(*float64)(unsafe.Pointer(&buff[shift]))
}

// FaceBox struct represents coordinates of one face in image.
type FaceBox struct {
	Top    uint64
	Right  uint64
	Bottom uint64
	Left   uint64
}

// FacialFeatures datatype represents facial features vector of one face.
type FacialFeatures []float64

// Face struct is a pair of FaceBox and FacialFeatures.
type Face struct {
	FaceBox        FaceBox
	FacialFeatures FacialFeatures
}

// BytesToImage converts bytes buffer to Golang image.
func BytesToImage(imgBuff []byte) (image.Image, string, error) {
	return image.Decode(bytes.NewReader(imgBuff))
}

// GetFaces returns slice of Faces from image buffer.
func (ffs *Scheduler) GetFaces(imgBuff []byte) ([]Face, error) {
	ffs.mu.RLock()
	idx := ffs.idx % uint64(len(ffs.cfg.FaceProcessors))
	url := ffs.cfg.FaceProcessors[idx]
	var cli *http.Client
	if ffs.clients[idx] != nil {
		cli = ffs.clients[idx]
	} else {
		ffs.idx++
		cli = ffs.defaultClient
	}
	ffs.mu.RUnlock()

	req, err := http.NewRequest("PUT", url, bytes.NewReader(imgBuff))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create http-request")
	}

	resp, err := cli.Do(req)
	if err != nil {
		ffs.mu.Lock()
		if ffs.cfg.FaceProcessors[idx] == url {
			ffs.cfg.FaceProcessors = append(ffs.cfg.FaceProcessors[:idx], ffs.cfg.FaceProcessors[idx+1:]...)
			ffs.clients = append(ffs.clients[:idx], ffs.clients[idx+1:]...)
		}
		ffs.mu.Unlock()

		return nil, errors.Wrap(err, "unable to get response from faceprocessor")
	}

	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read response from body")
	}

	/*
		Expected, that faces buff data file will be produced by script facerecognition.py,
		placed in this directory.
		Also it is expected that Go uint64 and float64 sizes and structures are similar to
		Python numpy.uint64 and numpy.float64 on one machine.
		It firstly stores all FaceBoxes (every FaceBox is sequence of 4 uint64 numbers),
		and secondly - all FacialFeatures (every FacialFeatures is sequence of FacialFeaturesSize float64 numbers).
		So number of faces if: nfaces := fsize / (4 * uint64Size + FaceFeaturesSize * float64Size).
	*/

	fbsz := 4 * uint64Size
	ffsz := ffs.cfg.FacialFeaturesSize * float64Size
	sz := fbsz + ffsz
	nfaces := uint64(len(buff)) / sz
	if uint64(len(buff))%sz != 0 {
		return nil, fmt.Errorf("response body is corrupted")
	}

	faces := make([]Face, 0, nfaces)
	for i := uint64(0); i < nfaces; i++ {
		fb := FaceBox{
			Top:    readUint64(buff, i*sz+0*uint64Size),
			Right:  readUint64(buff, i*sz+1*uint64Size),
			Bottom: readUint64(buff, i*sz+2*uint64Size),
			Left:   readUint64(buff, i*sz+3*uint64Size),
		}
		ff := make([]float64, 0, sz)
		for j := uint64(0); j < ffs.cfg.FacialFeaturesSize; j++ {
			ff = append(ff, readFloat64(buff, i*sz+fbsz+j*float64Size))
		}
		faces = append(faces, Face{
			FaceBox:        fb,
			FacialFeatures: ff,
		})
	}

	return faces, nil
}

// FrameFaces puts every face, found in image, to green box.
func FrameFaces(img image.Image, faces []Face) error {
	r := uint8(0x6B)
	g := uint8(0xF3)
	b := uint8(0x08)
	a := uint8(0xFF)

	c := color.RGBA{r, g, b, a}

	type changeable interface {
		Set(x, y int, c color.Color)
	}

	cimg, ok := img.(changeable)
	if !ok {
		return fmt.Errorf("image is readonly")
	}

	for _, face := range faces {
		faceBox := face.FaceBox
		for j := faceBox.Left; j < faceBox.Right; j++ {
			cimg.Set(int(j), int(faceBox.Bottom)-1, c)
			cimg.Set(int(j), int(faceBox.Bottom), c)
			cimg.Set(int(j), int(faceBox.Bottom)+1, c)
			cimg.Set(int(j), int(faceBox.Top)-1, c)
			cimg.Set(int(j), int(faceBox.Top), c)
			cimg.Set(int(j), int(faceBox.Top)+1, c)
		}
		for j := faceBox.Top; j < faceBox.Bottom; j++ {
			cimg.Set(int(faceBox.Left)-1, int(j), c)
			cimg.Set(int(faceBox.Left), int(j), c)
			cimg.Set(int(faceBox.Left)+1, int(j), c)
			cimg.Set(int(faceBox.Right)-1, int(j), c)
			cimg.Set(int(faceBox.Right), int(j), c)
			cimg.Set(int(faceBox.Right)+1, int(j), c)
		}
	}

	return nil
}
