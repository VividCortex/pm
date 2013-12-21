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
		err = errors.New("HTTP Status Code: " + resp.Status)
	}

	if resp != nil {
		defer resp.Body.Close()
		if err == nil && result != nil {
			json.NewDecoder(resp.Body).Decode(result)
		}
	}
	return err
}

func (c *Client) Processes() (*pm.ProcResponse, error) {
	var result pm.ProcResponse
	if err := c.makeRequest("GET", "/proc", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Journal(id string) (*pm.JournalResponse, error) {
	var result pm.JournalResponse
	endpoint := fmt.Sprintf("/proc/%s/journal", id)

	if err := c.makeRequest("GET", endpoint, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Kill(id, message string) error {
	body := pm.CancelRequest{Message: message}
	endpoint := fmt.Sprintf("/proc/%s/cancel", id)

	if err := c.makeRequest("PUT", endpoint, body, nil); err != nil {
		return err
	}
	return nil
}
