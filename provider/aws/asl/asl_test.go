package asl

import (
	"encoding/json"
	"strings"
	"testing"
)

func parse(t *testing.T, src string) any {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCount(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		want    Transitions
		wantErr string
	}{
		{
			// The pricing page's first example: Start, two tasks, End.
			name: "two tasks",
			src:  `{"StartAt":"Upload","States":{"Upload":{"Type":"Task","Next":"Delete"},"Delete":{"Type":"Task","End":true}}}`,
			want: Transitions{PerExecution: 4},
		},
		{
			name: "a choice takes its longest branch; Catch paths are left out",
			src: `{"StartAt":"Check","States":{
				"Check":{"Type":"Task","Next":"Kind","Catch":[{"ErrorEquals":["States.ALL"],"Next":"Cleanup"}]},
				"Kind":{"Type":"Choice","Choices":[{"Next":"Long"},{"Next":"Bad"}],"Default":"Done"},
				"Long":{"Type":"Pass","Next":"Longer"},"Longer":{"Type":"Pass","Next":"Done"},
				"Cleanup":{"Type":"Pass","Next":"A"},"A":{"Type":"Pass","Next":"B"},"B":{"Type":"Pass","Next":"C"},"C":{"Type":"Pass","Next":"Bad"},
				"Bad":{"Type":"Fail"},"Done":{"Type":"Succeed"}}}`,
			want: Transitions{PerExecution: 7},
		},
		{
			name: "every state of every Parallel branch counts",
			src: `{"StartAt":"Both","States":{"Both":{"Type":"Parallel","End":true,"Branches":[
				{"StartAt":"A","States":{"A":{"Type":"Task","End":true}}},
				{"StartAt":"B","States":{"B":{"Type":"Task","Next":"C"},"C":{"Type":"Pass","End":true}}}]}}}`,
			want: Transitions{PerExecution: 6},
		},
		{
			name: "an inline Map runs its states per item, in a Parallel branch too",
			src: `{"StartAt":"Both","States":{"Both":{"Type":"Parallel","Next":"Each","Branches":[
				{"StartAt":"Old","States":{"Old":{"Type":"Map","End":true,"Iterator":{"StartAt":"X","States":{"X":{"Type":"Task","End":true}}}}}}]},
				"Each":{"Type":"Map","End":true,"ItemProcessor":{"ProcessorConfig":{"Mode":"INLINE"},"StartAt":"Y","States":{"Y":{"Type":"Task","Next":"Z"},"Z":{"Type":"Pass","End":true}}}}}}`,
			want: Transitions{PerExecution: 5, PerItem: 3},
		},
		{
			name: "a branch with more per-item states is longer",
			src: `{"StartAt":"Kind","States":{
				"Kind":{"Type":"Choice","Choices":[{"Next":"Each"}],"Default":"A"},
				"A":{"Type":"Pass","Next":"B"},"B":{"Type":"Pass","Next":"C"},"C":{"Type":"Pass","End":true},
				"Each":{"Type":"Map","End":true,"ItemProcessor":{"StartAt":"Y","States":{"Y":{"Type":"Task","End":true}}}}}}`,
			want: Transitions{PerExecution: 4, PerItem: 1},
		},
		{
			name:    "a loop",
			src:     `{"StartAt":"Wait","States":{"Wait":{"Type":"Wait","Next":"Ready"},"Ready":{"Type":"Choice","Choices":[{"Next":"Done"}],"Default":"Wait"},"Done":{"Type":"Succeed"}}}`,
			wantErr: "the states loop (Wait → Ready → Wait)",
		},
		{
			name:    "a nested Map",
			src:     `{"StartAt":"Outer","States":{"Outer":{"Type":"Map","End":true,"ItemProcessor":{"StartAt":"Inner","States":{"Inner":{"Type":"Map","End":true,"ItemProcessor":{"StartAt":"X","States":{"X":{"Type":"Pass","End":true}}}}}}}}}`,
			wantErr: `the Map state "Inner" runs inside another Map`,
		},
		{
			name:    "a distributed Map",
			src:     `{"StartAt":"Each","States":{"Each":{"Type":"Map","End":true,"ItemProcessor":{"ProcessorConfig":{"Mode":"DISTRIBUTED","ExecutionType":"EXPRESS"},"StartAt":"X","States":{"X":{"Type":"Pass","End":true}}}}}}`,
			wantErr: `the Map state "Each" is distributed`,
		},
		{
			name:    "a type not known yet",
			src:     `{"StartAt":"A","States":{"A":{"Type":"Teleport","End":true}}}`,
			wantErr: `state "A" has type Teleport, which is not counted yet`,
		},
		{
			name:    "a missing state",
			src:     `{"StartAt":"A","States":{"A":{"Type":"Pass","Next":"Gone"}}}`,
			wantErr: `state "Gone" is not defined`,
		},
		{
			name:    "not a state machine",
			src:     `{"Version":"2012-10-17"}`,
			wantErr: "no StartAt or States",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Count(parse(t, c.src))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("Count = %+v, %v; want %+v", got, err, c.want)
			}
		})
	}
}
