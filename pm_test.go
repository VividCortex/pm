package pm

import (
	"fmt"
	"strings"
	"testing"
)

func Test1(t *testing.T) {
	SetCols("col1", "long_col")
	Start("proc1", map[string]interface{}{"col1": 123, "long_col": "the quick brown fox jumped over the lazy red dog"})
	defer Done("proc1")
	Start("proc2", map[string]interface{}{"col1": 345, "long_col": "it was the best of times, it was the worst of times"})
	defer Done("proc2")
	Status("proc2", "wait")
	r := Contents()
	c := fmt.Sprint(r)
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
}
