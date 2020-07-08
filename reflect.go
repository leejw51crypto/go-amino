package amino

import (
	"encoding/json"
	"fmt"
	"reflect"
)

//----------------------------------------
// Constants

var (
	jsonMarshalerType   = reflect.TypeOf(new(json.Marshaler)).Elem()
	jsonUnmarshalerType = reflect.TypeOf(new(json.Unmarshaler)).Elem()
	errorType           = reflect.TypeOf(new(error)).Elem()
)

//----------------------------------------
// encode: see binary-encode.go and json-encode.go
// decode: see binary-decode.go and json-decode.go

//----------------------------------------
// Misc.

func getTypeFromPointer(ptr interface{}) reflect.Type {
	rt := reflect.TypeOf(ptr)
	if rt.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("expected pointer, got %v", rt))
	}
	return rt.Elem()
}

func checkUnsafe(field FieldInfo) {
	if field.Unsafe {
		return
	}
	switch field.TypeInfo.Type.Kind() {
	case reflect.Float32, reflect.Float64:
		panic("floating point types are unsafe for go-amino")
	}
	switch field.TypeInfo.ReprType.Type.Kind() {
	case reflect.Float32, reflect.Float64:
		panic("floating point types are unsafe for go-amino, even for repr types")
	}
}

// CONTRACT: by the time this is called, len(bz) >= _n
// Returns true so you can write one-liners.
func slide(bz *[]byte, n *int, _n int) bool {
	if bz != nil {
		if _n < 0 || _n > len(*bz) {
			panic(fmt.Sprintf("impossible slide: len:%v _n:%v", len(*bz), _n))
		}
		*bz = (*bz)[_n:]
	}
	if n != nil {
		*n += _n
	}
	return true
}

// maybe dereference if pointer.
// drv: the final non-pointer value (which may be invalid).
// isPtr: whether rv.Kind() == reflect.Ptr.
// isNilPtr: whether a nil pointer at any level.
func maybeDerefValue(rv reflect.Value) (drv reflect.Value, rvIsPtr bool, rvIsNilPtr bool) {
	if rv.Kind() == reflect.Ptr {
		rvIsPtr = true
		if rv.IsNil() {
			rvIsNilPtr = true
			return
		}
		rv = rv.Elem()
	}
	drv = rv
	return
}

// Dereference-and-construct pointers.
func maybeDerefAndConstruct(rv reflect.Value) reflect.Value {
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			newPtr := reflect.New(rv.Type().Elem())
			rv.Set(newPtr)
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Ptr {
		panic("unexpected pointer pointer")
	}
	return rv
}

// Returns isDefaultValue=true iff is zero.
// NOTE: Also works for Maps, Chans, and Funcs, though they are not
// otherwise supported by Amino.  For future?
// Doesn't work for structs.
func isNonstructDefaultValue(rv reflect.Value) (isDefault bool) {
	switch rv.Kind() {
	case reflect.Ptr:
		return rv.IsNil()
	case reflect.Bool:
		return rv.Bool() == false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.String:
		return rv.Len() == 0
	case reflect.Chan, reflect.Map, reflect.Slice:
		return rv.IsNil() || rv.Len() == 0
	case reflect.Func, reflect.Interface:
		return rv.IsNil()
	case reflect.Struct:
		panic("not supported (yet?)")
	default:
		return false
	}
}

// Returns the default value of a type.  For a time type or a
// pointer(s) to time, the default value is not zero (or nil), but the
// time value of 1970.
func defaultValue(rt reflect.Type) (rv reflect.Value) {
	switch rt.Kind() {
	case reflect.Ptr:
		// Dereference all the way and see if it's a time type.
		refType := rt.Elem()
		for refType.Kind() == reflect.Ptr {
			refType = refType.Elem()
		}
		if refType == timeType {
			// Start from the top and construct pointers as needed.
			rv = reflect.New(rt).Elem()
			refType, refValue := rt, rv
			for refType.Kind() == reflect.Ptr {
				newPtr := reflect.New(refType.Elem())
				refValue.Set(newPtr)
				refType = refType.Elem()
				refValue = refValue.Elem()
			}
			// Set to 1970, the whole point of this function.
			refValue.Set(reflect.ValueOf(zeroTime))
			return rv
		}
	case reflect.Struct:
		if rt == timeType {
			// Set to 1970, the whole point of this function.
			rv = reflect.New(rt).Elem()
			rv.Set(reflect.ValueOf(zeroTime))
			return rv
		}
	}

	// Just return the default Go zero object.
	// Return an empty struct.
	return reflect.Zero(rt)
}

// NOTE: Also works for Maps and Chans, though they are not
// otherwise supported by Amino.  For future?
func isNil(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Interface, reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// constructConcreteType creates the concrete value as
// well as the corresponding settable value for it.
// Return irvSet which should be set on caller's interface rv.
func constructConcreteType(cinfo *TypeInfo) (crv, irvSet reflect.Value) {
	// Construct new concrete type.
	if cinfo.PointerPreferred {
		cPtrRv := reflect.New(cinfo.Type)
		crv = cPtrRv.Elem()
		irvSet = cPtrRv
	} else {
		crv = reflect.New(cinfo.Type).Elem()
		irvSet = crv
	}
	return
}

// Like constructConcreteType(), but if pointer preferred, returns a nil one.
// We like nil pointers for efficiency.
func constructConcreteTypeNilPreferred(cinfo *TypeInfo) (crv reflect.Value) {
	// Construct new concrete type.
	if cinfo.PointerPreferred {
		crv = reflect.Zero(cinfo.PtrToType)
	} else {
		crv = reflect.New(cinfo.Type).Elem()
	}
	return
}

func toReprObject(rv reflect.Value) (rrv reflect.Value, err error) {
	var mwrm reflect.Value
	if rv.CanAddr() {
		mwrm = rv.Addr().MethodByName("MarshalAmino")
	} else {
		mwrm = rv.MethodByName("MarshalAmino")
	}
	mwouts := mwrm.Call(nil)
	if !mwouts[1].IsNil() {
		erri := mwouts[1].Interface()
		if erri != nil {
			err = erri.(error)
			return rrv, err
		}
	}
	rrv = mwouts[0]
	return
}
