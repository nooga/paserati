package vm

import (
	"testing"
)

func TestPlainObjectBasic(t *testing.T) {
	poVal := NewObject(DefaultObjectPrototype)
	po := poVal.AsPlainObject()
	// No properties initially
	if po.HasOwn("foo") {
		t.Errorf("expected HasOwn(\"foo\") to be false on new object")
	}
	if v, ok := po.GetOwn("foo"); ok {
		t.Errorf("expected GetOwn(\"foo\") ok=false, got ok=true, v=%v", v)
	}
	// Define a property
	po.SetOwn("foo", IntegerValue(42))
	if !po.HasOwn("foo") {
		t.Errorf("expected HasOwn(\"foo\") true after SetOwn")
	}
	v, ok := po.GetOwn("foo")
	if !ok {
		t.Fatalf("expected GetOwn(\"foo\") ok=true after SetOwn")
	}
	if v.AsInteger() != 42 {
		t.Errorf("expected GetOwn to return 42, got %d", v.AsInteger())
	}
	// Overwrite existing property
	po.SetOwn("foo", IntegerValue(7))
	v2, ok2 := po.GetOwn("foo")
	if !ok2 || v2.AsInteger() != 7 {
		t.Errorf("expected overwritten value 7, got %v (ok=%v)", v2, ok2)
	}
	// OwnKeys should list "foo"
	keys := po.OwnKeys()
	if len(keys) != 1 || keys[0] != "foo" {
		t.Errorf("OwnKeys mismatch, expected [foo], got %v", keys)
	}
}

func TestPlainObjectShapeTransitions(t *testing.T) {
	po := NewObject(DefaultObjectPrototype).AsPlainObject()
	root := po.shape
	// first definition creates new shape
	po.SetOwn("a", IntegerValue(1))
	s1 := po.shape
	if s1 == root {
		t.Errorf("expected new shape after first property, got same shape")
	}
	// redefining same property should keep shape
	po.SetOwn("a", IntegerValue(2))
	s2 := po.shape
	if s2 != s1 {
		t.Errorf("expected same shape on overwrite, got different shapes")
	}
	// adding another property creates another shape
	po.SetOwn("b", IntegerValue(3))
	s3 := po.shape
	if s3 == s2 {
		t.Errorf("expected new shape after adding second property, got same shape")
	}
	// fields order
	keys := po.OwnKeys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("OwnKeys order mismatch, expected [a b], got %v", keys)
	}
}

func TestDictObjectBasic(t *testing.T) {
	dVal := NewDictObject(DefaultObjectPrototype)
	d := dVal.AsDictObject()
	// No properties initially
	if d.HasOwn("x") {
		t.Errorf("expected HasOwn(\"x\") to be false on new dict object")
	}
	if v, ok := d.GetOwn("x"); ok {
		t.Errorf("expected GetOwn(\"x\") ok=false, got ok=true, v=%v", v)
	}
	// Define a property
	d.SetOwn("x", IntegerValue(100))
	if !d.HasOwn("x") {
		t.Errorf("expected HasOwn(\"x\") true after SetOwn")
	}
	v, ok := d.GetOwn("x")
	if !ok || v.AsInteger() != 100 {
		t.Errorf("expected GetOwn to return 100, got %v (ok=%v)", v, ok)
	}
	// Delete property
	if !d.DeleteOwn("x") {
		t.Errorf("expected DeleteOwn(\"x\") to return true")
	}
	if d.HasOwn("x") {
		t.Errorf("expected HasOwn(\"x\") false after DeleteOwn")
	}
	if _, ok2 := d.GetOwn("x"); ok2 {
		t.Errorf("expected GetOwn(\"x\") ok=false after DeleteOwn")
	}
	// Delete non-existing
	if d.DeleteOwn("x") {
		t.Errorf("expected DeleteOwn(\"x\") false when property absent")
	}
	// OwnKeys sorted
	d.SetOwn("b", IntegerValue(2))
	d.SetOwn("a", IntegerValue(1))
	keys := d.OwnKeys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("OwnKeys sorting mismatch, expected [a b], got %v", keys)
	}
}
