package bridge

import "net/url"

func NewMock(uri *url.URL) (RegistryAdapter, error) {
	return &fakeAdapter{}, nil
}

type fakeAdapter struct{}

func (f *fakeAdapter) Ping() error {
	return ErrCallNotSupported
}
func (f *fakeAdapter) Register(service *Service) error {
	return ErrCallNotSupported
}
func (f *fakeAdapter) Deregister(service *Service) error {
	return ErrCallNotSupported
}
func (f *fakeAdapter) Refresh(service *Service) error {
	return ErrCallNotSupported
}

func (f *fakeAdapter) Services() ([]*Service, error) {
	return nil, ErrCallNotSupported
}
