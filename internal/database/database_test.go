package database

import (
	"testing"
)

func TestDriverManager_RegisterAndList(t *testing.T) {
	dm := NewDriverManager()
	names := dm.List()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestDriverManager_Get_NotFound(t *testing.T) {
	dm := NewDriverManager()
	_, err := dm.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent database")
	}
}

func TestDriverManager_Remove(t *testing.T) {
	dm := NewDriverManager()
	dm.Remove("nonexistent")
}
