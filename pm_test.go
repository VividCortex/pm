// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.
package pm

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestBasicFunctionality(t *testing.T) {
	SetCols("col1", "long_col")
	Start("proc1", map[string]interface{}{"col1": 123, "long_col": "the quick brown fox jumped over the lazy red dog"})
	defer Done("proc1")
	Start("proc2", map[string]interface{}{"col1": 345, "long_col": "it was the best of times, it was the worst of times"})
	defer Done("proc2")
	Status("proc2", "wait")
	c := fmt.Sprint(Contents())
	t.Log("\n" + c)
	if !strings.Contains(c, "proc2  wait") {
		t.Error("proc2 isn't wait")
	}
	if !strings.Contains(c, "123   the quick") {
		t.Error("bad alignment on 123   the quick")
	}
	if !strings.Contains(c, "proc2  wait        0") {
		t.Error("bad alignment on proc2  wait        0")
	}

	Done("proc2")
	c = fmt.Sprint(Contents())
	t.Log("\n" + c)
	if strings.Contains(c, "proc2") {
		t.Error("proc2 still exists")
	}

	var msg string
	Kill("proc1", "houston, we have a problem")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		Status("proc1", "this should die")()
	}()
	time.Sleep(time.Second) // wait for the panic
	if !strings.Contains(msg, "houston, we have a problem") {
		t.Error("kill wasn't successful")
	}

}

func TestNetListener(t *testing.T) {
	SetCols("col1", "long_col")
	Start("proc1", map[string]interface{}{"col1": 123, "long_col": "the quick brown fox jumped over the lazy red dog"})

	go func() {
		t.Log(ListenAndServe(":9999"))
	}()
	time.Sleep(time.Second) // give it time to start listening

	c, err := net.Dial("tcp", ":9999")
	if err != nil {
		t.Error(err)
	}
	defer c.Close()
	buf := make([]byte, 16*1024)
	n, err := c.Read(buf)
	if n > 0 {
		list := string(buf[:n])
		t.Log("\n" + list)
		if !strings.Contains(list, "quick brown fox") {
			t.Error("didn't get quick brown fox from net listener")
		}
	} else if err != nil {
		t.Error(err)
	}
}
