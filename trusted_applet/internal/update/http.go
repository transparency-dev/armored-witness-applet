// Copyright 2023 The Armored Witness Applet authors. All Rights Reserved.
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

package update

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/transparency-dev/armored-witness-applet/api"
)

// NewLogClient returns a log client that uses the HTTP client to fetch
// resources from the log with the given base URL.
func NewLogClient(logAddress url.URL, client http.Client) LogClient {
	return &httpLogClient{
		logAddress: logAddress,
		client:     client,
	}
}

type httpLogClient struct {
	logAddress url.URL
	client     http.Client
}

func (c *httpLogClient) GetLeafAndInclusion(index, treeSize uint64) ([]byte, [][]byte, error) {
	// TODO(alcutter): use serverless API to fetch the leaf
	location := c.logAddress.JoinPath(fmt.Sprintf("/TODO/v1/getLeaf/%d", index))
	leaf, err := c.getBody(location)
	// TODO(alcutter): user serverless API to generate inclusion proof
	return leaf, nil, err
}

func (c *httpLogClient) GetBinary(release api.FirmwareRelease) ([]byte, error) {
	// TODO(mhutchinson): determine real URL
	location := c.logAddress.JoinPath(fmt.Sprintf("/TODO/v1/getBinary/%x", release.FirmwareDigestSha256))
	return c.getBody(location)
}

func (c *httpLogClient) GetLatestCheckpoint() ([]byte, error) {
	// TODO(alcutter): use serverless API to fetch the checkpoint
	location := c.logAddress.JoinPath("/TODO/v1/getCheckpoint")
	return c.getBody(location)
}

func (c *httpLogClient) getBody(location *url.URL) ([]byte, error) {
	resp, err := c.client.Get(location.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get %q: %v", location, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non-OK error code from %q: %d", location, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %v", err)
	}
	return body, nil
}
