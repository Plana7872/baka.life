package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type DNSProvider interface {
	ListRecords(context.Context) ([]Record, error)
	CreateRecord(context.Context, Record) (Record, error)
	UpdateRecord(context.Context, Record) (Record, error)
	DeleteRecord(context.Context, Record) error
}

type Record struct {
	ID       string
	Type     string
	Name     string
	Content  string
	Priority *int
	Proxied  *bool
	Data     map[string]any
}

func (r Record) Key() string {
	return Key(r.Name, r.Type)
}

func Key(name, recordType string) string {
	return strings.ToLower(strings.TrimSuffix(name, ".")) + " " + strings.ToUpper(recordType)
}

func (r Record) DisplayContent() string {
	if r.Content != "" {
		return r.Content
	}
	if len(r.Data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(r.Data))
	for key := range r.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, r.Data[key]))
	}
	return strings.Join(parts, ",")
}
