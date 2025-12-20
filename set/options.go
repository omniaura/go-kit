package set

type options struct {
	capacity int
}

type optFunc func(*options)

func WithCapacity(capacity int) optFunc {
	if capacity < 0 {
		panic("capacity must be non-negative")
	}
	return func(o *options) {
		o.capacity = capacity
	}
}
