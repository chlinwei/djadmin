package pagination

import "testing"

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		number   int32
		size     int32
		expected Page
	}{
		{name: "defaults invalid values", number: 0, size: 0, expected: Page{Number: 1, Size: 10, Offset: 0}},
		{name: "calculates offset", number: 3, size: 20, expected: Page{Number: 3, Size: 20, Offset: 40}},
		{name: "clamps maximum size", number: 2, size: 100, expected: Page{Number: 2, Size: 30, Offset: 30}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := New(test.number, test.size)
			if actual != test.expected {
				t.Fatalf("New(%d, %d) = %+v, expected %+v", test.number, test.size, actual, test.expected)
			}
		})
	}
}
