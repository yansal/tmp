package hooks

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
)

// Wrap wraps an http.RoundTripper.
func Wrap(rt http.RoundTripper) http.RoundTripper {
	return &transport{wrapped: rt}
}

type transport struct{ wrapped http.RoundTripper }

func (tr *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqbody, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	reqb, err := ioutil.ReadAll(reqbody)
	if err != nil {
		return nil, err
	}
	log.Printf("%s\n", reqb)
	resp, err := tr.wrapped.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Printf("%s\n", respb)
	resp.Body = ioutil.NopCloser(bytes.NewReader(respb))
	return resp, nil

}
