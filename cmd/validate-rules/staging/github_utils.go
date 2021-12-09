/*
Copyright 2021 The Kubernetes Authors.

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

package staging

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type File struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// fetchKubernetesStagingDirectoryFiles uses the GH API to get the contents
// of the contents/staging/src/k8s.io directory in a specified branch of kubernetes
func fetchKubernetesStagingDirectoryFiles(branch string) ([]File, error) {
	url := "https://api.github.com/repos/kubernetes/kubernetes/contents/staging/src/k8s.io?ref=" + branch

	spaceClient := http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var res *http.Response
	count := 0
	for {
		res, err = spaceClient.Do(req)
		if err != nil {
			return nil, err
		}

		if res.Body != nil {
			defer res.Body.Close()
		}

		if res.StatusCode == http.StatusForbidden {
			// try after some time as we hit GH API limit
			time.Sleep(5 * time.Second)
			count++
		} else {
			// try for 10 mins then give up!
			if count == 120 {
				return nil, fmt.Errorf("hitting github API limits, bailing out")
			}

			break
		}
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}

	var result []File
	jsonErr := json.Unmarshal(body, &result)
	if jsonErr != nil {
		return nil, jsonErr
	}

	return result, nil
}
