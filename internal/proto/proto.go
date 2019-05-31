package proto

import (
	"time"
)

// Header contains basic JSON headers, which
// every message should have.
type Header struct {
	SrcAddr string `json:"src_addr"`
	UUID    string `json:"uuid"` // UUID of message (used for identifying images).
}

// FaceBox is tuple of 4 integers: top, right, bottom and left of image.
type FaceBox []uint64

// Top ...
func (fb FaceBox) Top() uint64 {
	return fb[0]
}

// Right ...
func (fb FaceBox) Right() uint64 {
	return fb[1]
}

// Bottom ...
func (fb FaceBox) Bottom() uint64 {
	return fb[2]
}

// Left ...
func (fb FaceBox) Left() uint64 {
	return fb[3]
}

// FacialFeaturesVector is tuple of 128 floats in [-1.0, 1.0] diapason.
type FacialFeaturesVector []float64

// FaceData is a pair of FaceBox and FacialFeaturesVector.
type FaceData struct {
	FaceBox              FaceBox              `json:"facebox"`
	FacialFeaturesVector FacialFeaturesVector `json:"facial_features_vector"`
}

const (
	// InvalidRequestMethodCode ...
	InvalidRequestMethodCode = -1
	// CorruptedBodyCode ...
	CorruptedBodyCode = -2
	// UnableToEnqueue ...
	UnableToEnqueue = -3
	// UnableToSend ...
	UnableToSend = -4
	// InternalServerError ...
	InternalServerError = -5
)

// ErrorData describes error.
type ErrorData struct {
	Code int64  `json:"code"`
	Info string `json:"info"`
	Text string `json:"text"`
}

// ImmedResp ...
type ImmedResp struct {
	Header    Header     `json:"header"`
	ErrorData *ErrorData `json:"error_data"`
}

// PutImageReq is sent from camera microservices of from GUI client.
type PutImageReq struct {
	Header    Header    `json:"header"`
	ImgBuff   string    `json:"img_buff"`
	FaceBoxes []FaceBox `json:"faceboxes"`
}

// ProcessImageReq is sent from DB server to facerecognition microservices.
type ProcessImageReq struct {
	Header    Header    `json:"header"`
	ImgBuff   string    `json:"img_buff"`
	FaceBoxes []FaceBox `json:"faceboxes"`
}

// PutFacesDataReq is sent from facerecognition microservices to DB server.
type PutFacesDataReq struct {
	Header    Header     `json:"header"`
	ErrorData *ErrorData `json:"error_data"`
	FacesData []FaceData `json:"faces_data"`
}

// DefaultStringField ...
const DefaultStringField = "-"

const (
	// MaleSex ...
	MaleSex = `male`
	// FemaleSex ...
	FemaleSex = `female`
	// UnknowSex ...
	UnknowSex = DefaultStringField
)

// ControlObject describes one control object (human in DB).
type ControlObject struct {
	// Special DB fields.
	ID   string `json:"id"`
	DBTS *time.Time
	TS   time.Time
	// Business-Logic fields.
	Passport   string `json:"passport"`
	Surname    string `json:"surname"`
	Name       string `json:"name"`
	Patronymic string `json:"patronymic"`
	Sex        string `json:"sex"`
	BirthDate  string `json:"birthdate"`
	PhoneNum   string `json:"phone_num"`
	Email      string `json:"email"`
	Address    string `json:"address"`
}

// CreateDefaultControlObject creates ControlObject with
// all fields set to default value.
func CreateDefaultControlObject() *ControlObject {
	return &ControlObject{
		ID:         DefaultStringField,
		DBTS:       nil,
		TS:         time.Now(),
		Passport:   DefaultStringField,
		Surname:    DefaultStringField,
		Name:       DefaultStringField,
		Patronymic: DefaultStringField,
		Sex:        UnknowSex,
		BirthDate:  DefaultStringField,
		PhoneNum:   DefaultStringField,
		Email:      DefaultStringField,
		Address:    DefaultStringField,
	}
}

// CompareByID returns true if ControlObjects ID's are same.
func (cob0 *ControlObject) CompareByID(cob1 *ControlObject) bool {
	return cob0.ID == cob1.ID
}

// CompareByPassport returns true if ControlObjects PassPort's are same.
func (cob0 *ControlObject) CompareByPassport(cob1 *ControlObject) bool {
	return cob0.Passport == cob1.Passport
}

// Compare returns true if ControlObjects are fully equal.
func (cob0 *ControlObject) Compare(cob1 *ControlObject) bool {

	return cob0.CompareByID(cob1) &&
		(cob0.Passport == cob1.Passport) &&
		(cob0.Surname == cob1.Surname) &&
		(cob0.Name == cob1.Name) &&
		(cob0.Patronymic == cob1.Patronymic) &&
		(cob0.Sex == cob1.Sex) &&
		(cob0.PhoneNum == cob1.PhoneNum) &&
		(cob0.Email == cob1.Email) &&
		(cob0.Address == cob1.Address)
}

// ImageControlObject is a pair of ControlObject and FaceData.
type ImageControlObject struct {
	ControlObject ControlObject `json:"control_object"`
	FaceBox       FaceBox       `json:"facebox"`
}

// NotifyControlReq is sent from DB server to GUI client.
type NotifyControlReq struct {
	Header              Header               `json:"header"`
	ImgBuff             string               `json:"img_buff"`
	ImageControlObjects []ImageControlObject `json:"image_control_objects"`
}

const (
	// SubmitCommand pushes all data to DB.
	SubmitCommand = `submit`
	// CancelCommand drops request.
	CancelCommand = `cancel`
	// ProcessAgainCommand processes image again with marked faces.
	ProcessAgainCommand = `process_again`
)

// PutControlReq is sent from GUI client to DB server.
type PutControlReq struct {
	Header              Header               `json:"header"`
	Command             string               `json:"command"`
	ImageControlObjects []ImageControlObject `json:"image_control_objects"`
}

// ControlObjectPart is a part of AddControlObjectReq.
type ControlObjectPart struct {
	ControlObject ControlObject `json:"control_object"`
	ImagesNum     uint64        `json:"images_num"`
}

// ImagePart is a part of AddControlObjectReq.
type ImagePart struct {
	CurrNum uint64  `json:"curr_num"`
	ImgBuff string  `json:"img_buff"`
	FaceBox FaceBox `json:"facebox"`
}

// AddControlObjectReq is sent from GUI client to DB server.
type AddControlObjectReq struct {
	Header            Header             `json:"header"`
	ControlObjectPart *ControlObjectPart `json:"control_object_part"`
	ImagePart         *ImagePart         `json:"image_part"`
}

// AddControlObjectResp is sent from DB server to GUI client.
type AddControlObjectResp struct {
	Header    Header     `json:"header"`
	ErrorData *ErrorData `json:"error_data"`
}
