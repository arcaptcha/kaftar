// fp is a package for functional programming utilities
package fp

func Must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

func Mapper[I, O any](s []I, f func(I) O) []O {
	slice := make([]O, len(s))
	for i := range s {
		slice[i] = f(s[i])
	}
	return slice
}
