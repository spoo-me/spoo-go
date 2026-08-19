package spoo

import (
	"encoding/json"
	"testing"
)

func TestOptTriStateMarshal(t *testing.T) {
	type payload struct {
		Password  Opt[string] `json:"password,omitzero"`
		MaxClicks Opt[int]    `json:"max_clicks,omitzero"`
		BlockBots Opt[bool]   `json:"block_bots,omitzero"`
	}

	for name, tc := range map[string]struct {
		in   payload
		want string
	}{
		"all omitted": {payload{}, `{}`},
		"null clears": {payload{Password: Null[string]()}, `{"password":null}`},
		"set value":   {payload{Password: Set("hunter22")}, `{"password":"hunter22"}`},
		"set zeroes":  {payload{MaxClicks: Set(0), BlockBots: Set(false)}, `{"max_clicks":0,"block_bots":false}`},
	} {
		data, err := json.Marshal(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(data) != tc.want {
			t.Errorf("%s: marshal = %s, want %s", name, data, tc.want)
		}
	}
}

func TestOptUnmarshal(t *testing.T) {
	var o Opt[int]
	if err := json.Unmarshal([]byte(`7`), &o); err != nil {
		t.Fatal(err)
	}
	if v, ok := o.Value(); !ok || v != 7 {
		t.Fatalf("o = %+v", o)
	}
	if err := json.Unmarshal([]byte(`null`), &o); err != nil {
		t.Fatal(err)
	}
	if !o.IsNull() {
		t.Fatalf("o = %+v, want null", o)
	}
}

func TestOptAccessors(t *testing.T) {
	var omitted Opt[string]
	if !omitted.IsZero() || omitted.IsNull() {
		t.Fatalf("zero Opt = %+v", omitted)
	}
	if _, ok := omitted.Value(); ok {
		t.Fatal("omitted must carry no value")
	}
	if Null[string]().IsZero() {
		t.Fatal("null is not omitted")
	}
}
