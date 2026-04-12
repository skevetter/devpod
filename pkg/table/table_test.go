package table

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	headers := []string{"Name", "Age"}
	rows := [][]string{
		{"Alice", "30"},
		{"Bob", "25"},
	}

	out := Render(headers, rows)

	if !strings.Contains(out, "Alice") {
		t.Errorf("expected output to contain 'Alice', got:\n%s", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("expected output to contain 'Bob', got:\n%s", out)
	}
	if !strings.Contains(out, "Name") {
		t.Errorf("expected output to contain header 'Name', got:\n%s", out)
	}
	if !strings.Contains(out, "Age") {
		t.Errorf("expected output to contain header 'Age', got:\n%s", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	headers := []string{"Name", "Age"}
	rows := [][]string{}

	out := Render(headers, rows)

	if !strings.Contains(out, "Name") {
		t.Errorf("expected output to contain header 'Name', got:\n%s", out)
	}
}
