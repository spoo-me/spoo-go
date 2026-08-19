package spoo

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimestampUnmarshalMixedFormats(t *testing.T) {
	for wire, want := range map[string]time.Time{
		`1781524800`:                   time.Unix(1781524800, 0).UTC(),
		`1781524800.5`:                 time.Unix(1781524800, 500000000).UTC(),
		`"2026-06-01T10:00:00Z"`:       time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		`"2026-06-01T10:00:00+05:30"`:  time.Date(2026, 6, 1, 10, 0, 0, 0, time.FixedZone("", 5*3600+1800)),
		`"2026-06-01T10:00:00.123456"`: time.Date(2026, 6, 1, 10, 0, 0, 123456000, time.UTC),
		`"2026-06-01"`:                 time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		`null`:                         {},
		`""`:                           {},
	} {
		var ts Timestamp
		if err := json.Unmarshal([]byte(wire), &ts); err != nil {
			t.Errorf("unmarshal %s: %v", wire, err)
			continue
		}
		if want.IsZero() != ts.IsZero() || (!want.IsZero() && !ts.Equal(want)) {
			t.Errorf("unmarshal %s = %v, want %v", wire, ts.Time, want)
		}
	}
}

func TestTimestampUnmarshalRejectsGarbage(t *testing.T) {
	var ts Timestamp
	if err := json.Unmarshal([]byte(`"not a time"`), &ts); err == nil {
		t.Fatal("want an error for an unparsable timestamp")
	}
}

func TestTimestampMarshal(t *testing.T) {
	data, err := json.Marshal(Timestamp{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"2027-01-01T00:00:00Z"` {
		t.Fatalf("marshal = %s", data)
	}
	data, err = json.Marshal(Timestamp{})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "null" {
		t.Fatalf("zero marshal = %s, want null", data)
	}
}

func TestParseExpiry(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	got, err := ParseExpiry("30m", now)
	if err != nil || !got.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("duration: got %v, err %v", got, err)
	}
	got, err = ParseExpiry("1781524800", now)
	if err != nil || !got.Equal(time.Unix(1781524800, 0)) {
		t.Fatalf("epoch: got %v, err %v", got, err)
	}
	got, err = ParseExpiry("2027-01-01T00:00:00Z", now)
	if err != nil || !got.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("iso: got %v, err %v", got, err)
	}
	got, err = ParseExpiry("", now)
	if err != nil || !got.IsZero() {
		t.Fatalf("empty: got %v, err %v", got, err)
	}
	if _, err = ParseExpiry("-5m", now); err == nil {
		t.Fatal("negative duration must error")
	}
	if _, err = ParseExpiry("whenever", now); err == nil {
		t.Fatal("garbage must error")
	}
}
