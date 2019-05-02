package httpserver

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/h2non/filetype"
	"github.com/nofacedb/facedb/internal/facerecognition"
	"github.com/pkg/errors"
	"golang.org/x/image/bmp"
)

func (rest *restAPI) notificationHandler(resp http.ResponseWriter, req *http.Request) {
	imgBuff, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	if rest.immedResp {
		go processImage(rest, imgBuff)
		resp.WriteHeader(http.StatusOK)
		return
	}
	if err := processImage(rest, imgBuff); err != nil {
		fmt.Println(err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

const (
	pngExt  = "png"
	jpegExt = "jpg"
	bmpExt  = "bmp"
)

func processImage(rest *restAPI, imgBuff []byte) error {
	kind, err := filetype.Match(imgBuff)
	if err != nil {
		return errors.Wrap(err, "unable to recognize type of image")
	}
	if kind == filetype.Unknown {
		return fmt.Errorf("unable to recognize type of image")
	}
	var img image.Image
	switch kind.Extension {
	case pngExt:
		img, err = png.Decode(bytes.NewReader(imgBuff))
	case jpegExt:
		img, err = jpeg.Decode(bytes.NewReader(imgBuff))
	case bmpExt:
		img, err = bmp.Decode(bytes.NewReader(imgBuff))
	default:
		return fmt.Errorf("unknown type of image \"%s\"", kind.Extension)
	}
	if err != nil {
		return errors.Wrap(err, "unable to read image from bytes buffer")
	}

	faces, err := rest.frsch.GetFaces(imgBuff)
	if err != nil {
		return err
	}

	r := uint8(0x6B)
	g := uint8(0xF3)
	b := uint8(0x08)
	a := uint8(0xFF)
	c := color.RGBA{r, g, b, a}
	cimg := facerecognition.FrameFaces(img, faces, c)

	out, _ := os.Create("/home/mikhail/kek.png")
	png.Encode(out, cimg)

	return nil
}
