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

// AwaitingImage is awaiting images queue element.
type AwaitingImage struct {
	TS                    time.Time
	SrcAddr               string
	UUID                  string
	ImgBuff               string
	FaceBoxes             []proto.FaceBox
	FacialFeaturesVectors []proto.FacialFeaturesVector
}

// AwaitingImagesQueue ...
type AwaitingImagesQueue struct {
	cleanTimeoutMS int
	maxSize        int
	active         bool
	queue          map[string]*AwaitingImage
	mu             sync.Mutex
	logger         *log.Logger
}

// CreateAwaitingImagesQueue ...
func CreateAwaitingImagesQueue(cleanTimeoutMS, maxSize int, logger *log.Logger) *AwaitingImagesQueue {
	q := &AwaitingImagesQueue{
		cleanTimeoutMS: cleanTimeoutMS,
		maxSize:        maxSize,
		active:         true,
		queue:          make(map[string]*AwaitingImage),
		mu:             sync.Mutex{},
		logger:         logger,
	}
	q.runCleaner()
	return q
}

// Clean cleanups old data.
func (q *AwaitingImagesQueue) Clean() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.logger.Debug("scheduled cleaning of outdated \"PutImageReq\"s")

	for k := range q.queue {
		if time.Now().Sub(q.queue[k].TS) > time.Duration(q.cleanTimeoutMS)*time.Millisecond {
			q.logger.Infof("cleaning outdated \"PutImageReq\" with key \"%s\"", k)
			delete(q.queue, k)
		}
	}
}

func (q *AwaitingImagesQueue) runCleaner() {
	go func() {
		if !q.active {
			return
		}
		time.Sleep(time.Duration(q.cleanTimeoutMS) * time.Millisecond)
		q.Clean()
	}()
}

// Push ...
func (q *AwaitingImagesQueue) Push(k string, v *AwaitingImage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) > q.maxSize {
		return fmt.Errorf("attempt to exceed maximum \"PutImageReq\"s queue size")
	}

	if _, ok := q.queue[k]; ok {
		return fmt.Errorf("\"PutImageReq\"s with key \"%s\" already exists", k)
	}

	q.queue[k] = v

	return nil
}

// Pop ...
func (q *AwaitingImagesQueue) Pop(k string) *AwaitingImage {
	q.mu.Lock()
	defer q.mu.Unlock()

	v, ok := q.queue[k]
	if ok {
		delete(q.queue, k)
	}

	return v
}

// FaceRecognitionScheduler handles all image processing tasks.
type FaceRecognitionScheduler struct {
	srcAddr         string
	client          *http.Client
	faceRecognizers []string
	faceRecIdx      uint64
	AwImgsQ         *AwaitingImagesQueue
	logger          *log.Logger
}

// CreateFaceRecognitionScheduler returns new FaceRecognition Scheduler.
func CreateFaceRecognitionScheduler(cfg *cfgparser.FaceRecognizersCFG, srcAddr string,
	client *http.Client, logger *log.Logger) *FaceRecognitionScheduler {
	return &FaceRecognitionScheduler{
		srcAddr:         srcAddr,
		faceRecognizers: cfg.FaceRecognizers,
		AwImgsQ: CreateAwaitingImagesQueue(
			cfg.AwImgsQCleanMS,
			cfg.AwImgsQMaxSize,
			logger,
		),
		client: client,
		logger: logger,
	}
}

const (
	frsAPIBase                     = "/api/v1"
	frsAPIGetFaceBoxes             = frsAPIBase + "/get_faceboxes"
	frsAPIGetFacialFeaturesVectors = frsAPIBase + "/get_facial_features_vectors"
	frsAPIProcessImage             = frsAPIBase + "/process_image"
)

func (s *FaceRecognitionScheduler) createURL(idx uint64, req *proto.ProcessImageReq) string {
	url := s.faceRecognizers[idx]
	if len(req.FaceBoxes) != 0 {
		url += frsAPIGetFacialFeaturesVectors
	} else {
		url += frsAPIProcessImage
	}
	return url
}

// SendProcessImageReq ...
func (s *FaceRecognitionScheduler) SendProcessImageReq(req *proto.ProcessImageReq) error {
	idx := atomic.AddUint64(&(s.faceRecIdx), 1) % uint64(len(s.faceRecognizers))
	url := s.createURL(idx, req)

	data, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("PUT", url, bytes.NewReader(data))

	_, err := s.client.Do(httpReq)
	if err != nil {
		s.logger.Warn(errors.Wrapf(err,
			"unable to send \"ProcessImageReq\" with key \"%s\" the first time",
			req.Header.UUID))
	} else {
		return nil
	}

	i := (idx + 1) % uint64(len(s.faceRecognizers))
	rn := uint64(1)
	for {
		url = s.createURL(i, req)
		httpReq, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
		_, err = s.client.Do(httpReq)
		if err != nil {
			s.logger.Warn(errors.Wrapf(err,
				"unable to send \"ProcessImageReq\" with key \"%s\" for %d-th extra time",
				req.Header.UUID, rn))
		} else {
			return nil
		}

		i = (i + 1) % uint64(len(s.faceRecognizers))
		rn++
		if i == idx {
			break
		}
	}

	return fmt.Errorf(
		"unable to send \"ProcessImageReq\" with key \"%s\" for all available face recognizers",
		req.Header.UUID)
}
