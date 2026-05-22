package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestItem_ParallelGroupRoundTrips(t *testing.T) {
	body := `{"name":"a","file":"/tmp/a","type":"rootscript","parallel_group":"alpha"}`
	var it Item
	if err := json.Unmarshal([]byte(body), &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if it.ParallelGroup != "alpha" {
		t.Fatalf("ParallelGroup=%q want alpha", it.ParallelGroup)
	}
}

func TestBatchByParallelGroup_Positional(t *testing.T) {
	cases := []struct {
		name string
		in   []string // group names; empty = no group
		want [][]int  // indexes per batch
	}{
		{
			name: "all ungrouped runs as singletons",
			in:   []string{"", "", ""},
			want: [][]int{{0}, {1}, {2}},
		},
		{
			name: "swift example: alpha alpha beta alpha => 3 batches",
			in:   []string{"alpha", "alpha", "beta", "alpha"},
			want: [][]int{{0, 1}, {2}, {3}},
		},
		{
			name: "ungrouped between groups flushes",
			in:   []string{"a", "a", "", "b", "b"},
			want: [][]int{{0, 1}, {2}, {3, 4}},
		},
		{
			name: "single grouped item still counts as a group of 1",
			in:   []string{"a"},
			want: [][]int{{0}},
		},
		{
			name: "leading ungrouped",
			in:   []string{"", "a", "a"},
			want: [][]int{{0}, {1, 2}},
		},
		{
			name: "trailing ungrouped",
			in:   []string{"a", "a", ""},
			want: [][]int{{0, 1}, {2}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]Item, len(tc.in))
			for i, g := range tc.in {
				items[i] = Item{Name: g + "#" + string(rune('a'+i)), File: "/tmp/x", Type: "rootscript", ParallelGroup: g}
			}
			batches := BatchByParallelGroup(items)
			got := make([][]int, len(batches))
			for bi, batch := range batches {
				ids := make([]int, len(batch))
				for ii, it := range batch {
					// Recover the original index from the trailing rune we encoded.
					last := it.Name[len(it.Name)-1]
					ids[ii] = int(last - 'a')
				}
				got[bi] = ids
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("batches got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBatchByParallelGroup_Empty(t *testing.T) {
	if batches := BatchByParallelGroup(nil); batches != nil {
		t.Fatalf("nil input -> nil; got %v", batches)
	}
	if batches := BatchByParallelGroup([]Item{}); batches != nil {
		t.Fatalf("empty input -> nil; got %v", batches)
	}
}
