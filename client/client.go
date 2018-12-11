// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

/*
This package provides an HTTP client to use with pm-enabled processes.
*/
package client

import (
	"github.com/VividCortex/pm"

	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type Client struct {
	*http.Client
	BaseURI string
	Headers map[string]string
}

// NewClient returns a new client set to connect to the given URI.
func NewClient(uri string) *Client {
	return &Client{
		Client:  &http.Client{},
		BaseURI: uri,
		Headers: map[string]string{
			"Accept":       "application/json",
			"Content-Type": "application/json",
			"User-Agent":   "go-pm",
		},
	}
}

func (c *Client) makeRequest(verb, endpoint string, body, result interface{}) error {
	buf := new(bytes.Buffer)
	if body != nil {
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(verb, c.BaseURI+endpoint, buf)
	if err != nil {
		return err
	}
	for header, value := range c.Headers {
		req.Header.Add(header, value)
	}

	resp, err := c.Do(req)
	if err == nil && resp.StatusCode > 299 {
		msg := fmt.Sprintf("HTTP Status Code %d from %s %s\n", resp.Status, verb, c.BaseURI+endpoint)
		err = errors.New(msg)
	}

	if resp != nil {
		defer resp.Body.Close()
		if err == nil && result != nil {
			json.NewDecoder(resp.Body).Decode(result)
		}
	}
	return err
}

// Processes issues a GET to /proc, thus retrieving the full process list from
// the server. The result is provided as a ProcResponse.
func (c *Client) Processes() (*pm.ProcResponse, error) {
	var result pm.ProcResponse
	if err := c.makeRequest("GET", "/procs/", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// History issues a GET to /proc/<id>/history for a given id, thus returning the
// complete history for the task <id> at the server.
func (c *Client) History(id string) (*pm.HistoryResponse, error) {
	var result pm.HistoryResponse
	endpoint := fmt.Sprintf("/procs/%s/history", id)

	if err := c.makeRequest("GET", endpoint, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Kill requests the cancellation of a given task. Note that it will effectively
// be cancelled as soon as the task reaches its next cancellation point.
func (c *Client) Kill(id, message string) error {
	body := pm.CancelRequest{Message: message}
	endpoint := fmt.Sprintf("/procs/%s", id)

	if err := c.makeRequest("DELETE", endpoint, body, nil); err != nil {
		return err
	}
	return nil
}
