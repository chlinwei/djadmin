package pagination

const (
	DefaultSize = 10
	MaxSize     = 30
)

type Page struct {
	Number int32
	Size   int32
	Offset int32
}

func New(number int32, size int32) Page {
	if number < 1 {
		number = 1
	}
	if size < 1 {
		size = DefaultSize
	}
	if size > MaxSize {
		size = MaxSize
	}
	return Page{Number: number, Size: size, Offset: (number - 1) * size}
}
