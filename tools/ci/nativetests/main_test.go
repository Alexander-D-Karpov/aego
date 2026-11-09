package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestPackagesFromList(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        []string
	}{
		{"valid", "aego/app\naego/tests/integration\naego/ui\n", []string{"aego/app", integration, "aego/ui"}},
		{"missing integration", "aego/app\naego/ui\n", nil},
		{"integration only", integration + "\n", nil},
		{"duplicate integration", integration + "\n" + integration + "\naego/app\n", nil},
		{"duplicate other", "aego/app\n" + integration + "\naego/app\n", nil},
		{"unexpected package", integration + "\nunexpected/package\n", nil},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := packagesFromList(tc.input)
			if tc.want == nil {
				if err == nil {
					t.Fatalf("invalid input accepted: %v", got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestRunTestsCoversBothGroupsAndPropagatesFailures(t *testing.T) {
	for _, fail := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+fail)), func(t *testing.T) {
			var calls [][]string
			err := runTests([]string{"aego/app", integration, "aego/ui"}, func(args ...string) error {
				calls = append(calls, append([]string(nil), args...))
				if fail&(1<<(len(calls)-1)) != 0 {
					return errors.New("failed")
				}
				return nil
			})
			want := [][]string{
				{"test", "-count=1", "aego/app", "aego/ui"},
				{"test", "-count=1", "-timeout=30m", integration},
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("commands = %v, want %v", calls, want)
			}
			if (err != nil) != (fail != 0) {
				t.Fatalf("fail mask %d: got error %v", fail, err)
			}
		})
	}
}
