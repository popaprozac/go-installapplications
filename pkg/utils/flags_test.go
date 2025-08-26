package utils

import (
	"reflect"
	"testing"
)

func TestNormalizeBooleanFlags(t *testing.T) {
	args := []string{"cmd", "--debug", "false", "--name", "value", "--verbose", "true"}
	out := NormalizeBooleanFlags(args, map[string]struct{}{"debug": {}, "verbose": {}})
	expected := []string{"cmd", "--debug=false", "--name", "value", "--verbose=true"}
	if !reflect.DeepEqual(out, expected) {
		t.Fatalf("unexpected normalization: %#v", out)
	}
}

func TestMultiValueHeader(t *testing.T) {
	var mvh MultiValueHeader
	if err := mvh.Set("A=B"); err != nil {
		t.Fatal(err)
	}
	if err := mvh.Set("C=D=E"); err != nil {
		t.Fatal(err)
	} // first '=' splits
	if err := mvh.Set("X"); err != nil {
		t.Fatal(err)
	}
	got := mvh.Headers
	want := map[string]string{"A": "B", "C": "D=E", "X": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("headers mismatch: got %#v want %#v", got, want)
	}
}
