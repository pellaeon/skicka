//
// http.go
// Copyright(c)2014-2015 Google, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"time"
)

type addKeyTransport struct {
	transport http.RoundTripper
	key       string
}

func (akt addKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.RawQuery != "" {
		req.URL.RawQuery += "&"
	}
	req.URL.RawQuery += "key=" + akt.key
	return akt.transport.RoundTrip(req)
}

type LoggingTransport struct {
	transport http.RoundTripper
}

func (lt LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	dumpBody := false
	if ct, ok := req.Header["Content-Type"]; ok && len(ct) == 1 && ct[0] != "application/octet-stream" {
		dumpBody = true
	}

	dump, err := httputil.DumpRequestOut(req, dumpBody)
	if err != nil {
		// Don't report an error back from RoundTrip() just because
		// DumpRequestOut() ran into trouble.
		debug.Printf("error dumping http request: %v", err)
	}

	resp, err := lt.transport.RoundTrip(req)

	if resp != nil && dumpBody {
		respBody, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = ioutil.NopCloser(bytes.NewReader(respBody))
		log.Printf("http request: %s--->response: %+v\nresponse body: %s\n--->err: %v",
			sanitize(string(dump)), resp, string(respBody), err)
	} else {
		log.Printf("http request: %s--->response: %+v\n--->err: %v", sanitize(string(dump)),
			resp, err)
	}

	return resp, err
}

type flakyTransport struct {
	transport http.RoundTripper
	rng       *rand.Rand
	endTime   time.Time
}

func NewFlakyTransport(transport http.RoundTripper) http.RoundTripper {
	seed := time.Now().UTC().UnixNano()
	log.Printf("Flaky rand seed %d", seed)
	return flakyTransport{transport: transport, rng: rand.New(rand.NewSource(seed))}
}

func (ft flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if time.Now().After(ft.endTime) {
		if ft.rng.Float32() > .03 {
			return ft.transport.RoundTrip(req)
		}
		delta := time.Duration(ft.rng.Int31()%(90*1000)) * time.Millisecond
		ft.endTime = time.Now().Add(delta)
		debug.Printf("Flaky http for %s", delta.String())
	}

	reqstr := sanitize(fmt.Sprintf("%+v", req))
	if (ft.rng.Int() % 2) == 0 {
		codes := []int{401, 403, 404, 408, 500, 503}
		c := codes[int(ft.rng.Int31())%len(codes)]
		debug.Printf("Dropping http request %s -> %d", reqstr, c)
		return &http.Response{
				Body:       ioutil.NopCloser(bytes.NewReader([]byte("flaky error body"))),
				Status:     fmt.Sprintf("%d Flaky Error", c),
				StatusCode: c,
				Request:    req},
			nil
	}
	debug.Printf("Returning error from http request %s", reqstr)
	return nil, fmt.Errorf("flaky http error")
}
