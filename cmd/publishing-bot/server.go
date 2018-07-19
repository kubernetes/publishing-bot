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

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

type Server struct {
	Issue   int
	RunChan chan bool

	mutex    sync.RWMutex
	response HealthResponse
	config   config.Config
}

type HealthBranch struct {
	Name                       string `json:"branchName"`
	UpstreamHash               string `json:"upstreamHash,omitempty"`
	LastSuccessfulUpstreamHash string `json:"lastSuccessfulUpstreamHash,omitempty"`
}

type HealthResponse struct {
	Successful         *bool          `json:"successful,omitempty"`
	Time               *time.Time     `json:"time,omitempty"`
	SourceBranches     []HealthBranch `json:"sourceBranches"`
	Issue              string         `json:"issue,omitempty"`
	LastSuccessfulTime *time.Time     `json:"lastSuccessfulTime,omitempty"`
	LastFailureTime    *time.Time     `json:"lastFailureTime,omitempty"`
}

func (h *Server) SetHealth(healthy bool, hashes map[string]string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.response.Successful = &healthy
	now := time.Now()
	h.response.Time = &now
	for branchName, hash := range hashes {
		b := HealthBranch{
			Name:         branchName,
			UpstreamHash: hash,
		}
		if healthy {
			b.LastSuccessfulUpstreamHash = hash
		}
		h.response.SourceBranches = append(h.response.SourceBranches, b)
	}
	if healthy {
		h.response.LastSuccessfulTime = h.response.Time
		return
	}
	h.response.LastFailureTime = h.response.Time
}

func (h *Server) Run(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/run", h.runHandler)
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	glog.Infof("Listening on %v", addr)
	go func() {
		err := http.ListenAndServe(addr, mux)
		glog.Fatalf("Failed ListenAndServer: %v", err)
	}()
	return nil
}

func (h *Server) runHandler(w http.ResponseWriter, r *http.Request) {
	if h.RunChan == nil {
		http.Error(w, "run channel is closed", http.StatusInternalServerError)
		return
	}
	select {
	case h.RunChan <- true:
	default:
	}
	w.Write([]byte("OK"))
}

func (h *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	h.mutex.RLock()
	resp := h.response
	if h.Issue != 0 {
		// We chose target org so the issue can be opened in different org than
		// a source repository.
		resp.Issue = fmt.Sprintf("https://github.com/%s/%s/issues/%d", h.config.TargetOrg, h.config.SourceRepo, h.Issue)
	}
	h.mutex.RUnlock()

	bytes, err := json.MarshalIndent(resp, "", "\t")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(bytes)
}
