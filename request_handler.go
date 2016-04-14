package main

import (
	"bytes"
	_ "github.com/GeoNet/log/logentries"
	"log"
	"net/http"
	"strings"
)

type result struct {
	ok   bool   // set true to indicated success
	code int    // http status code for writing back the client e.g., http.StatusOK for success.
	msg  string // any error message for logging or to send to the client.
}

var (
	statusOK         = result{ok: true, code: http.StatusOK, msg: ""}
	methodNotAllowed = result{ok: false, code: http.StatusMethodNotAllowed, msg: "method not allowed"}
	notFound         = result{ok: false, code: http.StatusNotFound, msg: ""}
	notAcceptable    = result{ok: false, code: http.StatusNotAcceptable, msg: "specify accept"}
)

/*
requestHandler for handling http requests.  The response for the request
should be written into.
*/
type requestHandler func(w http.ResponseWriter, r *http.Request, b *bytes.Buffer) *result

func internalServerError(err error) *result {
	return &result{ok: false, code: http.StatusInternalServerError, msg: err.Error()}
}

func badRequest(message string) *result {
	return &result{ok: false, code: http.StatusBadRequest, msg: message}
}

func notFoundError(message string) *result {
	return &result{ok: false, code: http.StatusNotFound, msg: message}
}

/*
checkQuery inspects r and makes sure all required query parameters
are present and that no more than the required and optional parameters
are present.
*/
func checkQuery(r *http.Request, required, optional []string) *result {
	if strings.Contains(r.URL.Path, ";") {
		return badRequest("cache buster")
	}

	v := r.URL.Query()

	if len(required) == 0 && len(optional) == 0 {
		if len(v) == 0 {
			return &statusOK
		} else {
			return badRequest("found unexpected query parameters")
		}
	}

	var missing []string

	for _, k := range required {
		if v.Get(k) == "" {
			missing = append(missing, k)
		} else {
			v.Del(k)
		}
	}

	switch len(missing) {
	case 0:
	case 1:
		return badRequest("missing required query parameter: " + missing[0])
	default:
		return badRequest("missing required query parameters: " + strings.Join(missing, ", "))
	}

	for _, k := range optional {
		v.Del(k)
	}

	if len(v) > 0 {
		return badRequest("found additional query parameters")
	}

	return &statusOK
}

// copied from request_handler.go from mtr/mtr_api/.  We could unify later.
func toHandler(f requestHandler) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		// the default content type, wrapped functions can overload if necessary
		w.Header().Set("Content-Type", "text/html")

		switch r.Method {
		// case "PUT", "DELETE":
		// 	// PUT and DELETE do not have a response body for the client so pass a nil buffer.
		// 	res := f(w, r, nil)

		// 	switch res.code {
		// 	case http.StatusOK:
		// 		w.WriteHeader(http.StatusOK)
		// 	case http.StatusInternalServerError:
		// 		http.Error(w, res.msg, res.code)
		// 		log.Printf("500 serving %s %s %s", r.Method, r.URL, res.msg)
		// 	default:
		// 		http.Error(w, res.msg, res.code)
		// 	}

		case "GET":
			var b bytes.Buffer
			res := f(w, r, &b)

			switch res.code {
			case http.StatusOK:
				b.WriteTo(w)
			case http.StatusInternalServerError:
				http.Error(w, res.msg, res.code)
				log.Printf("500 serving GET %s %s", r.URL, res.msg)
			default:
				http.Error(w, res.msg, res.code)
			}

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
