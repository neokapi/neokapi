package aiprovider

import "context"

// Unavailable returns a provider that cannot be reached: every call returns
// err. A tool built for a step whose provider has no key holds one, so the
// step runs, and fails only if a unit actually needs the provider. A
// translate step whose every unit the content memory answered makes no call
// and succeeds.
func Unavailable(name ProviderID, err error) LLMProvider {
	return &unavailable{name: name, err: err}
}

type unavailable struct {
	name ProviderID
	err  error
}

func (u *unavailable) Name() ProviderID { return u.name }

func (u *unavailable) InputModalities() []Modality { return nil }

func (u *unavailable) Translate(context.Context, TranslateRequest) (*TranslateResponse, error) {
	return nil, u.err
}

func (u *unavailable) Chat(context.Context, []Message) (*ChatResponse, error) {
	return nil, u.err
}

func (u *unavailable) ChatStructured(context.Context, []Message, JSONSchema) (*ChatResponse, error) {
	return nil, u.err
}

func (u *unavailable) Close() error { return nil }
