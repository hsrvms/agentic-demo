package ingestion

import "testing"

func TestDedup_FirstCallReturnsFalse(t *testing.T) {
	d := NewDedup()
	if d.Seen("hello") {
		t.Error("Seen(\"hello\") first call should return false")
	}
}

func TestDedup_SecondSameCallReturnsTrue(t *testing.T) {
	d := NewDedup()
	d.Seen("hello")
	if !d.Seen("hello") {
		t.Error("Seen(\"hello\") second call should return true")
	}
}

func TestDedup_DifferentContentReturnsFalse(t *testing.T) {
	d := NewDedup()
	d.Seen("alpha")
	if d.Seen("beta") {
		t.Error("Seen(\"beta\") after Seen(\"alpha\") should return false")
	}
}

func TestDedup_EmptyString(t *testing.T) {
	d := NewDedup()
	if d.Seen("") {
		t.Error("Seen(\"\") first call should return false")
	}
	if !d.Seen("") {
		t.Error("Seen(\"\") second call should return true")
	}
}
