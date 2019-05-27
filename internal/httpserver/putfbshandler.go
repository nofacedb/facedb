package httpserver

import "net/http"

const apiV1PutFBs = "/api/v1/put_fbs"

func (rest *restAPI) putFBsHandler(resp http.ResponseWriter, req *http.Request) {
	rest.logger.Debugf("got \"%s\" message", apiV1PutFBs)
}
