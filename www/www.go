package www

import "fmt"

type Endpoint struct {
	Protocol  string
	IpAddress string
	Port      int
	Path      string
}

func (e *Endpoint) URL() string {
	return fmt.Sprintf("%s://%s:%d%s", e.Protocol, e.IpAddress, e.Port, e.Path)
}
