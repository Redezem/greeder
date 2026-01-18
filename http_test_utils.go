package main

import (
	"bytes"
	"io"
	"net/http"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newResponse(status int, body string, headers map[string]string, req *http.Request) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
		Request:    req,
	}
	for k, v := range headers {
		resp.Header.Set(k, v)
	}
	return resp
}

func clientForResponse(status int, body string, headers map[string]string) *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return newResponse(status, body, headers, r), nil
	})}
}
