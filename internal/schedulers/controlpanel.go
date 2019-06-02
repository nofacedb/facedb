package schedulers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// AwaitingControlObject is awaiting control objects queue element.
type AwaitingControlObject struct {
	TS                time.Time
	SrcAddr           string
	UUID              string
	ControlObjectPart *proto.ControlObjectPart
	ImagesNum         uint64
	Mu                sync.Mutex
	Images            map[string]proto.ImagePart
	FacesData         map[string]proto.FaceData
}

// CreateAwaitingControlObject ...
func CreateAwaitingControlObject() *AwaitingControlObject {
	return &AwaitingControlObject{
		TS: time.Now(),
	}
}

// AwaitingControlObjectsQueue enqueues requests to add control object to DB.
type AwaitingControlObjectsQueue struct {
	cleanTimeoutMS int
	maxSize        int
	active         bool
	queue          map[string]*AwaitingControlObject
	mu             sync.Mutex
	logger         *log.Logger
}

// CreateAwaitingControlObjectsQueue returns new queue for awaiting control objects.
func CreateAwaitingControlObjectsQueue(cleanTimeoutMS, maxSize int, logger *log.Logger) *AwaitingControlObjectsQueue {
	q := &AwaitingControlObjectsQueue{
		cleanTimeoutMS: cleanTimeoutMS,
		maxSize:        maxSize,
		active:         true,
		queue:          make(map[string]*AwaitingControlObject),
		mu:             sync.Mutex{},
		logger:         logger,
	}
	q.runCleaner()
	return q
}

// Clean cleanups old data.
func (q *AwaitingControlObjectsQueue) Clean() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.logger.Debug("scheduled cleaning of outdated \"AddControlObjectReq\"s")

	for k := range q.queue {
		if time.Now().Sub(q.queue[k].TS) > time.Duration(q.cleanTimeoutMS)*time.Millisecond {
			q.logger.Infof("removing outdated awaiting \"AddControlObjectReq\" with key: %v", k)
			delete(q.queue, k)
		}
	}
}

func (q *AwaitingControlObjectsQueue) runCleaner() {
	go func() {
		if !q.active {
			return
		}
		time.Sleep(time.Duration(q.cleanTimeoutMS) * time.Millisecond)
		q.Clean()
	}()
}

// Push ...
func (q *AwaitingControlObjectsQueue) Push(k string, v *AwaitingControlObject) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) > q.maxSize {
		return fmt.Errorf("attempt to exceed maximum \"AddControlObjectReq\"s queue size")
	}

	if _, ok := q.queue[k]; ok {
		return fmt.Errorf("\"AddControlObjectReq\"s with key \"%s\" already exists", k)
	}

	q.queue[k] = v

	return nil
}

// Pop ...
func (q *AwaitingControlObjectsQueue) Pop(k string) *AwaitingControlObject {
	q.mu.Lock()
	defer q.mu.Unlock()

	v, ok := q.queue[k]
	if ok {
		delete(q.queue, k)
	}

	return v
}

// Get ...
func (q *AwaitingControlObjectsQueue) Get(k string) *AwaitingControlObject {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.queue[k]
}

// PushWithCheck ...
func (q *AwaitingControlObjectsQueue) PushWithCheck(k string, v *AwaitingControlObject) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, ok := q.queue[k]
	if ok {
		return false, nil
	}

	if len(q.queue) > q.maxSize {
		return false, fmt.Errorf("attempt to exceed maximum \"AddControlObjectReq\"s queue size")
	}

	if _, ok := q.queue[k]; ok {
		return false, fmt.Errorf("\"AddControlObjectReq\"s with key \"%s\" already exists", k)
	}

	q.queue[k] = v

	return true, nil
}

// GetAwaitingCobByImgID ...
func (q *AwaitingControlObjectsQueue) GetAwaitingCobByImgID(imgK string) *AwaitingControlObject {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, v := range q.queue {
		if _, ok := v.Images[imgK]; ok {
			return v
		}
	}
	return nil
}

// AwaitingControl is awaiting control queue element.
type AwaitingControl struct {
	TS                    time.Time
	SrcAddr               string
	UUID                  string
	ImgBuff               string
	ImageControlObjects   []proto.ImageControlObject
	FacialFeaturesVectors []proto.FacialFeaturesVector
}

// AwaitingControlsQueue enqueues requests to process images on client.
type AwaitingControlsQueue struct {
	cleanTimeoutMS int
	maxSize        int
	active         bool
	queue          map[string]*AwaitingControl
	mu             sync.Mutex
	logger         *log.Logger
}

// CreateAwaitingControlsQueue returns new queue for awaiting control objects.
func CreateAwaitingControlsQueue(cleanTimeoutMS, maxSize int, logger *log.Logger) *AwaitingControlsQueue {
	q := &AwaitingControlsQueue{
		cleanTimeoutMS: cleanTimeoutMS,
		maxSize:        maxSize,
		active:         true,
		queue:          make(map[string]*AwaitingControl),
		mu:             sync.Mutex{},
		logger:         logger,
	}
	q.runCleaner()
	return q
}

// Clean cleanups old data.
func (q *AwaitingControlsQueue) Clean() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.logger.Debugf("scheduled cleaning of outdated \"NotifyControlReq\"s")

	for k := range q.queue {
		if time.Now().Sub(q.queue[k].TS) > time.Duration(q.cleanTimeoutMS)*time.Millisecond {
			q.logger.Infof("removing outdated \"NotifyControlReq\" with key: %v", k)
			delete(q.queue, k)
		}
	}
}

func (q *AwaitingControlsQueue) runCleaner() {
	go func() {
		if !q.active {
			return
		}
		time.Sleep(time.Duration(q.cleanTimeoutMS) * time.Millisecond)
		q.Clean()
	}()
}

// Push ...
func (q *AwaitingControlsQueue) Push(k string, v *AwaitingControl) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) > q.maxSize {
		return fmt.Errorf("attempt to exceed maximum \"NotifyControlReq\"s queue size")
	}

	if _, ok := q.queue[k]; ok {
		return fmt.Errorf("\"NotifyControlReq\"s with key \"%s\" already exists", k)
	}

	q.queue[k] = v

	return nil
}

// Pop ...
func (q *AwaitingControlsQueue) Pop(k string) *AwaitingControl {
	q.mu.Lock()
	defer q.mu.Unlock()

	v, ok := q.queue[k]
	if ok {
		delete(q.queue, k)
	}

	return v
}

// ControlPanelScheduler handles all control tasks.
type ControlPanelScheduler struct {
	srcAddr       string
	controlPanels []string
	ACOQ          *AwaitingControlObjectsQueue
	ACQ           *AwaitingControlsQueue
	client        *http.Client
	logger        *log.Logger
}

// CreateControlPanelScheduler returns new ControlPanels Scheduler.
func CreateControlPanelScheduler(cfg *cfgparser.ControlPanelsCFG, srcAddr string,
	client *http.Client, logger *log.Logger) *ControlPanelScheduler {
	return &ControlPanelScheduler{
		srcAddr:       srcAddr,
		controlPanels: cfg.ControlPanels,
		ACOQ: CreateAwaitingControlObjectsQueue(
			cfg.ACOQCleanMS,
			cfg.ACOQMaxSize,
			logger),
		ACQ: CreateAwaitingControlsQueue(
			cfg.ACQCleanMS,
			cfg.ACQMaxSize,
			logger),
		client: client,
		logger: logger,
	}
}

// GetControlPanelsNum ...
func (s *ControlPanelScheduler) GetControlPanelsNum() int {
	return len(s.controlPanels)
}

const (
	cpsAPIBase                   = "/api/v1"
	cpsAPINotifyControl          = cpsAPIBase + "/notify_control"
	cpsAPINotifyAddControlObject = cpsAPIBase + "/notify_add_control_object"
)

// SendNotifyControlReq requests client decision about photo.
func (s *ControlPanelScheduler) SendNotifyControlReq(req *proto.NotifyControlReq, broadCast bool, url string) error {
	if broadCast {
		return s.sendNotifyControlReqBroadCast(req)
	}
	return s.sendNotifyControlReq(req, url)
}

func (s *ControlPanelScheduler) sendNotifyControlReqBroadCast(req *proto.NotifyControlReq) error {
	data, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "unable to marshal \"NotifyControlReq\" to JSON")
	}

	wg := sync.WaitGroup{}
	wellDone := uint64(0)
	for i := 0; i < len(s.controlPanels); i++ {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			httpReq, err := http.NewRequest("PUT", url, bytes.NewReader(data))
			if err != nil {
				s.logger.Error(errors.Wrap(err, "unable to create \"NotifyControlReq\" HTTP request"))
				return
			}
			_, err = s.client.Do(httpReq)
			if err != nil {
				s.logger.Error(errors.Wrapf(err, "unable to send \"NotifyControlReq\" to controlpanel \"%s\"", url))
			}
			atomic.AddUint64(&wellDone, 1)
		}(s.controlPanels[i] + cpsAPINotifyControl)
	}

	wg.Wait()

	if wellDone == 0 {
		return fmt.Errorf("unable to send any \"NotifyControlReq\" in broadcast mode")
	}

	return nil
}

func (s *ControlPanelScheduler) sendNotifyControlReq(req *proto.NotifyControlReq, baseURL string) error {
	url := baseURL + cpsAPINotifyControl
	data, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "unable to marshal NotifyControlReq to JSON")
	}

	httpReq, err := http.NewRequest("PUT", url+cpsAPINotifyControl, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "unable to create NotifyControlReq HTTP request")
	}

	_, err = s.client.Do(httpReq)
	if err != nil {
		return errors.Wrapf(err, "unable to send NotifyControlReq to controlpanel \"%s\"", url)
	}
	return nil
}

// SendAddControlObjectResp sends notification about adding control object to requester.
func (s *ControlPanelScheduler) SendAddControlObjectResp(req *proto.AddControlObjectResp, broadCast bool, baseURL string) error {
	if broadCast {
		return s.sendAddControlObjectRespBroadCast(req)
	}
	return s.sendAddControlObjectResp(req, baseURL)
}

func (s *ControlPanelScheduler) sendAddControlObjectRespBroadCast(req *proto.AddControlObjectResp) error {
	data, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "unable to marshal AddControlObjectResp to JSON")
	}

	wg := sync.WaitGroup{}
	wellDone := uint64(0)
	for i := 0; i < len(s.controlPanels); i++ {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			httpReq, err := http.NewRequest("PUT", url, bytes.NewReader(data))
			if err != nil {
				s.logger.Error(errors.Wrap(err, "unable to create HTTP request"))
				return
			}

			_, err = s.client.Do(httpReq)
			if err != nil {
				s.logger.Error(errors.Wrapf(err, "unable to send AddControlObjectResp to controlpanel \"%s\"", url))
			}
			atomic.AddUint64(&wellDone, 1)
		}(s.controlPanels[i] + cpsAPINotifyAddControlObject)
	}

	wg.Wait()

	if wellDone == 0 {
		return fmt.Errorf("unable to send any AddControlObjectResp in broadcast mode")
	}

	return nil
}

func (s *ControlPanelScheduler) sendAddControlObjectResp(req *proto.AddControlObjectResp, baseURL string) error {
	url := baseURL + cpsAPINotifyAddControlObject
	data, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "unable to marshal AddControlObjectResp to JSON")
	}

	httpReq, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "unable to create AddControlObjectResp HTTP request")
	}

	_, err = s.client.Do(httpReq)
	if err != nil {
		return errors.Wrapf(err, "unable to send AddControlObjectResp resp to controlpanel \"%s\"", url)
	}
	return nil
}
