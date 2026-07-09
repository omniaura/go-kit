package errgroup

type Group struct{}

func (*Group) Wait() error {
	return nil
}
