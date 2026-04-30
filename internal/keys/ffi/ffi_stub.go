//go:build !cgo

package ffi

import "github.com/JunoPayServer/juno-pay-server/internal/keys"

type Deriver struct{}

func New() *Deriver { return &Deriver{} }

func (d *Deriver) UFVKFromSeedBase64(string, string, uint32, uint32) (string, error) {
	return "", keys.ErrUnavailable
}

func (d *Deriver) AddressFromUFVK(string, string, keys.Scope, uint32) (string, error) {
	return "", keys.ErrUnavailable
}
