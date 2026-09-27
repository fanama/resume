package model

import "reflect"

// isEmpty reports whether a section entry carries no information at all: an
// entry whose every string field and every slice field is blank.
func isEmpty(v any) bool {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return rv.IsZero()
	}
	t := rv.Type()
	for i := range t.NumField() {
		f := rv.Field(i)
		switch f.Kind() {
		case reflect.String:
			if f.String() != "" {
				return false
			}
		case reflect.Slice:
			if f.Len() > 0 {
				return false
			}
		}
	}
	return true
}
