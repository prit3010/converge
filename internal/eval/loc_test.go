package eval

import "testing"

func TestCountLOC(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		want     int
	}{
		{name: "go comments", filename: "main.go", content: "package main\n\n// comment\nfunc main() {}\n", want: 2},
		{name: "python", filename: "app.py", content: "# c\nx = 1\n\ny = 2\n", want: 2},
		{name: "unknown", filename: "data.txt", content: "a\n\nb\n", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountLOC(tt.filename, tt.content); got != tt.want {
				t.Fatalf("CountLOC()=%d want %d", got, tt.want)
			}
		})
	}
}
