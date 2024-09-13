package www

import (
	"encoding/json"
	"errors"
	"os"

	"golang.org/x/exp/rand"
)

type WWW map[int]Machine

func (w *WWW) Get(address int) (Machine, error) {
	machine, ok := (*w)[address]
	if !ok {
		return nil, errors.New("machine not found")
	}
	return machine, nil
}

func (w *WWW) RegisterMachine(machine Machine) error {
	// random address that is not already in the map
	newAddress := rand.Intn(1000000)
	for _, ok := (*w)[newAddress]; ok; newAddress = rand.Intn(1000000) {
		_, ok = (*w)[newAddress]
	}

	(*w)[newAddress] = machine
	return nil
}

func (w *WWW) AllAddresses() []int {
	addresses := []int{}
	for address := range *w {
		addresses = append(addresses, address)
	}
	return addresses
}

type Machine interface {
	Address() (int, error)
	Request() (*WebPage, error)
}

type PubAPIMachine struct {
	address int
	webPage *WebPage
}

func NewPubAPIMachine(address int, webPage *WebPage) (Machine, error) {
	return &PubAPIMachine{address: address, webPage: webPage}, nil
}

func (m *PubAPIMachine) Address() (int, error) {
	if m.address == 0 {
		return 0, errors.New("address is not set")
	}
	return m.address, nil
}

func (m *PubAPIMachine) Request() (*WebPage, error) {
	if m.webPage == nil {
		return nil, errors.New("webPage is nil")
	}
	return m.webPage, nil
}

func NewWWWFromPath(path string) (*WWW, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var data []*WebPage
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, err
	}
	return NewWWWFromContent(data)
}

func NewWWWFromContent(content []*WebPage) (*WWW, error) {
	www := make(WWW)
	for _, c := range content {
		machine, err := NewPubAPIMachine(0, c)
		if err != nil {
			return nil, err
		}
		err = www.RegisterMachine(machine)
		if err != nil {
			return nil, err
		}
	}
	return &www, nil
}
