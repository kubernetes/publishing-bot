/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang/glog"
)

type Healthz struct {
	Issue int

	mutex    sync.RWMutex
	response HealthResponse
}

type HealthResponse struct {
	Successful   *bool      `json:"successful,omitempty"`
	Time         *time.Time `json:"time,omitempty"`
	UpstreamHash string     `json:"upstreamHash,omitempty"`

	LastSuccessfulTime         *time.Time `json:"lastSuccessfulTime,omitempty"`
	LastFailureTime            *time.Time `json:"lastFailureTime,omitempty"`
	LastSuccessfulUpstreamHash string     `json:"lastSuccessfulUpstreamHash,omitempty"`

	Issue string `json:"issue,omitempty"`
}

func (h *Healthz) SetHealth(healthy bool, hash string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.response.Successful = &healthy
	now := time.Now()
	h.response.Time = &now
	h.response.UpstreamHash = hash

	if healthy {
		h.response.LastSuccessfulTime = h.response.Time
		h.response.LastSuccessfulUpstreamHash = h.response.UpstreamHash
	} else {
		h.response.LastFailureTime = h.response.Time
	}
}

func (h *Healthz) Run(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handler)
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	glog.Infof("Listening on %v", addr)
	go func() {
		err := http.ListenAndServe(addr, mux)
		glog.Fatalf("Failed ListenAndServer: %v", err)
	}()
	return nil
}

func (h *Healthz) handler(w http.ResponseWriter, r *http.Request) {
	h.mutex.RLock()
	resp := h.response
	if h.Issue != 0 {
		resp.Issue = fmt.Sprintf("https://github.com/kubernetes/kubernetes/issues/%d", h.Issue)
	}
	h.mutex.RUnlock()

	bytes, err := json.MarshalIndent(resp, "", "\t")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(bytes)
}
