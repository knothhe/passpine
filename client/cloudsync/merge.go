package cloudsync

import (
	"bytes"
	"encoding/json"
	"sort"
)

func equalJSON(a, b json.RawMessage) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// Canonicalizing JSON avoids conflicts caused only by whitespace or map key
	// order while preserving numbers exactly (no float64 conversion).
	var av, bv any
	ad, bd := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	ad.UseNumber()
	bd.UseNumber()
	if ad.Decode(&av) != nil || bd.Decode(&bv) != nil {
		return bytes.Equal(a, b)
	}
	aa, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return bytes.Equal(aa, bb)
}

func validateValues(v Values) error {
	if len(v) > 4096 {
		return &Error{Code: "invalid_document"}
	}
	for k, raw := range v {
		if k == "" || len(k) > 256 || len(raw) > MaxDocumentBytes || !json.Valid(raw) {
			return &Error{Code: "invalid_document"}
		}
	}
	encoded, err := json.Marshal(v)
	if err != nil || len(encoded) > MaxDocumentBytes {
		return &Error{Code: "invalid_document"}
	}
	return nil
}

func cloneValues(v Values) Values {
	out := make(Values, len(v))
	for k, raw := range v {
		out[k] = append(json.RawMessage(nil), raw...)
	}
	return out
}

// Merge performs a field-level three-way merge. Unknown remote keys survive
// unless a caller explicitly removes a key present in its own baseline. A nil
// baseline denotes initial setup and imports existing remote values.
func Merge(local, baseline, remote Values) (Values, error) {
	for _, v := range []Values{local, baseline, remote} {
		if err := validateValues(v); err != nil {
			return nil, err
		}
	}
	out := cloneValues(remote)
	if baseline == nil {
		for k, v := range local {
			if _, exists := remote[k]; !exists {
				out[k] = append(json.RawMessage(nil), v...)
			}
		}
		return out, validateValues(out)
	}
	keys := make(map[string]bool)
	for _, v := range []Values{local, baseline, remote} {
		for k := range v {
			keys[k] = true
		}
	}
	var conflicts []string
	for k := range keys {
		lv, bv, rv := local[k], baseline[k], remote[k]
		localChanged := !equalJSON(lv, bv)
		remoteChanged := !equalJSON(rv, bv)
		if localChanged && remoteChanged && !equalJSON(lv, rv) {
			conflicts = append(conflicts, k)
			continue
		}
		if localChanged {
			if lv == nil {
				delete(out, k)
			} else {
				out[k] = append(json.RawMessage(nil), lv...)
			}
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return nil, &Error{Code: "conflict", Keys: conflicts}
	}
	return out, validateValues(out)
}
