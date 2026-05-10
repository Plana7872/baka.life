package registry

import (
	"strings"
)

const (
	TypeA     = "A"
	TypeAAAA  = "AAAA"
	TypeCNAME = "CNAME"
	TypeTXT   = "TXT"
	TypeMX    = "MX"
	TypeNS    = "NS"
	TypeSRV   = "SRV"
	TypeCAA   = "CAA"
)

var SupportedRecordTypes = map[string]struct{}{
	TypeA:     {},
	TypeAAAA:  {},
	TypeCNAME: {},
	TypeTXT:   {},
	TypeMX:    {},
	TypeNS:    {},
	TypeSRV:   {},
	TypeCAA:   {},
}

type Registry struct {
	Maintainers map[string]*Maintainer
	Domains     map[string]*Domain
}

type Maintainer struct {
	Name     string
	NameLine int
	Descr    []string
	Auth     []Auth
	File     string
}

type Auth struct {
	Method string
	Value  string
	Raw    string
	Line   int
}

type Domain struct {
	Name           string
	NameLine       int
	Descr          []string
	Maintainer     string
	MaintainerLine int
	Records        []Record
	File           string
}

type Record struct {
	Name     string
	Type     string
	Content  string
	Priority *int
	Weight   *int
	Port     *int
	Target   string
	Proxied  bool
	Line     int
	Raw      string
}

func New() *Registry {
	return &Registry{
		Maintainers: map[string]*Maintainer{},
		Domains:     map[string]*Domain{},
	}
}

func NormalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".")
	return strings.ToLower(name)
}

func FQDN(zone, name string) string {
	zone = NormalizeName(zone)
	name = NormalizeName(name)
	if name == "" || name == "@" {
		return zone
	}
	if name == zone || strings.HasSuffix(name, "."+zone) {
		return name
	}
	return name + "." + zone
}

func TargetName(zone, name string) string {
	name = NormalizeName(name)
	if name == "@" {
		return NormalizeName(zone)
	}
	if name == "" {
		return name
	}
	if !strings.Contains(name, ".") {
		return name + "." + NormalizeName(zone)
	}
	return name
}

func IsProxyable(recordType string) bool {
	switch strings.ToUpper(recordType) {
	case TypeA, TypeAAAA, TypeCNAME:
		return true
	default:
		return false
	}
}
