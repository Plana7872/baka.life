package validator

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/NyanLoli-Network/baka.life/registryctl/provider"
	"github.com/NyanLoli-Network/baka.life/registryctl/registry"
)

type Error struct {
	File string
	Line int
	Text string
	Msg  string
}

func (e Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %q", e.File, e.Line, e.Msg, e.Text)
	}
	if e.File != "" {
		return fmt.Sprintf("%s: %s", e.File, e.Msg)
	}
	return e.Msg
}

type Errors []Error

func (e Errors) Error() string {
	parts := make([]string, 0, len(e))
	for _, err := range e {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}

func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

type Validator struct{}

func Validate(reg *registry.Registry) error {
	return Validator{}.Validate(reg)
}

func AuthorizeGitHub(reg *registry.Registry, author string) error {
	return Validator{}.AuthorizeGitHub(reg, author)
}

func HasGitHubAuth(reg *registry.Registry, author string) bool {
	if reg == nil {
		return false
	}
	for _, name := range sortedMaintainers(reg) {
		if githubAuthorized(reg.Maintainers[name], author) {
			return true
		}
	}
	return false
}

func AuthorizeDomainChanges(base, head *registry.Registry, changedFiles []string, author string) error {
	return Validator{}.AuthorizeDomainChanges(base, head, changedFiles, author)
}

func (Validator) Validate(reg *registry.Registry) error {
	var errs Errors
	if reg == nil {
		return Errors{{Msg: "registry is nil"}}
	}

	maintainerNames := sortedMaintainers(reg)
	for _, name := range maintainerNames {
		errs = append(errs, validateMaintainer(reg.Maintainers[name])...)
	}

	domainNames := sortedDomains(reg)
	for _, name := range domainNames {
		errs = append(errs, validateDomain(reg, reg.Domains[name])...)
	}

	return errs.Err()
}

func (Validator) AuthorizeGitHub(reg *registry.Registry, author string) error {
	author = strings.TrimSpace(author)
	if author == "" {
		return Errors{{Msg: "GitHub author is required for authorization"}}
	}
	if reg == nil {
		return Errors{{Msg: "registry is nil"}}
	}

	var errs Errors
	for _, name := range sortedDomains(reg) {
		domain := reg.Domains[name]
		mntner := reg.Maintainers[domain.Maintainer]
		if mntner == nil {
			continue
		}
		if !githubAuthorized(mntner, author) {
			errs = append(errs, Error{File: domain.File, Line: domain.MaintainerLine, Msg: fmt.Sprintf("GitHub author %q is not authorized by maintainer %q", author, domain.Maintainer)})
		}
	}
	return errs.Err()
}

func (Validator) AuthorizeDomainChanges(base, head *registry.Registry, changedFiles []string, author string) error {
	author = strings.TrimSpace(author)
	if author == "" {
		return Errors{{Msg: "GitHub author is required for authorization"}}
	}
	if base == nil || head == nil {
		return Errors{{Msg: "base and PR registries are required for authorization"}}
	}

	var errs Errors
	for _, file := range changedFiles {
		domainName := registry.NormalizeName(strings.TrimPrefix(file, "registry/domain/"))
		baseDomain := base.Domains[domainName]
		headDomain := head.Domains[domainName]

		if baseDomain != nil && !maintainerAuthorized(base, baseDomain.Maintainer, author) {
			errs = append(errs, Error{File: file, Msg: fmt.Sprintf("GitHub author %q is not authorized to change existing domain %q", author, domainName)})
		}
		if headDomain != nil && !maintainerAuthorized(base, headDomain.Maintainer, author) {
			errs = append(errs, Error{File: file, Msg: fmt.Sprintf("GitHub author %q is not authorized by base maintainer %q", author, headDomain.Maintainer)})
		}
	}
	return errs.Err()
}

func validateMaintainer(mntner *registry.Maintainer) Errors {
	var errs Errors
	if mntner == nil {
		return append(errs, Error{Msg: "maintainer is nil"})
	}
	if strings.TrimSpace(mntner.Name) == "" {
		errs = append(errs, Error{File: mntner.File, Line: mntner.NameLine, Msg: "mntner field is required"})
	}
	if len(mntner.Auth) == 0 {
		errs = append(errs, Error{File: mntner.File, Msg: "at least one auth field is required"})
	}
	for _, auth := range mntner.Auth {
		switch auth.Method {
		case "github":
			if !validGitHubUsername(auth.Value) {
				errs = append(errs, Error{File: mntner.File, Line: auth.Line, Text: "auth: " + auth.Raw, Msg: "invalid github auth username"})
			}
		case "ssh-ed25519":
			if !strings.HasPrefix(auth.Raw, "ssh-ed25519 ") || strings.TrimSpace(auth.Value) == "" {
				errs = append(errs, Error{File: mntner.File, Line: auth.Line, Text: "auth: " + auth.Raw, Msg: "invalid ssh-ed25519 auth"})
			}
		default:
			errs = append(errs, Error{File: mntner.File, Line: auth.Line, Text: "auth: " + auth.Raw, Msg: "unsupported auth method"})
		}
	}
	return errs
}

func validateDomain(reg *registry.Registry, domain *registry.Domain) Errors {
	var errs Errors
	if domain == nil {
		return append(errs, Error{Msg: "domain is nil"})
	}

	if domain.Name == "" {
		errs = append(errs, Error{File: domain.File, Line: domain.NameLine, Msg: "domain field is required"})
	} else if !validFQDN(domain.Name) {
		errs = append(errs, Error{File: domain.File, Line: domain.NameLine, Msg: "domain must be a valid FQDN"})
	}

	if domain.Maintainer == "" {
		errs = append(errs, Error{File: domain.File, Line: domain.MaintainerLine, Msg: "mnt-by field is required"})
	} else if reg.Maintainers[domain.Maintainer] == nil {
		errs = append(errs, Error{File: domain.File, Line: domain.MaintainerLine, Msg: "mnt-by references unknown maintainer " + domain.Maintainer})
	}

	if domain.Name != "" && filepath.Base(domain.File) != domain.Name {
		errs = append(errs, Error{File: domain.File, Msg: fmt.Sprintf("filename must match domain name %q", domain.Name)})
	}

	errs = append(errs, validateRecords(domain)...)
	return errs
}

func validateRecords(domain *registry.Domain) Errors {
	var errs Errors
	byName := map[string][]registry.Record{}
	keys := map[string]registry.Record{}

	for _, record := range domain.Records {
		fqdn := registry.FQDN(domain.Name, record.Name)
		if !validRecordName(record.Name, fqdn) {
			errs = append(errs, recordError(domain, record, "record name must produce a valid FQDN"))
		}
		byName[fqdn] = append(byName[fqdn], record)

		key := provider.Key(fqdn, record.Type)
		if existing, ok := keys[key]; ok {
			errs = append(errs, recordError(domain, record, fmt.Sprintf("duplicate record key also defined on line %d", existing.Line)))
		} else {
			keys[key] = record
		}

		if record.Proxied && !registry.IsProxyable(record.Type) {
			errs = append(errs, recordError(domain, record, "proxied is only valid for A, AAAA, and CNAME records"))
		}

		switch record.Type {
		case registry.TypeA:
			if ip := net.ParseIP(record.Content); ip == nil || ip.To4() == nil {
				errs = append(errs, recordError(domain, record, "A record content must be an IPv4 address"))
			}
		case registry.TypeAAAA:
			if ip := net.ParseIP(record.Content); ip == nil || ip.To4() != nil {
				errs = append(errs, recordError(domain, record, "AAAA record content must be an IPv6 address"))
			}
		case registry.TypeCNAME:
			target := registry.TargetName(domain.Name, record.Content)
			if net.ParseIP(record.Content) != nil {
				errs = append(errs, recordError(domain, record, "CNAME target must be a domain name, not an IP address"))
			} else if !validFQDN(target) {
				errs = append(errs, recordError(domain, record, "CNAME target must be a valid FQDN"))
			} else if target == fqdn {
				errs = append(errs, recordError(domain, record, "CNAME target cannot reference itself"))
			}
		case registry.TypeTXT:
			if record.Content == "" {
				errs = append(errs, recordError(domain, record, "TXT content is required"))
			}
		case registry.TypeMX:
			errs = append(errs, validateRange(domain, record, record.Priority, "MX priority", 0, 65535)...)
			if !validFQDN(record.Target) {
				errs = append(errs, recordError(domain, record, "MX target must be a valid FQDN"))
			}
		case registry.TypeNS:
			if !validFQDN(record.Target) {
				errs = append(errs, recordError(domain, record, "NS target must be a valid FQDN"))
			}
		case registry.TypeSRV:
			errs = append(errs, validateRange(domain, record, record.Priority, "SRV priority", 0, 65535)...)
			errs = append(errs, validateRange(domain, record, record.Weight, "SRV weight", 0, 65535)...)
			errs = append(errs, validateRange(domain, record, record.Port, "SRV port", 1, 65535)...)
			if !validSRVName(record.Name) {
				errs = append(errs, recordError(domain, record, "SRV name must start with _service._proto"))
			}
			if !validFQDN(record.Target) {
				errs = append(errs, recordError(domain, record, "SRV target must be a valid FQDN"))
			}
		case registry.TypeCAA:
			if !validCAA(record.Content) {
				errs = append(errs, recordError(domain, record, "CAA content must contain flags, tag, and value"))
			}
		default:
			errs = append(errs, recordError(domain, record, "unsupported DNS record type"))
		}
	}

	for name, records := range byName {
		errs = append(errs, validateNameCombinations(domain, name, records)...)
	}
	return errs
}

func validateNameCombinations(domain *registry.Domain, name string, records []registry.Record) Errors {
	var errs Errors
	hasCNAME := false
	hasNonCNAME := false
	var cname registry.Record
	hasNS := false
	hasNonNS := false

	for _, record := range records {
		if record.Type == registry.TypeCNAME {
			hasCNAME = true
			cname = record
		} else {
			hasNonCNAME = true
		}
		if record.Type == registry.TypeNS {
			hasNS = true
		} else {
			hasNonNS = true
		}
	}

	if hasCNAME && hasNonCNAME {
		errs = append(errs, recordError(domain, cname, "CNAME cannot coexist with other record types at the same name"))
	}
	if hasNS && hasNonNS && name != domain.Name {
		errs = append(errs, Error{File: domain.File, Msg: "NS records cannot be combined with other record types except at zone apex"})
	}
	return errs
}

func validateRange(domain *registry.Domain, record registry.Record, value *int, field string, min, max int) Errors {
	if value == nil {
		return Errors{recordError(domain, record, field+" is required")}
	}
	if *value < min || *value > max {
		return Errors{recordError(domain, record, fmt.Sprintf("%s must be between %d and %d", field, min, max))}
	}
	return nil
}

func recordError(domain *registry.Domain, record registry.Record, msg string) Error {
	return Error{File: domain.File, Line: record.Line, Text: record.Raw, Msg: msg}
}

func githubAuthorized(mntner *registry.Maintainer, author string) bool {
	if mntner == nil {
		return false
	}
	for _, auth := range mntner.Auth {
		if auth.Method == "github" && strings.EqualFold(auth.Value, author) {
			return true
		}
	}
	return false
}

func maintainerAuthorized(reg *registry.Registry, maintainer, author string) bool {
	return githubAuthorized(reg.Maintainers[maintainer], author)
}

var githubUsernameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

func validGitHubUsername(username string) bool {
	return githubUsernameRE.MatchString(username)
}

func validFQDN(name string) bool {
	name = registry.NormalizeName(name)
	if name == "" || len(name) > 253 || strings.Contains(name, "..") {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !validDNSLabel(label, false) {
			return false
		}
	}
	return true
}

func validRecordName(name, fqdn string) bool {
	if name == "@" {
		return validFQDN(fqdn)
	}
	labels := strings.Split(registry.NormalizeName(fqdn), ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !validDNSLabel(label, true) {
			return false
		}
	}
	return true
}

func validDNSLabel(label string, allowUnderscore bool) bool {
	if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, r := range label {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		if allowUnderscore && r == '_' {
			continue
		}
		return false
	}
	return true
}

func validSRVName(name string) bool {
	labels := strings.Split(registry.NormalizeName(name), ".")
	return len(labels) >= 2 && strings.HasPrefix(labels[0], "_") && strings.HasPrefix(labels[1], "_")
}

func validCAA(content string) bool {
	fields := strings.Fields(content)
	if len(fields) < 3 {
		return false
	}
	if fields[1] == "" || strings.ContainsAny(fields[1], " \t") {
		return false
	}
	var flags int
	if _, err := fmt.Sscanf(fields[0], "%d", &flags); err != nil {
		return false
	}
	return flags >= 0 && flags <= 255
}

func sortedMaintainers(reg *registry.Registry) []string {
	names := make([]string, 0, len(reg.Maintainers))
	for name := range reg.Maintainers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedDomains(reg *registry.Registry) []string {
	names := make([]string, 0, len(reg.Domains))
	for name := range reg.Domains {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
