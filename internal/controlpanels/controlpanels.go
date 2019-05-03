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
	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/pkg/errors"
)

// Scheduler controls
type Scheduler struct {
	cfg           cfgparser.ControlPanelsCFG
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
	}
}

// Face ...
type Face struct {
	Box []facerecognition.FaceBox `json:"box"`
}

// ImageNotification contains notification for control panel.
type ImageNotification struct {
	ImgBuff   string                    `json:"img_buff"`
	FaceBoxes []facerecognition.FaceBox `json:"face_boxes"`
}

// Notify sends notifications to all control panels.
func (cps *Scheduler) Notify(imgBuff []byte, faceBoxes []facerecognition.FaceBox) error {
	idx := 0
	url := cps.cfg.ControlPanels[idx]
	var cli *http.Client
	if cps.clients[idx] != nil {
		cli = cps.clients[idx]
	} else {
		cps.idx++
		cli = cps.defaultClient
	}

	base64ImgBuff := make([]byte, base64.StdEncoding.EncodedLen(len(imgBuff)))
	base64.StdEncoding.Encode(base64ImgBuff, imgBuff)
	data, err := json.Marshal(ImageNotification{ImgBuff: string(base64ImgBuff)})
	if err != nil {
		return errors.Wrap(err, "unable to marshal image buffer to JSON")
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "unable to create http-request")
	}

	resp, err := cli.Do(req)
	if err != nil {
		return errors.Wrap(err, "unable to get response from faceprocessor")
	}

	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "unable to read response from body")
	}
	fmt.Println(string(buff))

	return nil
}
