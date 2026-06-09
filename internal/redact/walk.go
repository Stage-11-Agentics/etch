package redact

import (
	"reflect"

	"github.com/Stage-11-Agentics/etch/internal/config"
)

// redactor applies the builtin patterns plus custom patterns compiled once,
// so a deep walk doesn't recompile the custom set per string.
type redactor struct {
	custom []secretPattern
}

func newRedactor(settings config.Settings) *redactor {
	return &redactor{custom: compileCustomPatterns(settings.RedactionPatterns)}
}

func (r *redactor) apply(text string) string {
	text = ScanSecrets(text)
	for _, p := range r.custom {
		text = p.Regex.ReplaceAllString(text, "[REDACTED:"+p.Name+"]")
	}
	return text
}

// DeepRedact applies redaction to every string-bearing field reachable from
// v: struct fields, slice elements, map keys and values, pointers, and
// interface-boxed values. This is the single commit-boundary pass — callers
// redact the whole finalized record instead of remembering per-field calls
// (ETCH-40 finding 5). v must be a non-nil pointer; anything else is a no-op.
func DeepRedact(v any, settings config.Settings) {
	if v == nil {
		return
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	walkValue(rv, newRedactor(settings))
}

// walkValue returns a (possibly new) value with all reachable strings
// redacted. Map values and interface-boxed values are not addressable, so
// the walker returns results and containers write them back (SetMapIndex,
// field/index Set) rather than relying on pure in-place mutation.
func walkValue(v reflect.Value, r *redactor) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		s := v.String()
		red := r.apply(s)
		if red == s {
			return v
		}
		nv := reflect.New(v.Type()).Elem() // preserves named string types
		nv.SetString(red)
		return nv

	case reflect.Ptr:
		if v.IsNil() {
			return v
		}
		elem := v.Elem()
		res := walkValue(elem, r)
		if elem.CanSet() {
			elem.Set(res)
		}
		return v

	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		// The dynamic value behind an interface is not addressable:
		// copy it out, walk the copy, and re-box.
		dyn := v.Elem()
		cp := reflect.New(dyn.Type()).Elem()
		cp.Set(dyn)
		res := walkValue(cp, r)
		out := reflect.New(v.Type()).Elem()
		out.Set(res)
		return out

	case reflect.Struct:
		sv := v
		if !sv.CanAddr() {
			cp := reflect.New(v.Type()).Elem()
			cp.Set(v)
			sv = cp
		}
		for i := 0; i < sv.NumField(); i++ {
			f := sv.Field(i)
			if !f.CanSet() { // unexported field
				continue
			}
			f.Set(walkValue(f, r))
		}
		return sv

	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		for i := 0; i < v.Len(); i++ {
			el := v.Index(i)
			el.Set(walkValue(el, r))
		}
		return v

	case reflect.Array:
		av := v
		if !av.CanAddr() {
			cp := reflect.New(v.Type()).Elem()
			cp.Set(v)
			av = cp
		}
		for i := 0; i < av.Len(); i++ {
			el := av.Index(i)
			if el.CanSet() {
				el.Set(walkValue(el, r))
			}
		}
		return av

	case reflect.Map:
		if v.IsNil() {
			return v
		}
		for _, k := range v.MapKeys() {
			val := v.MapIndex(k)
			cv := reflect.New(val.Type()).Elem()
			cv.Set(val)
			nv := walkValue(cv, r)

			nk := k
			if k.Kind() == reflect.String {
				redK := r.apply(k.String())
				if redK != k.String() {
					nk = reflect.New(k.Type()).Elem()
					nk.SetString(redK)
					// Two distinct secret keys can collapse onto the
					// same marker; last write wins (best-effort).
					v.SetMapIndex(k, reflect.Value{}) // delete old key
				}
			}
			v.SetMapIndex(nk, nv)
		}
		return v

	default:
		return v
	}
}
