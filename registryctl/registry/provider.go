package registry

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/NyanLoli-Network/baka.life/registryctl/provider"
)

func (r *Registry) DesiredRecords() ([]provider.Record, error) {
	domains := make([]string, 0, len(r.Domains))
	for name := range r.Domains {
		domains = append(domains, name)
	}
	sort.Strings(domains)

	var records []provider.Record
	for _, name := range domains {
		domainRecords, err := r.Domains[name].DesiredRecords()
		if err != nil {
			return nil, err
		}
		records = append(records, domainRecords...)
	}
	return records, nil
}

func (d *Domain) DesiredRecords() ([]provider.Record, error) {
	records := make([]provider.Record, 0, len(d.Records))
	for _, record := range d.Records {
		converted, err := d.desiredRecord(record)
		if err != nil {
			return nil, err
		}
		records = append(records, converted)
	}
	return records, nil
}

func (d *Domain) desiredRecord(record Record) (provider.Record, error) {
	recordType := strings.ToUpper(record.Type)
	converted := provider.Record{
		Type: recordType,
		Name: FQDN(d.Name, record.Name),
	}

	switch recordType {
	case TypeA, TypeAAAA:
		converted.Content = record.Content
		converted.Proxied = boolPtr(record.Proxied)
	case TypeCNAME:
		converted.Content = TargetName(d.Name, record.Content)
		converted.Proxied = boolPtr(record.Proxied)
	case TypeTXT:
		converted.Content = record.Content
	case TypeMX:
		converted.Content = TargetName(d.Name, record.Target)
		converted.Priority = copyInt(record.Priority)
	case TypeNS:
		converted.Content = TargetName(d.Name, record.Target)
	case TypeSRV:
		if record.Priority == nil || record.Weight == nil || record.Port == nil {
			return provider.Record{}, fmt.Errorf("SRV record %s is missing numeric fields", record.Name)
		}
		target := TargetName(d.Name, record.Target)
		converted.Content = fmt.Sprintf("%d %d %d %s", *record.Priority, *record.Weight, *record.Port, target)
		converted.Data = srvData(converted.Name, *record.Priority, *record.Weight, *record.Port, target)
	case TypeCAA:
		converted.Content = record.Content
		converted.Data = caaData(record.Content)
	default:
		converted.Content = record.Content
	}

	return converted, nil
}

func srvData(name string, priority, weight, port int, target string) map[string]any {
	labels := strings.Split(NormalizeName(name), ".")
	data := map[string]any{
		"priority": priority,
		"weight":   weight,
		"port":     port,
		"target":   target,
	}
	if len(labels) >= 3 && strings.HasPrefix(labels[0], "_") && strings.HasPrefix(labels[1], "_") {
		data["service"] = labels[0]
		data["proto"] = labels[1]
		data["name"] = strings.Join(labels[2:], ".")
	}
	return data
}

func caaData(content string) map[string]any {
	fields := strings.Fields(content)
	if len(fields) < 3 {
		return nil
	}
	flags, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil
	}
	return map[string]any{
		"flags": flags,
		"tag":   fields[1],
		"value": strings.Join(fields[2:], " "),
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
