package vm

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Helper function to check for panics using standard library
func expectPanic(t *testing.T, fn func(), containsMsg string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected a panic, but function did not panic")
			return
		}
		if containsMsg != "" {
			var panicMsg string
			switch v := r.(type) {
			case string:
				panicMsg = v
			case error:
				panicMsg = v.Error()
			default:
				panicMsg = fmt.Sprintf("%v", r)
			}
			if !strings.Contains(panicMsg, containsMsg) {
				t.Errorf("Panic message mismatch.\nExpected to contain: %q\nActual: %q", containsMsg, panicMsg)
			}
		}
	}()
	fn()
}

// Helper to compare floats with tolerance, especially for NaN
func floatsEqual(t *testing.T, expected, actual float64, msgAndArgs ...interface{}) {
	t.Helper()
	if math.IsNaN(expected) {
		if !math.IsNaN(actual) {
			t.Errorf("Expected NaN, got %v. %s", actual, fmt.Sprint(msgAndArgs...))
		}
		return // Both are NaN
	}
	if math.IsNaN(actual) {
		t.Errorf("Expected %v, got NaN. %s", expected, fmt.Sprint(msgAndArgs...))
		return
	}
	// Use tolerance for large numbers or direct comparison for others
	delta := 1e-9
	if math.Abs(expected) > 1e6 {
		delta = math.Abs(expected * 1e-9)
	}
	if math.Abs(expected-actual) > delta {
		t.Errorf("Float mismatch. Expected %v, got %v (delta %v). %s", expected, actual, delta, fmt.Sprint(msgAndArgs...))
	}
}

// Helper to compare potentially nil pointers for sameness
func assertSame(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	// Use reflect carefully for pointer comparison
	expectedPtr := reflect.ValueOf(expected)
	actualPtr := reflect.ValueOf(actual)

	if expectedPtr.Kind() != reflect.Ptr || actualPtr.Kind() != reflect.Ptr {
		if expected != actual { // Fallback for non-pointers or mixed types
			t.Errorf("assertSame failed: values not equal. Expected '%v', got '%v'. %s", expected, actual, fmt.Sprint(msgAndArgs...))
		}
		return
	}

	if expectedPtr.IsNil() != actualPtr.IsNil() {
		t.Errorf("assertSame failed: one pointer is nil, the other is not. Expected '%v', got '%v'. %s", expected, actual, fmt.Sprint(msgAndArgs...))
		return
	}

	if !expectedPtr.IsNil() && expectedPtr.Pointer() != actualPtr.Pointer() {
		t.Errorf("assertSame failed: pointers do not match. Expected '%v' (%p), got '%v' (%p). %s", expected, expected, actual, actual, fmt.Sprint(msgAndArgs...))
	}
}

// Helper to compare potentially nil pointers for difference
func assertNotSame(t *testing.T, notExpected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	// Use reflect carefully for pointer comparison
	notExpectedPtr := reflect.ValueOf(notExpected)
	actualPtr := reflect.ValueOf(actual)

	if notExpectedPtr.Kind() != reflect.Ptr || actualPtr.Kind() != reflect.Ptr {
		// If not pointers, comparing for non-sameness doesn't make sense in the same way
		return
	}

	if !notExpectedPtr.IsNil() && !actualPtr.IsNil() && notExpectedPtr.Pointer() == actualPtr.Pointer() {
		t.Errorf("assertNotSame failed: pointers match unexpectedly. Got '%v' (%p). %s", actual, actual, fmt.Sprint(msgAndArgs...))
	}
	// No error if they are different, or if one/both are nil and they aren't the same nil value (which isn't possible here)
}

// Helper to create a basic FunctionObject for tests
func createTestFunctionObject(name string, upvalueCount int) *FunctionObject {
	return &FunctionObject{
		Name:         name,
		Arity:        0,
		UpvalueCount: upvalueCount,
		RegisterSize: 8,        // Arbitrary
		Chunk:        &Chunk{}, // Dummy chunk
	}
}

func TestValueSize(t *testing.T) {
	var v Value
	size := unsafe.Sizeof(v)
	expectedSize := uintptr(24) // For 64-bit

	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		if size != expectedSize {
			t.Errorf("Value size mismatch on 64-bit. Expected %d, got %d", expectedSize, size)
		}
	} else {
		t.Logf("Skipping exact size check on non-64-bit arch (%s), size is %d", runtime.GOARCH, size)
		if size > expectedSize { // General sanity check
			t.Errorf("Value size (%d) seems too large on %s arch (expected <= %d)", size, runtime.GOARCH, expectedSize)
		}
	}
	fmt.Printf("Size of Value: %d bytes on %s\n", size, runtime.GOARCH) // Informational
}

func TestConstants(t *testing.T) {
	if Undefined.Type() != TypeUndefined {
		t.Errorf("Undefined type mismatch. Expected %v, got %v", TypeUndefined, Undefined.Type())
	}
	if Null.Type() != TypeNull {
		t.Errorf("Null type mismatch. Expected %v, got %v", TypeNull, Null.Type())
	}
	if True.Type() != TypeBoolean {
		t.Errorf("True type mismatch. Expected %v, got %v", TypeBoolean, True.Type())
	}
	if !True.AsBoolean() {
		t.Errorf("True value mismatch. Expected true, got false")
	}
	if False.Type() != TypeBoolean {
		t.Errorf("False type mismatch. Expected %v, got %v", TypeBoolean, False.Type())
	}
	if False.AsBoolean() {
		t.Errorf("Expected False.AsBoolean() == false")
	}
	if NaN.Type() != TypeFloatNumber {
		t.Errorf("NaN type mismatch. Expected %v, got %v", TypeFloatNumber, NaN.Type())
	}
	if !math.IsNaN(NaN.AsFloat()) {
		t.Errorf("NaN value mismatch. Expected NaN, got %v", NaN.AsFloat())
	}
}

func TestNumberValues(t *testing.T) {
	t.Run("Integer", func(t *testing.T) {
		v := IntegerValue(123)
		if v.Type() != TypeIntegerNumber {
			t.Errorf("Type mismatch. Expected %v, got %v", TypeIntegerNumber, v.Type())
		}
		if !v.IsNumber() {
			t.Errorf("Expected IsNumber() == true")
		}
		if !v.IsIntegerNumber() {
			t.Errorf("Expected IsIntegerNumber() == true")
		}
		if v.IsFloatNumber() {
			t.Errorf("Expected IsFloatNumber() == false")
		}
		if got := v.AsInteger(); got != 123 {
			t.Errorf("AsInteger mismatch. Expected 123, got %d", got)
		}

		expectPanic(t, func() { v.AsFloat() }, "not a float")
		expectPanic(t, func() { v.AsBigInt() }, "not a big int")
	})

	t.Run("Float", func(t *testing.T) {
		f := 3.14
		v := NumberValue(f)
		if v.Type() != TypeFloatNumber {
			t.Errorf("Type mismatch. Expected %v, got %v", TypeFloatNumber, v.Type())
		}
		if !v.IsNumber() {
			t.Errorf("Expected IsNumber() == true")
		}
		if v.IsIntegerNumber() {
			t.Errorf("Expected IsIntegerNumber() == false")
		}
		if !v.IsFloatNumber() {
			t.Errorf("Expected IsFloatNumber() == true")
		}
		if got := v.AsFloat(); got != f {
			t.Errorf("AsFloat mismatch. Expected %f, got %f", f, got)
		}

		expectPanic(t, func() { v.AsInteger() }, "not an integer")
		expectPanic(t, func() { v.AsBigInt() }, "not a big int")
	})

	t.Run("NaN", func(t *testing.T) {
		v := NumberValue(math.NaN())
		if v.Type() != TypeFloatNumber {
			t.Errorf("Type mismatch. Expected %v, got %v", TypeFloatNumber, v.Type())
		}
		if !v.IsNumber() {
			t.Errorf("Expected IsNumber() == true")
		}
		if !v.IsFloatNumber() {
			t.Errorf("Expected IsFloatNumber() == true")
		}
		if !math.IsNaN(v.AsFloat()) {
			t.Errorf("Expected AsFloat() to be NaN")
		}
	})

	t.Run("Infinity", func(t *testing.T) {
		v := NumberValue(math.Inf(1))
		if v.Type() != TypeFloatNumber {
			t.Errorf("Type mismatch. Expected %v, got %v", TypeFloatNumber, v.Type())
		}
		if !v.IsNumber() {
			t.Errorf("Expected IsNumber() == true")
		}
		if !v.IsFloatNumber() {
			t.Errorf("Expected IsFloatNumber() == true")
		}
		if !math.IsInf(v.AsFloat(), 1) {
			t.Errorf("Expected AsFloat() to be +Inf")
		}

		vNeg := NumberValue(math.Inf(-1))
		if !math.IsInf(vNeg.AsFloat(), -1) {
			t.Errorf("Expected AsFloat() to be -Inf")
		}
	})
}

func TestBooleanValue(t *testing.T) {
	vTrue := BooleanValue(true)
	if vTrue.Type() != TypeBoolean {
		t.Errorf("True type mismatch. Expected %v, got %v", TypeBoolean, vTrue.Type())
	}
	if !vTrue.IsBoolean() {
		t.Errorf("Expected True.IsBoolean() == true")
	}
	if !vTrue.AsBoolean() {
		t.Errorf("Expected True.AsBoolean() == true")
	}
	assertSame(t, True, vTrue, "BooleanValue(true) should return True singleton")

	vFalse := BooleanValue(false)
	if vFalse.Type() != TypeBoolean {
		t.Errorf("False type mismatch. Expected %v, got %v", TypeBoolean, vFalse.Type())
	}
	if !vFalse.IsBoolean() {
		t.Errorf("Expected False.IsBoolean() == true")
	}
	if vFalse.AsBoolean() {
		t.Errorf("Expected False.AsBoolean() == false")
	}
	assertSame(t, False, vFalse, "BooleanValue(false) should return False singleton")

	expectPanic(t, func() { vTrue.AsInteger() }, "not an integer")
	expectPanic(t, func() { vFalse.AsString() }, "not a string")
}

func TestBigIntValue(t *testing.T) {
	bi := big.NewInt(1234567890123456789)
	v := NewBigInt(bi)

	if v.Type() != TypeBigInt {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeBigInt, v.Type())
	}
	if !v.IsBigInt() {
		t.Errorf("Expected IsBigInt() == true")
	}
	if v.IsNumber() {
		t.Errorf("Expected IsNumber() == false for BigInt")
	}
	gotBi := v.AsBigInt()
	if gotBi == nil {
		t.Fatalf("AsBigInt() returned nil unexpectedly")
	}
	if bi.Cmp(gotBi) != 0 {
		t.Errorf("BigInt value mismatch. Expected %s, got %s", bi.String(), gotBi.String())
	}

	// Test pointer is distinct if we create another one
	bi2 := big.NewInt(1234567890123456789)
	v2 := NewBigInt(bi2)
	if v.obj == v2.obj { // Compare underlying unsafe.Pointer
		t.Errorf("Expected distinct pointers for different NewBigInt calls")
	}
	assertNotSame(t, v.obj, v2.obj, "Expected distinct pointers for different NewBigInt calls")

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsFloat() }, "not a float")

	// Note: BigInt.toString() does NOT include the "n" suffix per ECMAScript spec
	expectedStr := "1234567890123456789"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
}

func TestStringValue(t *testing.T) {
	s := "hello world"
	v := NewString(s)

	if v.Type() != TypeString {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeString, v.Type())
	}
	if !v.IsString() {
		t.Errorf("Expected IsString() == true")
	}
	if v.IsSymbol() {
		t.Errorf("Expected IsSymbol() == false")
	}
	if got := v.AsString(); got != s {
		t.Errorf("AsString mismatch. Expected %q, got %q", s, got)
	}

	// Test pointer is distinct if we create another one
	v2 := NewString(s)
	assertNotSame(t, v.obj, v2.obj, "Expected distinct pointers for different NewString calls")

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsSymbol() }, "not a symbol")
}

func TestSymbolValue(t *testing.T) {
	s := "mySymbol"
	v := NewSymbol(s)

	if v.Type() != TypeSymbol {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeSymbol, v.Type())
	}
	if !v.IsSymbol() {
		t.Errorf("Expected IsSymbol() == true")
	}
	if v.IsString() {
		t.Errorf("Expected IsString() == false")
	}
	if got := v.AsSymbol(); got != s {
		t.Errorf("AsSymbol mismatch. Expected %q, got %q", s, got)
	}

	// Test pointer is distinct if we create another one
	v2 := NewSymbol(s)
	assertNotSame(t, v.obj, v2.obj, "Expected distinct pointers for different NewSymbol calls")

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsString() }, "not a string")

	expectedStr := "Symbol(mySymbol)"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
}

func TestObjectValue(t *testing.T) {
	v := NewObject(DefaultObjectPrototype) // Creates a PlainObject

	if v.Type() != TypeObject {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeObject, v.Type())
	}
	if !v.IsObject() {
		t.Errorf("Expected IsObject() == true")
	}
	if v.IsArray() {
		t.Errorf("Expected IsArray() == false")
	}

	plainObjPtr := v.AsPlainObject()
	if plainObjPtr == nil {
		t.Fatalf("AsPlainObject() returned nil")
	}

	if plainObjPtr.shape != RootShape {
		t.Errorf("Expected initial shape to be nil, got %v", plainObjPtr.shape)
	}

	// Check that the prototype is the shared DefaultObjectPrototype
	if !plainObjPtr.prototype.Is(DefaultObjectPrototype) { // Use Is() for comparison
		t.Errorf("Expected prototype to be DefaultObjectPrototype (%v), got %v", DefaultObjectPrototype, plainObjPtr.prototype)
	}

	// Optional: Check that the prototype of the prototype is Null
	if DefaultObjectPrototype.Type() == TypeObject { // Ensure DefaultObjectPrototype is initialized and is an object
		protoOfProto := DefaultObjectPrototype.AsPlainObject().prototype
		if !protoOfProto.Is(Null) {
			t.Errorf("Expected prototype's prototype to be Null, got %v", protoOfProto)
		}
	} else {
		t.Errorf("DefaultObjectPrototype is not TypeObject or not initialized properly")
	}

	if plainObjPtr.properties != nil {
		t.Errorf("Expected initial properties to be nil, got %v", plainObjPtr.properties)
	}

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsArray() }, "not an array")
	expectPanic(t, func() { NewString("test").AsPlainObject() }, "not an object") // Panic check

	expectedStr := "[object Object]"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
}

func TestArrayValue(t *testing.T) {
	v := NewArray()

	if v.Type() != TypeArray {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeArray, v.Type())
	}
	if !v.IsArray() {
		t.Errorf("Expected IsArray() == true")
	}
	if !v.IsObject() {
		t.Errorf("Expected IsObject() == true for Array") // Current IsObject behavior
	}

	arrObj := v.AsArray()
	if arrObj == nil {
		t.Fatalf("AsArray() returned nil")
	}

	if arrObj.length != 0 {
		t.Errorf("Expected initial length 0, got %d", arrObj.length)
	}
	if arrObj.elements != nil {
		t.Errorf("Expected initial elements slice to be nil, got %v", arrObj.elements)
	}

	// Add elements and check
	arrObj.elements = append(arrObj.elements, IntegerValue(1))
	arrObj.length++
	if arrObj.length != 1 {
		t.Errorf("Expected length 1 after append, got %d", arrObj.length)
	}
	if len(arrObj.elements) != 1 {
		t.Fatalf("Expected elements slice length 1, got %d", len(arrObj.elements))
	}

	if gotInt := arrObj.elements[0].AsInteger(); gotInt != 1 {
		t.Errorf("Expected element[0] to be 1, got %d", gotInt)
	}

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsObject() }, "not an object") // AsObject specifically checks for TypeObject

	expectedStr := "1" // Placeholder
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
}

func TestFunctionValue(t *testing.T) {
	dummyChunk := &Chunk{}
	v := NewFunction(2, 2, 0, 8, false, "testFn", dummyChunk, false, false, false, false) // Use constructor

	if v.Type() != TypeFunction {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeFunction, v.Type())
	}
	if !v.IsCallable() {
		t.Errorf("Expected IsCallable() == true")
	}
	if !v.IsFunction() {
		t.Errorf("Expected IsFunction() == true")
	}
	if v.IsClosure() {
		t.Errorf("Expected IsClosure() == false")
	}
	if v.IsNativeFunction() {
		t.Errorf("Expected IsNativeFunction() == false")
	}

	fnObj := v.AsFunction()
	if fnObj == nil {
		t.Fatalf("AsFunction() returned nil")
	}
	if fnObj.Arity != 2 {
		t.Errorf("Arity mismatch. Expected 2, got %d", fnObj.Arity)
	}
	if fnObj.Variadic {
		t.Errorf("Expected variadic to be false")
	}
	if fnObj.UpvalueCount != 0 {
		t.Errorf("UpvalueCount mismatch. Expected 0, got %d", fnObj.UpvalueCount)
	}
	if fnObj.RegisterSize != 8 {
		t.Errorf("RegisterSize mismatch. Expected 8, got %d", fnObj.RegisterSize)
	}
	if fnObj.Name != "testFn" {
		t.Errorf("Name mismatch. Expected %q, got %q", "testFn", fnObj.Name)
	}
	assertSame(t, dummyChunk, fnObj.Chunk, "Chunk mismatch")

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsNativeFunction() }, "not a native function")
	expectPanic(t, func() { v.AsClosure() }, "not a closure")
	expectPanic(t, func() { v.AsObject() }, "not an object")

	expectedStr := "<function testFn>"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
	vNoName := NewFunction(0, 0, 0, 0, false, "", nil, false, false, false, false)
	expectedNoNameStr := "<function>"
	if gotNoNameStr := vNoName.ToString(); gotNoNameStr != expectedNoNameStr {
		t.Errorf("ToString (no name) mismatch. Expected %q, got %q", expectedNoNameStr, gotNoNameStr)
	}
}

func TestNativeFunctionValue(t *testing.T) {
	var called bool
	dummyNativeFn := func(args []Value) (Value, error) { called = true; return Null, nil }
	v := NewNativeFunction(1, true, "nativeLog", dummyNativeFn) // Use constructor

	if v.Type() != TypeNativeFunction {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeNativeFunction, v.Type())
	}
	if !v.IsCallable() {
		t.Errorf("Expected IsCallable() == true")
	}
	if v.IsFunction() {
		t.Errorf("Expected IsFunction() == false")
	}
	if v.IsClosure() {
		t.Errorf("Expected IsClosure() == false")
	}
	if !v.IsNativeFunction() {
		t.Errorf("Expected IsNativeFunction() == true")
	}

	nativeFnObj := v.AsNativeFunction()
	if nativeFnObj == nil {
		t.Fatalf("AsNativeFunction() returned nil")
	}
	if nativeFnObj.Arity != 1 {
		t.Errorf("Arity mismatch. Expected 1, got %d", nativeFnObj.Arity)
	}
	if !nativeFnObj.Variadic {
		t.Errorf("Expected variadic to be true")
	}
	if nativeFnObj.Name != "nativeLog" {
		t.Errorf("Name mismatch. Expected %q, got %q", "nativeLog", nativeFnObj.Name)
	}
	if nativeFnObj.Fn == nil {
		t.Fatalf("Native function fn is nil")
	}

	// Check that calling the retrieved function works
	result, _ := nativeFnObj.Fn(nil)
	if !result.Is(Null) {
		t.Errorf("Native function call result mismatch. Expected Null, got %v", result)
	}
	if !called {
		t.Errorf("Native function fn was not called when invoked via object")
	}

	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsFunction() }, "not a function template")
	expectPanic(t, func() { v.AsClosure() }, "not a closure")
	expectPanic(t, func() { v.AsObject() }, "not an object")

	expectedStr := "<native function nativeLog>"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
	vNoName := NewNativeFunction(0, false, "", nil)
	expectedNoNameStr := "<native function>"
	if gotNoNameStr := vNoName.ToString(); gotNoNameStr != expectedNoNameStr {
		t.Errorf("ToString (no name) mismatch. Expected %q, got %q", expectedNoNameStr, gotNoNameStr)
	}
}

func TestClosureValue(t *testing.T) {
	// 1. Setup: Create a FunctionObject and some Upvalues
	fnObj := createTestFunctionObject("closureFn", 2)
	val1 := IntegerValue(10)
	val2 := NewString("hello")
	upval1 := &Upvalue{Location: &val1} // Open upvalue
	upval2 := &Upvalue{Closed: val2}    // Closed upvalue (Location is nil)
	upvalues := []*Upvalue{upval1, upval2}

	// 2. Test NewClosure
	v := NewClosure(fnObj, upvalues)
	if v.Type() != TypeClosure {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeClosure, v.Type())
	}
	if !v.IsCallable() {
		t.Errorf("Expected IsCallable() == true")
	}
	if v.IsFunction() {
		t.Errorf("Expected IsFunction() == false")
	}
	if !v.IsClosure() {
		t.Errorf("Expected IsClosure() == true")
	}
	if v.IsNativeFunction() {
		t.Errorf("Expected IsNativeFunction() == false")
	}

	// Test panic on wrong number of upvalues
	expectPanic(t, func() { NewClosure(fnObj, []*Upvalue{upval1}) }, "Incorrect number of upvalues")
	expectPanic(t, func() { NewClosure(nil, upvalues) }, "nil FunctionObject")

	// 3. Test AsClosure accessor
	closureObj := v.AsClosure()
	if closureObj == nil {
		t.Fatalf("AsClosure() returned nil")
	}

	assertSame(t, fnObj, closureObj.Fn, "Closure function object mismatch")

	if len(closureObj.Upvalues) != 2 {
		t.Fatalf("Expected 2 upvalues, got %d", len(closureObj.Upvalues))
	}

	assertSame(t, upval1, closureObj.Upvalues[0], "Upvalue[0] mismatch")

	assertSame(t, upval2, closureObj.Upvalues[1], "Upvalue[1] mismatch")

	// 4. Test accessors panic
	expectPanic(t, func() { v.AsInteger() }, "not an integer")
	expectPanic(t, func() { v.AsFunction() }, "not a function template")
	expectPanic(t, func() { v.AsNativeFunction() }, "not a native function")
	expectPanic(t, func() { v.AsObject() }, "not an object")

	// 5. Test ToString
	expectedStr := "<closure closureFn>"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
	fnObjNoName := createTestFunctionObject("", 2)
	vNoName := NewClosure(fnObjNoName, upvalues)
	expectedNoNameStr := "<closure>"
	if gotNoNameStr := vNoName.ToString(); gotNoNameStr != expectedNoNameStr {
		t.Errorf("ToString (no name) mismatch. Expected %q, got %q", expectedNoNameStr, gotNoNameStr)
	}
}

func TestUpvalue(t *testing.T) {
	stackVal := IntegerValue(42)
	closedVal := NewString("closed")

	// Test Open Upvalue
	upOpen := &Upvalue{Location: &stackVal}
	if upOpen.Location == nil {
		t.Fatalf("Expected open upvalue Location to be non-nil")
	}

	// Expect Undefined (zero value of Value type) for Closed initially
	if !upOpen.Closed.Is(Undefined) {
		t.Errorf("Expected open upvalue Closed field to be Undefined (zero value), got %v", upOpen.Closed)
	}
	resolvedOpen := upOpen.Resolve()
	if resolvedOpen == nil {
		t.Fatalf("Resolve() on open upvalue returned nil")
	}

	assertSame(t, &stackVal, resolvedOpen, "Resolve() open mismatch")
	if gotInt := resolvedOpen.AsInteger(); gotInt != 42 {
		t.Errorf("Resolved open value mismatch. Expected 42, got %d", gotInt)
	}

	// Test Closing the Upvalue
	upOpen.Close()
	if upOpen.Location != nil {
		t.Errorf("Expected Location to be nil after Close()")
	}
	if upOpen.Closed.Type() != TypeIntegerNumber {
		t.Errorf("Closed value type mismatch. Expected Integer, got %v", upOpen.Closed.Type())
	}
	if gotInt := upOpen.Closed.AsInteger(); gotInt != 42 {
		t.Errorf("Closed value mismatch. Expected 42, got %d", gotInt)
	}

	// Test Resolving after Close
	resolvedClosed := upOpen.Resolve()
	if resolvedClosed == nil {
		t.Fatalf("Resolve() after close returned nil")
	}

	assertSame(t, &upOpen.Closed, resolvedClosed, "Resolve() closed should point to Closed field")
	if gotInt := resolvedClosed.AsInteger(); gotInt != 42 {
		t.Errorf("Resolved closed value mismatch. Expected 42, got %d", gotInt)
	}

	// Test Already Closed Upvalue
	upClosed := &Upvalue{Closed: closedVal}
	if upClosed.Location != nil {
		t.Errorf("Expected already closed Location to be nil")
	}
	if !upClosed.Closed.Is(closedVal) { // Use Is for value comparison
		t.Errorf("Already closed 'Closed' field mismatch. Expected %v, got %v", closedVal, upClosed.Closed)
	}

	resolvedAlreadyClosed := upClosed.Resolve()
	if resolvedAlreadyClosed == nil {
		t.Fatalf("Resolve() on already closed returned nil")
	}

	assertSame(t, &upClosed.Closed, resolvedAlreadyClosed, "Resolve() already closed mismatch")
	if gotStr := resolvedAlreadyClosed.AsString(); gotStr != "closed" {
		t.Errorf("Resolved already closed value mismatch. Expected 'closed', got %q", gotStr)
	}

	// Test Close on already closed upvalue (should be no-op)
	upClosed.Close()
	if upClosed.Location != nil {
		t.Errorf("Location should remain nil after Close() on already closed")
	}
	if !upClosed.Closed.Is(closedVal) {
		t.Errorf("'Closed' field should remain unchanged after Close() on already closed. Expected %v, got %v", closedVal, upClosed.Closed)
	}
}

func TestTypeName(t *testing.T) {
	fnObj := createTestFunctionObject("test", 0)
	closureObj := NewClosure(fnObj, []*Upvalue{})
	nativeFn := NewNativeFunction(0, false, "", nil)

	testCases := []struct {
		input Value
		want  string
	}{
		{Undefined, "undefined"},
		{Null, "null"},
		{True, "boolean"},
		{False, "boolean"},
		{IntegerValue(1), "number"},
		{NumberValue(1.5), "number"},
		{NewBigInt(big.NewInt(1)), "bigint"},
		{NewString("a"), "string"},
		{NewSymbol("s"), "symbol"},
		{NewObject(DefaultObjectPrototype), "object"},
		{NewArray(), "object"}, // typeof [] is 'object'
		{NewFunction(0, 0, 0, 0, false, "", nil, false, false, false, false), "function"},
		{closureObj, "function"},
		{nativeFn, "function"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.input.TypeName(); got != tc.want {
				t.Errorf("TypeName() mismatch for %v. Expected %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestToStringConversion(t *testing.T) {
	fnObj := createTestFunctionObject("myFn", 0)
	closureObj := NewClosure(fnObj, []*Upvalue{})
	nativeFn := NewNativeFunction(0, false, "myNative", nil)

	testCases := []struct {
		name string
		in   Value
		want string
	}{
		{"String", NewString("test"), "test"},
		{"Symbol", NewSymbol("sym"), "Symbol(sym)"},
		{"Float", NumberValue(123.45), "123.45"},
		{"Integer", IntegerValue(987), "987"},
		{"BigInt", NewBigInt(big.NewInt(1000)), "1000"}, // No "n" suffix per ECMAScript spec
		{"BooleanTrue", True, "true"},
		{"BooleanFalse", False, "false"},
		{"Function", NewFunction(0, 0, 0, 0, false, "myFn", nil, false, false, false, false), "<function myFn>"},
		{"Closure", closureObj, "<closure myFn>"},
		{"NativeFunction", nativeFn, "<native function myNative>"},
		{"Object", NewObject(DefaultObjectPrototype), "[object Object]"},
		{"Array", NewArray(), ""},
		{"Null", Null, "null"},
		{"Undefined", Undefined, "undefined"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.ToString(); got != tc.want {
				t.Errorf("ToString() mismatch. Expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestToFloatConversion(t *testing.T) {
	testCases := []struct {
		name    string
		input   Value
		want    float64
		wantNaN bool
	}{
		{"Float", NumberValue(123.45), 123.45, false},
		{"Integer", IntegerValue(987), 987.0, false},
		{"BigInt", NewBigInt(big.NewInt(100)), 100.0, false},
		{"BigIntLarge", NewBigInt(new(big.Int).Lsh(big.NewInt(1), 100)), 1.2676506002282294e+30, false}, // Might lose precision
		{"BooleanTrue", True, 1.0, false},
		{"BooleanFalse", False, 0.0, false},
		{"StringNumber", NewString(" -1.5e2 "), -150.0, false},
		{"StringHex", NewString("0xff"), 255, false}, // ToNumber handles hex strings
		{"StringInvalid", NewString("test"), 0, true},
		{"Symbol", NewSymbol("sym"), 0, true},
		{"Null", Null, 0, false}, // ToNumber(null) === 0 per ECMAScript spec
		{"Undefined", Undefined, 0, true},
		{"Object", NewObject(DefaultObjectPrototype), 0, true},
		{"Function", NewFunction(0, 0, 0, 0, false, "", nil, false, false, false, false), 0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.ToFloat()
			if tc.wantNaN {
				if !math.IsNaN(got) {
					t.Errorf("Expected NaN, got %v", got)
				}
			} else {
				floatsEqual(t, tc.want, got) // Use helper for comparison
			}
		})
	}
}

func TestToIntegerConversion(t *testing.T) {
	largeBigInt := new(big.Int).Lsh(big.NewInt(1), 40) // Too big for int32
	maxInt32BigInt := big.NewInt(math.MaxInt32)
	minInt32BigInt := big.NewInt(math.MinInt32)

	testCases := []struct {
		name  string
		input Value
		want  int32
	}{
		{"Float", NumberValue(123.45), 123},
		{"FloatRound", NumberValue(123.99), 123},
		{"FloatNegative", NumberValue(-123.45), -123},
		{"FloatNaN", NumberValue(math.NaN()), 0},
		{"FloatInf", NumberValue(math.Inf(1)), 0},
		{"Integer", IntegerValue(987), 987},
		{"BigInt", NewBigInt(big.NewInt(100)), 100},
		{"BigIntLarge", NewBigInt(largeBigInt), 0},
		{"BigIntMaxInt32", NewBigInt(maxInt32BigInt), math.MaxInt32},
		{"BigIntMinInt32", NewBigInt(minInt32BigInt), math.MinInt32},
		{"BooleanTrue", True, 1},
		{"BooleanFalse", False, 0},
		{"StringInt", NewString(" -42 "), -42},
		{"StringFloat", NewString(" 123.9 "), 123},
		{"StringHex", NewString(" \t 0xFF \n"), 255},
		{"StringInvalid", NewString("test"), 0},
		{"StringInvalidFloat", NewString("12a"), 0},
		{"StringNaN", NewString("NaN"), 0},
		{"Symbol", NewSymbol("sym"), 0},
		{"Null", Null, 0},
		{"Undefined", Undefined, 0},
		{"Object", NewObject(DefaultObjectPrototype), 0},
		{"Function", NewFunction(0, 0, 0, 0, false, "", nil, false, false, false, false), 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.ToInteger(); got != tc.want {
				t.Errorf("ToInteger() mismatch. Expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestIsFunctions(t *testing.T) {
	fn := NewFunction(0, 0, 0, 0, false, "", nil, false, false, false, false)
	cl := NewClosure(createTestFunctionObject("", 0), nil)
	na := NewNativeFunction(0, false, "", nil)
	obj := NewObject(DefaultObjectPrototype)
	arr := NewArray()
	sym := NewSymbol("a")
	str := NewString("a")
	i := IntegerValue(1)
	// f := NumberValue(1.0)
	// b := NewBigInt(big.NewInt(1))

	if Undefined.IsNumber() {
		t.Errorf("Undefined.IsNumber() unexpected")
	}
	if Null.IsBoolean() {
		t.Errorf("Null.IsBoolean() unexpected")
	}
	if True.IsString() {
		t.Errorf("True.IsString() unexpected")
	}
	if i.IsBigInt() {
		t.Errorf("IntegerValue(1).IsBigInt() unexpected")
	}
	if str.IsSymbol() {
		t.Errorf("NewString(\"a\").IsSymbol() unexpected")
	}
	if sym.IsObject() {
		t.Errorf("NewSymbol(\"a\").IsObject() unexpected")
	}
	if obj.IsArray() {
		t.Errorf("NewObject().IsArray() unexpected")
	} // IsObject doesn't include Array
	if arr.IsCallable() {
		t.Errorf("NewArray().IsCallable() unexpected")
	}
	if fn.IsNativeFunction() {
		t.Errorf("Function.IsNativeFunction() unexpected")
	}
	if fn.IsClosure() {
		t.Errorf("Function.IsClosure() unexpected")
	}
	if cl.IsFunction() {
		t.Errorf("Closure.IsFunction() unexpected")
	}
	if cl.IsNativeFunction() {
		t.Errorf("Closure.IsNativeFunction() unexpected")
	}
	if na.IsFunction() {
		t.Errorf("NativeFunction.IsFunction() unexpected")
	}
	if na.IsClosure() {
		t.Errorf("NativeFunction.IsClosure() unexpected")
	}
	if na.IsObject() {
		t.Errorf("NativeFunction.IsObject() unexpected")
	}
	if !fn.IsCallable() {
		t.Errorf("Expected fn.IsCallable() == true")
	}
	if !cl.IsCallable() {
		t.Errorf("Expected cl.IsCallable() == true")
	}
	if !na.IsCallable() {
		t.Errorf("Expected na.IsCallable() == true")
	}
}

func TestIsSameValueZero(t *testing.T) {
	// Re-use objects for reference checks
	fnObjPtr1 := createTestFunctionObject("f1", 0)
	fnObjPtr2 := createTestFunctionObject("f1", 0) // Same content, different obj
	closureObjPtr1 := &ClosureObject{Fn: fnObjPtr1, Upvalues: nil}
	closureObjPtr1b := &ClosureObject{Fn: fnObjPtr1, Upvalues: nil} // Different closure obj, same func obj
	closureObjPtr2 := &ClosureObject{Fn: fnObjPtr2, Upvalues: nil}
	nativeFnObjPtr1 := &NativeFunctionObject{Name: "n1", Fn: nil}
	nativeFnObjPtr1b := &NativeFunctionObject{Name: "n1", Fn: nil} // Different obj
	plainObjPtr1 := &PlainObject{}
	plainObjPtr2 := &PlainObject{}
	arrObjPtr1 := &ArrayObject{}
	arrObjPtr2 := &ArrayObject{}
	symObjPtr1 := &SymbolObject{value: "s"}
	symObjPtr1b := &SymbolObject{value: "s"} // Different obj, same value

	// Create corresponding Value wrappers using unsafe.Pointer
	fn1 := Value{typ: TypeFunction, obj: unsafe.Pointer(fnObjPtr1)}
	fn2 := Value{typ: TypeFunction, obj: unsafe.Pointer(fnObjPtr2)}
	closure1 := Value{typ: TypeClosure, obj: unsafe.Pointer(closureObjPtr1)}
	closure1b := Value{typ: TypeClosure, obj: unsafe.Pointer(closureObjPtr1b)}
	closure2 := Value{typ: TypeClosure, obj: unsafe.Pointer(closureObjPtr2)}
	nativeFn1 := Value{typ: TypeNativeFunction, obj: unsafe.Pointer(nativeFnObjPtr1)}
	nativeFn1b := Value{typ: TypeNativeFunction, obj: unsafe.Pointer(nativeFnObjPtr1b)}
	obj1 := Value{typ: TypeObject, obj: unsafe.Pointer(plainObjPtr1)}
	obj2 := Value{typ: TypeObject, obj: unsafe.Pointer(plainObjPtr2)}
	arr1 := Value{typ: TypeArray, obj: unsafe.Pointer(arrObjPtr1)}
	arr2 := Value{typ: TypeArray, obj: unsafe.Pointer(arrObjPtr2)}
	sym1 := Value{typ: TypeSymbol, obj: unsafe.Pointer(symObjPtr1)}
	sym1b := Value{typ: TypeSymbol, obj: unsafe.Pointer(symObjPtr1b)}

	testCases := []struct {
		name string
		v1   Value
		v2   Value
		want bool
	}{
		{"Undefined vs Undefined", Undefined, Undefined, true},
		{"Null vs Null", Null, Null, true},
		{"True vs True", True, True, true},
		{"False vs False", False, False, true},
		{"True vs False", True, False, false},
		{"Int vs Int (same)", IntegerValue(5), IntegerValue(5), true},
		{"Int vs Int (diff)", IntegerValue(5), IntegerValue(6), false},
		{"Float vs Float (same)", NumberValue(3.14), NumberValue(3.14), true},
		{"Float vs Float (diff)", NumberValue(3.14), NumberValue(3.15), false},
		{"+0 vs -0", NumberValue(0.0), NumberValue(math.Copysign(0.0, -1)), true},
		{"BigInt vs BigInt (same)", NewBigInt(big.NewInt(100)), NewBigInt(big.NewInt(100)), true},
		{"BigInt vs BigInt (diff)", NewBigInt(big.NewInt(100)), NewBigInt(big.NewInt(101)), false},
		{"String vs String (same)", NewString("a"), NewString("a"), true},
		{"String vs String (diff)", NewString("a"), NewString("b"), false},
		{"Symbol vs Symbol (same obj)", sym1, sym1, true},
		{"Symbol vs Symbol (diff obj)", sym1, sym1b, false},
		{"Object vs Object (same obj)", obj1, obj1, true},
		{"Object vs Object (diff obj)", obj1, obj2, false},
		{"Array vs Array (same obj)", arr1, arr1, true},
		{"Array vs Array (diff obj)", arr1, arr2, false},
		{"Function vs Function (same obj)", fn1, fn1, true},
		{"Function vs Function (diff obj)", fn1, fn2, false},
		{"Closure vs Closure (same obj)", closure1, closure1, true},
		{"Closure vs Closure (diff obj, same func)", closure1, closure1b, false},
		{"Closure vs Closure (diff obj, diff func)", closure1, closure2, false},
		{"NativeFunc vs NativeFunc (same obj)", nativeFn1, nativeFn1, true},
		{"NativeFunc vs NativeFunc (diff obj)", nativeFn1, nativeFn1b, false},
		{"Int vs Null", IntegerValue(1), Null, false},
		{"String vs Int", NewString("1"), IntegerValue(1), false},
		{"Object vs Null", obj1, Null, false},
		{"Array vs Object", arr1, obj1, false},
		// BigInt vs Number comparisons are false by default in current Is() impl
		{"BigInt vs Int", NewBigInt(big.NewInt(5)), IntegerValue(5), false},
		{"BigInt vs Float", NewBigInt(big.NewInt(5)), NumberValue(5.0), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v1.Is(tc.v2); got != tc.want {
				t.Errorf("Is(%v, %v) mismatch. Expected %t, got %t", tc.v1, tc.v2, tc.want, got)
			}
			// Check symmetry
			if got := tc.v2.Is(tc.v1); got != tc.want {
				t.Errorf("Is(%v, %v) symmetry mismatch. Expected %t, got %t", tc.v2, tc.v1, tc.want, got)
			}
		})
	}
}

func TestIsFalsey(t *testing.T) {
	testCases := []struct {
		name string
		in   Value
		want bool // True if falsey, False if truthy
	}{
		{"Null", Null, true},
		{"Undefined", Undefined, true},
		{"BooleanFalse", False, true},
		{"FloatZero", NumberValue(0.0), true},
		{"FloatNegativeZero", NumberValue(math.Copysign(0.0, -1)), true},
		{"FloatNaN", NaN, true},
		{"IntegerZero", IntegerValue(0), true},
		{"BigIntZero", NewBigInt(big.NewInt(0)), true},
		{"EmptyString", NewString(""), true},

		{"BooleanTrue", True, false},
		{"FloatNonZero", NumberValue(1.5), false},
		{"FloatInfinity", NumberValue(math.Inf(1)), false},
		{"IntegerNonZero", IntegerValue(1), false},
		{"BigIntNonZero", NewBigInt(big.NewInt(1)), false},
		{"NonEmptyString", NewString("a"), false},
		{"Symbol", NewSymbol("s"), false},
		{"Object", NewObject(DefaultObjectPrototype), false},
		{"Array", NewArray(), false},
		{"Function", NewFunction(0, 0, 0, 0, false, "", nil, false, false, false, false), false},
		{"Closure", NewClosure(createTestFunctionObject("", 0), nil), false},
		{"NativeFunction", NewNativeFunction(0, false, "", nil), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.IsFalsey(); got != tc.want {
				t.Errorf("IsFalsey() mismatch. Expected %t, got %t for %v", tc.want, got, tc.in)
			}
			// Test IsTruthy for completeness
			if gotTruthy := tc.in.IsTruthy(); gotTruthy == tc.want {
				t.Errorf("IsTruthy() mismatch. Expected %t, got %t for %v", !tc.want, gotTruthy, tc.in)
			}
		})
	}
}

func TestStrictlyEquals(t *testing.T) {
	// Re-use objects from TestIsSameValueZero setup
	fnObjPtr1 := createTestFunctionObject("f1", 0)
	fnObjPtr2 := createTestFunctionObject("f1", 0)
	closureObjPtr1 := &ClosureObject{Fn: fnObjPtr1, Upvalues: nil}
	closureObjPtr1b := &ClosureObject{Fn: fnObjPtr1, Upvalues: nil}
	nativeFnObjPtr1 := &NativeFunctionObject{Name: "n1", Fn: nil}
	nativeFnObjPtr1b := &NativeFunctionObject{Name: "n1", Fn: nil}
	plainObjPtr1 := &PlainObject{prototype: DefaultObjectPrototype}
	plainObjPtr2 := &PlainObject{prototype: DefaultObjectPrototype}
	arrObjPtr1 := &ArrayObject{}
	arrObjPtr2 := &ArrayObject{}
	symObjPtr1 := &SymbolObject{value: "s"}
	symObjPtr1b := &SymbolObject{value: "s"}
	fn1 := Value{typ: TypeFunction, obj: unsafe.Pointer(fnObjPtr1)}
	fn2 := Value{typ: TypeFunction, obj: unsafe.Pointer(fnObjPtr2)}
	closure1 := Value{typ: TypeClosure, obj: unsafe.Pointer(closureObjPtr1)}
	closure1b := Value{typ: TypeClosure, obj: unsafe.Pointer(closureObjPtr1b)}
	nativeFn1 := Value{typ: TypeNativeFunction, obj: unsafe.Pointer(nativeFnObjPtr1)}
	nativeFn1b := Value{typ: TypeNativeFunction, obj: unsafe.Pointer(nativeFnObjPtr1b)}
	obj1 := Value{typ: TypeObject, obj: unsafe.Pointer(plainObjPtr1)}
	obj2 := Value{typ: TypeObject, obj: unsafe.Pointer(plainObjPtr2)}
	arr1 := Value{typ: TypeArray, obj: unsafe.Pointer(arrObjPtr1)}
	arr2 := Value{typ: TypeArray, obj: unsafe.Pointer(arrObjPtr2)}
	sym1 := Value{typ: TypeSymbol, obj: unsafe.Pointer(symObjPtr1)}
	sym1b := Value{typ: TypeSymbol, obj: unsafe.Pointer(symObjPtr1b)}

	testCases := []struct {
		name string
		v1   Value
		v2   Value
		want bool
	}{
		// Cases from Is/SameValueZero that should match ===
		{"Undefined vs Undefined", Undefined, Undefined, true},
		{"Null vs Null", Null, Null, true},
		{"True vs True", True, True, true},
		{"False vs False", False, False, true},
		{"True vs False", True, False, false},
		{"Int vs Int (same)", IntegerValue(5), IntegerValue(5), true},
		{"Int vs Int (diff)", IntegerValue(5), IntegerValue(6), false},
		{"Float vs Float (same)", NumberValue(3.14), NumberValue(3.14), true},
		{"Float vs Float (diff)", NumberValue(3.14), NumberValue(3.15), false},
		{"+0 vs -0", NumberValue(0.0), NumberValue(math.Copysign(0.0, -1)), true}, // === treats zeros as equal
		{"BigInt vs BigInt (same)", NewBigInt(big.NewInt(100)), NewBigInt(big.NewInt(100)), true},
		{"BigInt vs BigInt (diff)", NewBigInt(big.NewInt(100)), NewBigInt(big.NewInt(101)), false},
		{"String vs String (same)", NewString("a"), NewString("a"), true},
		{"String vs String (diff)", NewString("a"), NewString("b"), false},
		{"Symbol vs Symbol (same obj)", sym1, sym1, true},
		{"Symbol vs Symbol (diff obj)", sym1, sym1b, false},
		{"Object vs Object (same obj)", obj1, obj1, true},
		{"Object vs Object (diff obj)", obj1, obj2, false},
		{"Array vs Array (same obj)", arr1, arr1, true},
		{"Array vs Array (diff obj)", arr1, arr2, false},
		{"Function vs Function (same obj)", fn1, fn1, true},
		{"Function vs Function (diff obj)", fn1, fn2, false},
		{"Closure vs Closure (same obj)", closure1, closure1, true},
		{"Closure vs Closure (diff obj, same func)", closure1, closure1b, false},
		{"NativeFunc vs NativeFunc (same obj)", nativeFn1, nativeFn1, true},
		{"NativeFunc vs NativeFunc (diff obj)", nativeFn1, nativeFn1b, false},

		// Cases where === differs from Is/SameValueZero
		{"NaN vs NaN", NaN, NumberValue(math.NaN()), false}, // NaN !== NaN

		// Cases where types differ (always false for ===)
		// Note: Int and Float with same value are EQUAL in JS (5 === 5.0 is true)
		{"Int vs Float (same value)", IntegerValue(5), NumberValue(5.0), true},
		{"Int vs Null", IntegerValue(1), Null, false},
		{"String vs Int", NewString("1"), IntegerValue(1), false},
		{"Object vs Null", obj1, Null, false},
		{"Array vs Object", arr1, obj1, false},
		{"BigInt vs Int", NewBigInt(big.NewInt(5)), IntegerValue(5), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v1.StrictlyEquals(tc.v2); got != tc.want {
				t.Errorf("StrictlyEquals(%v, %v) mismatch. Expected %t, got %t", tc.v1, tc.v2, tc.want, got)
			}
			// Check symmetry
			if got := tc.v2.StrictlyEquals(tc.v1); got != tc.want {
				t.Errorf("StrictlyEquals(%v, %v) symmetry mismatch. Expected %t, got %t", tc.v2, tc.v1, tc.want, got)
			}
		})
	}
}

func TestEquals(t *testing.T) {
	// Use objects/symbols from StrictEquals test for reference checks where needed
	sym1 := Value{typ: TypeSymbol, obj: unsafe.Pointer(&SymbolObject{value: "s"})}
	obj1 := Value{typ: TypeObject, obj: unsafe.Pointer(&PlainObject{prototype: DefaultObjectPrototype})}

	testCases := []struct {
		name string
		v1   Value
		v2   Value
		want bool
	}{
		// Same type -> Strict Equality
		{"Null == Null", Null, Null, true},
		{"Undefined == Undefined", Undefined, Undefined, true},
		{"True == True", True, True, true},
		{"Int 5 == Int 5", IntegerValue(5), IntegerValue(5), true},
		{"Int 5 == Int 6", IntegerValue(5), IntegerValue(6), false},
		{"NaN == NaN", NaN, NumberValue(math.NaN()), false}, // NaN == NaN is false
		{"Str a == Str a", NewString("a"), NewString("a"), true},
		{"Str a == Str b", NewString("a"), NewString("b"), false},
		{"Obj1 == Obj1", obj1, obj1, true},
		{"Obj1 == Obj2", obj1, NewObject(DefaultObjectPrototype), false},
		{"Sym1 == Sym1", sym1, sym1, true},
		{"Sym1 == Sym2", sym1, NewSymbol("s"), false}, // Different symbol objects
		{"BigInt 100 == BigInt 100", NewBigInt(big.NewInt(100)), NewBigInt(big.NewInt(100)), true},
		{"BigInt 100 == BigInt 101", NewBigInt(big.NewInt(100)), NewBigInt(big.NewInt(101)), false},

		// Null / Undefined
		{"Null == Undefined", Null, Undefined, true},
		{"Undefined == Null", Undefined, Null, true},
		{"Null == 0", Null, IntegerValue(0), false},
		{"Undefined == 0", Undefined, IntegerValue(0), false},
		{"Null == False", Null, False, false},
		{"Undefined == False", Undefined, False, false},

		// Number / String Coercion
		{"Int 5 == Str 5", IntegerValue(5), NewString("5"), true},
		{"Float 5.0 == Str 5", NumberValue(5.0), NewString("5"), true},
		{"Str 5 == Int 5", NewString("5"), IntegerValue(5), true},
		{"Str 5 == Float 5.0", NewString("5"), NumberValue(5.0), true},
		{"Int 5 == Str 5.0", IntegerValue(5), NewString("5.0"), true},
		{"Float 5.1 == Str 5.1", NumberValue(5.1), NewString("5.1"), true},
		{"Int 0 == Str 0", IntegerValue(0), NewString("0"), true},
		{"Float 0.0 == Str 0", NumberValue(0.0), NewString("0"), true},
		{"Int 1 == Str 1.0", IntegerValue(1), NewString("1.0"), true},
		{"Int 1 == Str 0x1", IntegerValue(1), NewString("0x1"), true},       // ToNumber("0x1") === 1
		{"Int 255 == Str 0xFF", IntegerValue(255), NewString("0xFF"), true}, // ToNumber("0xFF") === 255
		{"Int 5 == Str abc", IntegerValue(5), NewString("abc"), false},       // "abc" -> NaN
		{"Str abc == Int 5", NewString("abc"), IntegerValue(5), false},
		{"NaN == Str NaN", NaN, NewString("NaN"), false}, // NaN == NaN(from string) is false

		// Boolean Coercion
		{"True == Int 1", True, IntegerValue(1), true},
		{"Int 1 == True", IntegerValue(1), True, true},
		{"True == Float 1.0", True, NumberValue(1.0), true},
		{"Float 1.0 == True", NumberValue(1.0), True, true},
		{"False == Int 0", False, IntegerValue(0), true},
		{"Int 0 == False", IntegerValue(0), False, true},
		{"False == Float 0.0", False, NumberValue(0.0), true},
		{"Float 0.0 == False", NumberValue(0.0), False, true},
		{"False == Float -0.0", False, NumberValue(math.Copysign(0.0, -1)), true}, // -0 == false
		{"True == Int 2", True, IntegerValue(2), false},
		{"False == Int 1", False, IntegerValue(1), false},
		{"True == Str 1", True, NewString("1"), true},
		{"Str 1 == True", NewString("1"), True, true},
		{"False == Str 0", False, NewString("0"), true},
		{"Str 0 == False", NewString("0"), False, true},
		{"True == Str true", True, NewString("true"), false},     // "true" -> NaN
		{"False == Str false", False, NewString("false"), false}, // "false" -> NaN
		{"False == EmptyStr", False, NewString(""), true},        // "" -> 0 == ToNumber(false)
		{"EmptyStr == False", NewString(""), False, true},
		{"True == EmptyStr", True, NewString(""), false}, // "" -> 0 != ToNumber(true)

		// BigInt Coercion
		{"BigInt 5 == Int 5", NewBigInt(big.NewInt(5)), IntegerValue(5), true},
		{"Int 5 == BigInt 5", IntegerValue(5), NewBigInt(big.NewInt(5)), true},
		{"BigInt 5 == Float 5.0", NewBigInt(big.NewInt(5)), NumberValue(5.0), true},
		{"Float 5.0 == BigInt 5", NumberValue(5.0), NewBigInt(big.NewInt(5)), true},
		{"BigInt 5 == Float 5.1", NewBigInt(big.NewInt(5)), NumberValue(5.1), false},
		{"Float 5.1 == BigInt 5", NumberValue(5.1), NewBigInt(big.NewInt(5)), false},
		{"BigInt 0 == Float 0.0", NewBigInt(big.NewInt(0)), NumberValue(0.0), true},
		{"BigInt 0 == Float -0.0", NewBigInt(big.NewInt(0)), NumberValue(math.Copysign(0.0, -1)), true},
		{"BigInt 1 == True", NewBigInt(big.NewInt(1)), True, true}, // BigInt(1) == ToNumber(True=1)
		{"True == BigInt 1", True, NewBigInt(big.NewInt(1)), true},
		{"BigInt 0 == False", NewBigInt(big.NewInt(0)), False, true},
		{"False == BigInt 0", False, NewBigInt(big.NewInt(0)), true},
		{"BigInt 1 == Str 1", NewBigInt(big.NewInt(1)), NewString("1"), true},
		{"Str 1 == BigInt 1", NewString("1"), NewBigInt(big.NewInt(1)), true},
		{"BigInt 1 == Str 1.0", NewBigInt(big.NewInt(1)), NewString("1.0"), false}, // StringToBigInt("1.0") fails
		{"Str 1.0 == BigInt 1", NewString("1.0"), NewBigInt(big.NewInt(1)), false},
		{"BigInt 1 == Str 0x1", NewBigInt(big.NewInt(1)), NewString("0x1"), true}, // StringToBigInt("0x1") works
		{"Str 0x1 == BigInt 1", NewString("0x1"), NewBigInt(big.NewInt(1)), true},

		// Objects - generally only equal to self, null, undefined (no ToPrimitive yet)
		{"Obj == Null", obj1, Null, false},
		{"Obj == Undefined", obj1, Undefined, false},
		{"Obj == True", obj1, True, false},
		{"True == Obj", True, obj1, false},
		{"Obj == 0", obj1, IntegerValue(0), false},
		{"Obj == Str", obj1, NewString("test"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v1.Equals(tc.v2); got != tc.want {
				t.Errorf("Equals(%v, %v) mismatch. Expected %t, got %t", tc.v1, tc.v2, tc.want, got)
			}
			// Check symmetry (should hold for ==)
			if got := tc.v2.Equals(tc.v1); got != tc.want {
				t.Errorf("Equals(%v, %v) symmetry mismatch. Expected %t, got %t", tc.v2, tc.v1, tc.want, got)
			}
		})
	}
}

func TestDictObjectValue(t *testing.T) {
	// Create a prototype (can be Null or DefaultObjectPrototype)
	proto := DefaultObjectPrototype
	v := NewDictObject(proto)

	if v.Type() != TypeDictObject {
		t.Errorf("Type mismatch. Expected %v, got %v", TypeDictObject, v.Type())
	}
	if !v.IsDictObject() {
		t.Errorf("Expected IsDictObject() == true")
	}
	if !v.IsObject() {
		t.Errorf("Expected IsObject() == true for DictObject")
	}
	dictObj := v.AsDictObject()
	if dictObj == nil {
		t.Fatalf("AsDictObject() returned nil")
	}
	if dictObj.prototype != proto {
		t.Errorf("Prototype mismatch. Expected %v, got %v", proto, dictObj.prototype)
	}
	if dictObj.properties == nil {
		t.Errorf("Expected properties map to be initialized")
	}
	// Add a property and check
	dictObj.properties["foo"] = IntegerValue(42)
	if got := dictObj.properties["foo"]; !got.Is(IntegerValue(42)) {
		t.Errorf("Property value mismatch. Expected 42, got %v", got)
	}
	// ToString and TypeName
	expectedStr := "[object Object]"
	if gotStr := v.ToString(); gotStr != expectedStr {
		t.Errorf("ToString mismatch. Expected %q, got %q", expectedStr, gotStr)
	}
	expectedTypeName := "object"
	if gotTypeName := v.TypeName(); gotTypeName != expectedTypeName {
		t.Errorf("TypeName mismatch. Expected %q, got %q", expectedTypeName, gotTypeName)
	}
	// Panic checks
	expectPanic(t, func() { v.AsPlainObject() }, "not an object")
	expectPanic(t, func() { v.AsArray() }, "not an array")
}
