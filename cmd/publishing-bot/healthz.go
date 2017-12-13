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
)

type Healthz struct {
	Issue int

	mutex   sync.RWMutex
	healthy *bool
}

type HealthResponse struct {
	LastSuccessfull *bool  `json:"lastSuccessful,omitempty"`
	Issue           string `json:"issue,omitempty"`
}

func (h *Healthz) SetHealth(healthy bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.healthy = &healthy
}

func (h *Healthz) Run(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handler)
	return http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), mux)
}

func (h *Healthz) handler(w http.ResponseWriter, r *http.Request) {
	h.mutex.RLock()
	resp := HealthResponse{
		LastSuccessfull: h.healthy,
	}
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
