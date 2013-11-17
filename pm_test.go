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
	Status("proc2", "waiting")
	r := Contents()
	c := fmt.Sprint(r)
	t.Log("\n" + c)
	if !strings.Contains(c, "proc2  waiting") {
		t.Error("proc2 isn't waiting")
	}
	if !strings.Contains(c, "123   the quick") {
		t.Error("bad alignment on 123   the quick")
	}
}
