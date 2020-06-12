// Copyright 2020 Simon Murray.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file  except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the  License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type AuthenticationResponse struct {
	// JWT authentication token.
	Token string `json:"token"`
}

type Page struct {
	// Number of resources in the full list.
	Count int `json:"count"`

	// Path to the next set of results.
	Next string `json:"next"`

	// Path the the previous set of results.
	Previous string `json:"previous"`

	// Set of results.
	Results []interface{} `json:"results"`
}

type Time struct {
	time.Time
}

func (t *Time) UnmarshalJSON(data []byte) error {
	var s string

	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tt, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}

	t.Time = tt

	return nil
}

type Tag struct {
	// Name of the tag.
	Name string `json:"name"`

	// When it was last updated, but we treat this as when it was created.
	LastUpdated Time `json:"last_updated"`
}

type TagList []Tag

func List(token, url string, results interface{}) error {
	resultsUntyped := []interface{}{}

	for {
		if url == "" {
			break
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", fmt.Sprintf("JWT %s", token))

		client := http.Client{}

		rsp, err := client.Do(req)
		if err != nil {
			return err
		}

		body, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return err
		}

		rsp.Body.Close()

		page := &Page{}
		if err := json.Unmarshal(body, page); err != nil {
			return err
		}

		resultsUntyped = append(resultsUntyped, page.Results...)

		url = page.Next
	}

	resultsRaw, err := json.Marshal(resultsUntyped)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(resultsRaw, results); err != nil {
		return err
	}

	return nil
}

func main() {
	// Docker username.
	var username string

	// Docker password.
	var password string

	// Docker image to interrogate.
	var image string

	// Threshold to delete after.
	var thresholdStr string

	flag.StringVar(&username, "u", "", "Docker Hub user name")
	flag.StringVar(&password, "p", "", "Docker Hub password")
	flag.StringVar(&image, "i", "", "Docker image")
	flag.StringVar(&thresholdStr, "t", "", "Tag age threshold")
	flag.Parse()

	threshold, err := time.ParseDuration(thresholdStr)
	if err != nil {
		fmt.Println("unable to parse threshold:", err)
		os.Exit(1)
	}

	client := http.Client{}

	// Authenticate and get an API token.
	data := map[string]string{
		"username": username,
		"password": password,
	}

	body, err := json.Marshal(data)
	if err != nil {
		fmt.Println("unable to marshal authentication credentials")
		os.Exit(1)
	}

	req, err := http.NewRequest(http.MethodPost, "https://hub.docker.com/v2/users/login/", bytes.NewBuffer(body))
	if err != nil {
		fmt.Println("unable to create authentication request:", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")

	rsp, err := client.Do(req)
	if err != nil {
		fmt.Println("unable to authenticate with registry:", err)
		os.Exit(1)
	}

	body, err = ioutil.ReadAll(rsp.Body)
	if err != nil {
		fmt.Println("unable to read authentication response:", err)
		os.Exit(1)
	}

	rsp.Body.Close()

	authenticationResponse := &AuthenticationResponse{}
	if err := json.Unmarshal(body, authenticationResponse); err != nil {
		fmt.Println("unable to unmarshal authentication response:", err)
		os.Exit(1)
	}

	// Get a list of tags for the requested image.
	tags := TagList{}
	if err := List(authenticationResponse.Token, fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/tags", username, image), &tags); err != nil {
		fmt.Println("unable to list tags:", err)
		os.Exit(0)
	}

	// Reap old tags...
	for _, tag := range tags {
		if time.Since(tag.LastUpdated.Time) > threshold {
			fmt.Println("deleting tag", tag.Name, "age", time.Since(tag.LastUpdated.Time))

			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/tags/%s/", username, image, tag.Name), nil)
			if err != nil {
				fmt.Println("unable to create delete request:", err)
				os.Exit(1)
			}

			req.Header.Add("Authorization", fmt.Sprintf("JWT %s", authenticationResponse.Token))

			rsp, err := client.Do(req)
			if err != nil {
				fmt.Println("unable to perform delete request:", err)
				os.Exit(1)
			}

			rsp.Body.Close()

			if rsp.StatusCode != http.StatusNoContent {
				fmt.Println("unexpected status code", rsp.StatusCode)
			}
		}
	}
}
