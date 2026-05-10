package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NyanLoli-Network/baka.life/registryctl/registry"
)

type Parser struct{}

type Error struct {
	File string
	Line int
	Text string
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s:%d: %s: %q", e.File, e.Line, e.Msg, e.Text)
}

func ParseMaintainer(filename string, reader io.Reader) (*registry.Maintainer, error) {
	return Parser{}.ParseMaintainer(filename, reader)
}

func ParseDomain(filename string, reader io.Reader) (*registry.Domain, error) {
	return Parser{}.ParseDomain(filename, reader)
}

func ParseRegistry(root string) (*registry.Registry, error) {
	return Parser{}.ParseRegistry(root)
}

func (Parser) ParseMaintainer(filename string, reader io.Reader) (*registry.Maintainer, error) {
	mntner := &registry.Maintainer{File: filename}
	scanner := bufio.NewScanner(reader)
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := header(line)
		if !ok {
			return nil, parseError(filename, lineNo, raw, "expected maintainer field")
		}

		switch key {
		case "mntner":
			if mntner.Name != "" {
				return nil, parseError(filename, lineNo, raw, "duplicate mntner field")
			}
			mntner.Name = value
			mntner.NameLine = lineNo
		case "descr":
			mntner.Descr = append(mntner.Descr, value)
		case "auth":
			auth := parseAuth(value)
			auth.Line = lineNo
			mntner.Auth = append(mntner.Auth, auth)
		default:
			return nil, parseError(filename, lineNo, raw, "unknown maintainer field "+key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	return mntner, nil
}

func (Parser) ParseDomain(filename string, reader io.Reader) (*registry.Domain, error) {
	domain := &registry.Domain{File: filename}
	scanner := bufio.NewScanner(reader)
	lineNo := 0
	seenRecord := false

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if key, value, ok := header(line); ok {
			if seenRecord {
				return nil, parseError(filename, lineNo, raw, "domain headers must appear before records")
			}
			switch key {
			case "domain":
				if domain.Name != "" {
					return nil, parseError(filename, lineNo, raw, "duplicate domain field")
				}
				domain.Name = registry.NormalizeName(value)
				domain.NameLine = lineNo
			case "descr":
				domain.Descr = append(domain.Descr, value)
			case "mnt-by":
				if domain.Maintainer != "" {
					return nil, parseError(filename, lineNo, raw, "duplicate mnt-by field")
				}
				domain.Maintainer = value
				domain.MaintainerLine = lineNo
			default:
				return nil, parseError(filename, lineNo, raw, "unknown domain field "+key)
			}
			continue
		}

		seenRecord = true
		record, err := parseRecord(domain.Name, filename, lineNo, raw)
		if err != nil {
			return nil, err
		}
		domain.Records = append(domain.Records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	return domain, nil
}

func (p Parser) ParseRegistry(root string) (*registry.Registry, error) {
	reg := registry.New()

	if err := parseObjects(filepath.Join(root, "registry", "mntner"), func(path string) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		mntner, err := p.ParseMaintainer(path, file)
		if err != nil {
			return err
		}
		if mntner.Name != "" {
			reg.Maintainers[mntner.Name] = mntner
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if err := parseObjects(filepath.Join(root, "registry", "domain"), func(path string) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		domain, err := p.ParseDomain(path, file)
		if err != nil {
			return err
		}
		if domain.Name != "" {
			reg.Domains[domain.Name] = domain
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return reg, nil
}

func parseObjects(dir string, parse func(string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: symlink registry objects are not supported", filepath.Join(dir, entry.Name()))
		}
		if entry.IsDir() {
			return fmt.Errorf("%s: nested registry object directories are not supported", filepath.Join(dir, entry.Name()))
		}
		if err := parse(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func parseRecord(zone, filename string, lineNo int, raw string) (registry.Record, error) {
	fields, err := splitFields(raw)
	if err != nil {
		return registry.Record{}, parseError(filename, lineNo, raw, err.Error())
	}
	if len(fields) < 3 {
		return registry.Record{}, parseError(filename, lineNo, raw, "record must contain name, type, and value")
	}

	record := registry.Record{
		Name: registry.NormalizeName(fields[0]),
		Type: strings.ToUpper(fields[1]),
		Line: lineNo,
		Raw:  raw,
	}
	values := append([]string(nil), fields[2:]...)
	if registry.IsProxyable(record.Type) && len(values) > 0 && strings.EqualFold(values[len(values)-1], "proxied") {
		record.Proxied = true
		values = values[:len(values)-1]
	}
	if _, ok := registry.SupportedRecordTypes[record.Type]; !ok {
		return registry.Record{}, parseError(filename, lineNo, raw, "unsupported DNS record type "+record.Type)
	}

	switch record.Type {
	case registry.TypeA, registry.TypeAAAA, registry.TypeCNAME, registry.TypeTXT, registry.TypeCAA:
		if len(values) == 0 {
			return registry.Record{}, parseError(filename, lineNo, raw, "record value is required")
		}
		record.Content = strings.Join(values, " ")
	case registry.TypeNS:
		if len(values) != 1 {
			return registry.Record{}, parseError(filename, lineNo, raw, "NS record must contain target")
		}
		record.Target = registry.TargetName(zone, values[0])
		record.Content = record.Target
	case registry.TypeMX:
		if len(values) != 2 {
			return registry.Record{}, parseError(filename, lineNo, raw, "MX record must contain priority and target")
		}
		priority, err := atoi(filename, lineNo, raw, values[0], "MX priority")
		if err != nil {
			return registry.Record{}, err
		}
		record.Priority = &priority
		record.Target = registry.TargetName(zone, values[1])
		record.Content = values[0] + " " + record.Target
	case registry.TypeSRV:
		if len(values) != 4 {
			return registry.Record{}, parseError(filename, lineNo, raw, "SRV record must contain priority, weight, port, and target")
		}
		priority, err := atoi(filename, lineNo, raw, values[0], "SRV priority")
		if err != nil {
			return registry.Record{}, err
		}
		weight, err := atoi(filename, lineNo, raw, values[1], "SRV weight")
		if err != nil {
			return registry.Record{}, err
		}
		port, err := atoi(filename, lineNo, raw, values[2], "SRV port")
		if err != nil {
			return registry.Record{}, err
		}
		record.Priority = &priority
		record.Weight = &weight
		record.Port = &port
		record.Target = registry.TargetName(zone, values[3])
		record.Content = strings.Join([]string{values[0], values[1], values[2], record.Target}, " ")
	}

	return record, nil
}

func parseAuth(value string) registry.Auth {
	if rest, ok := strings.CutPrefix(value, "github:"); ok {
		return registry.Auth{Method: "github", Value: rest, Raw: value}
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return registry.Auth{Raw: value}
	}
	return registry.Auth{Method: fields[0], Value: strings.Join(fields[1:], " "), Raw: value}
}

func header(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

func splitFields(line string) ([]string, error) {
	var fields []string
	var current strings.Builder
	inQuote := false
	escaped := false
	hadToken := false

	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			hadToken = true
			continue
		}

		if inQuote {
			switch r {
			case '\\':
				escaped = true
			case '"':
				inQuote = false
				hadToken = true
			default:
				current.WriteRune(r)
				hadToken = true
			}
			continue
		}

		switch r {
		case ' ', '\t':
			if hadToken {
				fields = append(fields, current.String())
				current.Reset()
				hadToken = false
			}
		case '"':
			inQuote = true
			hadToken = true
		default:
			current.WriteRune(r)
			hadToken = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("unfinished escape sequence")
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	if hadToken {
		fields = append(fields, current.String())
	}
	return fields, nil
}

func atoi(filename string, lineNo int, raw, value, field string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, parseError(filename, lineNo, raw, field+" must be an integer")
	}
	return parsed, nil
}

func parseError(filename string, lineNo int, raw, msg string) error {
	return &Error{File: filename, Line: lineNo, Text: raw, Msg: msg}
}
