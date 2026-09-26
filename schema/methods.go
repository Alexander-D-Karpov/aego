package schema

func (r *Registry) Register[T any](opts ...Option) (*TypeDescriptor, error) {
	return Register[T](r, opts...)
}

func (r *Registry) RegisterStruct[T any](opts ...Option) (*TypeDescriptor, error) {
	return RegisterStruct[T](r, opts...)
}
