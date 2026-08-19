package spoo

import "encoding/json"

// Opt is a tri-state optional for the few update fields where the API
// distinguishes JSON null from an absent key (null clears the setting,
// absent keeps it). The zero value is omitted; build the other states
// with [Set] and [Null]. Fields tagged omitzero drop omitted values
// from the request body.
//
// Everywhere the API does not make that distinction, request fields are
// plain values — Opt never spreads beyond the update endpoints.
type Opt[T any] struct {
	value T
	state optState
}

type optState uint8

const (
	optOmitted optState = iota
	optSet
	optNull
)

// Set returns an Opt carrying v.
func Set[T any](v T) Opt[T] {
	return Opt[T]{value: v, state: optSet}
}

// Null returns an Opt that serializes as JSON null.
func Null[T any]() Opt[T] {
	return Opt[T]{state: optNull}
}

// IsZero reports whether the Opt is omitted, wiring it into
// encoding/json's omitzero handling.
func (o Opt[T]) IsZero() bool { return o.state == optOmitted }

// IsNull reports whether the Opt is an explicit null.
func (o Opt[T]) IsNull() bool { return o.state == optNull }

// Value returns the carried value and whether one is set.
func (o Opt[T]) Value() (T, bool) {
	return o.value, o.state == optSet
}

// MarshalJSON writes the carried value, or null for the other states
// (omitted values are dropped earlier by omitzero tags).
func (o Opt[T]) MarshalJSON() ([]byte, error) {
	if o.state == optSet {
		return json.Marshal(o.value)
	}
	// Omitted values only reach here without an omitzero tag; null is
	// the only faithful spelling for both remaining states.
	return []byte("null"), nil
}

// UnmarshalJSON reads null as [Null] and any value as [Set].
func (o *Opt[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*o = Null[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*o = Set(v)
	return nil
}
