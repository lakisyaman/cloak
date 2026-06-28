package secrets

import (
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const DefaultService = "cloak"

type SecretRef struct {
	Store   string `json:"store"`
	Service string `json:"service"`
	User    string `json:"user"`
}

type Store interface {
	Set(ref SecretRef, value string) error
	Get(ref SecretRef) (string, error)
	Delete(ref SecretRef) error
}

type KeyringStore struct{}

func NewRef(managedCLI, contextName, field string) SecretRef {
	return SecretRef{
		Store:   "os",
		Service: DefaultService,
		User:    KeyringUser(managedCLI, contextName, field),
	}
}

func KeyringUser(managedCLI, contextName, field string) string {
	return strings.Join([]string{managedCLI, contextName, field}, "/")
}

func (KeyringStore) Set(ref SecretRef, value string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	return keyring.Set(ref.Service, ref.User, value)
}

func (KeyringStore) Get(ref SecretRef) (string, error) {
	if err := validateRef(ref); err != nil {
		return "", err
	}
	return keyring.Get(ref.Service, ref.User)
}

func (KeyringStore) Delete(ref SecretRef) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	return keyring.Delete(ref.Service, ref.User)
}

func validateRef(ref SecretRef) error {
	if ref.Store != "" && ref.Store != "os" {
		return fmt.Errorf("unsupported secret store %q", ref.Store)
	}
	if ref.Service == "" {
		return fmt.Errorf("secret service is required")
	}
	if ref.User == "" {
		return fmt.Errorf("secret user is required")
	}
	return nil
}
