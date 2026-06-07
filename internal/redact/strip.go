package redact

import (
	"reflect"
	"strings"
)

// StripLocalOnly removes the configured local_only_fields dot-paths from a
// record, in place, producing the pushable projection (ETCH-41). It returns
// the configured paths that actually stripped something, in configuration
// order, for the record's local_only_stripped manifest.
//
// This is a targeted path-descent walker sharing walk.go's reflection idioms
// (pointer/interface deref, json-tag resolution, settability handling) —
// deliberately distinct from DeepRedact's exhaustive traversal: it visits
// only the addressed paths.
//
// Path grammar: dot-separated JSON field names as they appear in
// session.json. Struct fields resolve by json tag, map entries by key, and
// arrays fan out implicitly (the remaining path applies to every element).
// A path ending at an object/array strips the whole subtree. Paths that
// match nothing in this record — or whose value is already zero — are
// no-ops. Paths covering a required identity field (see requiredPaths) are
// silently skipped.
//
// Strip semantics at the addressed value: strings (and *string) are replaced
// with a "[LOCAL_ONLY:<path>]" marker, consistent with the redaction marker
// style; anything that cannot carry a string is zeroed (pointers/slices/maps
// to null, numbers/bools/structs to their zero value). The manifest is the
// authoritative, type-safe record of what was stripped.
//
// The canonical input is *schema.Session; v must be a non-nil pointer to a
// struct, anything else is a no-op.
func StripLocalOnly(v any, fields []string) []string {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return nil
	}

	var applied []string
	seen := make(map[string]bool)
	for _, path := range fields {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] || isProtectedPath(path) {
			continue
		}
		seen[path] = true
		if stripPath(rv.Elem(), strings.Split(path, "."), path) {
			applied = append(applied, path)
		}
	}
	return applied
}

// requiredPaths are the etch.session.v1 fields a stripped record must keep to
// stay a valid, identifiable record: structural identity plus the fields
// OUTPUT_SPEC marks required, plus the strip manifest itself.
var requiredPaths = []string{
	"schema_version",
	"session_id",
	"status",
	"agent.runtime",
	"local_only_stripped",
}

// isProtectedPath reports whether stripping path would remove a required
// field: path is protected iff some required path equals it or sits beneath
// it (stripping "agent" whole would take required "agent.runtime" with it).
// Paths deeper than a required field (e.g. "agent.model") are fine.
func isProtectedPath(path string) bool {
	for _, req := range requiredPaths {
		if req == path || strings.HasPrefix(req, path+".") {
			return true
		}
	}
	return false
}

// stripPath descends v along segs and strips the addressed value. Returns
// true if anything was actually stripped (non-zero value cleared or marked).
func stripPath(v reflect.Value, segs []string, fullPath string) bool {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return false
		}
		return stripPath(v.Elem(), segs, fullPath)

	case reflect.Interface:
		if v.IsNil() {
			return false
		}
		// Not addressable behind the interface: copy out, descend the copy,
		// re-box (same idiom as walkValue).
		dyn := v.Elem()
		cp := reflect.New(dyn.Type()).Elem()
		cp.Set(dyn)
		if !stripPath(cp, segs, fullPath) {
			return false
		}
		if v.CanSet() {
			v.Set(cp)
			return true
		}
		return false

	case reflect.Struct:
		seg := segs[0]
		for i := 0; i < v.NumField(); i++ {
			ft := v.Type().Field(i)
			if !ft.IsExported() || jsonFieldName(ft) != seg {
				continue
			}
			f := v.Field(i)
			if len(segs) == 1 {
				return stripValue(f, fullPath)
			}
			return stripPath(f, segs[1:], fullPath)
		}
		return false

	case reflect.Slice, reflect.Array:
		// Implicit fan-out: the remaining path applies to every element.
		stripped := false
		for i := 0; i < v.Len(); i++ {
			if stripPath(v.Index(i), segs, fullPath) {
				stripped = true
			}
		}
		return stripped

	case reflect.Map:
		if v.IsNil() || v.Type().Key().Kind() != reflect.String {
			return false
		}
		seg := segs[0]
		key := reflect.New(v.Type().Key()).Elem()
		key.SetString(seg)
		val := v.MapIndex(key)
		if !val.IsValid() {
			return false
		}
		// Map values are not addressable: copy out, mutate, write back.
		cp := reflect.New(val.Type()).Elem()
		cp.Set(val)
		var stripped bool
		if len(segs) == 1 {
			stripped = stripValue(cp, fullPath)
		} else {
			stripped = stripPath(cp, segs[1:], fullPath)
		}
		if stripped {
			v.SetMapIndex(key, cp)
		}
		return stripped

	default:
		return false
	}
}

// stripValue clears the addressed value in place: marker for string-bearing
// values, zero for everything else. Returns false when the value is already
// zero (nothing to hide — stripping would only manufacture a marker) or
// cannot be set.
func stripValue(v reflect.Value, fullPath string) bool {
	marker := "[LOCAL_ONLY:" + fullPath + "]"

	// Deref *string so it carries the marker rather than going null.
	if v.Kind() == reflect.Ptr && !v.IsNil() && v.Elem().Kind() == reflect.String {
		v = v.Elem()
	}

	if !v.CanSet() || v.IsZero() {
		return false
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(marker)
	case reflect.Interface:
		if reflect.TypeOf(marker).AssignableTo(v.Type()) {
			v.Set(reflect.ValueOf(marker))
		} else {
			v.Set(reflect.Zero(v.Type()))
		}
	default:
		v.Set(reflect.Zero(v.Type()))
	}
	return true
}

// jsonFieldName resolves the JSON object key for a struct field: the json
// tag name when present, the Go field name otherwise.
func jsonFieldName(ft reflect.StructField) string {
	tag := ft.Tag.Get("json")
	if tag == "" {
		return ft.Name
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return ft.Name
	}
	return name
}
