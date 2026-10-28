package archive

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestDeterministicStandardJSONPreservesNumbersAndUnicode(t *testing.T) {
	raw := `{"z":"<>&\u2028\u2029/\b\u0001😀Кириллица","a":[1.0,1e-7,900719925474099312345,1.2345678901234567890123456789,-0.0]}`
	value, e := Decode(raw)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Canonical(value)
	if e != nil {
		t.Fatal(e)
	}
	want := `{"a":[1.0,1e-7,900719925474099312345,1.2345678901234567890123456789,-0.0],"z":"<>&\u2028\u2029/\b\u0001😀Кириллица"}`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatal("unexpected framing newline")
	}
	decoded, e := Decode(got)
	if e != nil || !reflect.DeepEqual(decoded, value) {
		t.Fatal("JSON values changed", decoded, e)
	}
	for i := 0; i < 10; i++ {
		again, e := Canonical(value)
		if e != nil || again != got {
			t.Fatal("nondeterministic encoding", again, e)
		}
	}
	float, e := Canonical(map[string]any{"float": float64(1)})
	if e != nil || float != `{"float":1}` {
		t.Fatal(float, e)
	}
}
func TestSerializationRejectsMalformedValues(t *testing.T) {
	badText := string([]byte{0xff})
	for _, value := range []any{badText, map[string]any{badText: "value"}, map[string]any{"nested": []any{badText}}, []string{badText}, []map[string]any{{"text": badText}}, math.Inf(1), math.NaN(), json.Number("1e9999"), json.Number("NaN")} {
		if raw, e := Canonical(value); e == nil {
			t.Fatalf("accepted malformed value %T as %s", value, raw)
		}
	}
}
