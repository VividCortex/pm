// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

package pm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func attrMapEquals(m1, m2 map[string]interface{}) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v1 := range m1 {
		if v2, present := m2[k]; !present || v1 != v2 {
			return false
		}
	}
	return true
}

func attrMap(t *testing.T, p *ProcDetail) map[string]interface{} {
	attrs := make(map[string]interface{})
	for _, attr := range p.Attrs {
		if _, present := attrs[attr.Name]; present {
			t.Error("attribute doubly defined:", attr.Name)
		}
		attrs[attr.Name] = attr.Value
	}
	return attrs
}

func procMapEquals(t *testing.T, m1, m2 map[string]ProcDetail) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v1 := range m1 {
		if v2, present := m2[k]; !present {
			return false
		} else if v1.Id != v2.Id || v1.Status != v2.Status || v1.Cancelling != v2.Cancelling {
			return false
		} else if !attrMapEquals(attrMap(t, &v1), attrMap(t, &v2)) {
			return false
		}
	}
	return true
}

func procMap(t *testing.T, pr *ProcResponse) map[string]ProcDetail {
	procs := make(map[string]ProcDetail)
	for _, proc := range pr.Procs {
		if _, present := procs[proc.Id]; present {
			t.Error("process doubly defined:", proc.Id)
		}
		procs[proc.Id] = proc
	}
	return procs
}

func checkProcResponse(t *testing.T, reply, expected *ProcResponse) {
	if !procMapEquals(t, procMap(t, reply), procMap(t, expected)) {
		t.Fatal("bad proclist result")
	}
}

func checkHistoryResponse(t *testing.T, reply, expected *HistoryResponse) {
	if len(reply.History) != len(expected.History) {
		t.Fatalf("bad history length; expected %d, received %d",
			len(expected.History), len(reply.History))
	}

	for i := range reply.History {
		if reply.History[i].Status != expected.History[i].Status {
			t.Fatal("bad history received")
		}
	}
}

func TestProclist(t *testing.T) {
	attrs1 := map[string]interface{}{
		"method": "GET",
		"uri":    "/hosts/1",
		"host":   "localhost:15233",
	}
	attrs2 := map[string]interface{}{
		"method": "PUT",
		"uri":    "/hosts/2/config",
		"host":   "localhost:12538",
	}
	Start("req1", &ProcOpts{ForbidCancel: true}, attrs1)
	defer Done("req1")
	Start("req2", &ProcOpts{StopCancelPanic: true}, attrs2)

	req1Status := []string{
		"init",
		"searching",
		"reading",
		"sending",
	}
	for _, s := range req1Status[1:] {
		Status("req1", s)
	}

	procs := DefaultProclist.getProcs()
	if len(procs) != 2 {
		t.Fatalf("len(procs) = %d; expecting 2", len(procs))
	}

	var p1, p2 ProcDetail
	if procs[0].Id == "req1" && procs[1].Id == "req2" {
		p1, p2 = procs[0], procs[1]
	} else if procs[0].Id == "req2" && procs[1].Id == "req1" {
		p1, p2 = procs[1], procs[0]
	} else {
		t.Fatalf("unexpected procs found: %s, %s", procs[0].Id, procs[1].Id)
	}

	if p1.Cancelling || p2.Cancelling {
		t.Error("cancellation pending but never enabled")
	}

	if p1.Status != req1Status[len(req1Status)-1] {
		t.Error("bad status for req1; expecting '",
			req1Status[len(req1Status)-1], "', got ", p1.Status)
	}
	if p2.Status != "init" {
		t.Error("bad status for req2; expecting 'init', got ", p2.Status)
	}

	if !attrMapEquals(attrMap(t, &p1), attrs1) {
		t.Error("bad attribute set for req1")
	}
	if !attrMapEquals(attrMap(t, &p2), attrs2) {
		t.Error("bad attribute set for req1")
	}

	func() {
		defer Done("req2")
		Kill("req2", "")
		Status("req2", "searching")
		t.Error("req2 was not cancelled when it had to")
	}()

	func() {
		defer func() {
			if e := recover(); e != nil {
				if _, ok := e.(CancelErr); ok {
					t.Fatal("req2 was incorrectly cancelled")
				} else {
					panic(e)
				}
			}
		}()

		Kill("req1", "")
		CheckCancel("req1")
	}()

	history, err := DefaultProclist.getHistory("req1")
	if err != nil {
		t.Fatal("unable to retrieve history")
	}
	for i, item := range history {
		if item.Status != req1Status[i] {
			t.Error("bad status at position %d; got %s, expected %s",
				i, item.Status, req1Status[i])
		}
	}
}

type Client struct {
	*http.Client
	BaseURI string
	Headers map[string]string
}

func newClient(uri string) *Client {
	return &Client{
		Client:  &http.Client{},
		BaseURI: uri,
		Headers: map[string]string{
			"Accept":       "application/json",
			"User-Agent":   "go-pm",
			"Content-Type": "application/json",
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
	if err == nil && resp.StatusCode != 200 {
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

func (c *Client) getProcs(t *testing.T) *ProcResponse {
	var result ProcResponse
	if err := c.makeRequest("GET", "/procs/", nil, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}

func (c *Client) getHistory(t *testing.T, id string) *HistoryResponse {
	var result HistoryResponse
	endpoint := fmt.Sprintf("/procs/%s/history", id)

	if err := c.makeRequest("GET", endpoint, nil, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}

func (c *Client) kill(t *testing.T, id, message string) {
	body := CancelRequest{Message: message}
	endpoint := fmt.Sprintf("/procs/%s", id)

	if err := c.makeRequest("DELETE", endpoint, body, nil); err != nil {
		t.Fatal(err)
	}
}

func TestHttpServer(t *testing.T) {
	procs := []struct {
		id      string
		status  []string
		readyCh chan struct{}
		exitCh  chan struct{}
	}{
		{id: "req1", status: []string{"S11", "S12", "S13"}},
		{id: "req2", status: []string{"S21", "S22"}},
	}

	SetOptions(ProclistOpts{StopCancelPanic: true})
	portError := make(chan error, 1)

	go func() {
		if err := ListenAndServe(":6680"); err != nil {
			// t.Fatal() has to be called in the main routine
			portError <- err
		}
	}()

	select {
	case err := <-portError:
		t.Fatal(err)
	case <-time.After(time.Duration(100) * time.Millisecond):
	}

	for i := range procs {
		procs[i].readyCh = make(chan struct{}, 1)
		procs[i].exitCh = make(chan struct{}, 1)

		go func(i int) {
			Start(procs[i].id, nil, map[string]interface{}{})
			defer Done(procs[i].id)
			for _, s := range procs[i].status {
				Status(procs[i].id, s)
			}

			procs[i].readyCh <- struct{}{}
			<-procs[i].exitCh
		}(i)
	}

	for _, p := range procs {
		<-p.readyCh
	}

	c := newClient("http://localhost:6680")
	checkProcResponse(t, c.getProcs(t), &ProcResponse{
		Procs: []ProcDetail{
			ProcDetail{
				Id:         "req1",
				Status:     "S13",
				Cancelling: false,
			},
			ProcDetail{
				Id:         "req2",
				Status:     "S22",
				Cancelling: false,
			},
		},
	})

	procs[1].exitCh <- struct{}{} // req2
	time.Sleep(time.Duration(100) * time.Millisecond)

	checkProcResponse(t, c.getProcs(t), &ProcResponse{
		Procs: []ProcDetail{
			ProcDetail{
				Id:         "req1",
				Status:     "S13",
				Cancelling: false,
			},
		},
	})

	checkHistoryResponse(t, c.getHistory(t, "req1"), &HistoryResponse{
		History: []HistoryDetail{
			HistoryDetail{Status: "init"},
			HistoryDetail{Status: "S11"},
			HistoryDetail{Status: "S12"},
			HistoryDetail{Status: "S13"},
		},
	})

	c.kill(t, "req1", "my message")
	checkHistoryResponse(t, c.getHistory(t, "req1"), &HistoryResponse{
		History: []HistoryDetail{
			HistoryDetail{Status: "init"},
			HistoryDetail{Status: "S11"},
			HistoryDetail{Status: "S12"},
			HistoryDetail{Status: "S13"},
			HistoryDetail{Status: "[cancel request: my message]"},
		},
	})

	procs[0].exitCh <- struct{}{} // req1
	time.Sleep(time.Duration(100) * time.Millisecond)
	checkProcResponse(t, c.getProcs(t), &ProcResponse{})

	for _, p := range procs {
		p.exitCh <- struct{}{}
	}
}
