package set

type options struct {
	capacity int
	keysLen  int
	addKeys  func(any)
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

func WithKeys[T comparable](keys []T) optFunc {
	return func(o *options) {
		o.keysLen = len(keys)
		o.addKeys = func(dst any) {
			s, ok := dst.(Set[T])
			if !ok {
				panic("set: WithKeys key type does not match set type")
			}
			s.AddAll(keys...)
		}
	}
}
