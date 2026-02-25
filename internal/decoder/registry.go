package decoder

import (
	"fmt"
	"sync"
)

// Registry manages registered protocol decoders.
type Registry struct {
	mu       sync.RWMutex
	decoders map[string]Decoder
}

// NewRegistry creates a new decoder registry.
func NewRegistry() *Registry {
	return &Registry{
		decoders: make(map[string]Decoder),
	}
}

// Register adds a decoder to the registry.
// Returns an error if a decoder for the same protocol is already registered.
func (r *Registry) Register(d Decoder) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := d.Protocol()
	if _, exists := r.decoders[name]; exists {
		return fmt.Errorf("decoder already registered for protocol: %s", name)
	}
	r.decoders[name] = d
	return nil
}

// Get returns the decoder for a given protocol name.
func (r *Registry) Get(protocol string) (Decoder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.decoders[protocol]
	return d, ok
}

// All returns all registered decoders.
func (r *Registry) All() []Decoder {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Decoder, 0, len(r.decoders))
	for _, d := range r.decoders {
		result = append(result, d)
	}
	return result
}

// Protocols returns the names of all registered protocols.
func (r *Registry) Protocols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.decoders))
	for name := range r.decoders {
		result = append(result, name)
	}
	return result
}
